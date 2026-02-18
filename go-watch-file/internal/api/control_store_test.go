package api

import (
	"path/filepath"
	"testing"
	"time"
)

func TestControlSQLiteStore_PersistAndLoad(t *testing.T) {
	store, err := newControlSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("create control sqlite store failed: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	agent := controlAgentState{
		ID:             "agt-000021",
		AgentKey:       "agent-persist-1",
		Hostname:       "node-1",
		Version:        "1.0.0",
		IP:             "10.1.1.8",
		GroupName:      "ops",
		Status:         "online",
		LastSeenAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
		HeartbeatCount: 2,
	}
	task := controlTaskState{
		ID:         "tsk-000031",
		Type:       "manual_upload",
		Target:     "D:/logs/gwf/backend-error.log",
		Payload:    map[string]any{"path": "D:/logs/gwf/backend-error.log"},
		Priority:   "high",
		Status:     "pending",
		RetryCount: 0,
		MaxRetries: 3,
		CreatedBy:  "tester",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := store.UpsertAgent(agent); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if err := store.UpsertTask(task); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}
	dbPath := store.DBPath()
	if err := store.Close(); err != nil {
		t.Fatalf("close store failed: %v", err)
	}

	reopen, err := newControlSQLiteStore(filepath.Dir(dbPath))
	if err != nil {
		t.Fatalf("reopen control sqlite store failed: %v", err)
	}
	defer func() { _ = reopen.Close() }()

	agents, err := reopen.LoadAgents()
	if err != nil {
		t.Fatalf("load agents failed: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("unexpected agents count: %d", len(agents))
	}
	if agents[0].AgentKey != agent.AgentKey || agents[0].HeartbeatCount != agent.HeartbeatCount {
		t.Fatalf("unexpected agent loaded: %+v", agents[0])
	}

	tasks, err := reopen.LoadTasks()
	if err != nil {
		t.Fatalf("load tasks failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("unexpected tasks count: %d", len(tasks))
	}
	if tasks[0].ID != task.ID || tasks[0].Priority != task.Priority || tasks[0].Status != task.Status {
		t.Fatalf("unexpected task loaded: %+v", tasks[0])
	}
}

func TestControlLoadSnapshot_ContinueSequence(t *testing.T) {
	store, err := newControlSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("create control sqlite store failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	now := time.Now().UTC()
	if err := store.UpsertAgent(controlAgentState{
		ID:         "agt-000007",
		AgentKey:   "agent-seq",
		Status:     "online",
		GroupName:  "default",
		LastSeenAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("seed agent failed: %v", err)
	}
	if err := store.UpsertTask(controlTaskState{
		ID:         "tsk-000011",
		Type:       "manual_upload",
		Target:     "x.log",
		Priority:   "normal",
		Status:     "pending",
		MaxRetries: 3,
		CreatedBy:  "seed",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	h := &handler{
		controlStore:       store,
		controlAgents:      map[string]controlAgentState{},
		controlAgentKeyIdx: map[string]string{},
		controlTasks:       map[string]controlTaskState{},
	}
	if err := h.loadControlSnapshot(); err != nil {
		t.Fatalf("load control snapshot failed: %v", err)
	}
	h.controlMu.Lock()
	nextAgentID := h.nextControlAgentIDLocked()
	nextTaskID := h.nextControlTaskIDLocked()
	h.controlMu.Unlock()

	if nextAgentID != "agt-000008" {
		t.Fatalf("unexpected next agent id: %s", nextAgentID)
	}
	if nextTaskID != "tsk-000012" {
		t.Fatalf("unexpected next task id: %s", nextTaskID)
	}
}

func TestControlSQLiteStore_AuditLogs(t *testing.T) {
	store, err := newControlSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("create control sqlite store failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := store.InsertAuditLog("tester", "task_create", "task", "tsk-000001", map[string]any{
		"type": "manual_upload",
	}, now); err != nil {
		t.Fatalf("insert audit log failed: %v", err)
	}
	if err := store.InsertAuditLog("tester", "task_retry", "task", "tsk-000001", map[string]any{
		"retry": 1,
	}, now.Add(1*time.Second)); err != nil {
		t.Fatalf("insert second audit log failed: %v", err)
	}

	items, err := store.ListAuditLogs(controlAuditLogFilter{
		ResourceType: "task",
		ResourceID:   "tsk-000001",
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("list audit logs failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("unexpected audit logs count: %d", len(items))
	}
	if items[0].Action != "task_retry" {
		t.Fatalf("unexpected latest action: %+v", items[0])
	}
	if items[0].Detail["retry"] != float64(1) {
		t.Fatalf("unexpected detail payload: %+v", items[0].Detail)
	}

	timeFiltered, err := store.ListAuditLogs(controlAuditLogFilter{
		ResourceType: "task",
		ResourceID:   "tsk-000001",
		From:         now.Add(500 * time.Millisecond),
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("list audit logs with from filter failed: %v", err)
	}
	if len(timeFiltered) != 1 || timeFiltered[0].Action != "task_retry" {
		t.Fatalf("unexpected time filtered audit logs: %+v", timeFiltered)
	}
}
