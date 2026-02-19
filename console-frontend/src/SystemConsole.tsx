/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于系统资源控制台页面 负责资源采样展示与进程操作 */

import { useEffect, useMemo, useRef, useState } from "react";
import "./SystemConsole.css";
import { systemGauges as mockSystemGauges, systemOverview as mockSystemOverview, systemProcesses as mockSystemProcesses, systemVolumes as mockSystemVolumes } from "./mockData";
import type { SystemDashboard, SystemOverview, SystemProcess, SystemProcessStatus, SystemResourceGauge, SystemVolume } from "./types";
import { buildApiHeaders } from "./console/dashboardApi";

type SortKey = "cpu" | "mem" | "pid" | "name";

const STATUS_LABELS: Record<SystemProcessStatus, string> = {
  running: "运行中",
  sleeping: "休眠",
  stopped: "已停止",
  zombie: "僵尸",
};

const STATUS_TONES: Record<SystemProcessStatus, "success" | "warning" | "danger"> = {
  running: "success",
  sleeping: "warning",
  stopped: "danger",
  zombie: "danger",
};

const CPU_HOTLINE = 35;
const MEM_HOTLINE = 10;
const POLL_MS = 3000;
const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
const USE_MOCK = import.meta.env.DEV && ((import.meta.env.VITE_USE_MOCK as string | undefined) ?? "").toLowerCase() === "true";
const PROCESS_PAGE_SIZE = 10;

const clampPct = (value: number) => Math.max(0, Math.min(100, value));
const clampProcessValue = (value: number) => Math.max(0, value);
const formatPct = (value: number) => `${value.toFixed(1)}%`;
const safeNumber = (value: unknown, fallback = 0) => {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string") {
    const parsed = Number.parseFloat(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
};
const safeString = (value: unknown, fallback = "--") => {
  if (typeof value === "string") return value;
  if (value === null || value === undefined) return fallback;
  return String(value);
};
const safeStringArray = (value: unknown) => {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => safeString(item, ""))
    .filter((item) => item !== "");
};

type LooseRecord = Record<string, unknown>;

// normalize* 系列函数统一处理后端返回的宽松结构
// 先收敛成稳定类型再渲染 能显著降低页面空值判断复杂度
const normalizeOverview = (raw: LooseRecord | null | undefined): SystemOverview => ({
  host: safeString(raw?.host, "--"),
  os: safeString(raw?.os, "--"),
  kernel: safeString(raw?.kernel, "--"),
  uptime: safeString(raw?.uptime, "--"),
  load: safeString(raw?.load, "--"),
  ip: safeString(raw?.ip, "--"),
  lastUpdated: safeString(raw?.lastUpdated, "--"),
  processes: Math.max(0, Math.floor(safeNumber(raw?.processes, 0))),
  connections: Math.max(0, Math.floor(safeNumber(raw?.connections, 0))),
  connectionsBreakdown: safeString(raw?.connectionsBreakdown, "--"),
  cpuTemp: safeString(raw?.cpuTemp, "--"),
  topProcess: safeString(raw?.topProcess, "--"),
});

const normalizeGauge = (raw: LooseRecord | null | undefined): SystemResourceGauge => {
  const tone = raw?.tone;
  const normalizedTone = tone === "warn" || tone === "critical" || tone === "normal" ? tone : undefined;
  const id = raw?.id === "cpu" || raw?.id === "memory" || raw?.id === "disk" ? raw.id : "cpu";
  return {
    id,
    label: safeString(raw?.label, "--"),
    usedPct: clampPct(safeNumber(raw?.usedPct, 0)),
    usedLabel: safeString(raw?.usedLabel, "--"),
    totalLabel: safeString(raw?.totalLabel, "--"),
    subLabel: safeString(raw?.subLabel, "--"),
    trend: safeString(raw?.trend, "--"),
    tone: normalizedTone,
  };
};

const normalizeVolume = (raw: LooseRecord | null | undefined): SystemVolume => ({
  mount: safeString(raw?.mount, "--"),
  usedPct: clampPct(safeNumber(raw?.usedPct, 0)),
  used: safeString(raw?.used, "--"),
  total: safeString(raw?.total, "--"),
});

