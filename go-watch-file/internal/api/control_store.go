// 本文件用于控制面 Agent/Task 的 SQLite 持久化存储。
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultControlDataDir = "data/control"
	controlTimeLayout     = time.RFC3339Nano
)

type controlSQLiteStore struct {
	mu     sync.Mutex
	db     *sql.DB
	dbPath string
}

// newControlSQLiteStore 初始化控制面持久化存储
// 初始化失败会交由上层回退到内存模式 不阻断 API 启动
func newControlSQLiteStore(dataDir string) (*controlSQLiteStore, error) {
	root := resolveControlDataDir(dataDir)
	// 启动时确保目录存在，避免数据库文件无法创建导致整个控制面不可用
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create control data dir failed: %w", err)
	}
	dbPath := filepath.Join(root, "control.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open control sqlite failed: %w", err)
	}
	// WAL 兼顾写入吞吐与崩溃恢复，适合当前控制面高频 upsert 场景
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set control sqlite wal failed: %w", err)
	}
	if err := migrateControlStore(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &controlSQLiteStore{db: db, dbPath: dbPath}, nil
}

func (s *controlSQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *controlSQLiteStore) DBPath() string {
	if s == nil {
		return ""
	}
	return s.dbPath
}

// LoadAgents 在服务启动时恢复 Agent 快照
// 该方法只负责读取 不做业务修正 便于问题排查
func (s *controlSQLiteStore) LoadAgents() ([]controlAgentState, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT
			id, agent_key, hostname, version, ip, group_name, status,
			last_seen_at, created_at, updated_at, heartbeat_count
		FROM control_agents
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]controlAgentState, 0)
	for rows.Next() {
		var (
			item                   controlAgentState
			lastSeenAt, createdAt  string
			updatedAt              string
			heartbeatCountDatabase int64
		)
		if err := rows.Scan(
			&item.ID,
			&item.AgentKey,
			&item.Hostname,
			&item.Version,
			&item.IP,
			&item.GroupName,
			&item.Status,
			&lastSeenAt,
			&createdAt,
			&updatedAt,
			&heartbeatCountDatabase,
		); err != nil {
			return nil, err
		}
		item.LastSeenAt = parseControlTime(lastSeenAt)
		item.CreatedAt = parseControlTime(createdAt)
		item.UpdatedAt = parseControlTime(updatedAt)
		// 历史数据中的负值或异常值按 0 处理，避免出现无意义的无符号溢出
		if heartbeatCountDatabase > 0 {
			item.HeartbeatCount = uint64(heartbeatCountDatabase)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *controlSQLiteStore) UpsertAgent(state controlAgentState) error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// 使用 ON CONFLICT 覆盖写入，确保注册与心跳都走同一条幂等写路径
	_, err := s.db.Exec(`
		INSERT INTO control_agents (
			id, agent_key, hostname, version, ip, group_name, status,
			last_seen_at, created_at, updated_at, heartbeat_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			agent_key = excluded.agent_key,
			hostname = excluded.hostname,
			version = excluded.version,
			ip = excluded.ip,
			group_name = excluded.group_name,
			status = excluded.status,
			last_seen_at = excluded.last_seen_at,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at,
			heartbeat_count = excluded.heartbeat_count
	`,
		state.ID,
		state.AgentKey,
		state.Hostname,
		state.Version,
		state.IP,
		state.GroupName,
		state.Status,
		formatControlTime(state.LastSeenAt),
		formatControlTime(state.CreatedAt),
		formatControlTime(state.UpdatedAt),
		int64(state.HeartbeatCount),
	)
	return err
}

