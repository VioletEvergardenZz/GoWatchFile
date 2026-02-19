package api

import (
	"net/http"
	"testing"
	"time"
)

func TestControlTaskFailureReasons_DefaultAndStatusFilter(t *testing.T) {
	h := &handler{}
	agentID := registerFailureSummaryAgent(t, h)

	mustRunFailedTask(t, h, agentID, "oss timeout")
	mustRunFailedTask(t, h, agentID, "permission denied")
	mustRunFailedTask(t, h, agentID, "oss timeout")
	mustRunTimeoutTask(t, h, agentID)
	mustRunCanceledTask(t, h)

	resp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodGet, "/api/control/tasks/failure-reasons?limit=10", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("failure reasons list failed: status=%d body=%s", resp.Code, resp.Body.String())
	}

	var listed struct {
		OK    bool                          `json:"ok"`
		Items []controlTaskFailureReasonDTO `json:"items"`
		Total int                           `json:"total"`
	}
	mustDecodeJSON(t, resp.Body.Bytes(), &listed)
	if !listed.OK {
		t.Fatalf("unexpected response: %+v", listed)
	}
	countByReason := map[string]int{}
	for _, item := range listed.Items {
		countByReason[item.Reason] = item.Count
	}
	if countByReason["oss timeout"] != 2 {
		t.Fatalf("expected reason oss timeout=2, got %d", countByReason["oss timeout"])
	}
	if countByReason["permission denied"] != 1 {
		t.Fatalf("expected reason permission denied=1, got %d", countByReason["permission denied"])
	}
	if countByReason["run_timeout"] != 1 {
		t.Fatalf("expected reason run_timeout=1, got %d", countByReason["run_timeout"])
	}
	if _, exists := countByReason["manual_cancel"]; exists {
		t.Fatalf("manual_cancel should not appear in default failed+timeout filter")
	}

	canceledResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodGet, "/api/control/tasks/failure-reasons?status=canceled&limit=10", nil)
	if canceledResp.Code != http.StatusOK {
		t.Fatalf("failure reasons canceled filter failed: status=%d body=%s", canceledResp.Code, canceledResp.Body.String())
	}
	var canceled struct {
		OK    bool                          `json:"ok"`
		Items []controlTaskFailureReasonDTO `json:"items"`
		Total int                           `json:"total"`
	}
	mustDecodeJSON(t, canceledResp.Body.Bytes(), &canceled)
	if !canceled.OK || canceled.Total != 1 || len(canceled.Items) != 1 {
		t.Fatalf("unexpected canceled filter result: %+v", canceled)
	}
	if canceled.Items[0].Reason != "manual_cancel" || canceled.Items[0].Count != 1 {
		t.Fatalf("unexpected canceled reason item: %+v", canceled.Items[0])
	}
}

func TestControlTaskFailureReasons_InvalidStatus(t *testing.T) {
	h := &handler{}

	resp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodGet, "/api/control/tasks/failure-reasons?status=running", nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for invalid status filter, got status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func registerFailureSummaryAgent(t *testing.T, h *handler) string {
	t.Helper()
	resp := doJSONRequest(t, h.controlAgentsHandler, http.MethodPost, "/api/control/agents", map[string]any{
		"agentKey": "agent-failure-summary",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("register agent failed: status=%d body=%s", resp.Code, resp.Body.String())
	}
	var registered struct {
		OK    bool            `json:"ok"`
		Agent controlAgentDTO `json:"agent"`
	}
	mustDecodeJSON(t, resp.Body.Bytes(), &registered)
	if !registered.OK || registered.Agent.ID == "" {
		t.Fatalf("unexpected register response: %+v", registered)
	}
	return registered.Agent.ID
}

func createFailureSummaryTask(t *testing.T, h *handler, maxRetries int) string {
	t.Helper()
	resp := doJSONRequest(t, h.controlTasksHandler, http.MethodPost, "/api/control/tasks", map[string]any{
		"type":       "manual_upload",
		"target":     "D:/logs/gwf/failure-summary.log",
		"maxRetries": maxRetries,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("create task failed: status=%d body=%s", resp.Code, resp.Body.String())
	}
	var created struct {
		OK   bool           `json:"ok"`
		Task controlTaskDTO `json:"task"`
	}
	mustDecodeJSON(t, resp.Body.Bytes(), &created)
	if !created.OK || created.Task.ID == "" {
		t.Fatalf("unexpected create response: %+v", created)
	}
	return created.Task.ID
}

func assignAndAckFailureSummaryTask(t *testing.T, h *handler, taskID, agentID string) {
	t.Helper()
	pullResp := doJSONRequest(t, h.controlDispatchPullHandler, http.MethodPost, "/api/control/dispatch/pull", map[string]any{
		"agentId":  agentID,
		"maxTasks": 1,
	})
	if pullResp.Code != http.StatusOK {
		t.Fatalf("pull failed: status=%d body=%s", pullResp.Code, pullResp.Body.String())
	}
	ackResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+taskID+"/ack", map[string]any{
		"agentId": agentID,
	})
	if ackResp.Code != http.StatusOK {
		t.Fatalf("ack failed: status=%d body=%s", ackResp.Code, ackResp.Body.String())
	}
}

func mustRunFailedTask(t *testing.T, h *handler, agentID, reason string) {
	t.Helper()
	taskID := createFailureSummaryTask(t, h, 2)
	assignAndAckFailureSummaryTask(t, h, taskID, agentID)
	completeResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+taskID+"/complete", map[string]any{
		"agentId": agentID,
		"status":  "failed",
		"error":   reason,
	})
	if completeResp.Code != http.StatusOK {
		t.Fatalf("complete failed task failed: status=%d body=%s", completeResp.Code, completeResp.Body.String())
	}
}

func mustRunTimeoutTask(t *testing.T, h *handler, agentID string) {
	t.Helper()
	taskID := createFailureSummaryTask(t, h, 1)
	assignAndAckFailureSummaryTask(t, h, taskID, agentID)

	now := time.Now().UTC()
	h.controlMu.Lock()
	state := h.controlTasks[taskID]
	state.RetryCount = state.MaxRetries
	state.UpdatedAt = now.Add(-defaultControlRunTimeout - time.Second)
	h.controlTasks[taskID] = state
	if err := h.controlApplyTimeoutsLocked(now); err != nil {
		h.controlMu.Unlock()
		t.Fatalf("apply timeout failed: %v", err)
	}
	h.controlMu.Unlock()
}

func mustRunCanceledTask(t *testing.T, h *handler) {
	t.Helper()
	taskID := createFailureSummaryTask(t, h, 1)
	cancelResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+taskID+"/cancel", nil)
	if cancelResp.Code != http.StatusOK {
		t.Fatalf("cancel task failed: status=%d body=%s", cancelResp.Code, cancelResp.Body.String())
	}
}