const normalizeProcess = (raw: LooseRecord | null | undefined): SystemProcess => {
  const status = raw?.status;
  const normalizedStatus: SystemProcessStatus =
    status === "running" || status === "sleeping" || status === "stopped" || status === "zombie" ? status : "sleeping";
  const pid = Math.max(0, Math.floor(safeNumber(raw?.pid, 0)));
  const note = safeString(raw?.note ?? "", "");
  const cpu = clampProcessValue(safeNumber(raw?.cpu, 0));
  const mem = clampPct(safeNumber(raw?.mem, 0));
  return {
    pid,
    name: safeString(raw?.name, "--"),
    command: safeString(raw?.command, "--"),
    user: safeString(raw?.user, "--"),
    status: normalizedStatus,
    cpu,
    mem,
    rss: safeString(raw?.rss, "--"),
    threads: Math.max(0, Math.floor(safeNumber(raw?.threads, 0))),
    start: safeString(raw?.start, "--"),
    uptime: safeString(raw?.uptime, "--"),
    ports: safeStringArray(raw?.ports),
    ioRead: safeString(raw?.ioRead, "--"),
    ioWrite: safeString(raw?.ioWrite, "--"),
    netIn: safeString(raw?.netIn, "--"),
    netOut: safeString(raw?.netOut, "--"),
    cwd: safeString(raw?.cwd, "--"),
    path: safeString(raw?.path, "--"),
    env: safeStringArray(raw?.env),
    note: note || undefined,
  };
};

const normalizeDashboard = (raw: Partial<SystemDashboard> | null | undefined) => {
  return {
    overview: normalizeOverview(raw?.systemOverview ?? {}),
    gauges: Array.isArray(raw?.systemGauges) ? raw?.systemGauges.map(normalizeGauge) : [],
    volumes: Array.isArray(raw?.systemVolumes) ? raw?.systemVolumes.map(normalizeVolume) : [],
    processes: Array.isArray(raw?.systemProcesses) ? raw?.systemProcesses.map(normalizeProcess) : [],
  };
};

type SystemConsoleProps = {
  embedded?: boolean;
  enabled?: boolean;
  toggleLoading?: boolean;
  toggleError?: string | null;
  onToggleEnabled?: (next: boolean) => void;
};

const emptyOverview: SystemOverview = {
  host: "--",
  os: "--",
  kernel: "--",
  uptime: "--",
  load: "--",
  ip: "--",
  lastUpdated: "--",
  processes: 0,
  connections: 0,
  connectionsBreakdown: "--",
  cpuTemp: "--",
  topProcess: "--",
};

