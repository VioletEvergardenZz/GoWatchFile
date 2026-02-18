// 本文件用于控制面审计日志查询接口
// 审计日志用于回答: 谁 在什么时间 对什么资源 做了什么操作
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type controlAuditLog struct {
	ID           int64
	Operator     string
	Action       string
	ResourceType string
	ResourceID   string
	Detail       map[string]any
	CreatedAt    time.Time
}

type controlAuditLogDTO struct {
	ID           int64          `json:"id"`
	Operator     string         `json:"operator"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId"`
	Detail       map[string]any `json:"detail,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
}

func toControlAuditLogDTO(item controlAuditLog) controlAuditLogDTO {
	return controlAuditLogDTO{
		ID:           item.ID,
		Operator:     item.Operator,
		Action:       item.Action,
		ResourceType: item.ResourceType,
		ResourceID:   item.ResourceID,
		Detail:       item.Detail,
		CreatedAt:    item.CreatedAt,
	}
}

func (h *handler) controlAuditHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	resourceType := strings.TrimSpace(r.URL.Query().Get("resourceType"))
	resourceID := strings.TrimSpace(r.URL.Query().Get("resourceId"))
	operator := strings.TrimSpace(r.URL.Query().Get("operator"))
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	fromTime, err := parseAuditTimeQuery(strings.TrimSpace(r.URL.Query().Get("from")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid from: %v", err)})
		return
	}
	toTime, err := parseAuditTimeQuery(strings.TrimSpace(r.URL.Query().Get("to")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid to: %v", err)})
		return
	}
	if !fromTime.IsZero() && !toTime.IsZero() && fromTime.After(toTime) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid range: from must be <= to"})
		return
	}
	limit := parseControlListLimit(r.URL.Query().Get("limit"), 200, 2000)

	if h == nil || h.controlStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"items": []controlAuditLogDTO{},
			"total": 0,
		})
		return
	}

	items, err := h.controlStore.ListAuditLogs(controlAuditLogFilter{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Operator:     operator,
		Action:       action,
		From:         fromTime,
		To:           toTime,
		Limit:        limit,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	dtos := make([]controlAuditLogDTO, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, toControlAuditLogDTO(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": dtos,
		"total": len(dtos),
	})
}

func parseAuditDetailJSON(raw string) map[string]any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseAuditTimeQuery(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	trimmed := strings.TrimSpace(raw)
	// 首选 RFC3339，兼容 datetime-local
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02T15:04", trimmed); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 or 2006-01-02T15:04")
}
