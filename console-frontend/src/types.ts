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
  systemResourceEnabled: boolean;
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

export type AiStatus = "idle" | "loading" | "success" | "error";

export type AiLogSummary = {
  summary: string;
  severity: "low" | "medium" | "high";
  keyErrors: string[];
  causes: string[];
  suggestions: string[];
  confidence?: number;
};

export type AiLogSummaryMeta = {
  usedLines: number;
  truncated: boolean;
  elapsedMs: number;
};

export type AiLogSummaryResponse = {
  ok: boolean;
  analysis?: AiLogSummary;
  meta?: AiLogSummaryMeta;
  error?: string;
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
  analysis?: string;
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

export type AlertRule = {
  id: string;
  title: string;
  level: AlertLevel;
  keywords: string[];
  excludes?: string[];
  suppress_window?: string;
  match_case?: boolean;
  notify?: boolean;
};

export type AlertRuleDefaults = {
  suppress_window?: string;
  match_case?: boolean;
};

export type AlertEscalationRule = {
  enabled?: boolean;
  level?: AlertLevel;
  window?: string;
  threshold?: number;
  suppress_window?: string;
  rule_id?: string;
  title?: string;
  message?: string;
};

export type AlertRuleset = {
  version?: number;
  defaults?: AlertRuleDefaults;
  escalation?: AlertEscalationRule;
  rules?: AlertRule[];
};

export type AlertRulesResponse = {
  ok: boolean;
  path?: string;
  rules?: AlertRuleset;
  error?: string;
};

export type AlertRulesSaveResponse = {
  ok: boolean;
  path?: string;
  rules?: AlertRuleset;
  error?: string;
};

export type SystemProcessStatus = "running" | "sleeping" | "stopped" | "zombie";

export type SystemResourceGauge = {
  id: "cpu" | "memory" | "disk";
  label: string;
  usedPct: number;
  usedLabel: string;
  totalLabel: string;
  subLabel: string;
  trend: string;
  tone?: "normal" | "warn" | "critical";
};

export type SystemVolume = {
  mount: string;
  usedPct: number;
  used: string;
  total: string;
};

export type SystemOverview = {
  host: string;
  os: string;
  kernel: string;
  uptime: string;
  load: string;
  ip: string;
  lastUpdated: string;
  processes: number;
  connections: number;
  connectionsBreakdown: string;
  cpuTemp: string;
  topProcess: string;
};

export type SystemProcess = {
  pid: number;
  name: string;
  command: string;
  user: string;
  status: SystemProcessStatus;
  cpu: number;
  mem: number;
  rss: string;
  threads: number;
  start: string;
  uptime: string;
  ports: string[];
  ioRead: string;
  ioWrite: string;
  netIn: string;
  netOut: string;
  cwd: string;
  path: string;
  env: string[];
  note?: string;
};

export type SystemDashboard = {
  systemOverview: SystemOverview;
  systemGauges: SystemResourceGauge[];
  systemVolumes: SystemVolume[];
  systemProcesses: SystemProcess[];
};
