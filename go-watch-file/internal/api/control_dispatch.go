// 本文件用于控制面任务分发与生命周期动作接口
// 目标是落地最小闭环: pull -> ack -> progress -> complete
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"file-watch/internal/logger"
	"file-watch/internal/metrics"
)

const (
	controlTaskStatusPending  = "pending"
	controlTaskStatusAssigned = "assigned"
	controlTaskStatusRunning  = "running"
	controlTaskStatusSuccess  = "success"
	controlTaskStatusFailed   = "failed"
	controlTaskStatusCanceled = "canceled"
	controlTaskStatusTimeout  = "timeout"

	controlAgentStatusOnline    = "online"
	controlAgentStatusOffline   = "offline"
	controlAgentStatusDraining  = "draining"
	controlAgentDefaultMaxTasks = 1
)

const (
	// assigned 状态下未 ack 的任务超过该时间会回收重新进入 pending
	defaultControlAssignTimeout = 30 * time.Second
	// running 状态下长时间无 progress/complete 更新会被标记为 timeout
	defaultControlRunTimeout = 10 * time.Minute
	// 用于粗略判断 agent 是否在线
	defaultControlAgentOfflineAfter = 45 * time.Second
)

type controlDispatchPullRequest struct {
	AgentID     string   `json:"agentId"`
	AcceptTypes []string `json:"acceptTypes"`
	MaxTasks    int      `json:"maxTasks"`
}

type controlDispatchPullResponse struct {
	OK    bool             `json:"ok"`
	Items []controlTaskDTO `json:"items"`
}

type controlTaskAckRequest struct {
	AgentID string `json:"agentId"`
	Message string `json:"message"`
}

type controlTaskProgressRequest struct {
	AgentID  string `json:"agentId"`
	Message  string `json:"message"`
	Progress int    `json:"progress"`
}

