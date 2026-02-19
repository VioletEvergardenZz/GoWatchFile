package api

import (
	"net/http"
	"testing"
	"time"
)

func TestControlTask_AssignTimeoutRequeue(t *testing.T) {
	h, taskID, _ := prepareAssignedTask(t, 2)
	now := time.Now().UTC()

	h.controlMu.Lock()
	state := h.controlTasks[taskID]
	state.UpdatedAt = now.Add(-defaultControlAssignTimeout - time.Second)
	h.controlTasks[taskID] = state
	if err := h.controlApplyTimeoutsLocked(now); err != nil {
		h.controlMu.Unlock()
		t.Fatalf("apply timeouts failed: %v", err)
	}
	got := h.controlTasks[taskID]
	h.controlMu.Unlock()

	if got.Status != controlTaskStatusPending {
		t.Fatalf("expected status pending after assign timeout, got %s", got.Status)
	}
	if got.AssignedAgentID != "" {
		t.Fatalf("expected assignedAgentID cleared after assign timeout, got %s", got.AssignedAgentID)
	}
	if got.RetryCount != 0 {
		t.Fatalf("expected retryCount unchanged after assign timeout, got %d", got.RetryCount)
	}
}

func TestControlTask_RunTimeoutAutoRetry(t *testing.T) {
	h, taskID, _ := prepareRunningTask(t, 2)
	now := time.Now().UTC()

	h.controlMu.Lock()
	state := h.controlTasks[taskID]
	state.UpdatedAt = now.Add(-defaultControlRunTimeout - time.Second)
	h.controlTasks[taskID] = state
	if err := h.controlApplyTimeoutsLocked(now); err != nil {
		h.controlMu.Unlock()
		t.Fatalf("apply timeouts failed: %v", err)
	}
	got := h.controlTasks[taskID]
	h.controlMu.Unlock()

	if got.Status != controlTaskStatusPending {
		t.Fatalf("expected status pending after auto retry, got %s", got.Status)
	}
	if got.RetryCount != 1 {
		t.Fatalf("expected retryCount=1 after auto retry, got %d", got.RetryCount)
	}
	if got.AssignedAgentID != "" {
		t.Fatalf("expected assignedAgentID cleared after auto retry, got %s", got.AssignedAgentID)
	}
	if got.FinishedAt != nil {
		t.Fatalf("expected finishedAt to be nil after auto retry")
	}
}

func TestControlTask_RunTimeoutNoRetryBudget(t *testing.T) {
	h, taskID, _ := prepareRunningTask(t, 1)
	now := time.Now().UTC()

	h.controlMu.Lock()
	state := h.controlTasks[taskID]
	state.RetryCount = 1
	state.UpdatedAt = now.Add(-defaultControlRunTimeout - time.Second)
	h.controlTasks[taskID] = state
	if err := h.controlApplyTimeoutsLocked(now); err != nil {
		h.controlMu.Unlock()
		t.Fatalf("apply timeouts failed: %v", err)
	}
	got := h.controlTasks[taskID]
	h.controlMu.Unlock()

	if got.Status != controlTaskStatusTimeout {
		t.Fatalf("expected status timeout when retry budget exhausted, got %s", got.Status)
	}
	if got.RetryCount != 1 {
		t.Fatalf("expected retryCount unchanged when timeout finalized, got %d", got.RetryCount)
	}
	if got.FinishedAt == nil {
		t.Fatalf("expected finishedAt set when timeout finalized")
	}
}

func TestControlTask_RetryRejectsPending(t *testing.T) {
	h := &handler{}
	taskID := createTaskOnly(t, h, 2)

	retryResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+taskID+"/retry", nil)
	if retryResp.Code != http.StatusConflict {
		t.Fatalf("expected retry conflict for pending task, got status=%d body=%s", retryResp.Code, retryResp.Body.String())
	}
}

func TestControlTask_CancelRejectsTerminal(t *testing.T) {
	h, taskID, agentID := prepareRunningTask(t, 1)
	completeResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+taskID+"/complete", map[string]any{
		"agentId": agentID,
		"status":  "success",
	})
	if completeResp.Code != http.StatusOK {
		t.Fatalf("complete failed: status=%d body=%s", completeResp.Code, completeResp.Body.String())
	}

	cancelResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+taskID+"/cancel", nil)
	if cancelResp.Code != http.StatusConflict {
		t.Fatalf("expected cancel conflict for terminal task, got status=%d body=%s", cancelResp.Code, cancelResp.Body.String())
	}
}

func prepareAssignedTask(t *testing.T, maxRetries int) (*handler, string, string) {
	t.Helper()
	h := &handler{}
	agentID := registerTestAgent(t, h)
	taskID := createTaskOnly(t, h, maxRetries)

	pullResp := doJSONRequest(t, h.controlDispatchPullHandler, http.MethodPost, "/api/control/dispatch/pull", map[string]any{
		"agentId":  agentID,
		"maxTasks": 1,
	})
	if pullResp.Code != http.StatusOK {
		t.Fatalf("pull failed: status=%d body=%s", pullResp.Code, pullResp.Body.String())
	}

	h.controlMu.RLock()
	state := h.controlTasks[taskID]
	h.controlMu.RUnlock()
	if state.Status != controlTaskStatusAssigned {
		t.Fatalf("expected task assigned after pull, got %s", state.Status)
	}
	return h, taskID, agentID
}

func prepareRunningTask(t *testing.T, maxRetries int) (*handler, string, string) {
	t.Helper()
	h, taskID, agentID := prepareAssignedTask(t, maxRetries)

	ackResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+taskID+"/ack", map[string]any{
		"agentId": agentID,
	})
	if ackResp.Code != http.StatusOK {
		t.Fatalf("ack failed: status=%d body=%s", ackResp.Code, ackResp.Body.String())
	}

	h.controlMu.RLock()
	state := h.controlTasks[taskID]
	h.controlMu.RUnlock()
	if state.Status != controlTaskStatusRunning {
		t.Fatalf("expected task running after ack, got %s", state.Status)
	}
	return h, taskID, agentID
}

func registerTestAgent(t *testing.T, h *handler) string {
	t.Helper()
	registerResp := doJSONRequest(t, h.controlAgentsHandler, http.MethodPost, "/api/control/agents", map[string]any{
		"agentKey": "agent-state-machine",
	})
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register agent failed: status=%d body=%s", registerResp.Code, registerResp.Body.String())
	}
	var registered struct {
		OK    bool            `json:"ok"`
		Agent controlAgentDTO `json:"agent"`
	}
	mustDecodeJSON(t, registerResp.Body.Bytes(), &registered)
	if !registered.OK || registered.Agent.ID == "" {
		t.Fatalf("unexpected register response: %+v", registered)
	}
	return registered.Agent.ID
}

func createTaskOnly(t *testing.T, h *handler, maxRetries int) string {
	t.Helper()
	createResp := doJSONRequest(t, h.controlTasksHandler, http.MethodPost, "/api/control/tasks", map[string]any{
		"type":       "manual_upload",
		"target":     "D:/logs/gwf/state-machine.log",
		"maxRetries": maxRetries,
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create task failed: status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		OK   bool           `json:"ok"`
		Task controlTaskDTO `json:"task"`
	}
	mustDecodeJSON(t, createResp.Body.Bytes(), &created)
	if !created.OK || created.Task.ID == "" {
		t.Fatalf("unexpected create response: %+v", created)
	}
	return created.Task.ID
}