func (s *controlSQLiteStore) LoadTasks() ([]controlTaskState, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT
			id, type, target, payload_json, priority, status, assigned_agent_id,
			retry_count, max_retries, created_by, created_at, updated_at, finished_at
		FROM control_tasks
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]controlTaskState, 0)
	for rows.Next() {
		var (
			item                              controlTaskState
			payloadJSON                       string
			createdAt, updatedAt, finishedRaw string
		)
		if err := rows.Scan(
			&item.ID,
			&item.Type,
			&item.Target,
			&payloadJSON,
			&item.Priority,
			&item.Status,
			&item.AssignedAgentID,
			&item.RetryCount,
			&item.MaxRetries,
			&item.CreatedBy,
			&createdAt,
			&updatedAt,
			&finishedRaw,
		); err != nil {
			return nil, err
		}
		item.Payload = parseControlPayload(payloadJSON)
		item.CreatedAt = parseControlTime(createdAt)
		item.UpdatedAt = parseControlTime(updatedAt)
		// finished_at 允许为空字符串，代表任务尚未进入终态
		if strings.TrimSpace(finishedRaw) != "" {
			finishedAt := parseControlTime(finishedRaw)
			item.FinishedAt = &finishedAt
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// UpsertTask 是任务状态落库统一入口
// 调度与执行状态变更都经由此函数 保证字段更新语义一致
func (s *controlSQLiteStore) UpsertTask(state controlTaskState) error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	payloadJSON, err := json.Marshal(state.Payload)
	if err != nil {
		return fmt.Errorf("marshal control task payload failed: %w", err)
	}
	// 统一把 nil 与空值写成空字符串，读取时再还原为 nil 指针
	finishedAt := ""
	if state.FinishedAt != nil {
		finishedAt = formatControlTime(*state.FinishedAt)
	}
	// 任务全量 upsert，避免状态迁移过程中出现多处局部更新导致的数据不一致
	_, err = s.db.Exec(`
		INSERT INTO control_tasks (
			id, type, target, payload_json, priority, status, assigned_agent_id,
			retry_count, max_retries, created_by, created_at, updated_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			target = excluded.target,
			payload_json = excluded.payload_json,
			priority = excluded.priority,
			status = excluded.status,
			assigned_agent_id = excluded.assigned_agent_id,
			retry_count = excluded.retry_count,
			max_retries = excluded.max_retries,
			created_by = excluded.created_by,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at,
			finished_at = excluded.finished_at
	`,
		state.ID,
		state.Type,
		state.Target,
		string(payloadJSON),
		state.Priority,
		state.Status,
		state.AssignedAgentID,
		state.RetryCount,
		state.MaxRetries,
		state.CreatedBy,
		formatControlTime(state.CreatedAt),
		formatControlTime(state.UpdatedAt),
		finishedAt,
	)
	return err
}

func (s *controlSQLiteStore) InsertTaskEvent(taskID, agentID, eventType, message string, at time.Time) error {
	if s == nil || s.db == nil {
		return nil
	}
	taskID = strings.TrimSpace(taskID)
	eventType = strings.TrimSpace(eventType)
	if taskID == "" || eventType == "" {
		return fmt.Errorf("invalid task event: task_id or event_type is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO control_task_events (task_id, agent_id, event_type, message, event_time)
		VALUES (?, ?, ?, ?, ?)
	`,
		taskID,
		strings.TrimSpace(agentID),
		eventType,
		strings.TrimSpace(message),
		formatControlTime(at),
	)
	return err
}

func (s *controlSQLiteStore) ListTaskEvents(taskID string, limit int) ([]controlTaskEvent, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT id, task_id, agent_id, event_type, message, event_time
		FROM control_task_events
		WHERE task_id = ?
		ORDER BY id DESC
		LIMIT ?
	`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]controlTaskEvent, 0)
	for rows.Next() {
		var (
			item     controlTaskEvent
			eventRaw string
		)
		if err := rows.Scan(&item.ID, &item.TaskID, &item.AgentID, &item.EventType, &item.Message, &eventRaw); err != nil {
			return nil, err
		}
		item.EventTime = parseControlTime(eventRaw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *controlSQLiteStore) InsertAuditLog(operator, action, resourceType, resourceID string, detail map[string]any, at time.Time) error {
	if s == nil || s.db == nil {
		return nil
	}
	action = strings.TrimSpace(action)
	resourceType = strings.TrimSpace(resourceType)
	if action == "" || resourceType == "" {
		return fmt.Errorf("invalid audit log: action or resource_type is empty")
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal audit detail failed: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = s.db.Exec(`
		INSERT INTO control_audit_logs (operator, action, resource_type, resource_id, detail_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		strings.TrimSpace(operator),
		action,
		resourceType,
		strings.TrimSpace(resourceID),
		string(detailJSON),
		formatControlTime(at),
	)
	return err
}

type controlAuditLogFilter struct {
	ResourceType string
	ResourceID   string
	Operator     string
	Action       string
	From         time.Time
	To           time.Time
	Limit        int
}

// ListAuditLogs 支持按资源 操作人 动作和时间窗口组合过滤
// 过滤逻辑集中在存储层 便于后续控制台复用同一查询语义
func (s *controlSQLiteStore) ListAuditLogs(filter controlAuditLogFilter) ([]controlAuditLog, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	builder := strings.Builder{}
	builder.WriteString(`
		SELECT id, operator, action, resource_type, resource_id, detail_json, created_at
		FROM control_audit_logs
		WHERE 1 = 1
	`)
	args := make([]any, 0, 8)
	if val := strings.TrimSpace(filter.ResourceType); val != "" {
		builder.WriteString(` AND resource_type = ?`)
		args = append(args, val)
	}
	if val := strings.TrimSpace(filter.ResourceID); val != "" {
		builder.WriteString(` AND resource_id = ?`)
		args = append(args, val)
	}
	if val := strings.TrimSpace(filter.Operator); val != "" {
		builder.WriteString(` AND operator = ?`)
		args = append(args, val)
	}
	if val := strings.TrimSpace(filter.Action); val != "" {
		builder.WriteString(` AND action = ?`)
		args = append(args, val)
	}
	if !filter.From.IsZero() {
		builder.WriteString(` AND created_at >= ?`)
		args = append(args, formatControlTime(filter.From))
	}
	if !filter.To.IsZero() {
		builder.WriteString(` AND created_at <= ?`)
		args = append(args, formatControlTime(filter.To))
	}
	builder.WriteString(` ORDER BY id DESC LIMIT ?`)
	args = append(args, limit)

	rows, err := s.db.Query(builder.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]controlAuditLog, 0)
	for rows.Next() {
		var (
			item      controlAuditLog
			detailRaw string
			createdAt string
		)
		if err := rows.Scan(
			&item.ID,
			&item.Operator,
			&item.Action,
			&item.ResourceType,
			&item.ResourceID,
			&detailRaw,
			&createdAt,
		); err != nil {
			return nil, err
		}
		item.Detail = parseAuditDetailJSON(detailRaw)
		item.CreatedAt = parseControlTime(createdAt)
		out = append(out, item)
	}
	return out, rows.Err()
}

// migrateControlStore 负责控制面表结构与索引的幂等迁移
// 迁移分步执行 便于线上增量升级时逐条定位失败语句
func migrateControlStore(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("control sqlite is nil")
	}
	// 迁移语句保持幂等，服务重启时重复执行不会破坏现有数据
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS control_agents (
			id TEXT PRIMARY KEY,
			agent_key TEXT NOT NULL UNIQUE,
			hostname TEXT NOT NULL DEFAULT '',
			version TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			group_name TEXT NOT NULL DEFAULT 'default',
			status TEXT NOT NULL DEFAULT 'online',
			last_seen_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			heartbeat_count INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_control_agents_status_updated
			ON control_agents(status, updated_at DESC);`,
		`CREATE TABLE IF NOT EXISTS control_tasks (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			target TEXT NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '{}',
			priority TEXT NOT NULL DEFAULT 'normal',
			status TEXT NOT NULL,
			assigned_agent_id TEXT NOT NULL DEFAULT '',
			retry_count INTEGER NOT NULL DEFAULT 0,
			max_retries INTEGER NOT NULL DEFAULT 3,
			created_by TEXT NOT NULL DEFAULT 'console',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE INDEX IF NOT EXISTS idx_control_tasks_status_updated
			ON control_tasks(status, updated_at DESC);`,
		`CREATE TABLE IF NOT EXISTS control_task_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL,
			message TEXT NOT NULL DEFAULT '',
			event_time TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_control_task_events_task_time
			ON control_task_events(task_id, event_time DESC);`,
		`CREATE TABLE IF NOT EXISTS control_audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			operator TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL DEFAULT '',
			detail_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_control_audit_logs_created_at
			ON control_audit_logs(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_control_audit_logs_resource
			ON control_audit_logs(resource_type, resource_id, created_at DESC);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate control sqlite failed: %w", err)
		}
	}
	return nil
}

func resolveControlDataDir(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		return trimmed
	}
	if env := strings.TrimSpace(os.Getenv("CONTROL_DATA_DIR")); env != "" {
		return env
	}
	return defaultControlDataDir
}

func formatControlTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(controlTimeLayout)
}

func parseControlTime(raw string) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}
	}
	// 先按纳秒精度解析，再兼容 RFC3339 老格式，保证历史数据可读
	if t, err := time.Parse(controlTimeLayout, trimmed); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func parseControlPayload(raw string) map[string]any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
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

func controlIDSequence(id, prefix string) uint64 {
	if strings.TrimSpace(id) == "" {
		return 0
	}
	if !strings.HasPrefix(id, prefix) {
		return 0
	}
	part := strings.TrimPrefix(id, prefix)
	seq, err := strconv.ParseUint(part, 10, 64)
	if err != nil {
		return 0
	}
	return seq
}
