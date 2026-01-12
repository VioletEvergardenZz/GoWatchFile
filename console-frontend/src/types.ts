export type FileNode = {
  name: string;
  path: string;
  type: "dir" | "file";
  autoUpload: boolean;
  size?: string;
  updated?: string;
  children?: FileNode[];
};

export type MetricCard = {
  label: string;
  value: string;
  trend: string;
  tone?: "up" | "down" | "warning" | "muted";
};

export type FileItem = {
  name: string;
  path: string;
  size: string;
  status: "uploaded" | "queued" | "failed" | "existing";
  time: string;
  autoUpload: boolean;
};

export type MonitorNote = {
  title: string;
  detail: string;
};

// 配置快照包含过滤信息
export type ConfigSnapshot = {
  watchDir: string;
  fileExt: string;
  silence: string;
  concurrency: string;
};

export type HeroCopy = {
  agent: string;
  watchDirs: string[];
  suffixFilter: string;
  silence: string;
  queue: string;
  concurrency: string;
};

export type ChartPoint = {
  label: string;
  uploads: number;
  failures: number;
  queue: number;
};

export type FileFilter = "all" | "auto" | "manual";

export type UploadRecord = {
  file: string;
  target: string;
  size: string;
  result: "success" | "failed" | "pending";
  latency: string;
  time: string;
  note?: string;
};

export type MonitorSummary = {
  label: string;
  value: string;
  desc: string;
};

export type DashboardPayload = {
  heroCopy: HeroCopy;
  metricCards: MetricCard[];
  directoryTree: FileNode[];
  files: FileItem[];
  monitorNotes: MonitorNote[];
  uploadRecords: UploadRecord[];
  monitorSummary: MonitorSummary[];
  configSnapshot: ConfigSnapshot;
  chartPoints: ChartPoint[];
};

export type AlertLevel = "ignore" | "business" | "system" | "fatal";

export type AlertDecisionStatus = "sent" | "suppressed" | "recorded";

export type AlertOverview = {
  window: string;
  risk: string;
  fatal: number;
  system: number;
  business: number;
  sent: number;
  suppressed: number;
  latest: string;
};

export type AlertDecision = {
  id: string;
  time: string;
  level: AlertLevel;
  rule: string;
  message: string;
  file: string;
  status: AlertDecisionStatus;
  reason?: string;
};

export type AlertStats = {
  sent: number;
  suppressed: number;
  recorded: number;
};

export type RuleLevelCount = {
  ignore: number;
  business: number;
  system: number;
  fatal: number;
};

export type RulesSummary = {
  source: string;
  lastLoaded: string;
  total: number;
  defaultSuppress: string;
  escalation: string;
  levels: RuleLevelCount;
  error?: string;
};

export type PollSummary = {
  interval: string;
  logFiles: string[];
  lastPoll: string;
  nextPoll: string;
  error?: string;
};

export type AlertDashboard = {
  overview: AlertOverview;
  decisions: AlertDecision[];
  stats: AlertStats;
  rules: RulesSummary;
  polling: PollSummary;
};

export type AlertResponse = {
  ok: boolean;
  enabled?: boolean;
  data?: AlertDashboard;
  error?: string;
};

export type AlertConfigSnapshot = {
  enabled: boolean;
  suppressEnabled: boolean;
  rulesFile: string;
  logPaths: string;
  pollInterval: string;
  startFromEnd: boolean;
};

export type AlertConfigResponse = {
  ok: boolean;
  config?: AlertConfigSnapshot;
  error?: string;
};
