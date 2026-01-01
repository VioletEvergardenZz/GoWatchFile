export type FileNode = {
  name: string;
  path: string;
  type: "dir" | "file";
  autoUpload: boolean;
  size?: string;
  updated?: string;
  content?: string;
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

export type TimelineEvent = {
  label: string;
  time: string;
  status: "info" | "success" | "warning" | "danger";
  host?: string;
};

export type FailureItem = {
  file: string;
  reason: string;
  attempts: number;
  next: string;
};

export type MonitorNote = {
  title: string;
  detail: string;
};

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
  tailLines: string[];
  timelineEvents: TimelineEvent[];
  monitorNotes: MonitorNote[];
  uploadRecords: UploadRecord[];
  monitorSummary: MonitorSummary[];
  configSnapshot: ConfigSnapshot;
  chartPoints: ChartPoint[];
};
