// 本文件用于 AI 日志总结接口
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"file-watch/internal/logger"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
)

const (
	aiDefaultTimeout = 20 * time.Second
	aiMaxItems       = 5
	aiRetryMinLines  = 30
	aiTailBytes      = 2 * 1024 * 1024
	aiKeywordContext = 2
)

var aiLogKeywords = []string{
	"error",
	"failed",
	"exception",
	"panic",
	"timeout",
	"timed out",
	"denied",
	"unauthorized",
	"refused",
	"fatal",
	"oom",
	"out of memory",
	"错误",
	"异常",
	"失败",
	"超时",
	"拒绝",
	"权限",
	"告警",
	"不可用",
	"不可达",
}

const aiLogSummarySystemPrompt = `
你是资深运维工程师，擅长从日志中定位问题并给出可执行建议
请根据用户提供的日志内容输出 JSON 对象，禁止使用 Markdown
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

type aiLogSummaryRequest struct {
	Path          string `json:"path"`
	Mode          string `json:"mode"`
	Query         string `json:"query"`
	Limit         int    `json:"limit"`
	CaseSensitive bool   `json:"caseSensitive"`
}

type aiLogSummaryResult struct {
	Summary     string   `json:"summary"`
	Severity    string   `json:"severity"`
	KeyErrors   []string `json:"keyErrors"`
	Causes      []string `json:"causes"`
	Suggestions []string `json:"suggestions"`
	Confidence  *float64 `json:"confidence,omitempty"`
}

type aiLogSummaryMeta struct {
	UsedLines int   `json:"usedLines"`
	Truncated bool  `json:"truncated"`
	ElapsedMs int64 `json:"elapsedMs"`
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

func (h *handler) aiLogSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req aiLogSummaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config not loaded"})
		return
	}
	if !cfg.AIEnabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "AI分析未启用"})
		return
	}
	if err := validateAISettings(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cleanedPath, err := validateLogPath(cfg, req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	mode, query, err := resolveLogMode(req.Mode, req.Query)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	limit := resolveLineLimit(req.Limit, cfg.AIMaxLines)
	lines, truncated, err := loadLogLines(cleanedPath, mode, query, limit, req.CaseSensitive)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(lines) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "没有可分析的内容"})
		return
	}
	compressedLines, compressed := compressLogLines(lines, limit)
	if compressed {
		truncated = true
	}
	linesForAI := compressedLines
	logText := strings.Join(linesForAI, "\n")
	logger.Info("AI日志分析输入准备: path=%s mode=%s lines=%d compressed=%v truncated=%v", cleanedPath, mode, len(linesForAI), compressed, truncated)
	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), parseAITimeout(cfg.AITimeout))
	defer cancel()
	analysisText, err := callAIForLogSummary(ctx, cfg, logText, cleanedPath, truncated)
	if err != nil && isTimeoutError(err) {
		retryLines, changed := buildRetryLines(lines, linesForAI)
		if changed {
			logger.Warn("AI分析超时，已降级重试: before=%d after=%d", len(linesForAI), len(retryLines))
			truncated = true
			linesForAI = retryLines
			logText = strings.Join(linesForAI, "\n")
			ctxRetry, cancelRetry := context.WithTimeout(r.Context(), parseAITimeout(cfg.AITimeout))
			analysisText, err = callAIForLogSummary(ctxRetry, cfg, logText, cleanedPath, truncated)
			cancelRetry()
		}
	}
	if err != nil {
		logger.Warn("AI分析失败: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := parseAIResult(analysisText)
	if err != nil {
		logger.Warn("AI结果解析失败: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "AI结果解析失败"})
		return
	}
	normalizeAIResult(&result)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"analysis": result,
		"meta": aiLogSummaryMeta{
			UsedLines: len(linesForAI),
			Truncated: truncated,
			ElapsedMs: time.Since(start).Milliseconds(),
		},
	})
}

func validateAISettings(cfg *models.Config) error {
	if strings.TrimSpace(cfg.AIBaseURL) == "" {
		return fmt.Errorf("AI_BASE_URL不能为空")
	}
	if strings.TrimSpace(cfg.AIAPIKey) == "" {
		return fmt.Errorf("AI_API_KEY不能为空")
	}
	if strings.TrimSpace(cfg.AIModel) == "" {
		return fmt.Errorf("AI_MODEL不能为空")
	}
	return nil
}

func validateLogPath(cfg *models.Config, rawPath string) (string, error) {
	cleanedPath := filepath.Clean(filepath.FromSlash(strings.TrimSpace(rawPath)))
	if strings.TrimSpace(cfg.WatchDir) == "" {
		return "", fmt.Errorf("watch dir not configured")
	}
	watchDirs := pathutil.SplitWatchDirs(cfg.WatchDir)
	if len(watchDirs) == 0 {
		return "", fmt.Errorf("watch dir not configured")
	}
	if _, _, err := pathutil.RelativePathAny(watchDirs, cleanedPath); err != nil {
		return "", err
	}
	info, err := os.Stat(cleanedPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory")
	}
	return cleanedPath, nil
}

func resolveLogMode(mode, query string) (string, string, error) {
	cleanMode := strings.ToLower(strings.TrimSpace(mode))
	cleanQuery := strings.TrimSpace(query)
	if cleanMode == "" {
		if cleanQuery == "" {
			return "tail", "", nil
		}
		return "search", cleanQuery, nil
	}
	if cleanMode != "tail" && cleanMode != "search" {
		return "", "", fmt.Errorf("mode only supports tail or search")
	}
	if cleanMode == "search" && cleanQuery == "" {
		return "", "", fmt.Errorf("query is required when mode is search")
	}
	return cleanMode, cleanQuery, nil
}

func resolveLineLimit(requested, max int) int {
	if max <= 0 {
		max = 200
	}
	if requested <= 0 {
		return max
	}
	if requested > max {
		return max
	}
	return requested
}

func loadLogLines(path, mode, query string, limit int, caseSensitive bool) ([]string, bool, error) {
	if mode == "search" {
		lines, truncated, err := searchFileLogLines(path, query, limit, caseSensitive)
		if err != nil {
			return nil, false, err
		}
		return lines, truncated, nil
	}
	lines, truncated, err := readTailLinesForAI(path, aiTailBytes)
	if err != nil {
		return nil, false, err
	}
	return lines, truncated, nil
}

func trimLines(lines []string, limit int) ([]string, bool) {
	if limit <= 0 || len(lines) <= limit {
		return lines, false
	}
	return lines[len(lines)-limit:], true
}

// compressLogLines 根据关键词与尾部上下文压缩日志行
func compressLogLines(lines []string, limit int) ([]string, bool) {
	if limit <= 0 || len(lines) <= limit {
		return lines, false
	}
	tailCount := resolveTailCount(limit)
	if tailCount > len(lines) {
		tailCount = len(lines)
	}
	important := make(map[int]struct{})
	for i := len(lines) - tailCount; i < len(lines); i++ {
		if i >= 0 {
			important[i] = struct{}{}
		}
	}
	for i, line := range lines {
		if containsKeyword(line) {
			for offset := -aiKeywordContext; offset <= aiKeywordContext; offset++ {
				idx := i + offset
				if idx < 0 || idx >= len(lines) {
					continue
				}
				important[idx] = struct{}{}
			}
		}
	}
	indices := make([]int, 0, len(important))
	for idx := range important {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	if len(indices) > limit {
		indices = indices[len(indices)-limit:]
	}
	selected := make([]string, 0, len(indices))
	for _, idx := range indices {
		selected = append(selected, lines[idx])
	}
	return selected, true
}

func resolveTailCount(limit int) int {
	if limit <= 0 {
		return 0
	}
	tail := limit / 3
	if tail < 20 {
		tail = 20
	}
	if tail > limit {
		tail = limit
	}
	return tail
}

func containsKeyword(line string) bool {
	lower := strings.ToLower(line)
	for _, keyword := range aiLogKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func readTailLinesForAI(path string, maxBytes int64) ([]string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}

	size := info.Size()
	if maxBytes <= 0 {
		maxBytes = maxFileLogBytes
	}
	var data []byte
	truncated := false
	if size > maxBytes {
		start := size - maxBytes
		buf := make([]byte, maxBytes)
		n, err := file.ReadAt(buf, start)
		if err != nil && err != io.EOF {
			return nil, false, err
		}
		data = buf[:n]
		truncated = true
	} else {
		data, err = io.ReadAll(file)
		if err != nil {
			return nil, false, err
		}
	}

	if len(data) == 0 {
		return []string{}, truncated, nil
	}
	if !isTextData(data) {
		return nil, false, fmt.Errorf("仅支持文本文件")
	}

	lines := strings.Split(string(data), "\n")
	if size > maxBytes && len(lines) > 1 {
		lines = lines[1:]
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}
	return lines, truncated, nil
}

func buildRetryLines(original, current []string) ([]string, bool) {
	if len(current) <= aiRetryMinLines {
		return current, false
	}
	retryLimit := len(current) / 2
	if retryLimit < aiRetryMinLines {
		retryLimit = aiRetryMinLines
	}
	retryLines, _ := compressLogLines(original, retryLimit)
	if len(retryLines) >= len(current) {
		retryLines, _ = trimLines(current, retryLimit)
	}
	if len(retryLines) >= len(current) {
		return current, false
	}
	return retryLines, true
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "timeout")
}

func parseAITimeout(raw string) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return aiDefaultTimeout
	}
	if d, err := time.ParseDuration(trimmed); err == nil && d > 0 {
		return d
	}
	if v, err := strconv.Atoi(trimmed); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return aiDefaultTimeout
}

func callAIForLogSummary(ctx context.Context, cfg *models.Config, logText, logPath string, truncated bool) (string, error) {
	endpoint, err := buildChatCompletionURL(cfg.AIBaseURL)
	if err != nil {
		return "", err
	}
	payload := openAIChatRequest{
		Model: cfg.AIModel,
		Messages: []openAIChatMessage{
			{Role: "system", Content: aiLogSummarySystemPrompt},
			{Role: "user", Content: buildLogSummaryUserContent(logText, logPath, truncated)},
		},
		Temperature: 0.2,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("AI请求构造失败: %w", err)
	}
	client := &http.Client{Timeout: parseAITimeout(cfg.AITimeout)}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("AI请求创建失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AIAPIKey)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("AI响应读取失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("AI响应异常: %s", strings.TrimSpace(string(data)))
	}
	var parsed openAIChatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("AI响应解析失败: %w", err)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return "", fmt.Errorf("AI响应错误: %s", strings.TrimSpace(parsed.Error.Message))
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("AI响应为空")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("AI响应为空")
	}
	return content, nil
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

func buildLogSummaryUserContent(logText, logPath string, truncated bool) string {
	truncatedNote := "否"
	if truncated {
		truncatedNote = "是"
	}
	return fmt.Sprintf("日志路径: %s\n是否截断: %s\n日志内容:\n%s", logPath, truncatedNote, logText)
}

func parseAIResult(raw string) (aiLogSummaryResult, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return aiLogSummaryResult{}, fmt.Errorf("AI结果为空")
	}
	var result aiLogSummaryResult
	if err := json.Unmarshal([]byte(clean), &result); err == nil {
		return result, nil
	}
	extracted := extractJSONObject(clean)
	if extracted == "" {
		return aiLogSummaryResult{}, fmt.Errorf("AI结果不是JSON")
	}
	if err := json.Unmarshal([]byte(extracted), &result); err != nil {
		return aiLogSummaryResult{}, err
	}
	return result, nil
}

func extractJSONObject(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return raw[start : end+1]
}

func normalizeAIResult(result *aiLogSummaryResult) {
	if result == nil {
		return
	}
	result.Summary = strings.TrimSpace(result.Summary)
	if result.Summary == "" {
		result.Summary = "未获取到有效摘要"
	}
	result.Severity = normalizeSeverity(result.Severity)
	result.KeyErrors = trimItems(result.KeyErrors, aiMaxItems)
	result.Causes = trimItems(result.Causes, 3)
	result.Suggestions = trimItems(result.Suggestions, 3)
	if result.Confidence != nil {
		if *result.Confidence < 0 || *result.Confidence > 1 {
			result.Confidence = nil
		}
	}
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
