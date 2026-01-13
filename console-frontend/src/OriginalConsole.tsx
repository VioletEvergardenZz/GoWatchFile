import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Line } from "react-chartjs-2";
import type { ChartOptions } from "chart.js";
import { CategoryScale, Chart as ChartJS, Filler, Legend, LineElement, LinearScale, PointElement, Tooltip } from "chart.js";
import { AlertConsole } from "./AlertConsole";
import { SystemConsole } from "./SystemConsole";
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
import type {
  ChartPoint,
  ConfigSnapshot,
  DashboardPayload,
  FileFilter,
  FileItem,
  FileNode,
  HeroCopy,
  MetricCard,
  MonitorNote,
  MonitorSummary,
  UploadRecord,
} from "./types";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Filler, Legend);

const SECTION_IDS = ["overview", "config", "directory", "files", "tail", "failures", "monitor"];
const SYSTEM_SECTION_IDS = ["system-overview", "system-resources", "system-volumes", "system-processes", "system-process-detail"];
const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
const USE_MOCK = ((import.meta.env.VITE_USE_MOCK as string | undefined) ?? "").toLowerCase() === "true";
const UPLOAD_PAGE_SIZE = 5;
const FILE_PAGE_SIZE = 10;
const LOG_POLL_MS = 2000;
const DASHBOARD_POLL_MS = 3000;
const THEME_STORAGE_KEY = "gwf-theme";
type ConsoleView = "console" | "alert" | "system";

type OriginalConsoleProps = {
  view: ConsoleView;
  onViewChange: (view: ConsoleView) => void;
};

type LogMode = "tail" | "search";

type LogFetchOptions = {
  mode?: LogMode;
  query?: string;
};

type FileLogResponse = {
  lines?: string[];
  mode?: LogMode;
  query?: string;
  matched?: number;
  truncated?: boolean;
};

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

const WATCH_DIR_CONFIG_SPLIT_RE = /[,\n\r;，；]+/;

const splitWatchDirs = (raw: string) =>
  raw
    .split(WATCH_DIR_CONFIG_SPLIT_RE)
    .map((dir) => dir.trim())
    .filter(Boolean);

const isWatchDirInputSeparator = (value: string) =>
  value === "," ||
  value === ";" ||
  value === "，" ||
  value === "；" ||
  value === "\n" ||
  value === "\r" ||
  value === "\t" ||
  value === " ";

const splitWatchDirsInput = (raw: string) => {
  const out: string[] = [];
  let buffer = "";
  let quote: "'" | '"' | null = null;
  let escaped = false;

  const push = () => {
    const trimmed = buffer.trim();
    if (trimmed) out.push(trimmed);
    buffer = "";
  };

  for (let i = 0; i < raw.length; i += 1) {
    const ch = raw[i];
    if (escaped) {
      buffer += ch;
      escaped = false;
      continue;
    }
    if (quote) {
      if (ch === "\\") {
        escaped = true;
        continue;
      }
      if (ch === quote) {
        quote = null;
        continue;
      }
      buffer += ch;
      continue;
    }
    if (ch === "'" || ch === "\"") {
      quote = ch;
      continue;
    }
    if (isWatchDirInputSeparator(ch)) {
      push();
      continue;
    }
    buffer += ch;
  }
  if (escaped) {
    buffer += "\\";
  }
  push();
  return out;
};

const normalizeWatchDirInput = (raw: string) => splitWatchDirsInput(raw).join(",");

const splitFileExtList = (raw: string) =>
  raw
    .split(/[,\s;]+/)
    .map((item) => item.trim())
    .filter(Boolean);

const resolveUploadStatusQuery = (raw: string) => {
  const cleaned = raw.trim().toLowerCase();
  if (!cleaned) return "";
  if (cleaned.includes("成功") || cleaned.includes("success") || cleaned.includes("uploaded") || cleaned.includes("ok")) {
    return "success";
  }
  if (cleaned.includes("失败") || cleaned.includes("failed") || cleaned.includes("error") || cleaned.includes("fail")) {
    return "failed";
  }
  if (cleaned.includes("排队") || cleaned.includes("队列") || cleaned.includes("等待") || cleaned.includes("pending") || cleaned.includes("queued")) {
    return "pending";
  }
  return "";
};

