import { useEffect, useMemo, useState } from "react";
import "./SystemConsole.css";
import { systemGauges, systemOverview, systemProcesses, systemVolumes } from "./mockData";
import type { SystemProcess, SystemProcessStatus } from "./types";

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

const clampPct = (value: number) => Math.max(0, Math.min(100, value));
const formatPct = (value: number) => `${value.toFixed(1)}%`;

type SystemConsoleProps = {
  embedded?: boolean;
};

export function SystemConsole({ embedded = false }: SystemConsoleProps) {
  const [searchTerm, setSearchTerm] = useState("");
  const [portTerm, setPortTerm] = useState("");
  const [statusFilter, setStatusFilter] = useState<SystemProcessStatus | "all">("all");
  const [sortKey, setSortKey] = useState<SortKey>("cpu");
  const [hotCpuOnly, setHotCpuOnly] = useState(false);
  const [hotMemOnly, setHotMemOnly] = useState(false);
  const [listeningOnly, setListeningOnly] = useState(false);
  const [selectedPid, setSelectedPid] = useState<number | null>(() => systemProcesses[0]?.pid ?? null);
  const [actionMessage, setActionMessage] = useState<string | null>(null);

  const filteredProcesses = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    const port = portTerm.trim().toLowerCase();
    return systemProcesses.filter((proc) => {
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
  }, [searchTerm, portTerm, statusFilter, hotCpuOnly, hotMemOnly, listeningOnly]);

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

  useEffect(() => {
    if (!sortedProcesses.length) {
      setSelectedPid(null);
      return;
    }
    if (selectedPid && sortedProcesses.some((proc) => proc.pid === selectedPid)) return;
    setSelectedPid(sortedProcesses[0].pid);
  }, [sortedProcesses, selectedPid]);

  useEffect(() => {
    setActionMessage(null);
  }, [selectedPid]);

  const selectedProcess = useMemo(() => {
    if (!selectedPid) return null;
    return sortedProcesses.find((proc) => proc.pid === selectedPid) ?? null;
  }, [selectedPid, sortedProcesses]);

  const handleTerminate = () => {
    if (!selectedProcess) return;
    setActionMessage(`已发送 ${selectedProcess.name} (PID ${selectedProcess.pid}) 的关闭指令`);
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
    <div className={`system-shell ${embedded ? "system-embedded" : ""}`}>
      <section className="system-hero" id="system-overview">
        <div className="system-hero-main">
          <p className="eyebrow">System Resource Manager</p>
          <h1>系统资源管理器</h1>
          <p className="subtitle">实时 CPU、内存、磁盘与进程资源概览，支持筛选与快速处置。</p>
          <div className="system-meta">
            <span className="badge ghost">主机 {systemOverview.host}</span>
            <span className="badge ghost">IP {systemOverview.ip}</span>
            <span className="badge ghost">运行 {systemOverview.uptime}</span>
            <span className="badge ghost">OS {systemOverview.os}</span>
            <span className="badge ghost">内核 {systemOverview.kernel}</span>
            <span className="badge ghost">更新 {systemOverview.lastUpdated}</span>
          </div>
        </div>
        <div className="system-hero-side">
          <div className="system-kpi">
            <small>系统负载</small>
            <div className="kpi-value">{systemOverview.load}</div>
            <span className="muted small">CPU 温度 {systemOverview.cpuTemp}</span>
          </div>
          <div className="system-kpi">
            <small>进程总数</small>
            <div className="kpi-value">{systemOverview.processes}</div>
            <span className="muted small">最高占用 {systemOverview.topProcess}</span>
          </div>
          <div className="system-kpi">
            <small>活动连接</small>
            <div className="kpi-value">{systemOverview.connections}</div>
            <span className="muted small">{systemOverview.connectionsBreakdown}</span>
          </div>
        </div>
      </section>

      <section className="panel" id="system-resources">
        <div className="section-title">
          <h2>资源总览</h2>
          <span>刷新间隔 3 秒</span>
        </div>
        <div className="system-gauge-grid">
          {systemGauges.map((gauge) => (
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
          {systemVolumes.map((volume) => (
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
              <div className="volume-meta">{volume.usedPct}% 已使用</div>
            </div>
          ))}
        </div>
      </section>

      <section className="panel" id="system-processes">
        <div className="section-title">
          <h2>进程列表 / 资源占用</h2>
          <span>
            展示 {sortedProcesses.length} / {systemProcesses.length}
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
                {sortedProcesses.length ? (
                  sortedProcesses.map((proc) => (
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
                      暂无匹配进程
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
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
                <button className="btn danger" type="button" onClick={handleTerminate}>
                  一键关闭
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
