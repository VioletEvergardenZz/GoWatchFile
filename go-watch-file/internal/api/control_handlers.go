// 本文件用于控制面 MVP 的 Agent/Task 最小接口。
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type controlAgentState struct {
	ID             string
	AgentKey       string
	Hostname       string
	Version        string
	IP             string
	GroupName      string
	Status         string
	LastSeenAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	HeartbeatCount uint64
}

type controlTaskState struct {
	ID              string
	Type            string
	Target          string
	Payload         map[string]any
	Priority        string
	Status          string
	AssignedAgentID string
	RetryCount      int
	MaxRetries      int
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	FinishedAt      *time.Time
}

type controlAgentDTO struct {
	ID             string    `json:"id"`
	AgentKey       string    `json:"agentKey"`
	Hostname       string    `json:"hostname"`
	Version        string    `json:"version"`
	IP             string    `json:"ip"`
	GroupName      string    `json:"groupName"`
	Status         string    `json:"status"`
	LastSeenAt     time.Time `json:"lastSeenAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	HeartbeatCount uint64    `json:"heartbeatCount"`
}

type controlTaskDTO struct {
	ID              string         `json:"id"`
	Type            string         `json:"type"`
	Target          string         `json:"target"`
	Payload         map[string]any `json:"payload,omitempty"`
	Priority        string         `json:"priority"`
	Status          string         `json:"status"`
	AssignedAgentID string         `json:"assignedAgentId,omitempty"`
	RetryCount      int            `json:"retryCount"`
	MaxRetries      int            `json:"maxRetries"`
	CreatedBy       string         `json:"createdBy"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	FinishedAt      *time.Time     `json:"finishedAt,omitempty"`
}

type controlRegisterAgentRequest struct {
	AgentKey  string `json:"agentKey"`
	Hostname  string `json:"hostname"`
	Version   string `json:"version"`
	IP        string `json:"ip"`
	GroupName string `json:"groupName"`
}

type controlAgentHeartbeatRequest struct {
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	IP       string `json:"ip"`
}

type controlCreateTaskRequest struct {
	Type       string         `json:"type"`
	Target     string         `json:"target"`
	Payload    map[string]any `json:"payload"`
	Priority   string         `json:"priority"`
	CreatedBy  string         `json:"createdBy"`
	MaxRetries int            `json:"maxRetries"`
}

func (h *handler) controlAgentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch r.Method {
	case http.MethodPost:
		h.controlRegisterAgent(w, r)
	case http.MethodGet:
		h.controlListAgents(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *handler) controlAgentByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// 这里采用前缀切分做子路由，避免在 net/http 上额外引入复杂路由依赖
	parts := splitPathSegments("/api/control/agents/", r.URL.Path)
	if len(parts) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
		return
	}
	agentID := parts[0]
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		h.controlGetAgentByID(w, agentID)
	case len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "heartbeat":
		h.controlHeartbeatAgent(w, r, agentID)
	case len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "drain":
		h.controlDrainAgent(w, agentID)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
	}
}

func (h *handler) controlTasksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch r.Method {
	case http.MethodPost:
		h.controlCreateTask(w, r)
	case http.MethodGet:
		h.controlListTasks(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *handler) controlTaskByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// 任务详情与任务动作共用一个入口，通过路径段决定具体动作
	parts := splitPathSegments("/api/control/tasks/", r.URL.Path)
	if len(parts) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
		return
	}
	taskID := parts[0]
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		h.controlGetTaskByID(w, taskID)
	case len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "cancel":
		h.controlCancelTask(w, taskID)
	case len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "retry":
		h.controlRetryTask(w, taskID)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
	}
}