const matchUploadSearch = (record: UploadRecord, raw: string) => {
  const trimmed = raw.trim();
  if (!trimmed) return true;
  const status = resolveUploadStatusQuery(trimmed);
  if (status) {
    return record.result === status;
  }
  const haystack = `${record.file} ${record.target ?? ""} ${record.note ?? ""}`.toLowerCase();
  return haystack.includes(trimmed.toLowerCase());
};

const formatTreeFilterBadge = (raw: string) => {
  const trimmed = raw.trim();
  if (!trimmed) return "--";
  if (trimmed.includes("全量")) return "全量目录";
  const cleaned = trimmed.replace(/^过滤\s*/i, "");
  const exts = splitFileExtList(cleaned);
  if (!exts.length) return trimmed;
  return `过滤 ${exts.join(", ")}`;
};

// 补齐配置快照默认值避免空引用
const normalizeConfigSnapshot = (value?: Partial<ConfigSnapshot>): ConfigSnapshot => {
  const base = value ?? {};
  return {
    watchDir: base.watchDir ?? "",
    fileExt: base.fileExt ?? "",
    silence: base.silence ?? "",
    concurrency: base.concurrency ?? "",
  };
};

const getPreferredTheme = (): "dark" | "light" => {
  if (typeof window === "undefined") return "dark";
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === "light" || stored === "dark") return stored;
  if (window.matchMedia?.("(prefers-color-scheme: light)").matches) return "light";
  return "dark";
};

const resolveRecordTimestamp = (value: string) => {
  if (!value || value === "--") return 0;
  const normalized = hasDatePrefix(value) ? value.replace(" ", "T") : `${localDatePrefix()}T${value}`;
  const parsed = Date.parse(normalized);
  return Number.isNaN(parsed) ? 0 : parsed;
};

const emptyHero: HeroCopy = {
  agent: "--",
  watchDirs: [],
  suffixFilter: "--",
  silence: "--",
  queue: "--",
  concurrency: "--",
};

const emptyConfig: ConfigSnapshot = {
  watchDir: "",
  fileExt: "",
  silence: "",
  concurrency: "",
};

const emptyMetricCards: MetricCard[] = [];
const emptyMonitorNotes: MonitorNote[] = [];
const emptyUploadRecords: UploadRecord[] = [];
const emptyMonitorSummary: MonitorSummary[] = [];
const emptyChartPoints: ChartPoint[] = [];
const emptyTree: FileNode[] = [];
const emptyFiles: FileItem[] = [];

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

