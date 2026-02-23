// 本文件用于上传通知场景的 AI 摘要能力
// 文件职责：上传完成后生成可降级的 AI 摘要并注入通知内容
// 关键路径：读取文件片段 -> 调用 AI -> 解析摘要 -> 失败降级
// 边界与容错：AI 不可用时必须快速降级，避免阻塞上传主链路

package service

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
	"strconv"
	"strings"
	"time"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

const (
	uploadAINotifyReadMaxBytes        int64 = 2 * 1024 * 1024
	uploadAINotifyDefaultTimeout            = 8 * time.Second
	uploadAINotifyMaxTimeout                = 15 * time.Second
	uploadAINotifyDefaultLineLimit          = 120
	uploadAINotifyMaxLineLimit              = 200
	uploadAINotifySummaryMaxRunes           = 240
	uploadAINotifyFallbackReasonRunes       = 80
)

const uploadAINotifySystemPrompt = `
你是资深运维工程师，请根据上传文件内容输出简洁分析结果
仅输出 JSON，不要输出 Markdown
JSON 字段要求：
summary: 1-2 句中文，重点说明异常现象与可能根因
severity: 只能是 low/medium/high 之一
suggestion: 1 句可执行处置建议，优先给“先检查/先验证”的动作
如果看不出异常，请在 summary 里明确说明“未发现明显异常”，severity 使用 low
`

type uploadAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type uploadAIChatRequest struct {
	Model       string                `json:"model"`
	Messages    []uploadAIChatMessage `json:"messages"`
	Temperature float64               `json:"temperature"`
}

type uploadAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type uploadAINotifyResult struct {
	Summary    string `json:"summary"`
	Severity   string `json:"severity"`
	Suggestion string `json:"suggestion"`
}

// buildUploadAISummary 在上传成功后生成通知摘要。
// 这里把超时和降级放在同一层处理，确保“通知一定可发”，避免 AI 故障拖慢上传消费。
func (fs *FileService) buildUploadAISummary(ctx context.Context, filePath string) string {
	cfg := fs.snapshotConfigForUploadAI()
	if !cfg.AIEnabled {
		return ""
	}
	if err := validateUploadAISettings(&cfg); err != nil {
		logger.Warn("上传通知AI分析降级: path=%s err=%v", filePath, err)
		return "AI研判降级: " + buildUploadAIFallbackReason(err)
	}

	lines, truncated, err := readUploadFileLinesForAI(filePath, resolveUploadAINotifyLineLimit(cfg.AIMaxLines))
	if err != nil {
		logger.Warn("上传通知AI分析降级: path=%s err=%v", filePath, err)
		return "AI研判降级: " + buildUploadAIFallbackReason(err)
	}
	if len(lines) == 0 {
		return "故障级别=低；根因判断=未读取到可分析内容；处置建议=继续观察后续上传与日志波动"
	}

	timeout := resolveUploadAINotifyTimeout(cfg.AITimeout)
	aiCtx, cancel := withUploadAINotifyTimeout(ctx, timeout)
	defer cancel()
	raw, err := callAIForUploadNotification(aiCtx, &cfg, filePath, strings.Join(lines, "\n"), truncated)
	if err != nil {
		logger.Warn("上传通知AI分析降级: path=%s err=%v", filePath, err)
		return "AI研判降级: " + buildUploadAIFallbackReason(err)
	}

	summary := parseUploadAINotifySummary(raw)
	if strings.TrimSpace(summary) == "" {
		return "AI研判降级: AI返回为空，未形成有效结论"
	}
	return formatNotificationAISummary(summary)
}

func (fs *FileService) snapshotConfigForUploadAI() models.Config {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.config == nil {
		return models.Config{}
	}
	return *fs.config
}

func validateUploadAISettings(cfg *models.Config) error {
	if cfg == nil {
		return fmt.Errorf("AI配置为空")
	}
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

func resolveUploadAINotifyTimeout(raw string) time.Duration {
	timeout := uploadAINotifyDefaultTimeout
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		if d, err := time.ParseDuration(trimmed); err == nil && d > 0 {
			timeout = d
		} else if v, err := strconv.Atoi(trimmed); err == nil && v > 0 {
			timeout = time.Duration(v) * time.Second
		}
	}
	// 上传通知的 AI 属于附加能力，不允许无限拉长 worker 占用时间。
	if timeout > uploadAINotifyMaxTimeout {
		return uploadAINotifyMaxTimeout
	}
	return timeout
}

func withUploadAINotifyTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = uploadAINotifyDefaultTimeout
	}
	if parent == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	if deadline, ok := parent.Deadline(); ok {
		remain := time.Until(deadline)
		if remain > 0 && remain <= timeout {
			return parent, func() {}
		}
	}
	return context.WithTimeout(parent, timeout)
}

func resolveUploadAINotifyLineLimit(raw int) int {
	limit := raw
	if limit <= 0 {
		limit = uploadAINotifyDefaultLineLimit
	}
	if limit > uploadAINotifyMaxLineLimit {
		limit = uploadAINotifyMaxLineLimit
	}
	return limit
}

