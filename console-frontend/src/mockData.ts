import type {
  AlertConfigSnapshot,
  AlertDashboard,
  ChartPoint,
  ConfigSnapshot,
  FileItem,
  FileNode,
  HeroCopy,
  MetricCard,
  MonitorNote,
  MonitorSummary,
  UploadRecord,
} from "./types";

export const heroCopy: HeroCopy = {
  agent: "srv-01",
  watchDirs: ["/data/logs/app", "/data/etl/raw"],
  suffixFilter: "关闭 · 全量目录",
  silence: "4s",
  queue: "队列 200",
  concurrency: "上传并发 8",
};

export const metricCards: MetricCard[] = [
  { label: "运行状态", value: "Running", trend: "心跳正常", tone: "up" },
  { label: "今日上传", value: "248", trend: "+12%", tone: "up" },
  { label: "失败率", value: "1.4%", trend: "-0.3%", tone: "down" },
  { label: "队列深度", value: "32", trend: "背压监控", tone: "warning" },
  { label: "平均耗时", value: "820 ms", trend: "-90 ms", tone: "up" },
];

export const directoryTree: FileNode[] = [
  {
    name: "/data/logs/app",
    path: "/data/logs/app",
    type: "dir",
    autoUpload: true,
    children: [
      {
        name: "app-2024-12-26.log",
        path: "/data/logs/app/app-2024-12-26.log",
        type: "file",
        size: "14.2 MB",
        updated: "10:32:10",
        autoUpload: true,
      },
      {
        name: "heap-2024-12-26.hprof",
        path: "/data/logs/app/heap-2024-12-26.hprof",
        type: "file",
        size: "1.2 GB",
        updated: "10:31:45",
        autoUpload: false,
      },
      {
        name: "archived",
        path: "/data/logs/app/archived",
        type: "dir",
        autoUpload: false,
        children: [
          {
            name: "trace-2024-12-24.log",
            path: "/data/logs/app/archived/trace-2024-12-24.log",
            type: "file",
            size: "4.2 MB",
            updated: "10:20:12",
            autoUpload: false,
          },
          {
            name: "trace-2024-12-25.log",
            path: "/data/logs/app/archived/trace-2024-12-25.log",
            type: "file",
            size: "4.5 MB",
            updated: "10:28:18",
            autoUpload: true,
          },
        ],
      },
    ],
  },
  {
    name: "/data/etl/raw",
    path: "/data/etl/raw",
    type: "dir",
    autoUpload: true,
    children: [
      {
        name: "etl-raw-2024-12-26-01.csv",
        path: "/data/etl/raw/etl-raw-2024-12-26-01.csv",
        type: "file",
        size: "8.6 MB",
        updated: "10:31:58",
        autoUpload: true,
      },
      {
        name: "etl-raw-2024-12-26-02.csv",
        path: "/data/etl/raw/etl-raw-2024-12-26-02.csv",
        type: "file",
        size: "9.1 MB",
        updated: "10:29:31",
        autoUpload: true,
      },
      {
        name: "error-2024-12-26.tar.gz",
        path: "/data/etl/raw/error-2024-12-26.tar.gz",
        type: "file",
        size: "380 MB",
        updated: "10:30:02",
        autoUpload: false,
      },
    ],
  },
];

export const files: FileItem[] = [
  {
    name: "app-2024-12-26.log",
    path: "/data/logs/app/app-2024-12-26.log",
    size: "14.2 MB",
    status: "uploaded",
    time: "10:32:10",
    autoUpload: true,
  },
  {
    name: "etl-raw-2024-12-26-01.csv",
    path: "/data/etl/raw/etl-raw-2024-12-26-01.csv",
    size: "8.6 MB",
    status: "uploaded",
    time: "10:31:58",
    autoUpload: true,
  },
  {
    name: "heap-2024-12-26.hprof",
    path: "/data/logs/app/heap-2024-12-26.hprof",
    size: "1.2 GB",
    status: "queued",
    time: "10:31:45",
    autoUpload: false,
  },
  {
    name: "vid-2301.mp4",
    path: "/data/logs/app/vid-2301.mp4",
    size: "92 MB",
    status: "failed",
    time: "10:31:12",
    autoUpload: false,
  },
  {
    name: "error-2024-12-26.tar.gz",
    path: "/data/etl/raw/error-2024-12-26.tar.gz",
    size: "380 MB",
    status: "queued",
    time: "10:30:02",
    autoUpload: false,
  },
  {
    name: "model-v12.zip",
    path: "/data/logs/app/model-v12.zip",
    size: "482 MB",
    status: "uploaded",
    time: "10:30:44",
    autoUpload: true,
  },
];

export const monitorNotes: MonitorNote[] = [
  { title: "S3 连接", detail: "endpoint=minio.local · force_path_style=true · region=us-east-1" },
  { title: "上传工作池", detail: "workers=8 · queue=200 · 当前 backlog=32" },
  { title: "通知", detail: "钉钉机器人已配置 · 失败自动重试 3 次" },
];