const collectDirPaths = (nodes: FileNode[]): string[] => {
  const paths: string[] = [];
  const walk = (items: FileNode[]) => {
    items.forEach((node) => {
      if (node.type !== "dir") return;
      paths.push(node.path);
      if (node.children) walk(node.children);
    });
  };
  walk(nodes);
  return paths;
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

export function OriginalConsole({ view, onViewChange }: OriginalConsoleProps) {
  const [tree, setTree] = useState<FileNode[]>(USE_MOCK ? treeSeed : emptyTree);
  const [files, setFiles] = useState<FileItem[]>(USE_MOCK ? fileSeed : emptyFiles);
  const [currentRoot, setCurrentRoot] = useState<string>(USE_MOCK ? treeSeed[0]?.path ?? "" : "");
  const [activePath, setActivePath] = useState<string>(USE_MOCK ? findFirstFile(treeSeed)?.path ?? "" : "");
  const [collapsed, setCollapsed] = useState<Set<string>>(() =>
    USE_MOCK ? new Set(collectDirPaths(treeSeed)) : new Set()
  );
  const [fileFilter, setFileFilter] = useState<FileFilter>("all");
  const [searchTerm, setSearchTerm] = useState("");
  const [manualUploadMap, setManualUploadMap] = useState<Record<string, string>>({});
  const [activeSection, setActiveSection] = useState<string>(SECTION_IDS[0]);
  const [configForm, setConfigForm] = useState(USE_MOCK ? configSnapshot : emptyConfig);
  const [heroState, setHeroState] = useState(USE_MOCK ? heroCopy : emptyHero);
  const [metricCardsState, setMetricCardsState] = useState(USE_MOCK ? metricCards : emptyMetricCards);
  const [monitorNotesState, setMonitorNotesState] = useState(USE_MOCK ? monitorNotes : emptyMonitorNotes);
  const [uploadRecordsState, setUploadRecordsState] = useState(USE_MOCK ? uploadRecords : emptyUploadRecords);
  const [monitorSummaryState, setMonitorSummaryState] = useState(USE_MOCK ? monitorSummary : emptyMonitorSummary);
  const [chartPointsState, setChartPointsState] = useState(USE_MOCK ? chartPoints : emptyChartPoints);
  const [tailLinesState, setTailLinesState] = useState<string[]>([]);
  const [activeLogPath, setActiveLogPath] = useState<string | null>(null);
  const [logMode, setLogMode] = useState<LogMode>("tail");
  const [logQuery, setLogQuery] = useState("");
  const [logQueryApplied, setLogQueryApplied] = useState("");
  const [logTruncated, setLogTruncated] = useState(false);
  const [followTail, setFollowTail] = useState(true);
  const [uploadPage, setUploadPage] = useState(1);
  const [filePage, setFilePage] = useState(1);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [timeframe, setTimeframe] = useState<"realtime" | "24h">("realtime");
  const [theme, setTheme] = useState<"dark" | "light">(() => getPreferredTheme());
  const [saveMessage, setSaveMessage] = useState<string | null>(null);
  const [uploadSearchTerm, setUploadSearchTerm] = useState("");
  const [loading, setLoading] = useState(!USE_MOCK);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [bootstrapped, setBootstrapped] = useState(USE_MOCK);
  const lastSavedConfig = useRef<ConfigSnapshot | null>(null);
  const actionTimerRef = useRef<number | undefined>(undefined);
  const tailBoxRef = useRef<HTMLDivElement | null>(null);
  const logRequestIdRef = useRef(0);
  const collapseInitRef = useRef(USE_MOCK);
  const lastSuffixFilterRef = useRef<string | null>(USE_MOCK ? heroCopy.suffixFilter : null);
  const lastRootRef = useRef<string | null>(USE_MOCK ? (treeSeed[0]?.path ?? "") : null);
  const dirPathsRef = useRef<Set<string>>(new Set());
  const dashboardFetchingRef = useRef(false);
  const visibleSectionsRef = useRef<Map<string, IntersectionObserverEntry>>(new Map());

  const rootNodes = useMemo(() => {
    const filtered = tree.filter((node) => !currentRoot || node.path === currentRoot);
    return filtered.length ? filtered : tree;
  }, [tree, currentRoot]);

  const dirPaths = useMemo(() => collectDirPaths(rootNodes), [rootNodes]);

  const activeNode = useMemo(() => {
    if (!activePath) return undefined;
    return findNode(rootNodes, activePath);
  }, [activePath, rootNodes]);

  useEffect(() => {
    if (collapseInitRef.current) return;
    if (!dirPaths.length) return;
    setCollapsed(new Set(dirPaths));
    dirPathsRef.current = new Set(dirPaths);
    collapseInitRef.current = true;
  }, [dirPaths]);

  useEffect(() => {
    if (!dirPaths.length) return;
    const current = heroState.suffixFilter ?? "";
    const last = lastSuffixFilterRef.current;
    if (last === null) {
      lastSuffixFilterRef.current = current;
      return;
    }
    if (last === current) return;
    setCollapsed(new Set(dirPaths));
    dirPathsRef.current = new Set(dirPaths);
    lastSuffixFilterRef.current = current;
  }, [heroState.suffixFilter, dirPaths]);

  useEffect(() => {
    if (!dirPaths.length) return;
    const rootKey = currentRoot || rootNodes[0]?.path || "";
    if (!rootKey) return;
    const last = lastRootRef.current;
    if (last === null) {
      lastRootRef.current = rootKey;
      return;
    }
    if (last === rootKey) return;
    setCollapsed(new Set(dirPaths));
    dirPathsRef.current = new Set(dirPaths);
    lastRootRef.current = rootKey;
  }, [currentRoot, rootNodes, dirPaths]);

  useEffect(() => {
    if (!dirPaths.length) return;
    const currentSet = new Set(dirPaths);
    const prevSet = dirPathsRef.current;
    if (prevSet.size === 0) {
      dirPathsRef.current = currentSet;
      return;
    }
    setCollapsed((prev) => {
      let changed = false;
      const next = new Set<string>();
      for (const path of prev) {
        if (currentSet.has(path)) {
          next.add(path);
        } else {
          changed = true;
        }
      }
      for (const path of currentSet) {
        if (!prevSet.has(path)) {
          next.add(path);
          changed = true;
        }
      }
      return changed ? next : prev;
    });
    dirPathsRef.current = currentSet;
  }, [dirPaths]);

  const refreshDashboard = useCallback(async () => {
    if (USE_MOCK) {
      setBootstrapped(true);
      return;
    }
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
      const heroData = data.heroCopy ?? emptyHero;
      const metrics = data.metricCards ?? emptyMetricCards;
      const notes = data.monitorNotes ?? emptyMonitorNotes;
      const uploads = data.uploadRecords ?? [];
      const summary = data.monitorSummary ?? emptyMonitorSummary;
      const chartPointsData = data.chartPoints ?? emptyChartPoints;

      let configData = normalizeConfigSnapshot(data.configSnapshot as ConfigSnapshot | undefined);
      if (lastSavedConfig.current) {
        configData = normalizeConfigSnapshot({ ...configData, ...lastSavedConfig.current });
      }

      const mergedHero = {
        ...heroData,
        silence: lastSavedConfig.current?.silence ?? heroData.silence,
        watchDirs: lastSavedConfig.current?.watchDir
          ? splitWatchDirs(lastSavedConfig.current.watchDir)
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
      setBootstrapped(true);
    }
  }, []);

  const refreshLiveData = useCallback(async () => {
    if (USE_MOCK) return;
    if (dashboardFetchingRef.current) return;
    dashboardFetchingRef.current = true;
    try {
      const res = await fetch(`${API_BASE}/api/dashboard?mode=light`);
      if (!res.ok) {
        throw new Error(`加载数据失败，状态码 ${res.status}`);
      }
      const data = (await res.json()) as Partial<DashboardPayload>;
      const heroData = data.heroCopy ?? emptyHero;
      const metrics = data.metricCards ?? emptyMetricCards;
      const notes = data.monitorNotes ?? emptyMonitorNotes;
      const uploads = data.uploadRecords ?? [];
      const summary = data.monitorSummary ?? emptyMonitorSummary;
      const chartPointsData = data.chartPoints ?? emptyChartPoints;

      const mergedHero = {
        ...heroData,
        silence: lastSavedConfig.current?.silence ?? heroData.silence,
        watchDirs: lastSavedConfig.current?.watchDir
          ? splitWatchDirs(lastSavedConfig.current.watchDir)
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
    if (USE_MOCK) return;
    void refreshDashboard();
  }, [refreshDashboard]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    document.body.dataset.theme = theme;
    document.body.style.colorScheme = theme;
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  }, [theme]);

  useEffect(() => {
    if (USE_MOCK) return;
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
    if (view === "alert") return;
    if (view === "console" && !bootstrapped) return;
    const sectionIds = view === "system" ? SYSTEM_SECTION_IDS : SECTION_IDS;
    visibleSectionsRef.current.clear();
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          const id = entry.target.id;
          if (!id) return;
          if (entry.isIntersecting) {
            visibleSectionsRef.current.set(id, entry);
          } else {
            visibleSectionsRef.current.delete(id);
          }
        });
        const visibleEntries = Array.from(visibleSectionsRef.current.values());
        if (!visibleEntries.length) return;
        const sorted = visibleEntries.sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        const firstBelow = sorted.find((entry) => entry.boundingClientRect.top >= 0);
        const next = (firstBelow ?? sorted[sorted.length - 1]).target.id;
        if (next) setActiveSection(next);
      },
      { threshold: [0, 0.25, 0.5], rootMargin: "-30% 0px -30% 0px" }
    );

    const targets = sectionIds.map((id) => document.getElementById(id)).filter(Boolean) as Element[];
    targets.forEach((el) => observer.observe(el));
    return () => {
      observer.disconnect();
      visibleSectionsRef.current.clear();
    };
  }, [bootstrapped, view]);

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

  const fetchLogLines = useCallback(async (path: string, options: LogFetchOptions = {}) => {
    const requestId = ++logRequestIdRef.current;
    const trimmedQuery = options.query?.trim() ?? "";
    const isSearch = options.mode === "search" || trimmedQuery !== "";
    try {
      const payload: { path: string; query?: string } = { path };
      if (trimmedQuery) {
        payload.query = trimmedQuery;
      }
      const res = await fetch(`${API_BASE}/api/file-log`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const text = await res.text();
        let message = text;
        try {
          const payload = JSON.parse(text) as { error?: string };
          if (payload?.error) message = payload.error;
        } catch {}
        if (message.includes("仅支持文本文件")) {
          if (requestId !== logRequestIdRef.current) return;
          setTailLinesState(["当前文件为二进制，无法展示内容。"]);
          setActiveLogPath(null);
          setLogTruncated(false);
          setError(null);
          return;
        }
        if (message.includes("文件路径不在监控目录下")) {
          if (requestId !== logRequestIdRef.current) return;
          setTailLinesState(["当前文件已不在监控目录，已停止内容拉取。"]);
          setActiveLogPath(null);
          setLogTruncated(false);
          setError(null);
          return;
        }
        throw new Error(message || `加载文件内容失败，状态码 ${res.status}`);
      }
      const data = (await res.json()) as FileLogResponse;
      if (requestId !== logRequestIdRef.current) return;
      setTailLinesState(data.lines ?? []);
      setLogTruncated(isSearch ? Boolean(data.truncated) : false);
      setError(null);
    } catch (err) {
      if (requestId !== logRequestIdRef.current) return;
      setError((err as Error).message);
    }
  }, []);

  const handleTailScroll = useCallback(() => {
    if (logMode !== "tail") return;
    const el = tailBoxRef.current;
    if (!el) return;
    const threshold = 24;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight <= threshold;
    setFollowTail((prev) => (prev === atBottom ? prev : atBottom));
  }, [logMode]);

  useEffect(() => {
    if (!activeLogPath || logMode !== "tail") return;
    void fetchLogLines(activeLogPath, { mode: "tail" });
    const timer = window.setInterval(() => {
      void fetchLogLines(activeLogPath, { mode: "tail" });
    }, LOG_POLL_MS);
    return () => window.clearInterval(timer);
  }, [activeLogPath, fetchLogLines, logMode]);

  useEffect(() => {
    if (!activeLogPath) return;
    setFollowTail(true);
  }, [activeLogPath]);

  useEffect(() => {
    if (!activeLogPath || logMode !== "tail" || !tailBoxRef.current || !followTail) return;
    const el = tailBoxRef.current;
    window.requestAnimationFrame(() => {
      el.scrollTop = el.scrollHeight;
    });
  }, [tailLinesState, activeLogPath, followTail, logMode]);

  const switchToTail = useCallback(() => {
    setLogMode("tail");
    setLogQueryApplied("");
    setLogTruncated(false);
    setFollowTail(true);
  }, []);

  const runLogSearch = useCallback(() => {
    if (!activeLogPath) return;
    const trimmed = logQuery.trim();
    if (!trimmed) return;
    setLogMode("search");
    setLogQueryApplied(trimmed);
    setLogTruncated(false);
    setFollowTail(false);
    void fetchLogLines(activeLogPath, { mode: "search", query: trimmed });
  }, [activeLogPath, fetchLogLines, logQuery]);

  const renderLogLine = useCallback(
    (line: string) => {
      if (logMode !== "search" || !logQueryApplied) return line;
      const lowerLine = line.toLowerCase();
      const lowerQuery = logQueryApplied.toLowerCase();
      if (!lowerQuery || !lowerLine.includes(lowerQuery)) return line;
      const parts: ReactNode[] = [];
      let cursor = 0;
      let index = lowerLine.indexOf(lowerQuery, cursor);
      while (index !== -1) {
        if (index > cursor) {
          parts.push(line.slice(cursor, index));
        }
        const match = line.slice(index, index + lowerQuery.length);
        parts.push(
          <span className="highlight" key={`${index}-${cursor}`}>
            {match}
          </span>
        );
        cursor = index + lowerQuery.length;
        index = lowerLine.indexOf(lowerQuery, cursor);
      }
      if (cursor < line.length) {
        parts.push(line.slice(cursor));
      }
      return parts;
    },
    [logMode, logQueryApplied]
  );

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
    setCollapsed(new Set(collectDirPaths(rootNodes)));
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
    setLogMode("tail");
    setLogQueryApplied("");
    setLogTruncated(false);
    setFollowTail(true);
    logRequestIdRef.current += 1;
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

  const treeFiles = useMemo(() => {
    return currentRoot ? files.filter((f) => f.path.startsWith(currentRoot)) : files;
  }, [files, currentRoot]);

  const treeFilterBadge = useMemo(() => formatTreeFilterBadge(heroState.suffixFilter ?? ""), [heroState.suffixFilter]);

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

  const filteredUploadRecords = useMemo(() => {
    return sortedUploadRecords.filter((record) => matchUploadSearch(record, uploadSearchTerm));
  }, [sortedUploadRecords, uploadSearchTerm]);

  const uploadPageCount = Math.max(1, Math.ceil(filteredUploadRecords.length / UPLOAD_PAGE_SIZE));
  const uploadPageSafe = Math.min(uploadPage, uploadPageCount);
  const uploadRecordsPage = useMemo(() => {
    const start = (uploadPageSafe - 1) * UPLOAD_PAGE_SIZE;
    return filteredUploadRecords.slice(start, start + UPLOAD_PAGE_SIZE);
  }, [uploadPageSafe, filteredUploadRecords]);

  useEffect(() => {
    if (uploadPage !== uploadPageSafe) {
      setUploadPage(uploadPageSafe);
    }
  }, [uploadPage, uploadPageSafe]);

  useEffect(() => {
    setUploadPage(1);
  }, [uploadSearchTerm]);

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
    logRequestIdRef.current += 1;
    setTailLinesState([]);
    setActiveLogPath(null);
    setLogMode("tail");
    setLogQuery("");
    setLogQueryApplied("");
    setLogTruncated(false);
    setFollowTail(true);
  };

  const handleSaveSnapshot = async () => {
    setSaveMessage(null);
    setError(null);
    const watchDir = normalizeWatchDirInput(configForm.watchDir ?? "");
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
      const nextWatchDirs = splitWatchDirs(nextConfig.watchDir);
      const nextRoot = nextWatchDirs.includes(currentRoot) ? currentRoot : nextWatchDirs[0] ?? "";
      lastSavedConfig.current = nextConfig;
      setConfigForm(nextConfig);
      setCurrentRoot(nextRoot);
      setActiveLogPath(null);
      setTailLinesState([]);
      setError(null);
      setHeroState((prev) => ({
        ...prev,
        watchDirs: nextWatchDirs.length ? nextWatchDirs : prev.watchDirs,
        silence: nextConfig.silence ?? prev.silence,
        suffixFilter: nextSuffix,
      }));
      const watchDirLabel = payloadConfig?.watchDir ?? watchDir;
      setSaveMessage(`已保存当前表单（监控目录：${watchDirLabel}，后缀：${fileExt || "全量"}），后端已应用`);
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

  const chartPalette = useMemo(
    () =>
      theme === "light"
        ? {
            legend: "#1f2937",
            ticks: "#64748b",
            grid: "rgba(15, 23, 42, 0.08)",
          }
        : {
            legend: "#e5e7eb",
            ticks: "#9ca3af",
            grid: "rgba(255,255,255,0.06)",
          },
    [theme]
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
      animation: false,
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          labels: { color: chartPalette.legend, usePointStyle: true },
        },
        tooltip: { intersect: false, mode: "index" },
      },
      scales: {
        x: {
          grid: { color: chartPalette.grid },
          ticks: { color: chartPalette.ticks },
        },
        y: {
          grid: { color: chartPalette.grid },
          ticks: { color: chartPalette.ticks },
        },
      },
    }),
    [chartPalette]
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
  const booting = view === "console" && !bootstrapped;

  if (booting) {
    return (
      <div className="page-shell booting">
        <div className="boot-overlay">
          <div className="boot-card">
            <div className="boot-title">正在加载实时数据</div>
            <div className="boot-sub">首次同步目录与指标可能需要几秒</div>
            <div className="boot-spinner" />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="page-shell">
      <div className={`layout ${view === "alert" ? "layout-alert" : "layout-full"}`}>
        <aside className="sidebar">
          <div className="nav-brand">
            <div className="brand-logo brand-logo-small">
              <div className="brand-logo-mark">GWF</div>
              <div className="brand-logo-sub">Go Watch File</div>
            </div>
          </div>
          <div className="view-switch" role="tablist" aria-label="控制台切换">
            <button
              className={`view-tab ${view === "console" ? "active" : ""}`}
              type="button"
              role="tab"
              aria-selected={view === "console"}
              onClick={() => onViewChange("console")}
            >
              文件监控控制台
            </button>
            <button
              className={`view-tab ${view === "alert" ? "active" : ""}`}
              type="button"
              role="tab"
              aria-selected={view === "alert"}
              onClick={() => onViewChange("alert")}
            >
              告警控制台
            </button>
            <button
              className={`view-tab ${view === "system" ? "active" : ""}`}
              type="button"
              role="tab"
              aria-selected={view === "system"}
              onClick={() => onViewChange("system")}
            >
              系统资源管理器
            </button>
          </div>
          {view === "console" ? (
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
                    ? "文件内容"
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
                    ? "文件内容"
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
                    ? "内容"
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
          ) : view === "system" ? (
            <nav className="nav-list">
              {SYSTEM_SECTION_IDS.map((id) => {
                const title =
                  id === "system-overview"
                    ? "系统概览"
                    : id === "system-resources"
                    ? "资源总览"
                    : id === "system-volumes"
                    ? "磁盘分区"
                    : id === "system-processes"
                    ? "进程列表"
                    : "进程详情";
                const desc =
                  id === "system-overview"
                    ? "主机 / 负载 / 连接"
                    : id === "system-resources"
                    ? "CPU / 内存 / 磁盘"
                    : id === "system-volumes"
                    ? "容量 / 使用率"
                    : id === "system-processes"
                    ? "筛选 / 排序"
                    : "指标 / 处置";
                const badge =
                  id === "system-overview"
                    ? "概览"
                    : id === "system-resources"
                    ? "资源"
                    : id === "system-volumes"
                    ? "分区"
                    : id === "system-processes"
                    ? "进程"
                    : "详情";
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
          ) : null}
        </aside>

        <div className={`page ${view === "alert" ? "page-alert" : "page-full"}`}>
          {view === "console" ? (
            <>
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
              <div className="theme-toggle">
                <span className="muted small">背景</span>
                <label className="switch mini">
                  <input
                    type="checkbox"
                    aria-label="切换深色/浅色背景"
                    checked={theme === "light"}
                    onChange={(e) => setTheme(e.target.checked ? "light" : "dark")}
                  />
                  <span className="slider" />
                </label>
                <span className="badge ghost">{theme === "light" ? "浅色" : "深色"}</span>
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
                  placeholder={
                    'Enter directories separated by space/comma/semicolon; quote paths with spaces (e.g. "/data/my logs" /data/other)'
                  }
                  value={configForm.watchDir}
                  onChange={(e) => setConfigForm((prev) => ({ ...prev, watchDir: e.target.value }))}
                />
              </div>
              <div className="input">
                <label>文件后缀过滤</label>
                {/* 支持多个后缀输入 */}
                <input
                  placeholder="支持多个后缀（逗号分隔），不填则默认显示监控目录下的所有文件和子目录"
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
                      const nextRoot = e.target.value;
                      const nextNodes = tree.filter((node) => !nextRoot || node.path === nextRoot);
                      setCollapsed(new Set(collectDirPaths(nextNodes)));
                      setCurrentRoot(nextRoot);
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
                  <span className="badge ghost">{treeFilterBadge}</span>
                  <span className="badge ghost">匹配文件: {treeFiles.length}</span>
                </div>
                {treeFiles.length === 0 ? <div className="empty-state">当前过滤下暂无匹配文件</div> : null}
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
              <h2>文件内容</h2>
            </div>
            <div className="toolbar space-between">
              <div className="toolbar-actions">
                <div className={`chip ${logMode === "tail" ? "active" : ""}`} onClick={switchToTail}>
                  实时
                </div>
                <div className={`chip ${logMode === "search" ? "active" : ""}`} onClick={runLogSearch}>
                  全文检索
                </div>
                {logMode === "search" && logQueryApplied ? (
                  <span className="badge ghost">
                    关键词 {logQueryApplied} · 匹配 {tailLinesState.length} 行{logTruncated ? " · 已截断" : ""}
                  </span>
                ) : null}
              </div>
              <div className="toolbar-actions">
                <input
                  className="search log-search"
                  placeholder="关键词/全文检索"
                  value={logQuery}
                  onChange={(e) => setLogQuery(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") runLogSearch();
                  }}
                />
                <button className="btn secondary" type="button" onClick={runLogSearch} disabled={!activeLogPath || !logQuery.trim()}>
                  检索
                </button>
                <button className="btn secondary" type="button" onClick={handleClearTail}>
                  清除
                </button>
              </div>
            </div>
            <div className="tail-box" ref={tailBoxRef} onScroll={handleTailScroll}>
              {logMode === "search" && logQueryApplied && tailLinesState.length === 0 ? (
                <div className="tail-line">未找到匹配内容</div>
              ) : (
                tailLinesState.map((line, idx) => (
                  <div className="tail-line" key={`${line}-${idx}`}>
                    {renderLogLine(line)}
                  </div>
                ))
              )}
            </div>
          </section>

          <section className="panel" id="failures">
            <div className="section-title">
              <h2>上传记录 / 最近动作</h2>
              <input
                className="search"
                placeholder="搜索状态：成功 / 失败 / 排队"
                value={uploadSearchTerm}
                onChange={(e) => setUploadSearchTerm(e.target.value)}
              />
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
              <tbody key={`${uploadPageSafe}-${uploadSearchTerm}`}>
                {uploadRecordsPage.length ? (
                  uploadRecordsPage.map((item, index) => (
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
                  ))
                ) : (
                  <tr>
                    <td className="table-empty" colSpan={6}>
                      暂无匹配记录
                    </td>
                  </tr>
                )}
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
          </>
          ) : view === "alert" ? (
            <AlertConsole embedded />
          ) : (
            <SystemConsole embedded />
          )}
        </div>
      </div>
    </div>
  );
}