func readUploadFileLinesForAI(path string, lineLimit int) ([]string, bool, error) {
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
	var data []byte
	truncated := false
	if size > uploadAINotifyReadMaxBytes {
		start := size - uploadAINotifyReadMaxBytes
		buf := make([]byte, uploadAINotifyReadMaxBytes)
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
	if !isTextDataForUploadAI(data) {
		return nil, false, fmt.Errorf("仅支持文本文件")
	}

	lines := strings.Split(string(data), "\n")
	if size > uploadAINotifyReadMaxBytes && len(lines) > 1 {
		lines = lines[1:]
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}
	if lineLimit > 0 && len(lines) > lineLimit {
		lines = lines[len(lines)-lineLimit:]
		truncated = true
	}
	return lines, truncated, nil
}

func isTextDataForUploadAI(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func callAIForUploadNotification(ctx context.Context, cfg *models.Config, filePath, fileText string, truncated bool) (string, error) {
	endpoint, err := buildUploadAIChatCompletionURL(cfg.AIBaseURL)
	if err != nil {
		return "", err
	}
	payload := uploadAIChatRequest{
		Model: cfg.AIModel,
		Messages: []uploadAIChatMessage{
			{Role: "system", Content: uploadAINotifySystemPrompt},
			{Role: "user", Content: buildUploadAIUserContent(filePath, fileText, truncated)},
		},
		Temperature: 0.1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("AI请求构造失败: %w", err)
	}

	client := &http.Client{Timeout: resolveUploadAINotifyTimeout(cfg.AITimeout)}
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

	var parsed uploadAIChatResponse
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

func buildUploadAIChatCompletionURL(base string) (string, error) {
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

func buildUploadAIUserContent(filePath, fileText string, truncated bool) string {
	truncatedNote := "否"
	if truncated {
		truncatedNote = "是"
	}
	cleanedPath := filepath.Clean(filePath)
	return fmt.Sprintf("上传文件路径: %s\n是否截断: %s\n文件内容:\n%s", cleanedPath, truncatedNote, fileText)
}

func parseUploadAINotifySummary(raw string) string {
	result, err := parseUploadAINotifyResult(raw)
	if err != nil {
		plain := trimNotificationText(raw, uploadAINotifySummaryMaxRunes)
		if plain == "" {
			return ""
		}
		return formatUploadAINotifyResult(uploadAINotifyResult{
			Summary:    plain,
			Severity:   "medium",
			Suggestion: "结合原始文件与运行态指标人工复核",
		})
	}
	return formatUploadAINotifyResult(result)
}

func parseUploadAINotifyResult(raw string) (uploadAINotifyResult, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return uploadAINotifyResult{}, fmt.Errorf("AI结果为空")
	}
	var result uploadAINotifyResult
	if err := json.Unmarshal([]byte(clean), &result); err == nil {
		return result, nil
	}
	extracted := extractJSONObjectFromAIText(clean)
	if extracted == "" {
		return uploadAINotifyResult{}, fmt.Errorf("AI结果不是JSON")
	}
	if err := json.Unmarshal([]byte(extracted), &result); err != nil {
		return uploadAINotifyResult{}, err
	}
	return result, nil
}

func formatUploadAINotifyResult(result uploadAINotifyResult) string {
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = "未发现明显异常"
	}
	suggestion := strings.TrimSpace(result.Suggestion)
	if suggestion == "" {
		suggestion = "继续观察后续日志与上传结果"
	}
	formatted := fmt.Sprintf(
		"故障级别=%s；根因判断=%s；处置建议=%s",
		normalizeUploadAISeverity(result.Severity),
		summary,
		suggestion,
	)
	return trimNotificationText(formatted, uploadAINotifySummaryMaxRunes)
}

func normalizeUploadAISeverity(raw string) string {
	clean := strings.ToLower(strings.TrimSpace(raw))
	switch clean {
	case "high", "高":
		return "高"
	case "low", "低":
		return "低"
	default:
		return "中"
	}
}

func extractJSONObjectFromAIText(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return raw[start : end+1]
}

func formatNotificationAISummary(summary string) string {
	cleaned := trimNotificationText(summary, uploadAINotifySummaryMaxRunes)
	if cleaned == "" {
		return "未获取到 AI 分析结果"
	}
	return cleaned
}

func trimNotificationText(raw string, limit int) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\r", "\n")
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	if limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func buildUploadAIFallbackReason(err error) string {
	if err == nil {
		return "AI服务暂不可用"
	}
	if isUploadAITimeoutError(err) {
		return "AI请求超时，按降级策略仅发送基础上传信息"
	}
	errText := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(errText, "仅支持文本文件"):
		return "文件为非文本内容，未执行AI研判"
	case strings.Contains(errText, "AI_BASE_URL"),
		strings.Contains(errText, "AI_API_KEY"),
		strings.Contains(errText, "AI_MODEL"),
		strings.Contains(errText, "AI配置为空"):
		return "AI配置不完整，未执行AI研判"
	default:
		return trimNotificationText(errText, uploadAINotifyFallbackReasonRunes)
	}
}

func isUploadAITimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
