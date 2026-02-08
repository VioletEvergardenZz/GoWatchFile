// 本文件用于告警 AI 分析与上下文压缩
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

const (
	alertAIContextLines  = 20
	alertAITailLines     = 20
	alertAIBufferSize    = 200
	alertAIDedupeWindow  = 10 * time.Minute
	alertAIWorkerLimit   = 2
	alertAIResultMaxSize = 600
)

const alertAISystemPrompt = `
你是资深运维工程师，擅长从告警日志中定位问题并给出可执行建议
请根据告警规则与日志片段输出 JSON 对象，禁止使用 Markdown
JSON 字段要求如下
summary: 用 1-3 句中文描述发生了什么
severity: 只能是 low/medium/high 之一
keyErrors: 关键错误行数组，最多 5 条
causes: 可能原因数组，最多 3 条
suggestions: 建议动作数组，最多 3 条
confidence: 0 到 1 的小数，表示可信度
如果日志里看不出明显问题，请在 summary 说明暂无明确异常，severity 使用 low
只输出 JSON，不要输出其他文字
`

type lineBuffer struct {
	max   int
	lines []string
}

func newLineBuffer(max int) *lineBuffer {
	if max <= 0 {
		max = alertAIBufferSize
	}
	return &lineBuffer{max: max, lines: make([]string, 0, max)}
}

func (b *lineBuffer) append(line string) {
	if b == nil {
		return
	}
	b.lines = append(b.lines, line)
	if len(b.lines) > b.max {
		b.lines = append([]string(nil), b.lines[len(b.lines)-b.max:]...)
	}
}

func (b *lineBuffer) snapshot() []string {
	if b == nil {
		return nil
	}
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type aiAlertResult struct {
	Summary     string   `json:"summary"`
	Severity    string   `json:"severity"`
	KeyErrors   []string `json:"keyErrors"`
	Causes      []string `json:"causes"`
	Suggestions []string `json:"suggestions"`
	Confidence  *float64 `json:"confidence,omitempty"`
}

func (m *Manager) shouldRunAlertAI(result decisionResult, line string, now time.Time) bool {
	if m == nil || !m.aiEnabled {
		return false
	}
	if result.status != StatusSent {
		return false
	}
	if result.file == "" {
		return false
	}
	if strings.TrimSpace(line) == "" {
		return false
	}
	return true
}

func buildAlertAISignature(result decisionResult, line string) string {
	lineKey := strings.ToLower(strings.TrimSpace(line))
	lineKey = truncateText(lineKey, 200)
	return strings.Join([]string{result.ruleID, result.file, lineKey}, "|")
}

func (m *Manager) captureLineContext(path, line string) (before []string, after []string) {
	if m == nil {
		return nil, nil
	}
	m.aiMu.Lock()
	defer m.aiMu.Unlock()
	if m.lineBuffers == nil {
		m.lineBuffers = make(map[string]*lineBuffer)
	}
	buf := m.lineBuffers[path]
	if buf == nil {
		buf = newLineBuffer(alertAIBufferSize)
		m.lineBuffers[path] = buf
	}
	before = buf.snapshot()
	buf.append(line)
	after = buf.snapshot()
	return before, after
}

func (m *Manager) allowAlertAI(signature string, now time.Time) bool {
	if m == nil {
		return false
	}
	if m.aiWindow <= 0 {
		return true
	}
	m.aiMu.Lock()
	defer m.aiMu.Unlock()
	if m.aiHistory == nil {
		m.aiHistory = make(map[string]time.Time)
	}
	if last, ok := m.aiHistory[signature]; ok && now.Sub(last) < m.aiWindow {
		return false
	}
	m.aiHistory[signature] = now
	if len(m.aiHistory) > 2000 {
		for key, ts := range m.aiHistory {
			if now.Sub(ts) > m.aiWindow {
				delete(m.aiHistory, key)
			}
		}
	}
	return true
}

func (m *Manager) enqueueAlertAI(result decisionResult, line string, contextLines []string) {
	if m == nil || m.aiLimiter == nil {
		return
	}
	select {
	case m.aiLimiter <- struct{}{}:
	default:
		logger.Warn("告警AI分析跳过: 并发已满 rule=%s file=%s", result.rule, result.file)
		return
	}
	go func() {
		defer func() { <-m.aiLimiter }()
		m.runAlertAI(result, line, contextLines)
	}()
}

func (m *Manager) runAlertAI(result decisionResult, line string, contextLines []string) {
	if m == nil || m.cfg == nil || !m.aiEnabled {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), parseAITimeout(m.cfg.AITimeout))
	defer cancel()
	logger.Info("告警AI分析开始: rule=%s file=%s lines=%d", result.rule, result.file, len(contextLines))
	analysis, err := analyzeAlertWithAI(ctx, m.cfg, result, line, contextLines)
	if err != nil {
		logger.Warn("告警AI分析失败: %v", err)
		return
	}
	summary := formatAIAlertSummary(analysis)
	summary = truncateText(summary, alertAIResultMaxSize)
	if strings.TrimSpace(summary) == "" {
		return
	}
	m.state.AttachAnalysis(result.id, summary)
}

