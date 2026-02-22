// 本文件用于知识库 HTTP 处理器 将知识库能力通过统一路由暴露给控制台

// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"file-watch/internal/alert"
	"file-watch/internal/kb"
	"file-watch/internal/metrics"
	"file-watch/internal/models"
)

// kbArticles 统一处理知识库列表查询与新增
// GET/POST 共用一个入口便于保持参数口径和错误返回一致
func (h *handler) kbArticles(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		page := parsePositiveInt(r.URL.Query().Get("page"), 1)
		pageSize := parsePositiveInt(r.URL.Query().Get("pageSize"), 20)
		items, total, err := h.kb.ListArticles(kb.ListQuery{
			Query:           strings.TrimSpace(r.URL.Query().Get("q")),
			Status:          strings.TrimSpace(r.URL.Query().Get("status")),
			Severity:        strings.TrimSpace(r.URL.Query().Get("severity")),
			Tag:             strings.TrimSpace(r.URL.Query().Get("tag")),
			Page:            page,
			PageSize:        pageSize,
			IncludeArchived: parseBoolQuery(r, "includeArchived"),
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"items":    items,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		})
		return
	case http.MethodPost:
		var req struct {
			Title      string   `json:"title"`
			Summary    string   `json:"summary"`
			Category   string   `json:"category"`
			Severity   string   `json:"severity"`
			Content    string   `json:"content"`
			Tags       []string `json:"tags"`
			CreatedBy  string   `json:"createdBy"`
			ChangeNote string   `json:"changeNote"`
			SourceType string   `json:"sourceType"`
			SourceRef  string   `json:"sourceRef"`
			RefTitle   string   `json:"refTitle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		article, err := h.kb.CreateArticle(kb.CreateArticleInput{
			Title:      req.Title,
			Summary:    req.Summary,
			Category:   req.Category,
			Severity:   req.Severity,
			Content:    req.Content,
			Tags:       req.Tags,
			CreatedBy:  req.CreatedBy,
			ChangeNote: req.ChangeNote,
			SourceType: req.SourceType,
			SourceRef:  req.SourceRef,
			RefTitle:   req.RefTitle,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"article": article,
		})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
}

// kbArticleByID 通过路径段分发详情 更新 审批与回滚动作
// 这里显式拆分 action 分支 防止不同动作共享参数时出现歧义
func (h *handler) kbArticleByID(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/kb/articles/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "article id required"})
		return
	}
	parts := strings.Split(path, "/")
	articleID := strings.TrimSpace(parts[0])
	if articleID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "article id required"})
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			article, err := h.kb.GetArticle(articleID)
			if err != nil {
				status := http.StatusBadRequest
				if err == kb.ErrNotFound {
					status = http.StatusNotFound
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "article": article})
			return
		case http.MethodPut:
			var req struct {
				Title      string   `json:"title"`
				Summary    string   `json:"summary"`
				Category   string   `json:"category"`
				Severity   string   `json:"severity"`
				Content    string   `json:"content"`
				Tags       []string `json:"tags"`
				UpdatedBy  string   `json:"updatedBy"`
				ChangeNote string   `json:"changeNote"`
				SourceType string   `json:"sourceType"`
				SourceRef  string   `json:"sourceRef"`
				RefTitle   string   `json:"refTitle"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			article, err := h.kb.UpdateArticle(articleID, kb.UpdateArticleInput{
				Title:      req.Title,
				Summary:    req.Summary,
				Category:   req.Category,
				Severity:   req.Severity,
				Content:    req.Content,
				Tags:       req.Tags,
				UpdatedBy:  req.UpdatedBy,
				ChangeNote: req.ChangeNote,
				SourceType: req.SourceType,
				SourceRef:  req.SourceRef,
				RefTitle:   req.RefTitle,
			})
			if err != nil {
				status := http.StatusBadRequest
				if err == kb.ErrNotFound {
					status = http.StatusNotFound
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "article": article})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
	}

	if len(parts) == 2 && r.Method == http.MethodPost {
		action := strings.TrimSpace(parts[1])
		switch action {
		case "submit", "approve", "reject", "archive":
			start := time.Now()
			var req struct {
				Operator string `json:"operator"`
				Comment  string `json:"comment"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			article, err := h.kb.ApplyAction(articleID, action, req.Operator, req.Comment)
			if err != nil {
				status := http.StatusBadRequest
				if err == kb.ErrNotFound {
					status = http.StatusNotFound
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			metrics.Global().ObserveKBReviewLatency(time.Since(start))
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "article": article})
			return
		case "rollback":
			start := time.Now()
			var req struct {
				TargetVersion int    `json:"targetVersion"`
				Operator      string `json:"operator"`
				Comment       string `json:"comment"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			article, err := h.kb.RollbackArticle(articleID, req.TargetVersion, req.Operator, req.Comment)
			if err != nil {
				status := http.StatusBadRequest
				if err == kb.ErrNotFound {
					status = http.StatusNotFound
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			metrics.Global().ObserveKBReviewLatency(time.Since(start))
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "article": article})
			return
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unsupported action"})
			return
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
}

func (h *handler) kbSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	var req struct {
		Query           string `json:"query"`
		Limit           int    `json:"limit"`
		IncludeArchived bool   `json:"includeArchived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	items, err := h.kb.Search(req.Query, req.Limit, req.IncludeArchived)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics.Global().ObserveKBSearch(len(items))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
	})
}

// kbAsk 是知识问答入口
// 先走检索问答基线 再按配置决定是否叠加 AI 生成 避免强依赖外部模型
func (h *handler) kbAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	var req struct {
		Question string `json:"question"`
		Limit    int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	result, meta, err := h.askKnowledge(req.Question, req.Limit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics.Global().ObserveKBAsk(len(result.Citations))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"answer":     result.Answer,
		"citations":  result.Citations,
		"confidence": result.Confidence,
		"meta":       meta,
	})
}

func (h *handler) kbPendingReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 20)
	items, err := h.kb.PendingReviews(limit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
	})
}

func (h *handler) kbGates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"gates": kb.DefaultQualityGates(),
	})
}

func (h *handler) kbImportDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	var req struct {
		Path     string `json:"path"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	result, err := h.kb.ImportDocs(req.Path, req.Operator)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"result": result,
	})
}

func (h *handler) kbRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	rule := strings.TrimSpace(r.URL.Query().Get("rule"))
	message := strings.TrimSpace(r.URL.Query().Get("message"))
	alertID := strings.TrimSpace(r.URL.Query().Get("alertId"))

	decision, found := h.findAlertDecision(alertID)
	if found {
		// 优先复用告警决策快照，避免前端拼接查询词导致推荐结果不可重放
		if rule == "" {
			rule = strings.TrimSpace(decision.Rule)
		}
		if message == "" {
			message = strings.TrimSpace(decision.Message)
		}
	}
	if query == "" {
		query = buildKBRecommendationQuery(rule, message, alertID)
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 3)
	if query == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"items": []kb.Article{},
			"trace": nil,
		})
		return
	}
	items, err := h.kb.Recommendations(query, limit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	trace := buildKBRecommendationTrace(alertID, query, rule, message, decision, found, items)
	if found {
		state := h.currentAlertState()
		if state != nil && trace != nil {
			state.AttachKnowledgeTrace(alertID, *trace)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
		"trace": trace,
	})
}

