// 本文件用于控制面任务事件与审计信息查询接口
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package api

import (
	"net/http"
	"strings"
	"time"
)

type controlTaskEvent struct {
	ID        int64
	TaskID    string
	AgentID   string
	EventType string
	Message   string
	EventTime time.Time
}

type controlTaskEventDTO struct {
	ID        int64     `json:"id"`
	TaskID    string    `json:"taskId"`
	AgentID   string    `json:"agentId,omitempty"`
	EventType string    `json:"eventType"`
	Message   string    `json:"message,omitempty"`
	EventTime time.Time `json:"eventTime"`
}

func toControlTaskEventDTO(item controlTaskEvent) controlTaskEventDTO {
	return controlTaskEventDTO{
		ID:        item.ID,
		TaskID:    item.TaskID,
		AgentID:   item.AgentID,
		EventType: item.EventType,
		Message:   item.Message,
		EventTime: item.EventTime,
	}
}

func (h *handler) controlListTaskEvents(w http.ResponseWriter, r *http.Request, taskID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
		return
	}
	limit := parseControlListLimit(r.URL.Query().Get("limit"), 200, 2000)
	if h == nil || h.controlStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"items": []controlTaskEventDTO{},
			"total": 0,
		})
		return
	}
	items, err := h.controlStore.ListTaskEvents(taskID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	dtos := make([]controlTaskEventDTO, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, toControlTaskEventDTO(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": dtos,
		"total": len(dtos),
	})
}
