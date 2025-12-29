import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Line } from "react-chartjs-2";
import type { ChartOptions } from "chart.js";
import { CategoryScale, Chart as ChartJS, Filler, Legend, LineElement, LinearScale, PointElement, Tooltip } from "chart.js";
import "./App.css";
import {
  chartPoints,
  configSnapshot,
  directoryTree as treeSeed,
  files as fileSeed,
  heroCopy,
  metricCards,
  monitorNotes,
  monitorSummary,
  tailLines,
  timelineEvents,
  uploadRecords,
} from "./mockData";
import type { ConfigSnapshot, DashboardPayload, FileFilter, FileItem, FileNode } from "./types";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Filler, Legend);

const SECTION_IDS = ["overview", "config", "directory", "files", "tail", "failures", "monitor"];
const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";

const fmt = (t: string) => `${new Date().toISOString().split("T")[0]} ${t}`;

const findFirstFile = (nodes: FileNode[]): FileNode | undefined => {
  for (const node of nodes) {
    if (node.type === "file") return node;
    if (node.children) {
      const child = findFirstFile(node.children);
      if (child) return child;
    }
  }
  return undefined;
};

const findNode = (nodes: FileNode[], path: string): FileNode | undefined => {
  for (const node of nodes) {
    if (node.path === path) return node;
    if (node.children) {
      const found = findNode(node.children, path);
      if (found) return found;
    }
  }
  return undefined;
};

const escapeRegExp = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

const propagateAuto = (children: FileNode[] | undefined, value: boolean): FileNode[] | undefined => {
  if (!children) return children;
  return children.map((child) => ({
    ...child,
    autoUpload: value,
    children: propagateAuto(child.children, value),
  }));
};

const updateAutoUpload = (nodes: FileNode[], path: string, value: boolean): FileNode[] => {
  return nodes.map((node) => {
    if (node.path === path) {
      return {
        ...node,
        autoUpload: value,
        children: propagateAuto(node.children, value),
      };
    }
    if (node.children) {
      return {
        ...node,
        children: updateAutoUpload(node.children, path, value),
      };
    }
    return node;
  });
};

