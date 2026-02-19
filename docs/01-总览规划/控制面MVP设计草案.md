# 控制面 MVP 设计草案（v0.1）

- 更新时间：2026-02-17
- 适用阶段：里程碑 C（第 3~4 周）
- 目标：在不扩张范围的前提下，给出“多 Agent 最小控制面”可落地方案，指导后续实现与验收。

## 1. 设计目标

1. 建立最小控制面闭环：`注册 -> 心跳 -> 分组 -> 任务下发 -> 状态回传 -> 可观测`。
2. 与现有能力兼容：不破坏当前 `go-watch-file` 的上传、告警、AI、知识库链路。
3. 提供可回滚实施路径：先旁路上线，再逐步接管，不做一次性替换。

## 2. 范围边界

### 2.1 本阶段必做（MVP）
- Agent 注册与心跳（在线状态可见）。
- 主机组/应用分组（最小分组能力）。
- 最小任务模型（手动触发、重试、取消、状态查询）。
- 控制面指标（在线率、任务成功率、任务滞留、超时数）。
- 审计日志（谁在什么时间执行了什么控制操作）。

### 2.2 本阶段不做
- 跨集群调度与全局一致性选主。
- 复杂 DSL 编排、审批流引擎、自动闭环处置全量放开。
- 完整 RBAC/多租户计费体系。

## 3. 架构草图（逻辑）

```mermaid
flowchart LR
  UI[Console Frontend] --> CP[Control API]
  CP --> DB[(Control Store)]
  CP --> BUS[任务分发器]
  AG1[Agent A] -->|register/heartbeat| CP
  AG2[Agent B] -->|register/heartbeat| CP
  BUS -->|pull/ack| AG1
  BUS -->|pull/ack| AG2
  AG1 -->|task result/event| CP
  AG2 -->|task result/event| CP
  CP --> METRICS[/metrics]
```

说明：
- `Control API` 与当前 API 服务同进程起步，后续可拆分。
- `Control Store` MVP 可先落 SQLite（单机场景），后续演进 MySQL/PostgreSQL。
- `任务分发器` MVP 采用“Agent 轮询拉取 + 显式 ack”模式，避免先做长连接编排。

## 4. 数据模型（v1）

## 4.1 agents
- `id`（主键，UUID）
- `agent_key`（唯一，可读标识）
- `hostname`
- `version`
- `ip`
- `group_name`
- `status`（`online|offline|draining`）
- `last_seen_at`
- `created_at` / `updated_at`

## 4.2 tasks
- `id`（主键，UUID）
- `type`（`manual_upload|log_tail|ai_summary|health_check`）
- `target`（路径或资源标识）
- `payload_json`
- `priority`（`low|normal|high`）
- `status`（`pending|assigned|running|success|failed|canceled|timeout`）
- `assigned_agent_id`（可空）
- `retry_count` / `max_retries`
- `created_by`
- `created_at` / `updated_at` / `finished_at`

## 4.3 task_events
- `id`（主键）
- `task_id`
- `agent_id`
- `event_type`（`assigned|started|progress|succeeded|failed|canceled|timeout`）
- `message`
- `event_time`

## 4.4 audit_logs
- `id`（主键）
- `operator`
- `action`
- `resource_type`
- `resource_id`
- `detail_json`
- `created_at`

## 5. API 草案（v1）

## 5.1 Agent 管理
- `POST /api/control/agents`（register）
- `POST /api/control/agents/{id}/heartbeat`
- `GET /api/control/agents`
- `GET /api/control/agents/{id}`
- `POST /api/control/agents/{id}/drain`

## 5.2 任务管理
- `POST /api/control/tasks`
- `GET /api/control/tasks`
- `GET /api/control/tasks/{id}`
- `POST /api/control/tasks/{id}/cancel`
- `POST /api/control/tasks/{id}/retry`

