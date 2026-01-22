import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { ChartOptions } from "chart.js";
import { CategoryScale, Chart as ChartJS, Filler, Legend, LineElement, LinearScale, PointElement, Tooltip } from "chart.js";
import { AlertConsole } from "./AlertConsole";
import { SystemConsole } from "./SystemConsole";
import { ConsoleHeader } from "./console/ConsoleHeader";
import { ConsoleSidebar } from "./console/ConsoleSidebar";
import {
  ConfigSection,
  DirectorySection,
  FilesSection,
  MonitorSection,
  OverviewSection,
  TailSection,
  UploadsSection,
} from "./console/ConsoleSections";
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
import {
  USE_MOCK,
  fetchDashboard,
  fetchDashboardLite,
  postAutoUpload,
  postConfig,
  postFileLog,
  postManualUpload,
  type FileLogResponse,
  type LogMode,
} from "./console/dashboardApi";
import {
  collectDirPaths,
  findFirstFile,
  findNode,
  formatTreeFilterBadge,
  fmt,
  matchUploadSearch,
  normalizeConfigSnapshot,
  normalizeWatchDirInput,
  resolveRecordTimestamp,
  splitWatchDirs,
  updateAutoUpload,
} from "./console/dashboardUtils";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Filler, Legend);

const SECTION_IDS = ["overview", "config", "directory", "files", "tail", "failures", "monitor"];
const SYSTEM_SECTION_IDS = ["system-overview", "system-resources", "system-volumes", "system-processes", "system-process-detail"];
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

type LogFetchOptions = {
  mode?: LogMode;
  query?: string;
};

const getPreferredTheme = (): "dark" | "light" => {
  if (typeof window === "undefined") return "dark";
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === "light" || stored === "dark") return stored;
  if (window.matchMedia?.("(prefers-color-scheme: light)").matches) return "light";
  return "dark";
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
  systemResourceEnabled: false,
};