func (h *handler) controlRegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req controlRegisterAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	req.AgentKey = strings.TrimSpace(req.AgentKey)
	if req.AgentKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agentKey is required"})
		return
	}
	now := time.Now().UTC()
	h.controlMu.Lock()
	defer h.controlMu.Unlock()
	h.ensureControlStoresLocked()

	// 使用 agentKey 作为幂等键，同一个 agentKey 重复注册会更新信息而不是创建新记录
	agentID, found := h.controlAgentKeyIdx[req.AgentKey]
	created := !found
	state := controlAgentState{}
	if found {
		state = h.controlAgents[agentID]
	} else {
		agentID = h.nextControlAgentIDLocked()
		state = controlAgentState{
			ID:        agentID,
			AgentKey:  req.AgentKey,
			CreatedAt: now,
		}
	}
	state.AgentKey = req.AgentKey
	state.Hostname = strings.TrimSpace(req.Hostname)
	state.Version = strings.TrimSpace(req.Version)
	state.IP = strings.TrimSpace(req.IP)
	state.GroupName = normalizeControlGroup(strings.TrimSpace(req.GroupName))
	state.Status = normalizeControlAgentStatus(state.Status)
	state.LastSeenAt = now
	state.UpdatedAt = now

	if err := h.persistControlAgentLocked(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.controlAgents[state.ID] = state
	h.controlAgentKeyIdx[state.AgentKey] = state.ID

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"created": created,
		"agent":   toControlAgentDTO(state),
	})
}

func (h *handler) controlListAgents(w http.ResponseWriter, r *http.Request) {
	statusFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	groupFilter := strings.TrimSpace(r.URL.Query().Get("group"))

	h.controlMu.RLock()
	defer h.controlMu.RUnlock()

	items := make([]controlAgentDTO, 0, len(h.controlAgents))
	for _, state := range h.controlAgents {
		if statusFilter != "" && state.Status != statusFilter {
			continue
		}
		if groupFilter != "" && state.GroupName != groupFilter {
			continue
		}
		items = append(items, toControlAgentDTO(state))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].LastSeenAt.After(items[j].LastSeenAt)
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
		"total": len(items),
	})
}

func (h *handler) controlGetAgentByID(w http.ResponseWriter, agentID string) {
	h.controlMu.RLock()
	defer h.controlMu.RUnlock()

	state, ok := h.controlAgents[agentID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"agent": toControlAgentDTO(state),
	})
}

func (h *handler) controlHeartbeatAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	var req controlAgentHeartbeatRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	now := time.Now().UTC()

	h.controlMu.Lock()
	defer h.controlMu.Unlock()
	h.ensureControlStoresLocked()

	state, ok := h.controlAgents[agentID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if v := strings.TrimSpace(req.Hostname); v != "" {
		state.Hostname = v
	}
	if v := strings.TrimSpace(req.Version); v != "" {
		state.Version = v
	}
	if v := strings.TrimSpace(req.IP); v != "" {
		state.IP = v
	}
	// 心跳会把离线节点拉回在线，并累计心跳次数供可观测与排障使用
	if state.Status == "offline" {
		state.Status = "online"
	}
	state.LastSeenAt = now
	state.UpdatedAt = now
	state.HeartbeatCount++
	if err := h.persistControlAgentLocked(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.controlAgents[agentID] = state

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"agent": toControlAgentDTO(state),
	})
}

func (h *handler) controlDrainAgent(w http.ResponseWriter, agentID string) {
	now := time.Now().UTC()

	h.controlMu.Lock()
	defer h.controlMu.Unlock()
	h.ensureControlStoresLocked()

	state, ok := h.controlAgents[agentID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	state.Status = "draining"
	state.UpdatedAt = now
	if err := h.persistControlAgentLocked(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.controlAgents[agentID] = state

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"agent": toControlAgentDTO(state),
	})
}