func analyzeAlertWithAI(ctx context.Context, cfg *models.Config, result decisionResult, line string, contextLines []string) (aiAlertResult, error) {
	if cfg == nil {
		return aiAlertResult{}, fmt.Errorf("AI配置为空")
	}
	endpoint, err := buildChatCompletionURL(cfg.AIBaseURL)
	if err != nil {
		return aiAlertResult{}, err
	}
	userContent := buildAlertUserContent(result, line, contextLines)
	payload := openAIChatRequest{
		Model: cfg.AIModel,
		Messages: []openAIChatMessage{
			{Role: "system", Content: alertAISystemPrompt},
			{Role: "user", Content: userContent},
		},
		Temperature: 0.2,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return aiAlertResult{}, fmt.Errorf("AI请求构造失败: %w", err)
	}
	client := &http.Client{Timeout: parseAITimeout(cfg.AITimeout)}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return aiAlertResult{}, fmt.Errorf("AI请求创建失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AIAPIKey)
	resp, err := client.Do(req)
	if err != nil {
		return aiAlertResult{}, fmt.Errorf("AI请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return aiAlertResult{}, fmt.Errorf("AI响应读取失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return aiAlertResult{}, fmt.Errorf("AI响应异常: %s", strings.TrimSpace(string(data)))
	}
	var parsed openAIChatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return aiAlertResult{}, fmt.Errorf("AI响应解析失败: %w", err)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return aiAlertResult{}, fmt.Errorf("AI响应错误: %s", strings.TrimSpace(parsed.Error.Message))
	}
	if len(parsed.Choices) == 0 {
		return aiAlertResult{}, fmt.Errorf("AI响应为空")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return aiAlertResult{}, fmt.Errorf("AI响应为空")
	}
	resultParsed, err := parseAIAlertResult(content)
	if err != nil {
		return aiAlertResult{}, err
	}
	normalizeAIAlertResult(&resultParsed)
	return resultParsed, nil
}

func buildAlertUserContent(result decisionResult, line string, contextLines []string) string {
	builder := strings.Builder{}
	builder.WriteString("告警规则: ")
	builder.WriteString(result.rule)
	builder.WriteString("\n告警级别: ")
	builder.WriteString(strings.ToLower(string(result.level)))
	builder.WriteString("\n日志文件: ")
	builder.WriteString(result.file)
	builder.WriteString("\n命中行: ")
	builder.WriteString(line)
	builder.WriteString("\n日志片段:\n")
	builder.WriteString(strings.Join(contextLines, "\n"))
	return builder.String()
}

func buildAlertContextLines(before []string, line string, after []string) []string {
	context := lastLines(before, alertAIContextLines-1)
	context = append(context, line)
	tail := lastLines(after, alertAITailLines)
	merged := mergeUnique(context, tail)
	return merged
}

func lastLines(lines []string, limit int) []string {
	if limit <= 0 || len(lines) <= limit {
		return append([]string(nil), lines...)
	}
	return append([]string(nil), lines[len(lines)-limit:]...)
}

func mergeUnique(parts ...[]string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 32)
	for _, part := range parts {
		for _, line := range part {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	return out
}

func parseAIAlertResult(raw string) (aiAlertResult, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return aiAlertResult{}, fmt.Errorf("AI结果为空")
	}
	var result aiAlertResult
	if err := json.Unmarshal([]byte(clean), &result); err == nil {
		return result, nil
	}
	extracted := extractJSONObject(clean)
	if extracted == "" {
		return aiAlertResult{}, fmt.Errorf("AI结果不是JSON")
	}
	if err := json.Unmarshal([]byte(extracted), &result); err != nil {
		return aiAlertResult{}, err
	}
	return result, nil
}

func normalizeAIAlertResult(result *aiAlertResult) {
	if result == nil {
		return
	}
	result.Summary = strings.TrimSpace(result.Summary)
	if result.Summary == "" {
		result.Summary = "未获取到有效摘要"
	}
	result.Severity = normalizeSeverity(result.Severity)
	result.KeyErrors = trimItems(result.KeyErrors, 5)
	result.Causes = trimItems(result.Causes, 3)
	result.Suggestions = trimItems(result.Suggestions, 3)
	if result.Confidence != nil {
		if *result.Confidence < 0 || *result.Confidence > 1 {
			result.Confidence = nil
		}
	}
}

func formatAIAlertSummary(result aiAlertResult) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(result.Summary) != "" {
		parts = append(parts, "摘要: "+strings.TrimSpace(result.Summary))
	}
	if len(result.Causes) > 0 {
		parts = append(parts, "原因: "+strings.Join(result.Causes, "；"))
	}
	if len(result.Suggestions) > 0 {
		parts = append(parts, "建议: "+strings.Join(result.Suggestions, "；"))
	}
	return strings.Join(parts, "\n")
}

