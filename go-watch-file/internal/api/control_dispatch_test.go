// 本文件用于控制面任务分发闭环测试
// 覆盖 pull -> ack -> complete 的最小生命周期，并校验事件写入能力
package api

import (
	"net/http"
	"testing"
)

func TestControlDispatch_PullAckComplete(t *testing.T) {
	h := &handler{}

	registerResp := doJSONRequest(t, h.controlAgentsHandler, http.MethodPost, "/api/control/agents", map[string]any{
		"agentKey":  "agent-dispatch-1",
		"hostname":  "node-1",
		"version":   "1.0.0",
		"ip":        "10.0.0.11",
		"groupName": "ops",
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

	createResp := doJSONRequest(t, h.controlTasksHandler, http.MethodPost, "/api/control/tasks", map[string]any{
		"type":       "manual_upload",
		"target":     "D:/logs/gwf/replay-1.log",
		"priority":   "normal",
		"createdBy":  "tester",
		"maxRetries": 1,
		"payload": map[string]any{
			"path": "D:/logs/gwf/replay-1.log",
		},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create task failed: status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		OK   bool           `json:"ok"`
		Task controlTaskDTO `json:"task"`
	}
	mustDecodeJSON(t, createResp.Body.Bytes(), &created)
	if !created.OK || created.Task.ID == "" || created.Task.Status != "pending" {
		t.Fatalf("unexpected create response: %+v", created)
	}

	pullResp := doJSONRequest(t, h.controlDispatchPullHandler, http.MethodPost, "/api/control/dispatch/pull", map[string]any{
		"agentId":  registered.Agent.ID,
		"maxTasks": 1,
	})
	if pullResp.Code != http.StatusOK {
		t.Fatalf("pull failed: status=%d body=%s", pullResp.Code, pullResp.Body.String())
	}
	var pulled struct {
		OK    bool             `json:"ok"`
		Items []controlTaskDTO `json:"items"`
	}
	mustDecodeJSON(t, pullResp.Body.Bytes(), &pulled)
	if !pulled.OK || len(pulled.Items) != 1 {
		t.Fatalf("unexpected pull response: %+v", pulled)
	}
	if pulled.Items[0].Status != "assigned" || pulled.Items[0].AssignedAgentID != registered.Agent.ID {
		t.Fatalf("unexpected assigned task: %+v", pulled.Items[0])
	}

	ackResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+created.Task.ID+"/ack", map[string]any{
		"agentId": registered.Agent.ID,
	})
	if ackResp.Code != http.StatusOK {
		t.Fatalf("ack failed: status=%d body=%s", ackResp.Code, ackResp.Body.String())
	}
	var acked struct {
		OK   bool           `json:"ok"`
		Task controlTaskDTO `json:"task"`
	}
	mustDecodeJSON(t, ackResp.Body.Bytes(), &acked)
	if !acked.OK || acked.Task.Status != "running" {
		t.Fatalf("unexpected ack response: %+v", acked)
	}

	completeResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+created.Task.ID+"/complete", map[string]any{
		"agentId": registered.Agent.ID,
		"status":  "success",
		"message": "ok",
	})
	if completeResp.Code != http.StatusOK {
		t.Fatalf("complete failed: status=%d body=%s", completeResp.Code, completeResp.Body.String())
	}
	var completed struct {
		OK   bool           `json:"ok"`
		Task controlTaskDTO `json:"task"`
	}
	mustDecodeJSON(t, completeResp.Body.Bytes(), &completed)
	if !completed.OK || completed.Task.Status != "success" || completed.Task.FinishedAt == nil {
		t.Fatalf("unexpected complete response: %+v", completed)
	}
}

func TestControlDispatch_TaskEventsPersisted(t *testing.T) {
	store, err := newControlSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	h := &handler{
		controlStore: store,
	}

	registerResp := doJSONRequest(t, h.controlAgentsHandler, http.MethodPost, "/api/control/agents", map[string]any{
		"agentKey": "agent-events-1",
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

	createResp := doJSONRequest(t, h.controlTasksHandler, http.MethodPost, "/api/control/tasks", map[string]any{
		"type":   "manual_upload",
		"target": "D:/logs/gwf/replay-2.log",
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

	pullResp := doJSONRequest(t, h.controlDispatchPullHandler, http.MethodPost, "/api/control/dispatch/pull", map[string]any{
		"agentId": registered.Agent.ID,
	})
	if pullResp.Code != http.StatusOK {
		t.Fatalf("pull failed: status=%d body=%s", pullResp.Code, pullResp.Body.String())
	}

	ackResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+created.Task.ID+"/ack", map[string]any{
		"agentId": registered.Agent.ID,
	})
	if ackResp.Code != http.StatusOK {
		t.Fatalf("ack failed: status=%d body=%s", ackResp.Code, ackResp.Body.String())
	}

	completeResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+created.Task.ID+"/complete", map[string]any{
		"agentId": registered.Agent.ID,
		"status":  "success",
	})
	if completeResp.Code != http.StatusOK {
		t.Fatalf("complete failed: status=%d body=%s", completeResp.Code, completeResp.Body.String())
	}

	eventsResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodGet, "/api/control/tasks/"+created.Task.ID+"/events?limit=20", nil)
	if eventsResp.Code != http.StatusOK {
		t.Fatalf("events failed: status=%d body=%s", eventsResp.Code, eventsResp.Body.String())
	}
	var events struct {
		OK    bool                  `json:"ok"`
		Items []controlTaskEventDTO `json:"items"`
		Total int                   `json:"total"`
	}
	mustDecodeJSON(t, eventsResp.Body.Bytes(), &events)
	if !events.OK || events.Total < 3 {
		t.Fatalf("unexpected events response: %+v", events)
	}
}

func TestControlAuditHandler(t *testing.T) {
	store, err := newControlSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	h := &handler{
		controlStore: store,
	}

	registerResp := doJSONRequest(t, h.controlAgentsHandler, http.MethodPost, "/api/control/agents", map[string]any{
		"agentKey": "agent-audit-1",
	})
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register agent failed: status=%d body=%s", registerResp.Code, registerResp.Body.String())
	}

	createResp := doJSONRequest(t, h.controlTasksHandler, http.MethodPost, "/api/control/tasks", map[string]any{
		"type":   "manual_upload",
		"target": "D:/logs/gwf/replay-3.log",
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

	auditResp := doJSONRequest(t, h.controlAuditHandler, http.MethodGet, "/api/control/audit?resourceType=task&resourceId="+created.Task.ID+"&limit=20", nil)
	if auditResp.Code != http.StatusOK {
		t.Fatalf("audit list failed: status=%d body=%s", auditResp.Code, auditResp.Body.String())
	}
	var listed struct {
		OK    bool                 `json:"ok"`
		Items []controlAuditLogDTO `json:"items"`
		Total int                  `json:"total"`
	}
	mustDecodeJSON(t, auditResp.Body.Bytes(), &listed)
	if !listed.OK || listed.Total == 0 || len(listed.Items) == 0 {
		t.Fatalf("unexpected audit response: %+v", listed)
	}
}

func TestControlAuditHandler_InvalidRange(t *testing.T) {
	h := &handler{}

	resp := doJSONRequest(t, h.controlAuditHandler, http.MethodGet, "/api/control/audit?from=2026-02-18T10:00:00Z&to=2026-02-17T10:00:00Z", nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got status=%d body=%s", resp.Code, resp.Body.String())
	}
}