func (h *handler) controlCreateTask(w http.ResponseWriter, r *http.Request) {
	var req controlCreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	req.Type = strings.TrimSpace(req.Type)
	req.Target = strings.TrimSpace(req.Target)
	if req.Type == "" || req.Target == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type and target are required"})
		return
	}
	// 控制面任务默认三次重试，避免未传参数时出现不可重试任务
	if req.MaxRetries <= 0 {
		req.MaxRetries = 3
	}
	now := time.Now().UTC()

	h.controlMu.Lock()
	defer h.controlMu.Unlock()
	h.ensureControlStoresLocked()

	taskID := h.nextControlTaskIDLocked()
	state := controlTaskState{
		ID:         taskID,
		Type:       req.Type,
		Target:     req.Target,
		Payload:    cloneMap(req.Payload),
		Priority:   normalizeControlPriority(req.Priority),
		Status:     "pending",
		CreatedBy:  normalizeCreatedBy(req.CreatedBy),
		MaxRetries: req.MaxRetries,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := h.persistControlTaskLocked(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.controlTasks[state.ID] = state

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"task": toControlTaskDTO(state),
	})
}

func (h *handler) controlListTasks(w http.ResponseWriter, r *http.Request) {
	statusFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	limit := parseControlListLimit(r.URL.Query().Get("limit"), 200, 1000)

	h.controlMu.RLock()
	defer h.controlMu.RUnlock()

	filtered := make([]controlTaskDTO, 0, len(h.controlTasks))
	for _, state := range h.controlTasks {
		if statusFilter != "" && state.Status != statusFilter {
			continue
		}
		if typeFilter != "" && state.Type != typeFilter {
			continue
		}
		filtered = append(filtered, toControlTaskDTO(state))
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	total := len(filtered)
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": filtered,
		"total": total,
	})
}

func (h *handler) controlGetTaskByID(w http.ResponseWriter, taskID string) {
	h.controlMu.RLock()
	defer h.controlMu.RUnlock()

	state, ok := h.controlTasks[taskID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"task": toControlTaskDTO(state),
	})
}

func (h *handler) controlCancelTask(w http.ResponseWriter, taskID string) {
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
	state.Status = "canceled"
	state.UpdatedAt = now
	state.FinishedAt = &now
	if err := h.persistControlTaskLocked(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.controlTasks[taskID] = state

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"task": toControlTaskDTO(state),
	})
}

func (h *handler) controlRetryTask(w http.ResponseWriter, taskID string) {
	now := time.Now().UTC()
	h.controlMu.Lock()
	defer h.controlMu.Unlock()
	h.ensureControlStoresLocked()

	state, ok := h.controlTasks[taskID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	// 仅允许终态任务进入重试，避免运行中任务被重复投递
	if state.Status != "failed" && state.Status != "timeout" && state.Status != "canceled" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "task status does not support retry"})
		return
	}
	// 达到最大重试次数后强制拒绝，防止异常任务无限循环
	if state.RetryCount >= state.MaxRetries {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "max retries reached"})
		return
	}
	state.RetryCount++
	state.Status = "pending"
	state.AssignedAgentID = ""
	state.UpdatedAt = now
	state.FinishedAt = nil
	if err := h.persistControlTaskLocked(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.controlTasks[taskID] = state

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"task": toControlTaskDTO(state),
	})
}

func (h *handler) ensureControlStoresLocked() {
	if h.controlAgents == nil {
		h.controlAgents = make(map[string]controlAgentState)
	}
	if h.controlAgentKeyIdx == nil {
		h.controlAgentKeyIdx = make(map[string]string)
	}
	if h.controlTasks == nil {
		h.controlTasks = make(map[string]controlTaskState)
	}
}

func (h *handler) loadControlSnapshot() error {
	if h == nil || h.controlStore == nil {
		return nil
	}
	agents, err := h.controlStore.LoadAgents()
	if err != nil {
		return fmt.Errorf("load control agents failed: %w", err)
	}
	tasks, err := h.controlStore.LoadTasks()
	if err != nil {
		return fmt.Errorf("load control tasks failed: %w", err)
	}
	h.controlMu.Lock()
	defer h.controlMu.Unlock()
	h.ensureControlStoresLocked()

	// 启动恢复时不仅要恢复数据，还要恢复自增序列，保证后续 ID 连续且不冲突
	for _, item := range agents {
		h.controlAgents[item.ID] = item
		if item.AgentKey != "" {
			h.controlAgentKeyIdx[item.AgentKey] = item.ID
		}
		if seq := controlIDSequence(item.ID, "agt-"); seq > h.controlNextAgentSeq {
			h.controlNextAgentSeq = seq
		}
	}
	for _, item := range tasks {
		h.controlTasks[item.ID] = item
		if seq := controlIDSequence(item.ID, "tsk-"); seq > h.controlNextTaskSeq {
			h.controlNextTaskSeq = seq
		}
	}
	return nil
}