func truncateText(raw string, limit int) string {
	if limit <= 0 {
		return raw
	}
	if len(raw) <= limit {
		return raw
	}
	runes := []rune(raw)
	if len(runes) <= limit {
		return raw
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func parseAITimeout(raw string) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 20 * time.Second
	}
	if d, err := time.ParseDuration(trimmed); err == nil && d > 0 {
		return d
	}
	if v, err := strconv.Atoi(trimmed); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return 20 * time.Second
}

func buildChatCompletionURL(base string) (string, error) {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return "", fmt.Errorf("AI_BASE_URL不能为空")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("AI_BASE_URL无效: %s", trimmed)
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	if strings.HasSuffix(path, "/chat/completions") {
		parsed.Path = path
		return parsed.String(), nil
	}
	if strings.HasSuffix(path, "/v1") {
		parsed.Path = path + "/chat/completions"
		return parsed.String(), nil
	}
	if path == "" {
		parsed.Path = "/v1/chat/completions"
		return parsed.String(), nil
	}
	parsed.Path = path + "/chat/completions"
	return parsed.String(), nil
}

func extractJSONObject(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return raw[start : end+1]
}

func trimItems(items []string, limit int) []string {
	if len(items) == 0 {
		return nil
	}
	trimmed := make([]string, 0, len(items))
	for _, item := range items {
		val := strings.TrimSpace(item)
		if val == "" {
			continue
		}
		trimmed = append(trimmed, val)
		if limit > 0 && len(trimmed) >= limit {
			break
		}
	}
	return trimmed
}

func normalizeSeverity(raw string) string {
	clean := strings.ToLower(strings.TrimSpace(raw))
	switch clean {
	case "low", "medium", "high":
		return clean
	case "低":
		return "low"
	case "中":
		return "medium"
	case "高":
		return "high"
	default:
		return "medium"
	}
}
