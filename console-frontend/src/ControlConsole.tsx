/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于控制面控制台页面 展示Agent与任务并提供最小观测与处置入口 */

import { useCallback, useEffect, useMemo, useState } from "react";
import "./ControlConsole.css";
import type { ControlAgent, ControlAuditLog, ControlTask, ControlTaskEvent, ControlTaskFailureReason } from "./types";
import {
  fetchControlAgents,
  fetchControlAuditLogs,
  fetchControlTaskFailureReasons,
  fetchControlTaskEvents,
  fetchControlTasks,
  postControlTaskCancel,
  postControlTaskRetry,
} from "./console/dashboardApi";

type TaskStatusFilter =
  | "all"
  | "pending"
  | "assigned"
  | "running"
  | "success"
  | "failed"
  | "timeout"
  | "canceled";

type AuditFilters = {
  operator: string;
  action: string;
  from: string;
  to: string;
};

const createEmptyAuditFilters = (): AuditFilters => ({
  operator: "",
  action: "",
  from: "",
  to: "",
});

const formatTime = (value: string | undefined) => {
  if (!value) return "--";
  const ts = Date.parse(value);
  if (!Number.isFinite(ts)) return value;
  return new Date(ts).toLocaleString();
};

const statusBadgeClass = (status: string) => {
  if (status === "success") return "pill success";
  if (status === "failed" || status === "timeout") return "pill danger";
  if (status === "running") return "pill warning";
  if (status === "assigned") return "pill info";
  if (status === "pending") return "pill";
  if (status === "canceled") return "pill";
  return "pill";
};

const AGENT_STATUS_LABELS: Record<string, string> = {
  online: "在线",
  offline: "离线",
  draining: "维护中",
};

const TASK_STATUS_LABELS: Record<string, string> = {
  pending: "待分配",
  assigned: "已分配",
  running: "执行中",
  success: "成功",
  failed: "失败",
  timeout: "超时",
  canceled: "已取消",
};

const toAgentStatusLabel = (status: string) => (AGENT_STATUS_LABELS[status] ?? status) || "--";
const toTaskStatusLabel = (status: string) => (TASK_STATUS_LABELS[status] ?? status) || "--";