func (h *handler) persistControlAgentLocked(state controlAgentState) error {
	if h == nil || h.controlStore == nil {
		return nil
	}
	if err := h.controlStore.UpsertAgent(state); err != nil {
		return fmt.Errorf("persist control agent failed: %w", err)
	}
	return nil
}

func (h *handler) persistControlTaskLocked(state controlTaskState) error {
	if h == nil || h.controlStore == nil {
		return nil
	}
	if err := h.controlStore.UpsertTask(state); err != nil {
		return fmt.Errorf("persist control task failed: %w", err)
	}
	return nil
}

func (h *handler) nextControlAgentIDLocked() string {
	h.controlNextAgentSeq++
	return fmt.Sprintf("agt-%06d", h.controlNextAgentSeq)
}

func (h *handler) nextControlTaskIDLocked() string {
	h.controlNextTaskSeq++
	return fmt.Sprintf("tsk-%06d", h.controlNextTaskSeq)
}

func toControlAgentDTO(state controlAgentState) controlAgentDTO {
	return controlAgentDTO{
		ID:             state.ID,
		AgentKey:       state.AgentKey,
		Hostname:       state.Hostname,
		Version:        state.Version,
		IP:             state.IP,
		GroupName:      state.GroupName,
		Status:         state.Status,
		LastSeenAt:     state.LastSeenAt,
		CreatedAt:      state.CreatedAt,
		UpdatedAt:      state.UpdatedAt,
		HeartbeatCount: state.HeartbeatCount,
	}
}

func toControlTaskDTO(state controlTaskState) controlTaskDTO {
	return controlTaskDTO{
		ID:              state.ID,
		Type:            state.Type,
		Target:          state.Target,
		Payload:         cloneMap(state.Payload),
		Priority:        state.Priority,
		Status:          state.Status,
		AssignedAgentID: state.AssignedAgentID,
		RetryCount:      state.RetryCount,
		MaxRetries:      state.MaxRetries,
		CreatedBy:       state.CreatedBy,
		CreatedAt:       state.CreatedAt,
		UpdatedAt:       state.UpdatedAt,
		FinishedAt:      state.FinishedAt,
	}
}

func normalizeControlAgentStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "offline":
		return "offline"
	case "draining":
		return "draining"
	default:
		return "online"
	}
}

func normalizeControlGroup(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "default"
	}
	return strings.TrimSpace(raw)
}

func normalizeControlPriority(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low":
		return "low"
	case "high":
		return "high"
	default:
		return "normal"
	}
}

func normalizeCreatedBy(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "console"
	}
	return strings.TrimSpace(raw)
}

func parseControlListLimit(raw string, defaultLimit, maxLimit int) int {
	if strings.TrimSpace(raw) == "" {
		return defaultLimit
	}
	// 解析失败或非正数统一回退默认值，避免把异常参数扩散到查询层
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return defaultLimit
	}
	if maxLimit > 0 && v > maxLimit {
		return maxLimit
	}
	return v
}

func splitPathSegments(prefix, path string) []string {
	// 统一切掉前缀与多余斜杠，返回稳定的路径段数组供上层动作分发
	trimmed := strings.TrimPrefix(path, prefix)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		val := strings.TrimSpace(part)
		if val == "" {
			continue
		}
		out = append(out, val)
	}
	return out
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func isTaskTerminal(status string) bool {
	return status == "success" || status == "failed" || status == "timeout" || status == "canceled"
}