export const uploadRecords: UploadRecord[] = [
  {
    file: "app-2024-12-26.log",
    target: "https://minio.local/logs-warm/app-2024-12-26.log",
    size: "14.2 MB",
    result: "success",
    latency: "640 ms",
    time: "10:32:10",
    note: "自动上传",
  },
  {
    file: "etl-raw-2024-12-26-01.csv",
    target: "https://minio.local/etl-raw/etl-raw-2024-12-26-01.csv",
    size: "8.6 MB",
    result: "success",
    latency: "520 ms",
    time: "10:31:58",
    note: "校验通过",
  },
  {
    file: "heap-2024-12-26.hprof",
    target: "",
    size: "1.2 GB",
    result: "pending",
    latency: "排队",
    time: "10:31:45",
    note: "排队中",
  },
  {
    file: "vid-2301.mp4",
    target: "",
    size: "92 MB",
    result: "failed",
    latency: "timeout",
    time: "10:31:12",
    note: "已触发告警",
  },
  {
    file: "model-v12.zip",
    target: "https://minio.local/artifacts/model-v12.zip",
    size: "482 MB",
    result: "success",
    latency: "2.4 s",
    time: "10:30:44",
    note: "断点续传",
  },
];

export const monitorSummary: MonitorSummary[] = [
  { label: "当前吞吐", value: "480/min", desc: "峰值 520/min" },
  { label: "成功率", value: "98.6%", desc: "失败率 1.4%" },
  { label: "平均延迟", value: "820 ms", desc: "P95 1.4s" },
  { label: "队列 backlog", value: "32", desc: "workers=8 / retry=3" },
];

// 过滤示例配置
export const configSnapshot: ConfigSnapshot = {
  watchDir: "/data/logs/app , /data/etl/raw",
  fileExt: "关闭 · 全量目录",
  silence: "4s",
  concurrency: "workers=8 / queue=200",
};

export const alertConfigSnapshot: AlertConfigSnapshot = {
  enabled: true,
  suppressEnabled: true,
  rulesFile: "/etc/gwf/alert-rules.yaml",
  logPaths: "/var/log/app/error.log,/var/log/app/worker.error.log",
  pollInterval: "2s",
  startFromEnd: true,
};

export const chartPoints: ChartPoint[] = [
  { label: "02:00", uploads: 40, failures: 2, queue: 20 },
  { label: "04:00", uploads: 52, failures: 1, queue: 34 },
  { label: "06:00", uploads: 60, failures: 3, queue: 40 },
  { label: "08:00", uploads: 78, failures: 4, queue: 52 },
  { label: "10:00", uploads: 92, failures: 2, queue: 38 },
  { label: "12:00", uploads: 88, failures: 3, queue: 32 },
];

export const alertDashboard: AlertDashboard = {
  overview: {
    window: "最近30分钟",
    risk: "高",
    fatal: 1,
    system: 6,
    business: 12,
    sent: 8,
    suppressed: 14,
    latest: "2025-01-08 22:33:05",
  },
  decisions: [
    {
      id: "1",
      time: "2025-01-08 22:33:05",
      level: "fatal",
      rule: "系统异常激增",
      message: "系统异常在5分钟内达到20次",
      file: "-",
      status: "sent",
    },
    {
      id: "2",
      time: "2025-01-08 22:32:41",
      level: "system",
      rule: "数据库连接池耗尽",
      message: "HikariPool-1 - Connection is not available",
      file: "/var/log/app/error.log",
      status: "sent",
    },
    {
      id: "3",
      time: "2025-01-08 22:32:30",
      level: "system",
      rule: "线程池拒绝",
      message: "RejectedExecutionException: thread pool is exhausted",
      file: "/var/log/app/error.log",
      status: "suppressed",
      reason: "5分钟内已告警",
    },
    {
      id: "4",
      time: "2025-01-08 22:32:22",
      level: "business",
      rule: "订单落库失败",
      message: "order write failed: duplicate key",
      file: "/var/log/app/error.log",
      status: "recorded",
    },
    {
      id: "5",
      time: "2025-01-08 22:31:58",
      level: "system",
      rule: "磁盘写入异常",
      message: "I/O error on device /dev/nvme0n1",
      file: "/var/log/app/error.log",
      status: "suppressed",
      reason: "5分钟内已告警",
    },
  ],
  stats: {
    sent: 8,
    suppressed: 14,
    recorded: 5,
  },
  rules: {
    source: "/etc/gwf/alert-rules.yaml",
    lastLoaded: "2025-01-08 22:33:00",
    total: 12,
    defaultSuppress: "5分钟",
    escalation: "阈值20次 / 5分钟 -> fatal",
    levels: {
      ignore: 2,
      business: 4,
      system: 5,
      fatal: 1,
    },
  },
  polling: {
    interval: "2s",
    logFiles: ["/var/log/app/error.log", "/var/log/app/worker.error.log"],
    lastPoll: "2025-01-08 22:33:04",
    nextPoll: "2025-01-08 22:33:06",
  },
};