const emptyMetricCards: MetricCard[] = [];
const emptyMonitorNotes: MonitorNote[] = [];
const emptyUploadRecords: UploadRecord[] = [];
const emptyMonitorSummary: MonitorSummary[] = [];
const emptyChartPoints: ChartPoint[] = [];
const emptyTree: FileNode[] = [];
const emptyFiles: FileItem[] = [];

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
  const [systemToggleSaving, setSystemToggleSaving] = useState(false);
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
      const data = (await fetchDashboard()) as Partial<DashboardPayload>;
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
      const data = (await fetchDashboardLite()) as Partial<DashboardPayload>;
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
      const data = (await postFileLog(payload)) as FileLogResponse;
      if (requestId !== logRequestIdRef.current) return;
      setTailLinesState(data.lines ?? []);
      setLogTruncated(isSearch ? Boolean(data.truncated) : false);
      setError(null);
    } catch (err) {
      const message = (err as Error).message;
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
      if (requestId !== logRequestIdRef.current) return;
      setError(message);
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
      await postAutoUpload(path, value);
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

  const handleRootChange = useCallback(
    (nextRoot: string) => {
      const nextNodes = tree.filter((node) => !nextRoot || node.path === nextRoot);
      setCollapsed(new Set(collectDirPaths(nextNodes)));
      setCurrentRoot(nextRoot);
    },
    [tree]
  );

  const handleConfigChange = useCallback((patch: Partial<ConfigSnapshot>) => {
    setConfigForm((prev) => ({ ...prev, ...patch }));
  }, []);

  const handleManualUpload = async () => {
    if (!activePath) return;
    const now = new Date().toTimeString().slice(0, 8);
    setManualUploadMap((prev) => ({ ...prev, [activePath]: now }));
    try {
      await postManualUpload(activePath);
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
      await postManualUpload(file.path);
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
      const data = await postConfig({
        watchDir,
        fileExt,
        uploadWorkers: workers,
        uploadQueueSize: queue,
        silence,
        systemResourceEnabled: configForm.systemResourceEnabled,
      });
      const payloadConfig = data.config;
      const nextSuffix = fileExt ? `过滤 ${fileExt}` : "关闭 · 全量目录";
      const nextConfig: ConfigSnapshot = {
        watchDir: payloadConfig?.watchDir ?? watchDir,
        fileExt: payloadConfig?.fileExt ?? fileExt,
        concurrency: payloadConfig?.concurrency ?? configForm.concurrency,
        silence: payloadConfig?.silence ?? silence ?? configForm.silence,
        systemResourceEnabled: payloadConfig?.systemResourceEnabled ?? configForm.systemResourceEnabled,
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

  const handleSystemResourceToggle = useCallback(async (next: boolean) => {
    setSystemToggleSaving(true);
    setError(null);
    try {
      const data = await postConfig({
        watchDir: "",
        fileExt: "",
        uploadWorkers: 0,
        uploadQueueSize: 0,
        silence: "",
        systemResourceEnabled: next,
      });
      const payloadConfig = data.config;
      const enabledValue = payloadConfig?.systemResourceEnabled ?? next;
      setConfigForm((prev) => ({ ...prev, systemResourceEnabled: enabledValue }));
      if (lastSavedConfig.current) {
        lastSavedConfig.current = { ...lastSavedConfig.current, systemResourceEnabled: enabledValue };
      }
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSystemToggleSaving(false);
    }
  }, []);

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
  const systemResourceEnabled = configForm.systemResourceEnabled;
  const handleGoConfig = useCallback(() => {
    onViewChange("console");
    if (typeof window === "undefined") return;
    window.setTimeout(() => {
      const target = document.getElementById("config");
      if (target) {
        target.scrollIntoView({ behavior: "smooth", block: "start" });
      }
      window.location.hash = "#config";
    }, 0);
  }, [onViewChange]);
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
        <ConsoleSidebar
          view={view}
          activeSection={activeSection}
          sectionIds={SECTION_IDS}
          systemSectionIds={SYSTEM_SECTION_IDS}
          onViewChange={onViewChange}
        />

        <div className={`page ${view === "alert" ? "page-alert" : "page-full"}`}>
          {view === "console" ? (
            <>
              <ConsoleHeader
                agent={heroState.agent}
                loading={loading}
                error={error}
                timeframe={timeframe}
                onTimeframeChange={setTimeframe}
                theme={theme}
                onThemeChange={setTheme}
              />
              <OverviewSection metricCards={metricCardsState} hero={heroState} silenceValue={silenceValue} />
              <ConfigSection
                configForm={configForm}
                saving={saving}
                saveMessage={saveMessage}
                onChange={handleConfigChange}
                onSave={() => void handleSaveSnapshot()}
              />
              <DirectorySection
                tree={tree}
                currentRoot={currentRoot}
                treeFilesCount={treeFiles.length}
                treeFilterBadge={treeFilterBadge}
                rootNodes={rootNodes}
                activeNode={activeNode}
                updatedTime={updatedTime}
                onRootChange={handleRootChange}
                onCollapseAll={handleCollapseAll}
                renderTree={renderTree}
                onAutoToggle={handleAutoToggle}
                onManualUpload={() => void handleManualUpload()}
              />
              <FilesSection
                searchTerm={searchTerm}
                fileFilter={fileFilter}
                actionMessage={actionMessage}
                filesPage={filteredFilesPage}
                page={filePageSafe}
                pageCount={filePageCount}
                onSearchChange={setSearchTerm}
                onFileFilterChange={setFileFilter}
                onPageChange={setFilePage}
                onViewLog={handleViewLog}
                onDownloadFile={(file) => void handleDownloadFile(file)}
                formatTime={fmt}
              />
              <TailSection
                logMode={logMode}
                logQuery={logQuery}
                logQueryApplied={logQueryApplied}
                logTruncated={logTruncated}
                tailLines={tailLinesState}
                canSearch={!!activeLogPath && !!logQuery.trim()}
                tailBoxRef={tailBoxRef}
                onSwitchTail={switchToTail}
                onRunSearch={runLogSearch}
                onLogQueryChange={setLogQuery}
                onClear={handleClearTail}
                onScroll={handleTailScroll}
                renderLogLine={renderLogLine}
              />
              <UploadsSection
                uploadSearchTerm={uploadSearchTerm}
                records={uploadRecordsPage}
                page={uploadPageSafe}
                pageCount={uploadPageCount}
                onUploadSearchChange={setUploadSearchTerm}
                onPageChange={setUploadPage}
                formatTime={fmt}
              />
              <MonitorSection
                summary={monitorSummaryState}
                notes={monitorNotesState}
                chartData={chartData}
                chartOptions={chartOptions}
              />
            </>
          ) : view === "alert" ? (
            <AlertConsole embedded />
          ) : (
            <SystemConsole embedded enabled={systemResourceEnabled} toggleLoading={systemToggleSaving || saving} onGoConfig={handleGoConfig} onToggleEnabled={handleSystemResourceToggle} />
          )}
        </div>
      </div>
    </div>
  );
}
