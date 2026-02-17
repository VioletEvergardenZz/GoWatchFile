// 本文件用于控制面 MVP 接口测试，覆盖 Agent/Task 最小生命周期
package api

import (
	"net/http"
	"testing"
)

func TestControlAgentsLifecycle(t *testing.T) {
	h := &handler{}

	registerResp := doJSONRequest(t, h.controlAgentsHandler, http.MethodPost, "/api/control/agents", map[string]any{
		"agentKey":  "agent-a",
		"hostname":  "node-a",
		"version":   "1.0.0",
		"ip":        "10.0.0.10",
		"groupName": "ops",
	})
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register agent failed: status=%d body=%s", registerResp.Code, registerResp.Body.String())
	}
	var registered struct {
		OK      bool            `json:"ok"`
		Created bool            `json:"created"`
		Agent   controlAgentDTO `json:"agent"`
	}
	mustDecodeJSON(t, registerResp.Body.Bytes(), &registered)
	if !registered.OK || !registered.Created || registered.Agent.ID == "" {
		t.Fatalf("unexpected register response: %+v", registered)
	}

	listResp := doJSONRequest(t, h.controlAgentsHandler, http.MethodGet, "/api/control/agents", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list agents failed: status=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var listed struct {
		OK    bool              `json:"ok"`
		Items []controlAgentDTO `json:"items"`
		Total int               `json:"total"`
	}
	mustDecodeJSON(t, listResp.Body.Bytes(), &listed)
	if !listed.OK || listed.Total != 1 || len(listed.Items) != 1 {
		t.Fatalf("unexpected list response: %+v", listed)
	}

	heartbeatResp := doJSONRequest(t, h.controlAgentByIDHandler, http.MethodPost, "/api/control/agents/"+registered.Agent.ID+"/heartbeat", map[string]any{
		"version": "1.0.1",
	})
	if heartbeatResp.Code != http.StatusOK {
		t.Fatalf("heartbeat failed: status=%d body=%s", heartbeatResp.Code, heartbeatResp.Body.String())
	}
	var heartbeated struct {
		OK    bool            `json:"ok"`
		Agent controlAgentDTO `json:"agent"`
	}
	mustDecodeJSON(t, heartbeatResp.Body.Bytes(), &heartbeated)
	if !heartbeated.OK || heartbeated.Agent.HeartbeatCount != 1 || heartbeated.Agent.Version != "1.0.1" {
		t.Fatalf("unexpected heartbeat response: %+v", heartbeated)
	}

	drainResp := doJSONRequest(t, h.controlAgentByIDHandler, http.MethodPost, "/api/control/agents/"+registered.Agent.ID+"/drain", nil)
	if drainResp.Code != http.StatusOK {
		t.Fatalf("drain failed: status=%d body=%s", drainResp.Code, drainResp.Body.String())
	}
	var drained struct {
		OK    bool            `json:"ok"`
		Agent controlAgentDTO `json:"agent"`
	}
	mustDecodeJSON(t, drainResp.Body.Bytes(), &drained)
	if !drained.OK || drained.Agent.Status != "draining" {
		t.Fatalf("unexpected drain response: %+v", drained)
	}

	getResp := doJSONRequest(t, h.controlAgentByIDHandler, http.MethodGet, "/api/control/agents/"+registered.Agent.ID, nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get by id failed: status=%d body=%s", getResp.Code, getResp.Body.String())
	}
	var got struct {
		OK    bool            `json:"ok"`
		Agent controlAgentDTO `json:"agent"`
	}
	mustDecodeJSON(t, getResp.Body.Bytes(), &got)
	if !got.OK || got.Agent.Status != "draining" {
		t.Fatalf("unexpected get response: %+v", got)
	}
}

func TestControlTasksLifecycle(t *testing.T) {
	h := &handler{}

	createResp := doJSONRequest(t, h.controlTasksHandler, http.MethodPost, "/api/control/tasks", map[string]any{
		"type":       "manual_upload",
		"target":     "D:/logs/gwf/backend-error.log",
		"priority":   "high",
		"createdBy":  "tester",
		"maxRetries": 2,
		"payload": map[string]any{
			"path": "D:/logs/gwf/backend-error.log",
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
	if !created.OK || created.Task.ID == "" || created.Task.Status != "pending" || created.Task.Priority != "high" {
		t.Fatalf("unexpected create task response: %+v", created)
	}

	listResp := doJSONRequest(t, h.controlTasksHandler, http.MethodGet, "/api/control/tasks?status=pending&limit=10", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list tasks failed: status=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var listed struct {
		OK    bool             `json:"ok"`
		Items []controlTaskDTO `json:"items"`
		Total int              `json:"total"`
	}
	mustDecodeJSON(t, listResp.Body.Bytes(), &listed)
	if !listed.OK || listed.Total != 1 || len(listed.Items) != 1 {
		t.Fatalf("unexpected list tasks response: %+v", listed)
	}

	cancelResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+created.Task.ID+"/cancel", nil)
	if cancelResp.Code != http.StatusOK {
		t.Fatalf("cancel task failed: status=%d body=%s", cancelResp.Code, cancelResp.Body.String())
	}
	var canceled struct {
		OK   bool           `json:"ok"`
		Task controlTaskDTO `json:"task"`
	}
	mustDecodeJSON(t, cancelResp.Body.Bytes(), &canceled)
	if !canceled.OK || canceled.Task.Status != "canceled" {
		t.Fatalf("unexpected cancel response: %+v", canceled)
	}

	retryResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodPost, "/api/control/tasks/"+created.Task.ID+"/retry", nil)
	if retryResp.Code != http.StatusOK {
		t.Fatalf("retry task failed: status=%d body=%s", retryResp.Code, retryResp.Body.String())
	}
	var retried struct {
		OK   bool           `json:"ok"`
		Task controlTaskDTO `json:"task"`
	}
	mustDecodeJSON(t, retryResp.Body.Bytes(), &retried)
	if !retried.OK || retried.Task.Status != "pending" || retried.Task.RetryCount != 1 {
		t.Fatalf("unexpected retry response: %+v", retried)
	}

	getResp := doJSONRequest(t, h.controlTaskByIDHandler, http.MethodGet, "/api/control/tasks/"+created.Task.ID, nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get task failed: status=%d body=%s", getResp.Code, getResp.Body.String())
	}
	var got struct {
		OK   bool           `json:"ok"`
		Task controlTaskDTO `json:"task"`
	}
	mustDecodeJSON(t, getResp.Body.Bytes(), &got)
	if !got.OK || got.Task.Status != "pending" || got.Task.RetryCount != 1 {
		t.Fatalf("unexpected get task response: %+v", got)
	}
}

func TestControlTasksCreateValidation(t *testing.T) {
	h := &handler{}

	resp := doJSONRequest(t, h.controlTasksHandler, http.MethodPost, "/api/control/tasks", map[string]any{
		"type": "",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got status=%d body=%s", resp.Code, resp.Body.String())
	}
}