// ControlConsole 负责控制面核心观测与操作
// 页面按“任务列表 -> 事件 -> 审计”三层展开 便于排障时逐步定位
export function ControlConsole() {
  const [agents, setAgents] = useState<ControlAgent[]>([]);
  const [tasks, setTasks] = useState<ControlTask[]>([]);
  const [taskStatus, setTaskStatus] = useState<TaskStatusFilter>("all");
  const [taskType, setTaskType] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [selectedTaskId, setSelectedTaskId] = useState<string>("");
  const [events, setEvents] = useState<ControlTaskEvent[]>([]);
  const [eventsLoading, setEventsLoading] = useState(false);
  const [failureReasons, setFailureReasons] = useState<ControlTaskFailureReason[]>([]);
  const [audits, setAudits] = useState<ControlAuditLog[]>([]);
  const [auditLoading, setAuditLoading] = useState(false);
  const [auditFiltersDraft, setAuditFiltersDraft] = useState<AuditFilters>(() => createEmptyAuditFilters());
  const [auditFiltersApplied, setAuditFiltersApplied] = useState<AuditFilters>(() => createEmptyAuditFilters());

  // refresh 统一刷新 agent 和 task 列表
  // 过滤条件变化后只触发这一处请求 便于控制并发与错误处理
  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [agentResp, taskResp, failureResp] = await Promise.all([
        fetchControlAgents(),
        fetchControlTasks({
          status: taskStatus === "all" ? "" : taskStatus,
          type: taskType.trim() ? taskType.trim() : "",
          limit: 200,
        }),
        fetchControlTaskFailureReasons({
          status: "failed,timeout",
          type: taskType.trim() ? taskType.trim() : "",
          limit: 10,
        }),
      ]);
      setAgents(agentResp.items ?? []);
      setTasks(taskResp.items ?? []);
      setFailureReasons(failureResp.items ?? []);
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [taskStatus, taskType]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const selectedTask = useMemo(() => tasks.find((item) => item.id === selectedTaskId), [selectedTaskId, tasks]);

  // refreshEvents 单独拉取任务事件流
  // 事件和主列表分离刷新 可避免高频日志更新影响主表交互
  const refreshEvents = useCallback(
    async (taskId: string) => {
      if (!taskId) {
        setEvents([]);
        return;
      }
      setEventsLoading(true);
      try {
        const resp = await fetchControlTaskEvents(taskId, 200);
        setEvents(resp.items ?? []);
      } catch (err) {
        setEvents([]);
        setError((err as Error).message);
      } finally {
        setEventsLoading(false);
      }
    },
    [setEvents]
  );

  // refreshAudits 按当前筛选条件查询审计日志
  // 使用 applied 状态而非 draft 状态 避免输入过程触发抖动请求
  const refreshAudits = useCallback(
    async (taskId: string) => {
      setAuditLoading(true);
      try {
        const resp = await fetchControlAuditLogs({
          resourceType: "task",
          resourceId: taskId || "",
          operator: auditFiltersApplied.operator.trim(),
          action: auditFiltersApplied.action.trim(),
          from: auditFiltersApplied.from || "",
          to: auditFiltersApplied.to || "",
          limit: 100,
        });
        setAudits(resp.items ?? []);
      } catch (err) {
        setAudits([]);
        setError((err as Error).message);
      } finally {
        setAuditLoading(false);
      }
    },
    [auditFiltersApplied]
  );

  useEffect(() => {
    void refreshEvents(selectedTaskId);
    void refreshAudits(selectedTaskId);
  }, [selectedTaskId, refreshEvents, refreshAudits]);

  const summary = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const task of tasks) {
      const key = task.status || "unknown";
      counts[key] = (counts[key] ?? 0) + 1;
    }
    return counts;
  }, [tasks]);

  const failureReasonMax = useMemo(() => {
    let max = 0;
    for (const item of failureReasons) {
      const count = Number(item.count ?? 0);
      if (Number.isFinite(count) && count > max) {
        max = count;
      }
    }
    return max;
  }, [failureReasons]);

  const handleCancel = useCallback(
    async (taskId: string) => {
      try {
        await postControlTaskCancel(taskId);
        await refresh();
        await refreshEvents(taskId);
        await refreshAudits(taskId);
      } catch (err) {
        setError((err as Error).message);
      }
    },
    [refresh, refreshEvents, refreshAudits]
  );

  const handleRetry = useCallback(
    async (taskId: string) => {
      try {
        await postControlTaskRetry(taskId);
        await refresh();
        await refreshEvents(taskId);
        await refreshAudits(taskId);
      } catch (err) {
        setError((err as Error).message);
      }
    },
    [refresh, refreshEvents, refreshAudits]
  );

  const handleAuditFilterChange = useCallback((key: keyof AuditFilters, value: string) => {
    setAuditFiltersDraft((prev) => ({ ...prev, [key]: value }));
  }, []);

  const handleApplyAuditFilters = useCallback(() => {
    const from = auditFiltersDraft.from.trim();
    const to = auditFiltersDraft.to.trim();
    if (from && to) {
      const fromTS = Date.parse(from);
      const toTS = Date.parse(to);
      if (Number.isFinite(fromTS) && Number.isFinite(toTS) && fromTS > toTS) {
        setError("审计时间范围无效：开始时间不能晚于结束时间");
        return;
      }
    }
    setError(null);
    setAuditFiltersApplied({
      operator: auditFiltersDraft.operator.trim(),
      action: auditFiltersDraft.action.trim(),
      from,
      to,
    });
  }, [auditFiltersDraft]);

  const handleResetAuditFilters = useCallback(() => {
    const next = createEmptyAuditFilters();
    setAuditFiltersDraft(next);
    setAuditFiltersApplied(next);
    setError(null);
  }, []);

  const handleExportAudits = useCallback(() => {
    if (typeof window === "undefined" || typeof document === "undefined") return;
    const payload = {
      exportedAt: new Date().toISOString(),
      resourceType: "task",
      resourceId: selectedTaskId || "",
      filters: auditFiltersApplied,
      total: audits.length,
      items: audits,
    };
    const blob = new Blob([JSON.stringify(payload, null, 2)], {
      type: "application/json;charset=utf-8",
    });
    const url = window.URL.createObjectURL(blob);
    const link = document.createElement("a");
    const stamp = new Date().toISOString().replace(/[:.]/g, "-");
    link.href = url;
    link.download = `control-audit-${selectedTaskId || "all"}-${stamp}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.URL.revokeObjectURL(url);
  }, [auditFiltersApplied, audits, selectedTaskId]);

  return (
    <div className="control-shell">
      <div className="panel">
        <div className="section-title">
          <h2>控制面控制台</h2>
          <span>多 Agent 任务调度 · 生命周期追踪 · 审计留痕</span>
        </div>
        <div className="control-intro">
          <span className="badge ghost">用途：查看任务从下发到完成的全过程，并支持取消/重试</span>
          <span className="badge ghost">适用：多 Agent 协同、回放压测、发布变更追踪</span>
          <span className="badge ghost">非适用：日常文件浏览与日志查看</span>
        </div>
        <div className="toolbar space-between">
          <div className="toolbar-actions">
            <button className="btn secondary" type="button" onClick={() => void refresh()} disabled={loading}>
              {loading ? "刷新中..." : "刷新"}
            </button>
          </div>
          <div className="row-sub">{error ? `错误：${error}` : "提示：Token 复用主控制台输入的 X-API-Token"}</div>
        </div>
      </div>

      <div className="control-grid">
        <div className="panel">
          <div className="section-title">
            <h2>Agent 列表</h2>
            <span>总数 {agents.length}</span>
          </div>
          <div className="control-table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Agent 标识</th>
                  <th>状态</th>
                  <th>分组</th>
                  <th>最近心跳</th>
                  <th>心跳</th>
                </tr>
              </thead>
              <tbody>
                {agents.length ? (
                  agents.map((agent) => (
                    <tr key={agent.id}>
                      <td>
                        <div className="row-title">{agent.agentKey || agent.id}</div>
                        <div className="row-sub">
                          {agent.hostname ? `${agent.hostname} · ` : ""}
                          {agent.version ? `v${agent.version}` : "--"}
                        </div>
                      </td>
                      <td>
                        <span className={statusBadgeClass(agent.status)}>{toAgentStatusLabel(agent.status)}</span>
                      </td>
                      <td>{agent.groupName || "--"}</td>
                      <td>{formatTime(agent.lastSeenAt)}</td>
                      <td>{agent.heartbeatCount ?? 0}</td>
                    </tr>
                  ))
                ) : (
                  <tr>
                    <td colSpan={5} className="row-sub">
                      暂无 Agent
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>

        <div className="panel">
          <div className="section-title">
            <h2>任务队列</h2>
            <span>
              待分配 {summary.pending ?? 0} · 执行中 {summary.running ?? 0} · 失败 {summary.failed ?? 0} · 超时{" "}
              {summary.timeout ?? 0}
            </span>
          </div>
          <div className="control-failure-card">
            <div className="section-title">
              <h2>失败原因分布（失败+超时）</h2>
              <span>Top {failureReasons.length}</span>
            </div>
            {failureReasons.length ? (
              <div className="failure-reason-list">
                {failureReasons.map((item) => {
                  const count = Number(item.count ?? 0);
                  const ratio = failureReasonMax > 0 ? Math.max(8, Math.round((count / failureReasonMax) * 100)) : 0;
                  return (
                    <div className="failure-reason-row" key={`${item.reason}-${count}`}>
                      <div className="failure-reason-head">
                        <div className="row-title">{item.reason || "unknown"}</div>
                        <div className="row-sub">次数 {count}</div>
                      </div>
                      <div className="failure-reason-bar-track">
                        <div className="failure-reason-bar" style={{ width: `${ratio}%` }} />
                      </div>
                      {item.statuses ? (
                        <div className="failure-reason-statuses">
                          {Object.entries(item.statuses)
                            .sort((a, b) => a[0].localeCompare(b[0]))
                            .map(([status, statusCount]) => (
                              <span className="pill" key={`${item.reason}-${status}`}>
                                {status}:{statusCount}
                              </span>
                            ))}
                        </div>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className="row-sub">暂无失败样本</div>
            )}
          </div>
          <div className="toolbar">
            <select
              className="select"
              value={taskStatus}
              onChange={(event) => setTaskStatus(event.target.value as TaskStatusFilter)}
            >
              <option value="all">全部状态</option>
              <option value="pending">待分配</option>
              <option value="assigned">已分配</option>
              <option value="running">执行中</option>
              <option value="success">成功</option>
              <option value="failed">失败</option>
              <option value="timeout">超时</option>
              <option value="canceled">已取消</option>
            </select>
            <input
              className="search"
              value={taskType}
              onChange={(event) => setTaskType(event.target.value)}
              placeholder="任务类型过滤（可选）"
            />
          </div>

          <div className="control-table-wrap">
            <table className="table control-task-table">
              <thead>
                <tr>
                  <th>任务</th>
                  <th>状态</th>
                  <th>Agent</th>
                  <th>重试</th>
                  <th>时间</th>
                  <th>动作</th>
                </tr>
              </thead>
              <tbody>
                {tasks.length ? (
                  tasks.map((task) => {
                    const isActive = task.id === selectedTaskId;
                    const terminal = task.status === "success" || task.status === "failed" || task.status === "timeout" || task.status === "canceled";
                    const canRetry = terminal && task.retryCount < task.maxRetries;
                    const canCancel = !terminal;

                    return (
                      <tr
                        key={task.id}
                        className={isActive ? "active" : ""}
                        onClick={() => setSelectedTaskId(task.id)}
                        role="button"
                      >
                        <td>
                          <div className="row-title">
                            {task.type} <span className="row-sub">· {task.id}</span>
                          </div>
                          <div className="row-sub">{task.target}</div>
                        </td>
                        <td>
                          <span className={statusBadgeClass(task.status)}>{toTaskStatusLabel(task.status)}</span>
                        </td>
                        <td className="row-sub">{task.assignedAgentId || "--"}</td>
                        <td className="row-sub">
                          {task.retryCount}/{task.maxRetries}
                        </td>
                        <td className="row-sub">
                          <div>创建 {formatTime(task.createdAt)}</div>
                          <div>更新 {formatTime(task.updatedAt)}</div>
                        </td>
                        <td>
                          <div className="control-actions" onClick={(event) => event.stopPropagation()}>
                            <button
                              className="btn tiny secondary"
                              type="button"
                              disabled={!canCancel}
                              onClick={() => void handleCancel(task.id)}
                            >
                              取消
                            </button>
                            <button
                              className="btn tiny secondary"
                              type="button"
                              disabled={!canRetry}
                              onClick={() => void handleRetry(task.id)}
                            >
                              重试
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })
                ) : (
                  <tr>
                    <td colSpan={6} className="row-sub">
                      暂无任务
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <div className="control-detail">
            <div className="section-title">
              <h2>任务事件</h2>
              <span>{selectedTask ? selectedTask.id : "未选择任务"}</span>
            </div>
            {eventsLoading ? (
              <div className="row-sub">加载中...</div>
            ) : selectedTask ? (
              <div className="control-events">
                {events.length ? (
                  events.map((evt) => (
                    <div className="control-event" key={`${evt.id}-${evt.eventType}`}>
                      <div className="control-event-meta">
                        <span className="pill">{evt.eventType}</span>
                        <span className="row-sub">{formatTime(evt.eventTime)}</span>
                        <span className="row-sub">{evt.agentId ? `Agent=${evt.agentId}` : ""}</span>
                      </div>
                      {evt.message ? <div className="row-sub">{evt.message}</div> : null}
                    </div>
                  ))
                ) : (
                  <div className="row-sub">暂无事件（仅在 SQLite 模式下会持久化）</div>
                )}
              </div>
            ) : (
              <div className="row-sub">请选择一条任务查看事件</div>
            )}
          </div>

          <div className="control-detail">
            <div className="section-title">
              <h2>审计日志</h2>
              <span>{selectedTask ? `资源=task/${selectedTask.id}` : "最近审计"}</span>
            </div>
            <div className="audit-toolbar">
              <input
                className="search"
                value={auditFiltersDraft.operator}
                onChange={(event) => handleAuditFilterChange("operator", event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    handleApplyAuditFilters();
                  }
                }}
                placeholder="操作人过滤（可选）"
              />
              <input
                className="search"
                value={auditFiltersDraft.action}
                onChange={(event) => handleAuditFilterChange("action", event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    handleApplyAuditFilters();
                  }
                }}
                placeholder="动作过滤（可选）"
              />
              <input
                className="search audit-time-input"
                type="datetime-local"
                value={auditFiltersDraft.from}
                onChange={(event) => handleAuditFilterChange("from", event.target.value)}
                title="开始时间（可选）"
              />
              <input
                className="search audit-time-input"
                type="datetime-local"
                value={auditFiltersDraft.to}
                onChange={(event) => handleAuditFilterChange("to", event.target.value)}
                title="结束时间（可选）"
              />
              <button className="btn tiny secondary" type="button" onClick={handleApplyAuditFilters} disabled={auditLoading}>
                筛选
              </button>
              <button className="btn tiny secondary" type="button" onClick={handleResetAuditFilters} disabled={auditLoading}>
                重置
              </button>
              <button className="btn tiny secondary" type="button" onClick={handleExportAudits} disabled={!audits.length}>
                导出 JSON
              </button>
            </div>
            <div className="row-sub audit-filter-hint">
              过滤条件：operator={auditFiltersApplied.operator || "*"} · action={auditFiltersApplied.action || "*"} · from=
              {auditFiltersApplied.from || "*"} · to={auditFiltersApplied.to || "*"}
            </div>
            {auditLoading ? (
              <div className="row-sub">加载中...</div>
            ) : audits.length ? (
              <div className="control-events">
                {audits.map((audit) => (
                  <div className="control-event" key={`${audit.id}-${audit.action}`}>
                    <div className="control-event-meta">
                      <span className="pill">{audit.action}</span>
                      <span className="row-sub">{formatTime(audit.createdAt)}</span>
                      <span className="row-sub">{audit.operator ? `operator=${audit.operator}` : "operator=--"}</span>
                    </div>
                    <div className="row-sub">
                      {audit.resourceType}/{audit.resourceId}
                    </div>
                    {audit.detail ? <pre className="audit-detail-json">{JSON.stringify(audit.detail, null, 2)}</pre> : null}
                  </div>
                ))}
              </div>
            ) : (
              <div className="row-sub">暂无审计日志</div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