func buildKBRecommendationQuery(rule, message, alertID string) string {
	rule = strings.TrimSpace(rule)
	message = strings.TrimSpace(message)
	switch {
	case rule != "" && message != "":
		return rule + " " + message
	case rule != "":
		return rule
	case message != "":
		return message
	default:
		return strings.TrimSpace(alertID)
	}
}

func buildKBRecommendationTrace(alertID, query, rule, message string, decision alert.Decision, found bool, items []kb.Article) *alert.RecommendationTrace {
	if strings.TrimSpace(alertID) == "" {
		return nil
	}
	articles := make([]alert.RecommendationArticle, 0, len(items))
	for _, item := range items {
		articles = append(articles, alert.RecommendationArticle{
			ArticleID: item.ID,
			Title:     item.Title,
			Version:   item.CurrentVersion,
			Status:    item.Status,
			Severity:  item.Severity,
		})
	}
	trace := &alert.RecommendationTrace{
		AlertID:  strings.TrimSpace(alertID),
		LinkedAt: time.Now().Format("2006-01-02 15:04:05"),
		Query:    strings.TrimSpace(query),
		Rule:     strings.TrimSpace(rule),
		Message:  strings.TrimSpace(message),
		HitCount: len(articles),
		Articles: articles,
	}
	if found {
		trace.DecisionStatus = strings.TrimSpace(decision.Status)
		trace.DecisionReason = strings.TrimSpace(decision.Reason)
	}
	trace.LinkID = fmt.Sprintf("kb-link-%s-%d", trace.AlertID, time.Now().UnixNano())
	return trace
}