// SystemConsole 负责系统资源看板与进程操作入口
// 页面采用“轮询 + 本地筛选”的模式 兼顾实时性和交互流畅度
export function SystemConsole({
  embedded = false,
  enabled = true,
  toggleLoading = false,
  toggleError = null,
  onToggleEnabled,
}: SystemConsoleProps) {
  const [overview, setOverview] = useState<SystemOverview>(USE_MOCK ? mockSystemOverview : emptyOverview);
  const [gauges, setGauges] = useState<SystemResourceGauge[]>(USE_MOCK ? mockSystemGauges : []);
  const [volumes, setVolumes] = useState<SystemVolume[]>(USE_MOCK ? mockSystemVolumes : []);
  const [processes, setProcesses] = useState<SystemProcess[]>(USE_MOCK ? mockSystemProcesses : []);
  const [loading, setLoading] = useState(!USE_MOCK);
  const [error, setError] = useState<string | null>(null);
  const [usingMockData, setUsingMockData] = useState(USE_MOCK);
  const fetchingRef = useRef(false);
  const aliveRef = useRef(true);

  const [searchTerm, setSearchTerm] = useState("");
  const [portTerm, setPortTerm] = useState("");
  const [statusFilter, setStatusFilter] = useState<SystemProcessStatus | "all">("all");
  const [sortKey, setSortKey] = useState<SortKey>("cpu");
  const [hotCpuOnly, setHotCpuOnly] = useState(false);
  const [hotMemOnly, setHotMemOnly] = useState(false);
  const [listeningOnly, setListeningOnly] = useState(false);
  const [selectedPid, setSelectedPid] = useState<number | null>(() => (USE_MOCK ? mockSystemProcesses[0]?.pid ?? null : null));
  const [procPage, setProcPage] = useState(1);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [terminating, setTerminating] = useState(false);

  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
    };
  }, []);

  useEffect(() => {
    if (!enabled) {
      fetchingRef.current = false;
      setLoading(false);
      setError(null);
      setUsingMockData(false);
      return;
    }
    // refresh 是系统看板的统一拉取函数
    // 同时使用 aliveRef 和 fetchingRef 避免组件卸载后 setState 与并发重入
    const refresh = async () => {
      if (fetchingRef.current || !aliveRef.current) return;
      fetchingRef.current = true;
      // 仅在启用时轮询，避免无效请求增加后端负载
      try {
        const resp = await fetch(`${API_BASE}/api/system?limit=0`, { cache: "no-store", headers: buildApiHeaders() });
        if (!resp.ok) {
          throw new Error(`HTTP ${resp.status}`);
        }
        const payload = (await resp.json()) as Partial<SystemDashboard>;
        if (!aliveRef.current) return;
        const normalized = normalizeDashboard(payload);
        setOverview(normalized.overview);
        setGauges(normalized.gauges);
        setVolumes(normalized.volumes);
        setProcesses(normalized.processes);
        setUsingMockData(false);
        setError(null);
      } catch (err) {
        if (!aliveRef.current) return;
        setError((err as Error).message || "数据获取失败");
      } finally {
        fetchingRef.current = false;
        if (aliveRef.current) {
          setLoading(false);
        }
      }
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [enabled]);

  const filteredProcesses = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    const port = portTerm.trim().toLowerCase();
    return processes.filter((proc) => {
      if (statusFilter !== "all" && proc.status !== statusFilter) return false;
      if (hotCpuOnly && proc.cpu < CPU_HOTLINE) return false;
      if (hotMemOnly && proc.mem < MEM_HOTLINE) return false;
      if (listeningOnly && proc.ports.length === 0) return false;
      if (term) {
        const haystack = `${proc.name} ${proc.command} ${proc.user} ${proc.pid}`.toLowerCase();
        if (!haystack.includes(term)) return false;
      }
      if (port) {
        const ports = proc.ports.join(" ").toLowerCase();
        if (!ports.includes(port)) return false;
      }
      return true;
    });
  }, [searchTerm, portTerm, statusFilter, hotCpuOnly, hotMemOnly, listeningOnly, processes]);

  const sortedProcesses = useMemo(() => {
    const next = [...filteredProcesses];
    if (sortKey === "name") {
      next.sort((a, b) => a.name.localeCompare(b.name));
      return next;
    }
    if (sortKey === "pid") {
      next.sort((a, b) => a.pid - b.pid);
      return next;
    }
    if (sortKey === "mem") {
      next.sort((a, b) => b.mem - a.mem);
      return next;
    }
    next.sort((a, b) => b.cpu - a.cpu);
    return next;
  }, [filteredProcesses, sortKey]);

  const pageCount = Math.max(1, Math.ceil(sortedProcesses.length / PROCESS_PAGE_SIZE));
  const pageSafe = Math.min(procPage, pageCount);

  useEffect(() => {
    if (procPage !== pageSafe) {
      setProcPage(pageSafe);
    }
  }, [pageSafe, procPage]);

  useEffect(() => {
    setProcPage(1);
  }, [searchTerm, portTerm, statusFilter, hotCpuOnly, hotMemOnly, listeningOnly, sortKey]);

  const pagedProcesses = useMemo(() => {
    const start = (pageSafe - 1) * PROCESS_PAGE_SIZE;
    return sortedProcesses.slice(start, start + PROCESS_PAGE_SIZE);
  }, [sortedProcesses, pageSafe]);

  useEffect(() => {
    if (!sortedProcesses.length) {
      setSelectedPid(null);
      return;
    }
    if (selectedPid && pagedProcesses.some((proc) => proc.pid === selectedPid)) return;
    if (selectedPid && sortedProcesses.some((proc) => proc.pid === selectedPid)) {
      setSelectedPid(pagedProcesses[0]?.pid ?? sortedProcesses[0].pid);
      return;
    }
    setSelectedPid(pagedProcesses[0]?.pid ?? sortedProcesses[0].pid);
  }, [pagedProcesses, selectedPid, sortedProcesses]);

  useEffect(() => {
    setActionMessage(null);
  }, [selectedPid]);

  const selectedProcess = useMemo(() => {
    if (!selectedPid) return null;
    return sortedProcesses.find((proc) => proc.pid === selectedPid) ?? null;
  }, [selectedPid, sortedProcesses]);

  const showTopline = (usingMockData || error || toggleError) && enabled;
  const toggleDisabled = !onToggleEnabled || toggleLoading;

  const handleTerminate = async () => {
    if (!selectedProcess || terminating) return;
    setError(null);
    setActionMessage(null);
    setTerminating(true);
    try {
      const resp = await fetch(`${API_BASE}/api/system/terminate`, {
        method: "POST",
        cache: "no-store",
        headers: buildApiHeaders(true),
        body: JSON.stringify({ pid: selectedProcess.pid }),
      });
      const payload = (await resp.json().catch(() => ({}))) as {
        error?: string;
        result?: { signal?: string; forced?: boolean };
      };
      if (!resp.ok) {
        throw new Error(payload.error?.trim() || `HTTP ${resp.status}`);
      }
      const signal = payload.result?.forced ? "KILL" : payload.result?.signal ?? "TERM";
      setActionMessage(`已执行 ${selectedProcess.name} (PID ${selectedProcess.pid}) 终止动作：${signal}`);
    } catch (err) {
      setError((err as Error).message || "终止失败");
    } finally {
      setTerminating(false);
    }
  };

  const renderProcessPorts = (proc: SystemProcess) =>
    proc.ports.length ? (
      <div className="port-list">
        {proc.ports.map((port) => (
          <span className="port-tag" key={`${proc.pid}-${port}`}>
            {port}
          </span>
        ))}
      </div>
    ) : (
      <span className="muted small">无监听</span>
    );

  return (
    <div className={`system-shell ${embedded ? "system-embedded" : ""}${enabled ? "" : " is-disabled"}`}>
      {!enabled ? (
        <div className="system-disabled">
          <div className="system-disabled-card">
            <h2>系统资源控制台未启用</h2>
            <p>启用后可查看 CPU、内存、磁盘与进程占用情况。</p>
            {toggleError ? <div className="system-toggle-error">启用失败：{toggleError}</div> : null}
            <div className="system-disabled-actions">
              {onToggleEnabled ? (
                <button className="btn" type="button" onClick={() => onToggleEnabled(true)} disabled={toggleDisabled}>
                  {toggleLoading ? "启用中..." : "立即启用"}
                </button>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}
      {showTopline ? (
        <div className="system-topline">
          {toggleError ? <div className="pill danger">开关更新失败：{toggleError}</div> : null}
          {error ? <div className="badge ghost">{error}</div> : null}
          {usingMockData ? <span className="pill warning">正在显示示例数据</span> : null}
        </div>
      ) : null}
      <section className="system-hero" id="system-overview">
        <div className="system-hero-main">
          <p className="eyebrow">系统资源控制台</p>
          <h1>系统资源控制台</h1>
          <p className="subtitle">实时查看 CPU、内存、磁盘与进程占用情况</p>
          <div className="system-toggle">
            <span className="muted small">系统资源控制台</span>
            <label className="switch">
              <input
                type="checkbox"
                checked={enabled}
                disabled={toggleDisabled}
                onChange={(e) => onToggleEnabled?.(e.target.checked)}
              />
              <span className="slider" />
            </label>
            <span className={`pill mini-pill ${enabled ? "success" : "warning"}`}>{enabled ? "已启用" : "未启用"}</span>
          </div>
          <div className="system-meta">
            <span className="badge ghost">主机 {overview.host}</span>
            <span className="badge ghost">IP {overview.ip}</span>
            <span className="badge ghost">运行 {overview.uptime}</span>
            <span className="badge ghost">OS {overview.os}</span>
            <span className="badge ghost">内核 {overview.kernel}</span>
            <span className="badge ghost">更新 {overview.lastUpdated}</span>
          </div>
        </div>
        <div className="system-hero-side">
          <div className="system-kpi">
            <small>系统负载</small>
            <div className="kpi-value">{overview.load}</div>
            <span className="muted small">CPU 温度 {overview.cpuTemp}</span>
          </div>
          <div className="system-kpi">
            <small>进程总数</small>
            <div className="kpi-value">{overview.processes}</div>
            <span className="muted small">最高占用 {overview.topProcess}</span>
          </div>
          <div className="system-kpi">
            <small>活动连接</small>
            <div className="kpi-value">{overview.connections}</div>
            <span className="muted small">{overview.connectionsBreakdown}</span>
          </div>
        </div>
      </section>

      <section className="panel" id="system-resources">
        <div className="section-title">
          <h2>资源总览</h2>
          <span>刷新间隔 3 秒</span>
        </div>
        <div className="system-gauge-grid">
          {gauges.map((gauge) => (
            <div className={`system-gauge tone-${gauge.tone ?? "normal"}`} key={gauge.id}>
              <div className="gauge-header">
                <div>
                  <small>{gauge.label}</small>
                  <div className="gauge-value">{gauge.usedLabel}</div>
                </div>
                <span className="badge ghost">{gauge.totalLabel}</span>
              </div>
              <div className="gauge-bar">
                <span className="gauge-fill" style={{ width: `${clampPct(gauge.usedPct)}%` }} />
              </div>
              <div className="gauge-meta">
                <span>{gauge.subLabel}</span>
                <span className="muted small">{gauge.trend}</span>
              </div>
            </div>
          ))}
        </div>
      </section>

      <section className="panel" id="system-volumes">
        <div className="section-title">
          <h2>磁盘分区</h2>
          <span>容量 / 使用率</span>
        </div>
        <div className="volume-grid">
          {volumes.map((volume) => (
            <div className="volume-item" key={volume.mount}>
              <div className="volume-head">
                <strong>{volume.mount}</strong>
                <span className="muted small">
                  {volume.used} / {volume.total}
                </span>
              </div>
              <div className="volume-bar">
                <span className="volume-fill" style={{ width: `${clampPct(volume.usedPct)}%` }} />
              </div>
              <div className="volume-meta">{volume.usedPct.toFixed(2)}% 已使用</div>
            </div>
          ))}
        </div>
      </section>

      <section className="panel" id="system-processes">
        <div className="section-title">
          <h2>进程列表 / 资源占用</h2>
          <span>
            展示 {pagedProcesses.length} / {sortedProcesses.length} · 总 {processes.length}
          </span>
        </div>
        <div className="toolbar">
          <input
            className="search"
            placeholder="搜索名称 / PID / 命令"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
          <input
            className="search"
            placeholder="端口过滤（例 8080 / 5432）"
            value={portTerm}
            onChange={(e) => setPortTerm(e.target.value)}
          />
          <select className="select" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value as SystemProcessStatus | "all")}>
            <option value="all">全部状态</option>
            <option value="running">运行中</option>
            <option value="sleeping">休眠</option>
            <option value="stopped">已停止</option>
            <option value="zombie">僵尸</option>
          </select>
          <select className="select" value={sortKey} onChange={(e) => setSortKey(e.target.value as SortKey)}>
            <option value="cpu">CPU 占用排序</option>
            <option value="mem">内存占用排序</option>
            <option value="pid">PID 排序</option>
            <option value="name">名称排序</option>
          </select>
          <div className={`chip ${hotCpuOnly ? "active" : ""}`} onClick={() => setHotCpuOnly((prev) => !prev)}>
            高 CPU
          </div>
          <div className={`chip ${hotMemOnly ? "active" : ""}`} onClick={() => setHotMemOnly((prev) => !prev)}>
            高内存
          </div>
          <div className={`chip ${listeningOnly ? "active" : ""}`} onClick={() => setListeningOnly((prev) => !prev)}>
            监听端口
          </div>
        </div>
        <div className="process-table-shell">
          <div className="table-scroll">
            <table className="table process-table">
              <thead>
                <tr>
                  <th>进程</th>
                  <th>CPU</th>
                  <th>内存</th>
                  <th>I/O &amp; 网络</th>
                  <th>端口</th>
                  <th>状态</th>
                </tr>
              </thead>
              <tbody>
                {pagedProcesses.length ? (
                  pagedProcesses.map((proc) => (
                    <tr
                      key={proc.pid}
                      className={proc.pid === selectedPid ? "selected" : ""}
                      onClick={() => setSelectedPid(proc.pid)}
                    >
                      <td>
                        <div className="row-title">{proc.name}</div>
                        <div className="row-sub">
                          PID {proc.pid} · {proc.user}
                        </div>
                      </td>
                      <td>
                        <div className="metric-cell">
                          <span>{formatPct(proc.cpu)}</span>
                          <div className="mini-meter">
                            <span className="mini-fill" style={{ width: `${clampPct(proc.cpu)}%` }} />
                          </div>
                        </div>
                        <div className="row-sub">{proc.threads} 线程</div>
                      </td>
                      <td>
                        <div className="metric-cell">
                          <span>{formatPct(proc.mem)}</span>
                          <div className="mini-meter">
                            <span className="mini-fill mem" style={{ width: `${clampPct(proc.mem)}%` }} />
                          </div>
                        </div>
                        <div className="row-sub">RSS {proc.rss}</div>
                      </td>
                      <td>
                        <div className="row-title">
                          读 {proc.ioRead} · 写 {proc.ioWrite}
                        </div>
                        <div className="row-sub">
                          入 {proc.netIn} · 出 {proc.netOut}
                        </div>
                      </td>
                      <td>{renderProcessPorts(proc)}</td>
                      <td>
                        <span className={`pill table-pill ${STATUS_TONES[proc.status]}`}>{STATUS_LABELS[proc.status]}</span>
                        {proc.note ? <div className="row-sub">{proc.note}</div> : null}
                      </td>
                    </tr>
                  ))
                ) : (
                  <tr>
                    <td className="table-empty" colSpan={6}>
                      {loading ? "数据加载中..." : "暂无匹配进程"}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
        <div className="table-actions process-pagination">
          <span className="muted small pagination-meta">
            第 {pageSafe} / {pageCount} 页 · 每页 {PROCESS_PAGE_SIZE} 条
          </span>
          <button className="btn secondary" type="button" disabled={pageSafe <= 1} onClick={() => setProcPage(1)}>
            首页
          </button>
          <button className="btn secondary" type="button" disabled={pageSafe <= 1} onClick={() => setProcPage((prev) => Math.max(1, prev - 1))}>
            上一页
          </button>
          <button className="btn secondary" type="button" disabled={pageSafe >= pageCount} onClick={() => setProcPage((prev) => Math.min(pageCount, prev + 1))}>
            下一页
          </button>
          <button className="btn secondary" type="button" disabled={pageSafe >= pageCount} onClick={() => setProcPage(pageCount)}>
            末页
          </button>
        </div>
      </section>

      <section className="panel" id="system-process-detail">
        <div className="section-title">
          <h2>进程详情</h2>
          <span>{selectedProcess ? `PID ${selectedProcess.pid} · ${selectedProcess.user}` : "暂无选择"}</span>
        </div>
        <div className="process-detail">
          {selectedProcess ? (
            <>
              <div className="detail-header">
                <div>
                  <span className={`pill ${STATUS_TONES[selectedProcess.status]}`}>{STATUS_LABELS[selectedProcess.status]}</span>
                  <h3>{selectedProcess.name}</h3>
                  <div className="row-sub">
                    PID {selectedProcess.pid} · {selectedProcess.user} · 启动 {selectedProcess.start}
                  </div>
                </div>
                <button className="btn danger" type="button" onClick={() => void handleTerminate()} disabled={terminating || !selectedProcess}>
                  {terminating ? "执行中..." : "一键关闭"}
                </button>
              </div>
              {actionMessage ? <div className="badge ghost">{actionMessage}</div> : null}
              <div className="detail-metrics">
                <div className="detail-metric">
                  <small>CPU</small>
                  <strong>{formatPct(selectedProcess.cpu)}</strong>
                  <span className="muted small">线程 {selectedProcess.threads}</span>
                </div>
                <div className="detail-metric">
                  <small>内存</small>
                  <strong>{formatPct(selectedProcess.mem)}</strong>
                  <span className="muted small">RSS {selectedProcess.rss}</span>
                </div>
                <div className="detail-metric">
                  <small>运行时长</small>
                  <strong>{selectedProcess.uptime}</strong>
                  <span className="muted small">启动 {selectedProcess.start}</span>
                </div>
                <div className="detail-metric">
                  <small>I/O</small>
                  <strong>
                    读 {selectedProcess.ioRead} / 写 {selectedProcess.ioWrite}
                  </strong>
                  <span className="muted small">
                    入 {selectedProcess.netIn} / 出 {selectedProcess.netOut}
                  </span>
                </div>
              </div>
              <div className="detail-grid">
                <div className="detail-row">
                  <div className="detail-label">启动命令</div>
                  <div className="detail-value">{selectedProcess.command}</div>
                </div>
                <div className="detail-row">
                  <div className="detail-label">执行路径</div>
                  <div className="detail-value">{selectedProcess.path}</div>
                </div>
                <div className="detail-row">
                  <div className="detail-label">工作目录</div>
                  <div className="detail-value">{selectedProcess.cwd}</div>
                </div>
                <div className="detail-row">
                  <div className="detail-label">监听端口</div>
                  <div className="detail-value">{renderProcessPorts(selectedProcess)}</div>
                </div>
                <div className="detail-row">
                  <div className="detail-label">环境变量</div>
                  <div className="detail-tags">
                    {selectedProcess.env.map((item) => (
                      <span className="env-tag" key={`${selectedProcess.pid}-${item}`}>
                        {item}
                      </span>
                    ))}
                  </div>
                </div>
              </div>
            </>
          ) : (
            <div className="empty-state">暂无进程可查看</div>
          )}
        </div>
      </section>
    </div>
  );
}

