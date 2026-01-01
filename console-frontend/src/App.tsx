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
  uploadRecords,
} from "./mockData";
import type { ConfigSnapshot, DashboardPayload, FileFilter, FileItem, FileNode } from "./types";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Filler, Legend);

const SECTION_IDS = ["overview", "config", "directory", "files", "tail", "failures", "monitor"];
const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
const UPLOAD_PAGE_SIZE = 5;
const FILE_PAGE_SIZE = 10;
const LOG_POLL_MS = 2000;
const DASHBOARD_POLL_MS = 3000;

const hasDatePrefix = (value: string) => /\d{4}-\d{2}-\d{2}/.test(value);
const localDatePrefix = () => {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
};
const fmt = (t: string) => {
  if (!t || t === "--") return t || "--";
  if (hasDatePrefix(t)) return t;
  return `${localDatePrefix()} ${t}`;
};

const resolveRecordTimestamp = (value: string) => {
  if (!value || value === "--") return 0;
  const normalized = hasDatePrefix(value) ? value.replace(" ", "T") : `${localDatePrefix()}T${value}`;
  const parsed = Date.parse(normalized);
  return Number.isNaN(parsed) ? 0 : parsed;
};

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
  const [heroState, setHeroState] = useState(heroCopy);
  const [metricCardsState, setMetricCardsState] = useState(metricCards);
  const [monitorNotesState, setMonitorNotesState] = useState(monitorNotes);
  const [uploadRecordsState, setUploadRecordsState] = useState(uploadRecords);
  const [monitorSummaryState, setMonitorSummaryState] = useState(monitorSummary);
  const [chartPointsState, setChartPointsState] = useState(chartPoints);
  const [tailLinesState, setTailLinesState] = useState<string[]>([]);
  const [activeLogPath, setActiveLogPath] = useState<string | null>(null);
  const [followTail, setFollowTail] = useState(true);
  const [uploadPage, setUploadPage] = useState(1);
  const [filePage, setFilePage] = useState(1);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [timeframe, setTimeframe] = useState<"realtime" | "24h">("realtime");
  const [saveMessage, setSaveMessage] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const lastSavedConfig = useRef<ConfigSnapshot | null>(null);
  const actionTimerRef = useRef<number | undefined>(undefined);
  const tailBoxRef = useRef<HTMLDivElement | null>(null);
  const logFetchingRef = useRef(false);
  const dashboardFetchingRef = useRef(false);

  const rootNodes = useMemo(() => {
    const filtered = tree.filter((node) => !currentRoot || node.path === currentRoot);
    return filtered.length ? filtered : tree;
  }, [tree, currentRoot]);

  const activeNode = useMemo(() => {
    if (!activePath) return undefined;
    return findNode(rootNodes, activePath);
  }, [activePath, rootNodes]);

  const refreshDashboard = useCallback(async () => {
    if (dashboardFetchingRef.current) return;
    dashboardFetchingRef.current = true;
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
      setCurrentRoot((prev) => directoryTree[0]?.path ?? mergedHero.watchDirs[0] ?? prev);
      setActivePath((prev) => {
        const nextActive = findFirstFile(directoryTree)?.path ?? filesData[0]?.path ?? prev;
        return nextActive || prev;
      });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      dashboardFetchingRef.current = false;
      setLoading(false);
    }
  }, []);

  const refreshLiveData = useCallback(async () => {
    if (dashboardFetchingRef.current) return;
    dashboardFetchingRef.current = true;
    try {
      const res = await fetch(`${API_BASE}/api/dashboard`);
      if (!res.ok) {
        throw new Error(`加载数据失败，状态码 ${res.status}`);
      }
      const data = (await res.json()) as Partial<DashboardPayload>;
      const heroData = data.heroCopy ?? heroCopy;
      const metrics = data.metricCards ?? metricCards;
      const notes = data.monitorNotes ?? monitorNotes;
      const uploads = data.uploadRecords ?? [];
      const summary = data.monitorSummary ?? monitorSummary;
      const chartPointsData = data.chartPoints ?? [];

      const mergedHero = {
        ...heroData,
        silence: lastSavedConfig.current?.silence ?? heroData.silence,
        watchDirs: lastSavedConfig.current?.watchDir
          ? lastSavedConfig.current.watchDir.split(",").map((d) => d.trim()).filter(Boolean)
          : heroData.watchDirs,
      };

      setHeroState(mergedHero);
      setMetricCardsState(metrics);
      setMonitorNotesState(notes);
      setUploadRecordsState(uploads);
      setMonitorSummaryState(summary);
      setChartPointsState(chartPointsData);
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      dashboardFetchingRef.current = false;
    }
  }, []);

  useEffect(() => {
    void refreshDashboard();
  }, [refreshDashboard]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      void refreshLiveData();
    }, DASHBOARD_POLL_MS);
    return () => window.clearInterval(timer);
  }, [refreshLiveData]);

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

  useEffect(() => {
    return () => {
      if (actionTimerRef.current) {
        window.clearTimeout(actionTimerRef.current);
      }
    };
  }, []);

  const showActionMessage = useCallback((message: string) => {
    if (actionTimerRef.current) {
      window.clearTimeout(actionTimerRef.current);
    }
    setActionMessage(message);
    actionTimerRef.current = window.setTimeout(() => {
      setActionMessage(null);
    }, 3000);
  }, []);

  const fetchLogLines = useCallback(async (path: string) => {
    if (logFetchingRef.current) return;
    logFetchingRef.current = true;
    try {
      const res = await fetch(`${API_BASE}/api/file-log`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path }),
      });
      if (!res.ok) {
        const text = await res.text();
        let message = text;
        try {
          const payload = JSON.parse(text) as { error?: string };
          if (payload?.error) message = payload.error;
        } catch {}
        if (message.includes("仅支持文本文件")) {
          setTailLinesState(["当前文件为二进制，无法展示日志。"]);
          setActiveLogPath(null);
          setError(null);
          return;
        }
        throw new Error(message || `加载文件日志失败，状态码 ${res.status}`);
      }
      const data = (await res.json()) as { lines?: string[] };
      setTailLinesState(data.lines ?? []);
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      logFetchingRef.current = false;
    }
  }, []);

  const handleTailScroll = useCallback(() => {
    const el = tailBoxRef.current;
    if (!el) return;
    const threshold = 24;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight <= threshold;
    setFollowTail((prev) => (prev === atBottom ? prev : atBottom));
  }, []);

  useEffect(() => {
    if (!activeLogPath) return;
    void fetchLogLines(activeLogPath);
    const timer = window.setInterval(() => {
      void fetchLogLines(activeLogPath);
    }, LOG_POLL_MS);
    return () => window.clearInterval(timer);
  }, [activeLogPath, fetchLogLines]);

  useEffect(() => {
    if (!activeLogPath) return;
    setFollowTail(true);
  }, [activeLogPath]);

  useEffect(() => {
    if (!activeLogPath || !tailBoxRef.current || !followTail) return;
    const el = tailBoxRef.current;
    window.requestAnimationFrame(() => {
      el.scrollTop = el.scrollHeight;
    });
  }, [tailLinesState, activeLogPath, followTail]);

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

  const handleViewLog = (file: FileItem) => {
    if (!file.path) return;
    setError(null);
    setFollowTail(true);
    setActiveLogPath(file.path);
  };

  const handleDownloadFile = async (file: FileItem) => {
    if (!file.path) return;
    setError(null);
    try {
      const res = await fetch(`${API_BASE}/api/manual-upload`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: file.path }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || `触发上传失败，状态码 ${res.status}`);
      }
      showActionMessage("已触发下载，请稍后在上传记录查看状态。");
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
          default:
            return true;
        }
      });
  }, [files, currentRoot, fileFilter, searchTerm]);

  const sortedUploadRecords = useMemo(() => {
    return uploadRecordsState
      .map((record, index) => ({ record, index }))
      .sort((a, b) => {
        const timeDelta = resolveRecordTimestamp(b.record.time) - resolveRecordTimestamp(a.record.time);
        if (timeDelta !== 0) return timeDelta;
        return a.index - b.index;
      })
      .map(({ record }) => record);
  }, [uploadRecordsState]);

  const uploadPageCount = Math.max(1, Math.ceil(sortedUploadRecords.length / UPLOAD_PAGE_SIZE));
  const uploadPageSafe = Math.min(uploadPage, uploadPageCount);
  const uploadRecordsPage = useMemo(() => {
    const start = (uploadPageSafe - 1) * UPLOAD_PAGE_SIZE;
    return sortedUploadRecords.slice(start, start + UPLOAD_PAGE_SIZE);
  }, [uploadPageSafe, sortedUploadRecords]);

  useEffect(() => {
    if (uploadPage !== uploadPageSafe) {
      setUploadPage(uploadPageSafe);
    }
  }, [uploadPage, uploadPageSafe]);

  const filePageCount = Math.max(1, Math.ceil(filteredFiles.length / FILE_PAGE_SIZE));
  const filePageSafe = Math.min(filePage, filePageCount);
  const filteredFilesPage = useMemo(() => {
    const start = (filePageSafe - 1) * FILE_PAGE_SIZE;
    return filteredFiles.slice(start, start + FILE_PAGE_SIZE);
  }, [filteredFiles, filePageSafe]);

  useEffect(() => {
    if (filePage !== filePageSafe) {
      setFilePage(filePageSafe);
    }
  }, [filePage, filePageSafe]);

  const handleClearTail = () => {
    setTailLinesState([]);
    setActiveLogPath(null);
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
      const nextSuffix = fileExt ? `过滤 ${fileExt}` : "关闭 · 全量目录";
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
        suffixFilter: nextSuffix,
      }));
      setSaveMessage(`已保存当前表单（监控目录：${watchDir}，后缀：${fileExt || "全量"}），后端已应用`);
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
            </div>
            <div className="node-body">
              <div className="node-title" title={node.path}>
                {node.name}
              </div>
            </div>
            <div className="node-actions">
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
  const updatedTime = manualUploadTime
    ? fmt(manualUploadTime)
    : activeNode?.updated
    ? fmt(activeNode.updated)
    : "--";
  const silenceValue = useMemo(() => heroState.silence?.replace(/静默/gi, "").trim() ?? "", [heroState.silence]);
  const watchDirsLabel = heroState.watchDirs.length ? heroState.watchDirs.join(", ") : "--";

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
                  ? "文件日志"
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
                  ? "文件日志"
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
                </div>
              </div>
            </div>
            <div className="controls">
              {loading ? <span className="badge">刷新中</span> : null}
              {error ? (
                <>
                  <span className="pill danger">接口异常</span>
                  <span className="badge ghost">{error}</span>
                </>
              ) : null}
              <div className={`chip ${timeframe === "realtime" ? "active" : ""}`} onClick={() => setTimeframe("realtime")}>
                实时
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
                  <button className="btn secondary" type="button" onClick={() => handleCollapseAll(true)}>
                    全部收起
                  </button>
                  <button className="btn secondary" type="button" onClick={() => handleCollapseAll(false)}>
                    全部展开
                  </button>
                </div>
                <div className="tree-meta">
                  <span className="badge ghost">过滤: {heroState.suffixFilter}</span>
                  <span className="badge ghost">匹配文件: {files.length}</span>
                </div>
                {files.length === 0 ? <div className="empty-state">当前过滤下暂无匹配文件</div> : null}
                <div className="tree">{renderTree(rootNodes)}</div>
              </div>

              <div className="file-preview">
                <div className="preview-header">
                  <div className="preview-main">
                    <span className={`pill compact ${activeNode?.autoUpload ? "success" : "warning"}`} id="previewState">
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
                <div className="info-grid">
                  <div className="info-item">
                    <span className="muted">大小</span>
                    <strong>{activeNode?.size ?? "--"}</strong>
                  </div>
                  <div className="info-item">
                    <span className="muted">更新时间</span>
                    <strong>{updatedTime}</strong>
                  </div>
                  <div className="info-item">
                    <span className="muted">模式</span>
                    <strong>{activeNode ? (activeNode.autoUpload ? "自动上传" : "手动上传") : "--"}</strong>
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section className="panel" id="files">
            <div className="section-title">
              <h2>文件列表</h2>
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
              {actionMessage ? <span className="badge ghost">{actionMessage}</span> : null}
            </div>
            <table className="table files-table">
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
                {filteredFilesPage.length ? (
                  filteredFilesPage.map((f) => (
                    <tr key={f.path}>
                      <td>
                        <div className="row-title">{f.name}</div>
                        <div className="row-sub" title={f.path}>
                          {f.path}
                        </div>
                      </td>
                      <td>{f.size}</td>
                      <td>
                        <span className={`pill table-pill ${f.autoUpload ? "success" : "warning"}`}>{f.autoUpload ? "开启" : "关闭"}</span>
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
                          <button className="btn secondary" type="button" onClick={() => handleViewLog(f)}>
                            查看
                          </button>
                          <button className="btn secondary" type="button" onClick={() => void handleDownloadFile(f)}>
                            下载
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))
                ) : (
                  <tr>
                    <td className="table-empty" colSpan={6}>
                      暂无匹配文件
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
            <div className="pagination">
              <button
                className="btn secondary"
                type="button"
                disabled={filePageSafe <= 1}
                onClick={() => setFilePage((prev) => Math.max(1, prev - 1))}
              >
                上一页
              </button>
              <span className="badge ghost">
                第 {filePageSafe} / {filePageCount} 页
              </span>
              <button
                className="btn secondary"
                type="button"
                disabled={filePageSafe >= filePageCount}
                onClick={() => setFilePage((prev) => Math.min(filePageCount, prev + 1))}
              >
                下一页
              </button>
            </div>
          </section>

          <section className="panel" id="tail">
            <div className="section-title">
              <h2>文件日志</h2>
            </div>
            <div className="toolbar">
              <div className="chip active">实时</div>
              <button className="btn secondary" type="button" onClick={handleClearTail}>
                清除
              </button>
            </div>
            <div className="tail-box" ref={tailBoxRef} onScroll={handleTailScroll}>
              {tailLinesState.map((line, idx) => (
                <div className="tail-line" key={`${line}-${idx}`}>
                  {line}
                </div>
              ))}
            </div>
          </section>

          <section className="panel" id="failures">
            <div className="section-title">
              <h2>上传记录 / 最近动作</h2>
              <span>成功 / 失败 / 排队</span>
            </div>
            <table className="table upload-table">
              <thead>
                <tr>
                  <th>文件</th>
                  <th>状态</th>
                  <th>耗时</th>
                  <th>下载地址</th>
                  <th>时间</th>
                  <th>备注</th>
                </tr>
              </thead>
              <tbody key={uploadPageSafe}>
                {uploadRecordsPage.map((item, index) => (
                  <tr key={`${item.file}-${item.time}-${item.result}-${index}`}>
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
                    <td className="upload-target" title={item.target ?? ""}>{item.target || "--"}</td>
                    <td>{fmt(item.time)}</td>
                    <td className="upload-note">
                      <div className="row-sub">{item.note ?? "--"}</div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div className="pagination">
              <button
                className="btn secondary"
                type="button"
                disabled={uploadPageSafe <= 1}
                onClick={() => setUploadPage((prev) => Math.max(1, prev - 1))}
              >
                上一页
              </button>
              <span className="badge ghost">
                第 {uploadPageSafe} / {uploadPageCount} 页
              </span>
              <button
                className="btn secondary"
                type="button"
                disabled={uploadPageSafe >= uploadPageCount}
                onClick={() => setUploadPage((prev) => Math.min(uploadPageCount, prev + 1))}
              >
                下一页
              </button>
            </div>
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