type controlTaskCompleteRequest struct {
	AgentID string `json:"agentId"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

// controlDispatchPullHandler 提供最小“任务拉取”能力
// 设计为 Agent 主动轮询拉取，避免先引入长连接与复杂调度
func (h *handler) controlDispatchPullHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req controlDispatchPullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	req.AgentID = strings.TrimSpace(req.AgentID)
	if req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agentId is required"})
		return
	}
	maxTasks := req.MaxTasks
	if maxTasks <= 0 {
		maxTasks = controlAgentDefaultMaxTasks
	}
	if maxTasks > 10 {
		maxTasks = 10
	}
	acceptTypes := normalizeStringSet(req.AcceptTypes)
	now := time.Now().UTC()

	h.controlMu.Lock()
	defer h.controlMu.Unlock()
	h.ensureControlStoresLocked()

	agent, ok := h.controlAgents[req.AgentID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if agent.Status == controlAgentStatusDraining {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "agent is draining"})
		return
	}
	// pull 请求天然具备“我还活着”的语义，这里把 pull 视为心跳，避免只接入 pull 的 agent 被误判离线
	agent.LastSeenAt = now
	agent.UpdatedAt = now
	agent.HeartbeatCount++
	if agent.Status == controlAgentStatusOffline {
		agent.Status = controlAgentStatusOnline
	}
	if err := h.persistControlAgentLocked(agent); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.controlAgents[agent.ID] = agent

	if err := h.controlApplyTimeoutsLocked(now); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	items := make([]controlTaskDTO, 0, maxTasks)
	for len(items) < maxTasks {
		next, found, err := h.controlAssignNextTaskLocked(agent.ID, acceptTypes, now)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			break
		}
		items = append(items, toControlTaskDTO(next))
	}

	writeJSON(w, http.StatusOK, controlDispatchPullResponse{
		OK:    true,
		Items: items,
	})
}

func normalizeStringSet(items []string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range items {
		val := strings.TrimSpace(raw)
		if val == "" {
			continue
		}
		out[val] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func controlAgentIsActive(agent controlAgentState, now time.Time) bool {
	if agent.Status == controlAgentStatusOffline {
		return false
	}
	if agent.LastSeenAt.IsZero() {
		return true
	}
	return now.Sub(agent.LastSeenAt) <= defaultControlAgentOfflineAfter
}

// controlApplyTimeoutsLocked 在每次 pull 前执行一次超时收敛
// 这样可以避免后台守护协程 也能保证调度视图始终包含最新超时状态
func (h *handler) controlApplyTimeoutsLocked(now time.Time) error {
	if h == nil {
		return nil
	}
	// 遍历全量任务并处理超时/回收，保证 pull 时看到的是“可调度视图”
	for id, state := range h.controlTasks {
		switch state.Status {
		case controlTaskStatusAssigned:
			if now.Sub(state.UpdatedAt) <= defaultControlAssignTimeout {
				continue
			}
			next := state
			next.Status = controlTaskStatusPending
			next.AssignedAgentID = ""
			next.UpdatedAt = now
			if err := h.persistControlTaskLocked(next); err != nil {
				return err
			}
			h.controlTasks[id] = next
			h.controlAppendTaskEventLocked(id, state.AssignedAgentID, "assign_timeout", "任务分配超时，已回收重新排队", now)
			continue
		case controlTaskStatusRunning:
			if now.Sub(state.UpdatedAt) <= defaultControlRunTimeout {
				continue
			}
			next := state
			next.Status = controlTaskStatusTimeout
			next.UpdatedAt = now
			next.FinishedAt = &now
			if err := h.persistControlTaskLocked(next); err != nil {
				return err
			}
			h.controlTasks[id] = next
			h.controlAppendTaskEventLocked(id, state.AssignedAgentID, "timeout", "任务运行超时，已标记为 timeout", now)
			h.controlAppendAuditLogLocked("system", "task_timeout", "task", id, map[string]any{
				"agentId": state.AssignedAgentID,
				"type":    state.Type,
			}, now)
			metrics.Global().IncControlTaskTimeout()
			metrics.Global().ObserveControlTaskDuration(state.Type, controlTaskStatusTimeout, now.Sub(state.CreatedAt))
			continue
		default:
			continue
		}
	}
	return nil
}

// controlAssignNextTaskLocked 执行“优先级优先 先进先出”的任务分配
// 同优先级下按创建时间排序 保证调度结果可预测且便于复盘
func (h *handler) controlAssignNextTaskLocked(agentID string, acceptTypes map[string]bool, now time.Time) (controlTaskState, bool, error) {
	candidates := make([]controlTaskState, 0)
	for _, state := range h.controlTasks {
		if state.Status != controlTaskStatusPending {
			continue
		}
		if len(acceptTypes) > 0 && !acceptTypes[state.Type] {
			continue
		}
		candidates = append(candidates, state)
	}
	if len(candidates) == 0 {
		return controlTaskState{}, false, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		pi := controlPriorityWeight(candidates[i].Priority)
		pj := controlPriorityWeight(candidates[j].Priority)
		if pi != pj {
			return pi > pj
		}
		if !candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
		}
		return candidates[i].ID < candidates[j].ID
	})
	selected := candidates[0]
	next := selected
	next.Status = controlTaskStatusAssigned
	next.AssignedAgentID = agentID
	next.UpdatedAt = now
	if err := h.persistControlTaskLocked(next); err != nil {
		return controlTaskState{}, false, err
	}
	h.controlTasks[next.ID] = next
	h.controlAppendTaskEventLocked(next.ID, agentID, "assigned", "任务已分配给 agent", now)
	h.controlAppendAuditLogLocked(agentID, "task_assigned", "task", next.ID, map[string]any{
		"type":     next.Type,
		"priority": next.Priority,
	}, now)
	return next, true, nil
}

func controlPriorityWeight(priority string) int {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "high":
		return 3
	case "normal":
		return 2
	case "low":
		return 1
	default:
		return 2
	}
}

// controlAckTask 把 assigned 任务推进到 running
// 只有分配给当前 agent 的任务才允许 ACK 防止跨 agent 抢占
func (h *handler) controlAckTask(w http.ResponseWriter, r *http.Request, taskID string) {
	var req controlTaskAckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	req.AgentID = strings.TrimSpace(req.AgentID)
	if req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agentId is required"})
		return
	}
	now := time.Now().UTC()

	h.controlMu.Lock()
	defer h.controlMu.Unlock()
	h.ensureControlStoresLocked()

	state, ok := h.controlTasks[taskID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	if state.Status != controlTaskStatusAssigned {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "task is not assigned"})
		return
	}
	if state.AssignedAgentID != req.AgentID {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "task is assigned to another agent"})
		return
	}
	next := state
	next.Status = controlTaskStatusRunning
	next.UpdatedAt = now
	if err := h.persistControlTaskLocked(next); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.controlTasks[taskID] = next

	message := strings.TrimSpace(req.Message)
	h.controlAppendTaskEventLocked(taskID, req.AgentID, "started", message, now)
	h.controlAppendAuditLogLocked(req.AgentID, "task_ack", "task", taskID, map[string]any{
		"message": message,
	}, now)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"task": toControlTaskDTO(next),
	})
}

// controlProgressTask 更新运行中任务的进度心跳
// 该接口不改变终态 只刷新 updatedAt 供超时判断使用
func (h *handler) controlProgressTask(w http.ResponseWriter, r *http.Request, taskID string) {
	var req controlTaskProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	req.AgentID = strings.TrimSpace(req.AgentID)
	if req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agentId is required"})
		return
	}
	now := time.Now().UTC()

	h.controlMu.Lock()
	defer h.controlMu.Unlock()
	h.ensureControlStoresLocked()

	state, ok := h.controlTasks[taskID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	if state.Status != controlTaskStatusRunning {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "task is not running"})
		return
	}
	if state.AssignedAgentID != req.AgentID {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "task is assigned to another agent"})
		return
	}

	next := state
	next.UpdatedAt = now
	if err := h.persistControlTaskLocked(next); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.controlTasks[taskID] = next

	message := strings.TrimSpace(req.Message)
	if message == "" && req.Progress > 0 {
		message = "progress=" + strconv.Itoa(req.Progress)
	}
	h.controlAppendTaskEventLocked(taskID, req.AgentID, "progress", message, now)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"task": toControlTaskDTO(next),
	})
}

// controlCompleteTask 负责任务终态收口
// 成功 失败 超时都在这里统一落库并写入审计 便于指标统计一致
func (h *handler) controlCompleteTask(w http.ResponseWriter, r *http.Request, taskID string) {
	var req controlTaskCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	req.AgentID = strings.TrimSpace(req.AgentID)
	if req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agentId is required"})
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != controlTaskStatusSuccess && status != controlTaskStatusFailed {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be success or failed"})
		return
	}
	now := time.Now().UTC()

	h.controlMu.Lock()
	defer h.controlMu.Unlock()
	h.ensureControlStoresLocked()

	state, ok := h.controlTasks[taskID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	if isTaskTerminal(state.Status) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "task already finished"})
		return
	}
	if state.Status != controlTaskStatusAssigned && state.Status != controlTaskStatusRunning {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "task status does not support complete"})
		return
	}
	if state.AssignedAgentID != req.AgentID {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "task is assigned to another agent"})
		return
	}

	next := state
	next.Status = status
	next.UpdatedAt = now
	next.FinishedAt = &now
	if err := h.persistControlTaskLocked(next); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.controlTasks[taskID] = next

	message := strings.TrimSpace(req.Message)
	if message == "" && strings.TrimSpace(req.Error) != "" {
		message = "error=" + strings.TrimSpace(req.Error)
	}
	eventType := "succeeded"
	if status == controlTaskStatusFailed {
		eventType = "failed"
	}
	h.controlAppendTaskEventLocked(taskID, req.AgentID, eventType, message, now)
	h.controlAppendAuditLogLocked(req.AgentID, "task_complete", "task", taskID, map[string]any{
		"status":  status,
		"message": message,
	}, now)

	metrics.Global().ObserveControlTaskDuration(state.Type, status, now.Sub(state.CreatedAt))

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"task": toControlTaskDTO(next),
	})
}

func (h *handler) controlAppendTaskEventLocked(taskID, agentID, eventType, message string, at time.Time) {
	if h == nil || h.controlStore == nil {
		return
	}
	if strings.TrimSpace(message) == "" {
		message = ""
	}
	if err := h.controlStore.InsertTaskEvent(taskID, agentID, eventType, message, at); err != nil {
		logger.Warn("写入控制面任务事件失败: task=%s event=%s err=%v", taskID, eventType, err)
	}
}

func (h *handler) controlAppendAuditLogLocked(operator, action, resourceType, resourceID string, detail map[string]any, at time.Time) {
	if h == nil || h.controlStore == nil {
		return
	}
	if err := h.controlStore.InsertAuditLog(operator, action, resourceType, resourceID, detail, at); err != nil {
		logger.Warn("写入控制面审计日志失败: action=%s resource=%s/%s err=%v", action, resourceType, resourceID, err)
	}
}
