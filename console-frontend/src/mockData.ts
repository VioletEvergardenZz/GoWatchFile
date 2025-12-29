import type {
  ChartPoint,
  ConfigSnapshot,
  FailureItem,
  FileItem,
  FileNode,
  HeroCopy,
  MetricCard,
  MonitorNote,
  MonitorSummary,
  TimelineEvent,
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
  { label: "通知发送", value: "9", trend: "全部成功", tone: "up" },
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
        content: `[INFO] app start
[INFO] upload success key=logs/2024/12/26/app.log
[INFO] latency=640ms route=logs-warm`,
      },
      {
        name: "heap-2024-12-26.hprof",
        path: "/data/logs/app/heap-2024-12-26.hprof",
        type: "file",
        size: "1.2 GB",
        updated: "10:31:45",
        autoUpload: false,
        content: `自动上传关闭 · 手动触发
size=1.2GB · md5 pending`,
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
            content: `[TRACE] archived trace sample
lines=120`,
          },
          {
            name: "trace-2024-12-25.log",
            path: "/data/logs/app/archived/trace-2024-12-25.log",
            type: "file",
            size: "4.5 MB",
            updated: "10:28:18",
            autoUpload: true,
            content: `[TRACE] archived trace upload allowed
lines=128`,
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
        content: `id,value
1,foo
2,bar`,
      },
      {
        name: "etl-raw-2024-12-26-02.csv",
        path: "/data/etl/raw/etl-raw-2024-12-26-02.csv",
        type: "file",
        size: "9.1 MB",
        updated: "10:29:31",
        autoUpload: true,
        content: `id,value
3,baz
4,qux`,
      },
      {
        name: "error-2024-12-26.tar.gz",
        path: "/data/etl/raw/error-2024-12-26.tar.gz",
        type: "file",
        size: "380 MB",
        updated: "10:30:02",
        autoUpload: false,
        content: "自动上传关闭 · 手动触发后再入云",
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

export const tailLines = [
  "[10:32:10] upload success key=logs/2024/12/26/app.log size=14.2MB latency=640ms",
  "[10:31:58] enqueue raw csv -> bucket=logs-warm",
  "[10:31:50] auto-upload disabled for /data/logs/app/heap-2024-12-26.hprof (manual trigger)",
  "[10:31:45] heap dump queued for upload size>1GB",
  "[10:31:12] upload failed key=vid-2301.mp4 err=timeout",
  "[10:30:44] uploaded model-v12.zip latency=2.4s route=artifacts",
  "[10:30:31] gc-2024-12-26.txt uploaded latency=530ms",
  "[10:30:02] throttled full-2024-12-26.tar.gz queue=high",
];

export const timelineEvents: TimelineEvent[] = [
  { label: "检测新文件", time: "10:31:45", status: "info", host: "srv-01" },
  { label: "静默窗口通过", time: "10:31:48", status: "success", host: "srv-01" },
  { label: "进入队列", time: "10:31:50", status: "warning", host: "srv-01" },
  { label: "等待处理", time: "10:31:55", status: "danger", host: "srv-01" },
];

export const failures: FailureItem[] = [
  { file: "vid-2301.mp4", reason: "S3 超时", attempts: 2, next: "1m 后重试" },
  { file: "2024-12-25-23.csv", reason: "MD5 不一致", attempts: 1, next: "排队" },
  { file: "heap-2024-12-26.hprof", reason: "超阈值需手动处理", attempts: 0, next: "待处理" },
];

export const monitorNotes: MonitorNote[] = [
  { title: "S3 连接", detail: "endpoint=minio.local · force_path_style=true · region=us-east-1" },
  { title: "上传工作池", detail: "workers=8 · queue=200 · 当前 backlog=32" },
  { title: "通知", detail: "钉钉机器人已配置 · 失败自动重试 3 次" },
];

export const uploadRecords: UploadRecord[] = [
  { file: "app-2024-12-26.log", target: "s3://logs-warm", size: "14.2 MB", result: "success", latency: "640 ms", time: "10:32:10", note: "自动上传" },
  { file: "etl-raw-2024-12-26-01.csv", target: "s3://etl-raw", size: "8.6 MB", result: "success", latency: "520 ms", time: "10:31:58", note: "校验通过" },
  { file: "heap-2024-12-26.hprof", target: "隔离区 /data/quarantine", size: "1.2 GB", result: "pending", latency: "排队", time: "10:31:45", note: "排队中" },
  { file: "vid-2301.mp4", target: "s3://logs-warm", size: "92 MB", result: "failed", latency: "timeout", time: "10:31:12", note: "已触发告警" },
  { file: "model-v12.zip", target: "s3://artifacts", size: "482 MB", result: "success", latency: "2.4 s", time: "10:30:44", note: "断点续传" },
];

export const monitorSummary: MonitorSummary[] = [
  { label: "当前吞吐", value: "480/min", desc: "峰值 520/min" },
  { label: "成功率", value: "98.6%", desc: "失败率 1.4%" },
  { label: "平均延迟", value: "820 ms", desc: "P95 1.4s" },
  { label: "队列 backlog", value: "32", desc: "workers=8 / retry=3" },
];

export const configSnapshot: ConfigSnapshot = {
  watchDir: "/data/logs/app , /data/etl/raw",
  fileExt: "关闭 · 全量目录",
  silence: "4s",
  concurrency: "workers=8 / queue=200",
};

export const chartPoints: ChartPoint[] = [
  { label: "02:00", uploads: 40, failures: 2, queue: 20 },
  { label: "04:00", uploads: 52, failures: 1, queue: 34 },
  { label: "06:00", uploads: 60, failures: 3, queue: 40 },
  { label: "08:00", uploads: 78, failures: 4, queue: 52 },
  { label: "10:00", uploads: 92, failures: 2, queue: 38 },
  { label: "12:00", uploads: 88, failures: 3, queue: 32 },
];