## 5.3 Agent 拉取与回传
- `POST /api/control/dispatch/pull`
- `POST /api/control/tasks/{id}/ack`
- `POST /api/control/tasks/{id}/progress`
- `POST /api/control/tasks/{id}/complete`
- `GET /api/control/tasks/{id}/events`
- `GET /api/control/audit`（支持 `resourceType/resourceId/operator/action/from/to/limit` 组合过滤）
  - `from/to` 支持 `RFC3339` 与 `YYYY-MM-DDTHH:mm`（便于控制台 `datetime-local` 直接透传）

接口约束：
- 统一复用现有 `X-API-Token` 鉴权头（后端启用鉴权时生效）。
- Agent 侧增加 `agent_token`（与控制台 token 分离）。
- 所有写操作写入 `audit_logs`。

## 6. 状态机与超时策略

任务状态流转：
- `pending -> assigned -> running -> success|failed|timeout|canceled`
- `failed|timeout|canceled -> pending`（当触发 retry 且 `retry_count < max_retries`）

超时与重试：
- `assigned` 超过 `assign_timeout` 未 ack，回退 `pending`。
- `running` 超过 `run_timeout`：
  - 若 `retry_count < max_retries`，自动重试并回到 `pending`
  - 否则标记 `timeout` 终态
- 默认重试：按 `max_retries` 控制最多重试次数（MVP 暂不引入任务级退避调度）。

## 7. 指标与告警（MVP）

新增建议指标：
- `gwf_control_agents_online`
- `gwf_control_agent_heartbeat_lag_seconds`
- `gwf_control_tasks_total{status,type}`
- `gwf_control_task_duration_seconds`（histogram）
- `gwf_control_task_backlog`
- `gwf_control_task_timeout_total`
- `gwf_control_dispatch_retry_total`

告警建议：
- 在线 Agent 数跌破阈值（如 `< 1`）。
- 任务 backlog 持续高位（如 `> 100` 且持续 5 分钟）。
- 任务超时率 10 分钟窗口超过阈值（如 `> 5%`）。

## 8. 实施拆分（2~4 周）

第 1 步（本周）：
- 落库 schema + 最小 API（register/heartbeat/tasks CRUD）。
- 前端增加“Agent 列表 + 任务列表”只读页。

第 2 步（次周）：
- Agent pull/ack/complete 闭环。
- 任务状态机与超时重试策略落地。

第 3 步（次周后半）：
- 指标与告警接入。
- 审计日志与最小运维手册补齐。

## 9. 验收标准（DoD）

1. 10 个模拟 Agent 心跳稳定 30 分钟，在线率统计准确。
2. 100 条任务回放，成功率、失败率、超时率可在控制台与 `/metrics` 对齐。
3. 任务可取消、可重试，状态流转无非法跳变。
4. `go test ./...` 通过，新增模块有单元测试与最小集成测试。
5. 文档同步：API、配置、运维运行手册、故障排查清单。

## 10. 风险与缓解

- 风险：单进程控制面成为瓶颈。
  - 缓解：先做读写分离接口边界，后续可独立拆服务。
- 风险：任务幂等不足导致重复执行。
  - 缓解：任务引入 `dedupe_key` 与 Agent 执行侧幂等检查。
- 风险：权限模型过粗导致误操作。
  - 缓解：先做控制台/Agent 双 token 隔离 + 审计全量记录。

## 11. 与现有代码的衔接建议

- 后端：
  - 在 `go-watch-file/internal/api` 新增 `control_*` handlers。
  - 在 `go-watch-file/internal/state` 保留现有运行态，控制面状态独立存储。
- 前端：
  - 在 `console-frontend/src` 新增控制面页签，先只读展示。
- 脚本：
  - 新增 `scripts/ops/control-replay.ps1`（后续实现）用于任务回放验收。

---

关联文档：
- `docs/01-总览规划/MVP范围控制.md`
- `docs/01-总览规划/下一阶段执行计划.md`
- `todo.md`