func parsePositiveInt(raw string, fallback int) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	val, err := strconv.Atoi(trimmed)
	if err != nil || val <= 0 {
		return fallback
	}
	return val
}

type kbAskMeta struct {
	Degraded       bool   `json:"degraded"`
	ErrorClass     string `json:"errorClass,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`
}

// askKnowledge 统一封装“检索问答 + 可选 AI 增强”的双阶段流程
// 任何阶段失败都要保留可用的检索结果 保障问答能力可降级
func (h *handler) askKnowledge(question string, limit int) (kb.AskResult, kbAskMeta, error) {
	trimmedQuestion := strings.TrimSpace(question)
	if trimmedQuestion == "" {
		return kb.AskResult{}, kbAskMeta{}, fmt.Errorf("question is required")
	}
	if limit <= 0 {
		limit = 3
	}
	if h == nil || h.kb == nil {
		return kb.AskResult{}, kbAskMeta{}, fmt.Errorf("knowledge service not ready")
	}
	items := make([]kb.Article, 0, limit)
	for _, candidate := range buildQuestionCandidates(trimmedQuestion) {
		found, err := h.kb.Search(candidate, limit, false)
		if err != nil {
			return kb.AskResult{}, kbAskMeta{}, err
		}
		if len(found) == 0 {
			continue
		}
		items = found
		break
	}
	if len(items) == 0 {
		return kb.AskResult{}, kbAskMeta{}, fmt.Errorf("知识库中未找到可引用条目")
	}

	citations := make([]kb.Citation, 0, len(items))
	for _, item := range items {
		citations = append(citations, kb.Citation{
			ArticleID: item.ID,
			Title:     item.Title,
			Version:   item.CurrentVersion,
		})
	}

	fallback, _ := h.kb.Ask(trimmedQuestion, limit)
	if len(fallback.Citations) == 0 {
		fallback.Citations = citations
	}
	if strings.TrimSpace(fallback.Answer) == "" {
		fallback.Answer = fmt.Sprintf("可先参考《%s》并根据建议动作执行排查。", items[0].Title)
	}

	if !h.isKnowledgeAIReady() {
		fallback.Confidence = 0.72
		return fallback, kbAskMeta{
			Degraded:       true,
			ErrorClass:     "ai_disabled",
			FallbackReason: "ai_disabled_or_unconfigured",
		}, nil
	}

	answer, confidence, err := h.callAIForKnowledgeAnswer(ragPayload{
		Question:  trimmedQuestion,
		Citations: citations,
		Articles:  items,
	})
	if err != nil {
		// AI 调用失败时降级回本地回答，但仍返回引用
		fallback.Confidence = 0.65
		return fallback, kbAskMeta{
			Degraded:       true,
			ErrorClass:     classifyKnowledgeAIError(err),
			FallbackReason: "ai_request_failed",
		}, nil
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		fallback.Confidence = 0.65
		return fallback, kbAskMeta{
			Degraded:       true,
			ErrorClass:     "empty_answer",
			FallbackReason: "ai_response_empty",
		}, nil
	}
	return kb.AskResult{
		Answer:     answer,
		Citations:  citations,
		Confidence: confidence,
	}, kbAskMeta{}, nil
}

func classifyKnowledgeAIError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "429"), strings.Contains(msg, "rate limit"):
		return "rate_limit"
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "dial tcp"),
		strings.Contains(msg, "request failed"):
		return "network"
	case strings.Contains(msg, "status 5"), strings.Contains(msg, "bad gateway"), strings.Contains(msg, "service unavailable"):
		return "upstream"
	default:
		return "request_error"
	}
}

type ragPayload struct {
	Question  string
	Citations []kb.Citation
	Articles  []kb.Article
}

func (h *handler) isKnowledgeAIReady() bool {
	cfg := h.resolveKBConfig()
	if cfg == nil {
		return false
	}
	if !cfg.AIEnabled {
		return false
	}
	return strings.TrimSpace(cfg.AIBaseURL) != "" &&
		strings.TrimSpace(cfg.AIAPIKey) != "" &&
		strings.TrimSpace(cfg.AIModel) != ""
}