function App() {
  const [tree, setTree] = useState<FileNode[]>(treeSeed);
  const [files, setFiles] = useState<FileItem[]>(fileSeed);
  const [currentRoot, setCurrentRoot] = useState<string>(treeSeed[0]?.path ?? "");
  const [activePath, setActivePath] = useState<string>(findFirstFile(treeSeed)?.path ?? "");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [fileFilter, setFileFilter] = useState<FileFilter>("all");
  const [searchTerm, setSearchTerm] = useState("");
  const [manualUploadMap, setManualUploadMap] = useState<Record<string, string>>({});
  const [activeSection, setActiveSection] = useState<string>(SECTION_IDS[0]);
  const [configForm, setConfigForm] = useState(configSnapshot);
  const [keyword, setKeyword] = useState("");
  const [showMatchesOnly, setShowMatchesOnly] = useState(false);
  const [heroState, setHeroState] = useState(heroCopy);
  const [metricCardsState, setMetricCardsState] = useState(metricCards);
  const [monitorNotesState, setMonitorNotesState] = useState(monitorNotes);
  const [uploadRecordsState, setUploadRecordsState] = useState(uploadRecords);
  const [monitorSummaryState, setMonitorSummaryState] = useState(monitorSummary);
  const [chartPointsState, setChartPointsState] = useState(chartPoints);
  const [tailLinesState, setTailLinesState] = useState(tailLines);
  const [timelineEventsState, setTimelineEventsState] = useState(timelineEvents);
  const [timeframe, setTimeframe] = useState<"realtime" | "24h">("realtime");
  const [saveMessage, setSaveMessage] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const lastSavedConfig = useRef<ConfigSnapshot | null>(null);

  const rootNodes = useMemo(() => {
    const filtered = tree.filter((node) => !currentRoot || node.path === currentRoot);
    return filtered.length ? filtered : tree;
  }, [tree, currentRoot]);

  const activeNode = useMemo(() => {
    if (!activePath) return undefined;
    return findNode(rootNodes, activePath);
  }, [activePath, rootNodes]);

  const refreshDashboard = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`${API_BASE}/api/dashboard`);
      if (!res.ok) {
        throw new Error(`加载数据失败，状态码 ${res.status}`);
      }
      const data = (await res.json()) as Partial<DashboardPayload>;
      const directoryTree = data.directoryTree ?? [];
      const filesData = data.files ?? [];
      const heroData = data.heroCopy ?? heroCopy;
      const metrics = data.metricCards ?? metricCards;
      const notes = data.monitorNotes ?? monitorNotes;
      const uploads = data.uploadRecords ?? [];
      const summary = data.monitorSummary ?? monitorSummary;
      const chartPointsData = data.chartPoints ?? [];
      const tails = data.tailLines ?? [];
      const timeline = data.timelineEvents ?? [];

      let configData = (data.configSnapshot as ConfigSnapshot | undefined) ?? configSnapshot;
      if (lastSavedConfig.current) {
        configData = { ...configData, ...lastSavedConfig.current };
      }

      const mergedHero = {
        ...heroData,
        silence: lastSavedConfig.current?.silence ?? heroData.silence,
        watchDirs: lastSavedConfig.current?.watchDir
          ? lastSavedConfig.current.watchDir.split(",").map((d) => d.trim()).filter(Boolean)
          : heroData.watchDirs,
      };

      setTree(directoryTree);
      setFiles(filesData);
      setHeroState(mergedHero);
      setMetricCardsState(metrics);
      setMonitorNotesState(notes);
      setUploadRecordsState(uploads);
      setMonitorSummaryState(summary);
      setConfigForm(configData);
      setChartPointsState(chartPointsData);
      setTailLinesState(tails);
      setTimelineEventsState(timeline);
      setCurrentRoot((prev) => directoryTree[0]?.path ?? mergedHero.watchDirs[0] ?? prev);
      setActivePath((prev) => {
        const nextActive = findFirstFile(directoryTree)?.path ?? filesData[0]?.path ?? prev;
        return nextActive || prev;
      });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refreshDashboard();
  }, [refreshDashboard]);

  useEffect(() => {
    const hasActiveUnderRoot = activePath && activePath.startsWith(currentRoot);
    if (hasActiveUnderRoot) return;
    const next = findFirstFile(rootNodes);
    setActivePath(next?.path ?? "");
  }, [currentRoot, rootNodes, activePath]);

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => b.intersectionRatio - a.intersectionRatio);
        if (visible[0]?.target?.id) {
          setActiveSection(visible[0].target.id);
        }
      },
      { threshold: [0.25, 0.5], rootMargin: "-30% 0px -30% 0px" }
    );

    const targets = SECTION_IDS.map((id) => document.getElementById(id)).filter(Boolean) as Element[];
    targets.forEach((el) => observer.observe(el));
    return () => observer.disconnect();
  }, []);

  const handleAutoToggle = async (path: string, value: boolean) => {
    const node = findNode(tree, path);
    const isDir = node?.type === "dir";
    setTree((prev) => updateAutoUpload(prev, path, value));
    setFiles((prev) =>
      prev.map((f) => {
        if (f.path === path || (isDir && f.path.startsWith(`${path}/`))) {
          return { ...f, autoUpload: value };
        }
        return f;
      })
    );
    try {
      await fetch(`${API_BASE}/api/auto-upload`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path, enabled: value }),
      });
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const handleCollapseToggle = (path: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  const handleCollapseAll = (collapse: boolean) => {
    if (!collapse) {
      setCollapsed(new Set());
      return;
    }
    const dirPaths: string[] = [];
    const walk = (nodes: FileNode[]) => {
      nodes.forEach((n) => {
        if (n.type === "dir") dirPaths.push(n.path);
        if (n.children) walk(n.children);
      });
    };
    walk(rootNodes);
    setCollapsed(new Set(dirPaths));
  };

  const handleManualUpload = async () => {
    if (!activePath) return;
    const now = new Date().toTimeString().slice(0, 8);
    setManualUploadMap((prev) => ({ ...prev, [activePath]: now }));
    try {
      const res = await fetch(`${API_BASE}/api/manual-upload`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: activePath }),
      });
      if (!res.ok) {
        throw new Error(`手动上传失败，状态码 ${res.status}`);
      }
      await refreshDashboard();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const filteredFiles = useMemo(() => {
    return files
      .filter((f) => (currentRoot ? f.path.startsWith(currentRoot) : true))
      .filter((f) =>
        searchTerm.trim()
          ? f.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
            f.path.toLowerCase().includes(searchTerm.toLowerCase())
          : true
      )
      .filter((f) => {
        switch (fileFilter) {
          case "auto":
            return f.autoUpload;
          case "manual":
            return !f.autoUpload;
          case "failed":
            return f.status === "failed";
          default:
            return true;
        }
      });
  }, [files, currentRoot, fileFilter, searchTerm]);

  const keywordNormalized = keyword.trim().toLowerCase();

  const tailDisplayLines = useMemo(() => {
    if (!keywordNormalized) return tailLinesState;
    const matched = tailLinesState.filter((line) => line.toLowerCase().includes(keywordNormalized));
    return showMatchesOnly ? matched : tailLinesState;
  }, [keywordNormalized, showMatchesOnly, tailLinesState]);

  const highlightTailLine = (line: string) => {
    if (!keywordNormalized) return line;
    const regex = new RegExp(escapeRegExp(keywordNormalized), "gi");
    const segments = line.split(regex);
    const matches = line.match(regex);
    return segments.flatMap((segment, idx) => {
      const match = matches?.[idx];
      return match
        ? [segment, <mark className="highlight" key={`${line}-${idx}`}>{match}</mark>]
        : [segment];
    });
  };

  const handleSaveSnapshot = async () => {
    setSaveMessage(null);
    setError(null);
    const watchDir = configForm.watchDir?.trim();
    const fileExt = configForm.fileExt?.trim() ?? "";
    const silence = configForm.silence?.trim() ?? "";
    if (!watchDir) {
      setSaveMessage("请填写监控目录");
      return;
    }
    const { workers, queue } = parseConcurrency(configForm.concurrency);
    if (!Number.isFinite(workers) || !Number.isFinite(queue)) {
      setSaveMessage("并发/队列格式不合法，示例：workers=3 / queue=100");
      return;
    }
    setSaving(true);
    try {
      const res = await fetch(`${API_BASE}/api/config`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          watchDir,
          fileExt,
          uploadWorkers: workers,
          uploadQueueSize: queue,
          silence,
        }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || `保存失败，状态码 ${res.status}`);
      }
      const data = (await res.json()) as {
        ok?: boolean;
        config?: { watchDir: string; fileExt: string; concurrency?: string; silence?: string };
      };
      const payloadConfig = data.config;
      const nextConfig: ConfigSnapshot = {
        watchDir: payloadConfig?.watchDir ?? watchDir,
        fileExt: payloadConfig?.fileExt ?? fileExt,
        concurrency: payloadConfig?.concurrency ?? configForm.concurrency,
        silence: payloadConfig?.silence ?? silence ?? configForm.silence,
      };
      lastSavedConfig.current = nextConfig;
      setConfigForm(nextConfig);
      setCurrentRoot(nextConfig.watchDir);
      setHeroState((prev) => ({
        ...prev,
        watchDirs: nextConfig.watchDir ? nextConfig.watchDir.split(",").map((d) => d.trim()).filter(Boolean) : prev.watchDirs,
        silence: nextConfig.silence ?? prev.silence,
      }));
      setSaveMessage(`已保存当前表单（监控目录：${watchDir}），后端已应用`);
      await refreshDashboard();
    } catch (err) {
      setError((err as Error).message);
      setSaveMessage(null);
    } finally {
      setSaving(false);
    }
  };

  const chartData = useMemo(
    () => ({
      labels: chartPointsState.map((p) => p.label),
      datasets: [
        {
          label: "Uploads",
          data: chartPointsState.map((p) => p.uploads),
          borderColor: "#22d3ee",
          backgroundColor: "rgba(34,211,238,0.12)",
          fill: true,
          tension: 0.35,
          pointRadius: 0,
        },
        {
          label: "Failures",
          data: chartPointsState.map((p) => p.failures),
          borderColor: "#f43f5e",
          backgroundColor: "rgba(244,63,94,0.10)",
          fill: true,
          tension: 0.35,
          pointRadius: 0,
        },
        {
          label: "Queue",
          data: chartPointsState.map((p) => p.queue),
          borderColor: "#f59e0b",
          backgroundColor: "rgba(245,158,11,0.08)",
          fill: true,
          tension: 0.35,
          pointRadius: 0,
        },
      ],
    }),
    [chartPointsState]
  );

  const parseConcurrency = (value: string) => {
    const workersMatch = value.match(/workers\s*=?\s*(\d+)/i) ?? value.match(/并发\s*=?\s*(\d+)/i);
    const queueMatch = value.match(/queue\s*=?\s*(\d+)/i) ?? value.match(/队列\s*=?\s*(\d+)/i);
    const workers = workersMatch ? Number.parseInt(workersMatch[1], 10) : Number.NaN;
    const queue = queueMatch ? Number.parseInt(queueMatch[1], 10) : Number.NaN;
    return { workers, queue };
  };

  const chartOptions: ChartOptions<"line"> = useMemo(
    () => ({
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          labels: { color: "#e5e7eb", usePointStyle: true },
        },
        tooltip: { intersect: false, mode: "index" },
      },
      scales: {
        x: {
          grid: { color: "rgba(255,255,255,0.06)" },
          ticks: { color: "#9ca3af" },
        },
        y: {
          grid: { color: "rgba(255,255,255,0.06)" },
          ticks: { color: "#9ca3af" },
        },
      },
    }),
    []
  );

  const renderTree = (nodes: FileNode[], depth = 0) =>
    nodes.map((node) => {
      const isFile = node.type === "file";
      const isActive = isFile && node.path === activePath;
      const isCollapsed = collapsed.has(node.path);
      return (
        <div className="tree-item" key={node.path}>
          <div
            className={`tree-row ${isActive ? "active" : ""}`}
            style={{ paddingLeft: depth * 10 + 6 }}
            onClick={() => {
              if (node.type === "file") {
                setActivePath(node.path);
              } else if (node.children) {
                const next = findFirstFile(node.children);
                if (!isCollapsed && next) setActivePath(next.path);
                handleCollapseToggle(node.path);
              }
            }}
          >
            <div className="node-head">
              {node.type === "dir" ? (
                <button
                  className="chevron"
                  aria-label={isCollapsed ? "展开目录" : "收起目录"}
                  onClick={(e) => {
                    e.stopPropagation();
                    handleCollapseToggle(node.path);
                  }}
                >
                  {isCollapsed ? "▸" : "▾"}
                </button>
              ) : (
                <span className="chevron placeholder">•</span>
              )}
              <span className={`node-icon ${isFile ? "file" : "dir"}`} />
              <span className={`pill ${isFile ? "info" : "success"} mini-pill`}>{isFile ? "FILE" : "DIR"}</span>
            </div>
            <div className="node-body">
              <div className="node-title">{node.name}</div>
              <div className="node-sub">{node.path}</div>
              <div className="node-sub" style={{ opacity: 0.9 }}>
                {isFile ? `${node.size ?? "--"} · 更新 ${node.updated ?? "--"}` : `${node.children?.length ?? 0} 项 · 层级 ${depth + 1}`}
              </div>
            </div>
            <div className="node-actions">
              <span className={`pill ${node.autoUpload ? "success" : "warning"}`}>{node.autoUpload ? "自动" : "手动"}</span>
              <label className="switch mini" onClick={(e) => e.stopPropagation()}>
                <input
                  type="checkbox"
                  checked={node.autoUpload}
                  onChange={(e) => handleAutoToggle(node.path, e.target.checked)}
                />
                <span className="slider" />
              </label>
            </div>
          </div>
          {node.children && !isCollapsed ? <div className="tree-children">{renderTree(node.children, depth + 1)}</div> : null}
        </div>
      );
    });

  const manualUploadTime = manualUploadMap[activePath];
  const updatedSegment = manualUploadTime
    ? `手动上传 ${fmt(manualUploadTime)}`
    : activeNode?.updated
    ? `更新 ${fmt(activeNode.updated)}`
    : "更新时间未知";
  const silenceValue = useMemo(() => heroState.silence?.replace(/静默/gi, "").trim() ?? "", [heroState.silence]);

  return (
    <div className="page-shell">
      <div className="layout">
        <aside className="sidebar">
          <div className="nav-brand">
            <div className="brand-logo brand-logo-small">
              <div className="brand-logo-mark">GWF</div>
              <div className="brand-logo-sub">Go Watch File</div>
            </div>
          </div>
          <nav className="nav-list">
            {SECTION_IDS.map((id) => {
              const title =
                id === "overview"
                  ? "总览"
                  : id === "config"
                  ? "上传与路由配置"
                  : id === "directory"
                  ? "目录浏览"
                  : id === "files"
                  ? "文件列表"
                  : id === "tail"
                  ? "Tail / 关键字"
                  : id === "failures"
                  ? "上传记录"
                  : "监控";
              const desc =
                id === "overview"
                  ? "心跳 · 策略摘要"
                  : id === "config"
                  ? "本机配置 · 路由"
                  : id === "directory"
                  ? "单目录展开 / 收起"
                  : id === "files"
                  ? "基础信息"
                  : id === "tail"
                  ? "实时匹配"
                  : id === "failures"
                  ? "最近上传"
                  : "吞吐 / 队列";
              const badge =
                id === "overview"
                  ? "状态"
                  : id === "config"
                  ? "配置"
                  : id === "directory"
                  ? "目录树"
                  : id === "files"
                  ? "列表"
                  : id === "tail"
                  ? "日志"
                  : id === "failures"
                  ? "记录"
                  : "图表";
              return (
                <a key={id} className={`nav-item ${activeSection === id ? "active" : ""}`} href={`#${id}`}>
                  <div className="nav-label">
                    <span className={`nav-dot ${activeSection === id ? "live" : ""}`} />
                    <div>
                      <div className="nav-label-title">{title}</div>
                      <small>{desc}</small>
                    </div>
                  </div>
                  <span className="badge ghost">{badge}</span>
                </a>
              );
            })}
          </nav>
        </aside>

        <div className="page">
          <header className="page-header">
            <div className="brand">
              <div className="title">
                <p className="eyebrow">Agent 控制台</p>
                <h1>文件监控 Agent 控制台</h1>
                <div className="title-meta">
                  <span className="badge ghost">主机 {heroState.agent}</span>
                  <span className="badge ghost">监听目录 {heroState.watchDirs.length}</span>
                  <span className="badge ghost">{heroState.queue}</span>
                </div>
              </div>
            </div>
            <div className="controls">
              {loading ? <span className="badge">刷新中</span> : <span className="badge ghost">{timeframe === "realtime" ? "最新数据" : "最近 24h"}</span>}
              {error ? (
                <>
                  <span className="pill danger">接口异常</span>
                  <span className="badge ghost">{error}</span>
                </>
              ) : null}
              <div className={`chip ${timeframe === "realtime" ? "active" : ""}`} onClick={() => setTimeframe("realtime")}>
                实时
              </div>
              <div className={`chip ${timeframe === "24h" ? "active" : ""}`} onClick={() => setTimeframe("24h")}>
                最近 24h
              </div>
            </div>
          </header>

          <div id="overview" className="stack" style={{ gap: 12 }}>
            <section className="metric-strip">
              {metricCardsState.map((card) => (
                <div className="metric-tile" key={card.label}>
                  <small>{card.label}</small>
                  <div className="value">
                    {card.value}{" "}
                    <span className={`trend ${card.tone === "up" ? "up" : card.tone === "down" ? "down" : card.tone === "warning" ? "warning" : ""}`}>
                      {card.trend}
                    </span>
                  </div>
                </div>
              ))}
            </section>

            <div className="hero hero-plain">
              <div className="hero-left">
                <div className="hero-status">
                  <span className="pill success">运行中</span>
                  <span className="badge ghost">Agent {heroState.agent}</span>
                  <span className="badge ghost">监听 {heroState.watchDirs.length} 目录</span>
                </div>
                <div className="hero-desc">
                  针对当前主机的目录监听、上云路由与告警视图，核心状态收敛在下方卡片；目录树与文件列表用于日常巡检。
                </div>
              </div>
              <div className="hero-right">
                <div className="hero-right-grid">
                  <div className="stat-compact">
                    <small>监听目录</small>
                    <div className="hero-tags">
                      {heroState.watchDirs.map((dir) => (
                        <span className="hero-tag" key={dir} title={dir}>
                          {dir}
                        </span>
                      ))}
                    </div>
                  </div>
                  <div className="stat-compact">
                    <small>后缀过滤</small>
                    <strong>{heroState.suffixFilter}</strong>
                  </div>
                  <div className="stat-compact">
                    <small>静默窗口</small>
                    <strong>{silenceValue}</strong>
                  </div>
                  <div className="stat-compact">
                    <small>并发数量</small>
                    <strong>{heroState.concurrency}</strong>
                  </div>
                  <div className="stat-compact">
                    <small>队列数量</small>
                    <strong>{heroState.queue}</strong>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <section className="panel" id="config">
            <div className="section-title">
              <h2>上传与路由配置</h2>
            </div>
            <div className="inputs">
              <div className="input">
                <label>监控目录</label>
                <input
                  placeholder="填写要监听的目录，例如 /opt/test"
                  value={configForm.watchDir}
                  onChange={(e) => setConfigForm((prev) => ({ ...prev, watchDir: e.target.value }))}
                />
              </div>
              <div className="input">
                <label>文件后缀过滤</label>
                <input
                  placeholder="不填则默认显示监控目录下的所有文件和子目录"
                  value={configForm.fileExt}
                  onChange={(e) => setConfigForm((prev) => ({ ...prev, fileExt: e.target.value }))}
                />
              </div>
              <div className="input">
                <label>静默窗口</label>
                <input
                  placeholder="例如 10s / 30s"
                  value={configForm.silence}
                  onChange={(e) => setConfigForm((prev) => ({ ...prev, silence: e.target.value }))}
                />
              </div>
              <div className="input">
                <label>并发/队列</label>
                <input
                  placeholder="示例：workers=3 / queue=100（必填，数字）"
                  value={configForm.concurrency}
                  onChange={(e) => setConfigForm((prev) => ({ ...prev, concurrency: e.target.value }))}
                />
              </div>
            </div>
            <div className="toolbar config-actions">
              <div className="toolbar-actions">
                <button className="btn" type="button" onClick={() => void handleSaveSnapshot()} disabled={saving}>
                  {saving ? "保存中..." : "保存配置"}
                </button>
              </div>
            </div>
            {saveMessage ? <div className="badge">{saveMessage}</div> : null}
          </section>

          <section className="panel" id="directory">
            <div className="section-title">
              <h2>目录浏览 / 自动上传控制</h2>
              <span>单个目录可展开/收起 · 右侧展示基础信息</span>
            </div>
            <div className="workspace">
              <div className="tree-panel">
                <div className="toolbar">
                  <select
                    className="select"
                    value={currentRoot}
                    onChange={(e) => {
                      setCollapsed(new Set());
                      setCurrentRoot(e.target.value);
                    }}
                  >
                    {tree.map((node) => (
                      <option key={node.path} value={node.path}>
                        {node.path}
                      </option>
                    ))}
                  </select>
                  <div className="chip active">递归监听</div>
                  <div className="chip">仅新文件</div>
                  <span className="badge">自动刷新</span>
                  <button className="btn secondary" type="button" onClick={() => handleCollapseAll(true)}>
                    全部收起
                  </button>
                  <button className="btn secondary" type="button" onClick={() => handleCollapseAll(false)}>
                    全部展开
                  </button>
                </div>
                <div className="tree">{renderTree(rootNodes)}</div>
              </div>

              <div className="file-preview">
                <div className="preview-header">
                  <div>
                    <span className={`pill ${activeNode?.autoUpload ? "success" : "warning"}`} id="previewState">
                      {activeNode ? (activeNode.autoUpload ? "自动上传开启" : "自动上传关闭") : "选择一个文件"}
                    </span>
                    <div className="preview-title" id="previewTitle">
                      {activeNode?.name ?? "未选择文件"}
                    </div>
                    <div className="preview-path" id="previewPath">
                      {activeNode?.path ?? "—"}
                    </div>
                  </div>
                  <div className="switch-group">
                    <span className="muted">自动上传</span>
                    <label className="switch">
                      <input
                        type="checkbox"
                        checked={!!activeNode?.autoUpload}
                        onChange={(e) => activeNode && handleAutoToggle(activeNode.path, e.target.checked)}
                        disabled={!activeNode}
                      />
                      <span className="slider" />
                    </label>
                    <button className="btn secondary" type="button" onClick={() => void handleManualUpload()} disabled={!activeNode}>
                      立即上传
                    </button>
                  </div>
                </div>
                <div className="preview-meta" id="previewMeta">
                  {activeNode ? `${updatedSegment} · 路径信息收敛在此，适合做基础确认` : "目录树展示全部文件，可切换自动上传"}
                </div>
                <div className="preview-hint">用于展示当前选中文件的基础信息、状态与上传策略，避免占用列表区域。</div>
                <div className="info-grid">
                  <div className="info-item">
                    <span className="muted">大小</span>
                    <strong>{activeNode?.size ?? "--"}</strong>
                  </div>
                  <div className="info-item">
                    <span className="muted">模式</span>
                    <strong>{activeNode ? (activeNode.autoUpload ? "自动上传" : "手动上传") : "--"}</strong>
                  </div>
                  <div className="info-item">
                    <span className="muted">更新时间</span>
                    <strong>{activeNode ? updatedSegment : "--"}</strong>
                  </div>
                </div>
                <div className="preview-content" id="previewContent">
                  {activeNode ? activeNode.content ?? "文件内容预览" : "点击左侧目录树中的文件预览内容。"}
                </div>
              </div>
            </div>
          </section>

          <section className="panel" id="files">
            <div className="section-title">
              <h2>文件列表（基础信息）</h2>
              <span>轻量查看 · 下载 · 删除</span>
            </div>
            <div className="toolbar">
              <input className="search" placeholder="搜索文件名 / 路径" value={searchTerm} onChange={(e) => setSearchTerm(e.target.value)} />
              <div className={`chip ${fileFilter === "all" ? "active" : ""}`} onClick={() => setFileFilter("all")}>
                全部
              </div>
              <div className={`chip ${fileFilter === "auto" ? "active" : ""}`} onClick={() => setFileFilter("auto")}>
                自动上传
              </div>
              <div className={`chip ${fileFilter === "manual" ? "active" : ""}`} onClick={() => setFileFilter("manual")}>
                手动上传
              </div>
              <div className={`chip ${fileFilter === "failed" ? "active" : ""}`} onClick={() => setFileFilter("failed")}>
                失败
              </div>
              <span className="badge">后缀过滤已关闭 · 按目录控制</span>
              <button className="btn secondary" type="button">
                批量删除
              </button>
            </div>
            <table className="table">
              <thead>
                <tr>
                  <th>文件</th>
                  <th>大小</th>
                  <th>自动上传</th>
                  <th>状态</th>
                  <th>更新时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {filteredFiles.map((f) => (
                  <tr key={f.path}>
                    <td>
                      <div className="row-title">{f.name}</div>
                      <div className="row-sub">{f.path}</div>
                    </td>
                    <td>{f.size}</td>
                    <td>
                      <span className={`pill ${f.autoUpload ? "success" : "warning"}`}>{f.autoUpload ? "开启" : "关闭"}</span>
                    </td>
                    <td>
                      <span className="badge">
                        {f.status === "uploaded"
                          ? "已上传"
                          : f.status === "queued"
                            ? "队列中"
                            : f.status === "existing"
                              ? "已存在"
                              : "失败"}
                      </span>
                    </td>
                    <td>{fmt(f.time)}</td>
                    <td>
                      <div className="table-actions">
                        <button className="btn secondary" type="button">
                          查看
                        </button>
                        <button className="btn secondary" type="button">
                          下载
                        </button>
                        <button className="btn secondary" type="button">
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>

          <section className="panel" id="tail">
            <div className="section-title">
              <h2>日志快速处理 / 关键字匹配</h2>
              <span>针对已打开的日志做关键词匹配与截取</span>
            </div>
            <div className="toolbar">
              <div className="chip active">实时</div>
              <div className="chip">最近 200 行</div>
              <input
                className="search"
                placeholder="匹配关键字 / TraceId / 错误码"
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
              />
              <label className="switch mini">
                <input type="checkbox" checked={showMatchesOnly} onChange={(e) => setShowMatchesOnly(e.target.checked)} />
                <span className="slider" />
              </label>
              <span className="muted">只看匹配</span>
              <span className="badge ghost">当前：{activeNode?.name ?? "未选择"}</span>
              <button className="btn secondary" type="button" onClick={() => setKeyword("")}>
                清除
              </button>
            </div>
            <div className="tail-box">
              {tailDisplayLines.map((line, idx) => (
                <div className="tail-line" key={`${line}-${idx}`}>
                  {highlightTailLine(line)}
                </div>
              ))}
            </div>
            <div className="timeline">
              {timelineEventsState.map((ev) => (
                <div className="timeline-item" key={`${ev.label}-${ev.time}`}>
                  <span className={`pill ${ev.status}`}>{ev.label}</span>
                  <div className="timeline-text">{fmt(ev.time)}</div>
                  <div className="timeline-text right">{ev.host ?? "srv-01"}</div>
                </div>
              ))}
            </div>
          </section>

          <section className="panel" id="failures">
            <div className="section-title">
              <h2>上传记录 / 最近动作</h2>
              <span>成功 / 失败 / 排队</span>
            </div>
            <table className="table">
              <thead>
                <tr>
                  <th>文件</th>
                  <th>状态</th>
                  <th>耗时</th>
                  <th>目标</th>
                  <th>时间</th>
                  <th>备注</th>
                </tr>
              </thead>
              <tbody>
                {uploadRecordsState.map((item) => (
                  <tr key={`${item.file}-${item.time}`}>
                    <td>
                      <div className="row-title">{item.file}</div>
                      <div className="row-sub">{item.size}</div>
                    </td>
                    <td>
                      <span
                        className={`pill ${item.result === "success" ? "success" : item.result === "failed" ? "danger" : "warning"}`}
                      >
                        {item.result === "success" ? "成功" : item.result === "failed" ? "失败" : "等待"}
                      </span>
                    </td>
                    <td>{item.latency}</td>
                    <td>{item.target}</td>
                    <td>{fmt(item.time)}</td>
                    <td>
                      <div className="row-sub">{item.note ?? "--"}</div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>

          <section className="panel" id="monitor">
            <div className="section-title">
              <h2>运行与上云监控</h2>
              <span>吞吐 / 队列 / 时延</span>
            </div>
            <div className="monitor-grid">
              {monitorSummaryState.map((item) => (
                <div className="card compact" key={item.label}>
                  <div className="value">{item.value}</div>
                  <div className="meta">{item.label}</div>
                  <div className="muted small">{item.desc}</div>
                </div>
              ))}
            </div>
            <div className="flex-2 stretch">
              <div className="chart-wrapper">
                <Line data={chartData} options={chartOptions} />
              </div>
              <div className="stack">
                {monitorNotesState.map((note) => (
                  <div className="stack-item" key={note.title}>
                    <strong>{note.title}</strong>
                    <div className="meta">{note.detail}</div>
                  </div>
                ))}
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}

export default App;