// callAIForKnowledgeAnswer 只负责调用模型并返回结构化结果
// 业务侧是否采用该结果由上层决策 避免网络波动直接污染问答主流程
func (h *handler) callAIForKnowledgeAnswer(payload ragPayload) (string, float64, error) {
	cfg := h.resolveKBConfig()
	if cfg == nil {
		return "", 0, fmt.Errorf("config not loaded")
	}
	endpoint, err := buildChatCompletionURL(cfg.AIBaseURL)
	if err != nil {
		return "", 0, err
	}

	systemPrompt := `
你是资深运维知识库助手。
你必须只根据提供的知识条目回答，不允许编造。
输出 JSON 对象：
answer: 中文回答，最多 5 句，强调可执行动作
confidence: 0 到 1 的小数
禁止输出 Markdown 和多余字段。
`
	userContent := buildKnowledgeRAGContent(payload)
	requestBody := openAIChatRequest{
		Model: cfg.AIModel,
		Messages: []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		Temperature: 0.2,
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request failed: %w", err)
	}

	timeout := parseAITimeout(cfg.AITimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", 0, fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AIAPIKey)

	client := &http.Client{Timeout: timeout + 2*time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("ai response status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed openAIChatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", 0, fmt.Errorf("parse response failed: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", 0, fmt.Errorf("empty ai response")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", 0, fmt.Errorf("empty ai content")
	}
	return parseKnowledgeAIResponse(content)
}

func (h *handler) resolveKBConfig() *models.Config {
	if h == nil {
		return nil
	}
	if h.fs != nil {
		if runtime := h.fs.Config(); runtime != nil {
			return runtime
		}
	}
	return h.cfg
}

func buildKnowledgeRAGContent(payload ragPayload) string {
	builder := strings.Builder{}
	builder.WriteString("问题:\n")
	builder.WriteString(payload.Question)
	builder.WriteString("\n\n候选知识条目:\n")
	for idx, article := range payload.Articles {
		builder.WriteString(fmt.Sprintf("[%d] %s (id=%s, version=%d, severity=%s)\n",
			idx+1, article.Title, article.ID, article.CurrentVersion, article.Severity))
		if summary := strings.TrimSpace(article.Summary); summary != "" {
			builder.WriteString("摘要: ")
			builder.WriteString(summary)
			builder.WriteString("\n")
		}
		content := strings.TrimSpace(article.Content)
		if content != "" {
			content = trimRunes(content, 800)
			builder.WriteString("正文片段: ")
			builder.WriteString(content)
			builder.WriteString("\n")
		}
		if len(article.Tags) > 0 {
			builder.WriteString("标签: ")
			builder.WriteString(strings.Join(article.Tags, ", "))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("请基于以上条目给出答案。\n")
	builder.WriteString("注意：最终引用列表由系统追加，你只输出 answer/confidence。")
	return builder.String()
}

// parseKnowledgeAIResponse 对模型输出做强约束解析
// 解析失败直接上抛 由上层触发降级路径 保证返回字段稳定
func parseKnowledgeAIResponse(raw string) (string, float64, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "", 0, fmt.Errorf("empty ai response")
	}
	var payload struct {
		Answer     string   `json:"answer"`
		Confidence *float64 `json:"confidence,omitempty"`
	}
	if err := json.Unmarshal([]byte(clean), &payload); err != nil {
		extracted := extractJSONObject(clean)
		if extracted == "" {
			return "", 0, err
		}
		if err := json.Unmarshal([]byte(extracted), &payload); err != nil {
			return "", 0, err
		}
	}
	answer := strings.TrimSpace(payload.Answer)
	if answer == "" {
		return "", 0, fmt.Errorf("ai answer is empty")
	}
	confidence := 0.78
	if payload.Confidence != nil {
		if *payload.Confidence >= 0 && *payload.Confidence <= 1 {
			confidence = *payload.Confidence
		}
	}
	return answer, confidence, nil
}

func trimRunes(input string, max int) string {
	if max <= 0 {
		return strings.TrimSpace(input)
	}
	r := []rune(strings.TrimSpace(input))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "..."
}

func buildQuestionCandidates(question string) []string {
	clean := strings.TrimSpace(question)
	if clean == "" {
		return nil
	}
	candidates := []string{clean}
	normalized := strings.NewReplacer(
		"？", " ",
		"?", " ",
		"，", " ",
		",", " ",
		"。", " ",
		".", " ",
		"！", " ",
		"!", " ",
		"；", " ",
		";", " ",
	).Replace(clean)
	for _, token := range strings.Fields(normalized) {
		if len([]rune(token)) >= 2 {
			candidates = append(candidates, token)
		}
	}
	runes := []rune(clean)
	for _, n := range []int{8, 6, 4} {
		if len(runes) >= n {
			candidates = append(candidates, string(runes[:n]))
		}
	}
	out := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		c := strings.TrimSpace(candidate)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}
