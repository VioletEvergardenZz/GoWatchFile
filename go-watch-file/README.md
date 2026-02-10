# File Watch Service（go-watch-file）

一个通用的文件监控与处理服务：递归监听目录、过滤/匹配文件、写入完成后并行上传到 S3 兼容存储，支持上传失败重试、钉钉机器人通知与邮件通知，并提供 AI 日志分析（可选）。内置控制台 API 用于目录树/文件列表/上传记录/日志 Tail/检索、系统资源面板与运行时配置。

## 配置说明（控制台优先）

- 运行时字段（watch_dir, file_ext, silence, upload_workers, upload_queue_size, upload_retry_enabled, upload_retry_delays, system_resource_enabled, alert_*）由控制台设置，并持久化到 `config.runtime.yaml`。
- `config.yaml` 保留静态配置（S3 连接参数/日志/API bind/队列持久化开关），也可被环境变量覆盖。
- 环境变量主要用于密钥、安全与 AI 配置（S3_AK/S3_SK, DINGTALK_*, EMAIL_*, API_*, AI_*），并支持可选覆盖 S3_BUCKET/S3_ENDPOINT/S3_REGION/S3_FORCE_PATH_STYLE/S3_DISABLE_SSL/UPLOAD_QUEUE_PERSIST_*。

## 工作方式（按实际代码）
1) fsnotify 递归监听 `watch_dir`（支持多目录 运行中自动发现新建子目录）
2) 按 `file_ext` 过滤目标文件（支持多后缀；为空表示不过滤），并忽略临时后缀（如 `.tmp/.part/.crdownload`）。
3) 文件写入完成判定：在静默窗口内无新写入才入队（默认 10s）。
4) 入队后由 WorkerPool 并发上传至 S3 兼容存储，失败按配置重试。
5) 写入状态与上传结果写入运行态（Dashboard/时间线/上传记录）。
6) 钉钉机器人可选通知，支持邮件通知（需配置 SMTP）。

## 快速上手
1) 环境：Go 1.23+（`go.mod` 含 `toolchain go1.24.3`）。
2) 复制并填写环境变量：
   ```bash
   cp .env.example .env
   # 填写密钥（S3_AK/S3_SK, DINGTALK_*, EMAIL_*）
   # 填写 API 安全（API_AUTH_TOKEN, API_CORS_ORIGINS）
   # 可选开启队列落盘：UPLOAD_QUEUE_PERSIST_ENABLED/UPLOAD_QUEUE_PERSIST_FILE
   # 如需覆盖 S3 参数，可设置 S3_BUCKET/S3_ENDPOINT/S3_REGION/S3_FORCE_PATH_STYLE/S3_DISABLE_SSL
   # 可选 AI 分析：AI_ENABLED/AI_BASE_URL/AI_API_KEY/AI_MODEL/AI_TIMEOUT/AI_MAX_LINES
   ```
3) 配置文件：密钥字段使用占位符，S3 连接参数默认在 `config.yaml`，也可用环境变量覆盖。
4) 构建与运行：
   ```bash
   go build -o bin/file-watch cmd/main.go
   ./bin/file-watch -config config.yaml
   ```
5) 停止：`Ctrl + C`，服务会优雅退出并等待队列 drain。

配置优先级：config.yaml -> config.runtime.yaml -> 环境变量覆盖 -> 默认值。
环境变量仅覆盖 S3 / 通知 / API 安全 / AI / 队列持久化相关字段，`watch_dir`/`file_ext`/`watch_exclude`/`log_level`/`alert_*` 不会被环境变量覆盖。
管理接口默认要求 `API_AUTH_TOKEN`，仅 `/api/health` 允许匿名访问。

`.env` 读取策略：会尝试加载当前目录的 `.env`，以及配置文件同目录的 `.env`；仅在系统环境未设置时生效。

## 配置详解

### 必填字段
- `watch_dir`：监控目录（支持多目录 逗号或分号分隔，可为空由控制台设置）
- `bucket` / `ak` / `sk` / `endpoint` / `region`：S3 访问配置。
- `log_level`：`debug|info|warn|error`。
- `api_bind`：API 监听地址（默认 `:8080`）。
- `api_auth_token`：管理接口鉴权令牌（建议从 `API_AUTH_TOKEN` 注入）。
- `api_cors_origins`：允许跨域来源列表（逗号分隔，可填 `*`）。

### 可选字段
- `watch_exclude`：排除目录（逗号/分号分隔），支持目录名或绝对路径，如 `.git,node_modules,/opt/homebrew`。
- `file_ext`：后缀过滤，支持多值（逗号/空格分隔，如 `.log,.txt`），为空则不过滤。
- `silence`：写入完成判定窗口，默认 `10s`。
  - 支持写法：`10s` / `10秒` / `10`。
- `upload_workers` / `upload_queue_size`：上传并发与队列容量。
- `upload_queue_persist_enabled` / `upload_queue_persist_file`：是否开启上传队列落盘与落盘文件路径（默认关闭，开启后为“至少一次”语义，需重启生效）。
  - 当持久化文件损坏时，会自动备份为 `<原文件>.corrupt-时间戳.bak`，并降级为空队列继续启动。
- `upload_retry_enabled`：是否启用上传失败重试（默认 true）。
- `upload_retry_delays`：重试间隔列表（逗号/空白/分号分隔），默认 `1s,2s,5s`，非法项会忽略。
- `system_resource_enabled`：系统资源面板开关（默认 false，开启后 `/api/system` 可用）。
- `force_path_style` / `disable_ssl`：S3 兼容性开关。
- `dingtalk_webhook` / `dingtalk_secret`：钉钉机器人（可选）。
- `email_host` / `email_port` / `email_user` / `email_pass` / `email_from` / `email_to` / `email_use_tls`：SMTP 邮件通知（与钉钉同内容，可选）。
- `robot_key`：预留字段，当前代码未使用。
- `log_file` / `log_to_std` / `log_show_caller`：日志输出配置。
- `alert_enabled`：是否启用告警决策（true/false）。
- `alert_suppress_enabled`：是否开启告警抑制（默认 true）。
- `alert_rules_file`：告警规则文件路径（YAML/JSON）。
- `alert_log_paths`：日志文件路径列表（逗号/分号/空白分隔）。
- `alert_poll_interval`：轮询间隔（默认 2s，支持 2s/2秒/2）。
- `alert_start_from_end`：是否从文件末尾开始追踪（默认 true）。
  - `true` 仅处理新写入日志，忽略历史内容。
  - `false` 启动时从头扫描，可能产生历史告警。
- `ai_enabled`：是否启用 AI 日志分析（true/false）。
- `ai_base_url` / `ai_api_key` / `ai_model`：AI 请求地址、密钥与模型。
- `ai_timeout`：AI 请求超时（支持 `20s` / `1m` 等）。
- `ai_max_lines`：AI 分析最大行数（默认 200）。

### 配置示例（config.yaml）
```yaml
watch_dir: ""
watch_exclude: ""
file_ext: ""

robot_key: "${ROBOT_KEY}"
dingtalk_webhook: "${DINGTALK_WEBHOOK}"
dingtalk_secret: "${DINGTALK_SECRET}"

email_host: ""
email_user: "${EMAIL_USER}"
email_pass: "${EMAIL_PASS}"
email_from: ""
email_to: ""
email_port: 587
email_use_tls: true

bucket: "go-watch-file"
ak: "${S3_AK}"
sk: "${S3_SK}"
endpoint: "oss-cn-hangzhou.aliyuncs.com"
region: "cn-hangzhou"
force_path_style: false
disable_ssl: false

log_level: "info"
log_file: "logs/file-monitor.log"
log_to_std: true
log_show_caller: false

upload_workers: 10
upload_queue_size: 100
upload_queue_persist_enabled: false
upload_queue_persist_file: "logs/upload-queue.json"
upload_retry_enabled: true
upload_retry_delays: "1s,2s,5s"
api_bind: ":8080"
system_resource_enabled: false

alert_enabled: false
alert_suppress_enabled: true
alert_rules_file: ""
alert_log_paths: ""
alert_poll_interval: "2s"
alert_start_from_end: true

ai_enabled: true
ai_base_url: "${AI_BASE_URL}"
ai_api_key: "${AI_API_KEY}"
ai_model: "${AI_MODEL}"
ai_timeout: "90s"
ai_max_lines: 200
```


### 环境变量模板（.env.example）
`.env.example` 已提供模板，可按需补充 `S3_BUCKET/S3_ENDPOINT/S3_REGION/S3_FORCE_PATH_STYLE/S3_DISABLE_SSL` 与 AI 相关变量。

### 告警规则文件示例
参考 `alert-rules.example.yaml`，按需调整关键词、级别与抑制窗口。

## HTTP API（控制台使用）

### 1) 获取仪表盘
- `GET /api/dashboard`
- 返回：`DashboardData`（目录树、文件列表、指标、监控摘要、配置快照等）。
- 全量仪表盘默认使用约 2 秒短缓存，减少目录高频扫描。
- 可选：`refresh=true` 强制绕过缓存（返回头 `X-Dashboard-Cache=miss`）。

### 2) 自动上传开关
- `POST /api/auto-upload`
- Body：`{ "path": "/path/to/dir-or-file", "enabled": true }`
- 说明：目录开关会联动子目录与文件。

### 3) 手动上传
- `POST /api/manual-upload`
- Body：`{ "path": "/path/to/file" }`
- 说明：即使该路径自动上传关闭，也会触发一次上传（单次）。

### 4) 文件内容 / 全文检索
- `POST /api/file-log`
- Body：`{ "path": "/path/to/file", "query": "keyword", "limit": 500, "caseSensitive": false }`
- 说明：不传 `query` 时读取文件尾部（仅文本文件，最多 **512KB / 500 行**）；传入 `query` 时进行全文检索并返回匹配行（默认最多 **2000 行**，超出会截断）。

### 5) AI 日志分析
- `POST /api/ai/log-summary`
- Body：`{ "path": "/path/to/file", "mode": "tail", "query": "", "limit": 200, "caseSensitive": false }`
- 说明：需 `ai_enabled=true` 且 AI_* 配置齐全；`mode=search` 必须带 `query`；`limit` 不传时使用 `ai_max_lines`。

### 6) 运行时配置更新
- `POST /api/config`
- Body：
  ```json
  {
    "watchDir": "/path/to/watch",
    "fileExt": ".log",
    "silence": "10s",
    "uploadWorkers": 3,
    "uploadQueueSize": 100,
    "uploadRetryDelays": "1s,2s,5s",
    "uploadRetryEnabled": true,
    "systemResourceEnabled": true
  }
  ```
- 说明：仅更新目录/后缀/静默窗口/并发/队列/重试参数/系统资源开关。S3 与通知配置需改配置文件并重启。

### 7) 健康检查
- `GET /api/health`
- 返回示例：
  ```json
  {
    "queue": 0,
    "workers": 3,
    "inFlight": 0,
    "queueFullTotal": 0,
    "retryTotal": 0,
    "uploadFailureTotal": 0,
    "failureReasons": []
  }
  ```

### 8) 告警决策面板
- `GET /api/alerts`
- 返回：`AlertDashboard`（告警概览、列表、统计、规则摘要、轮询摘要）
- 说明：概览窗口为最近 24 小时，控制台“最新窗口”展示 `overview.window`

### 9) 告警配置
- `GET /api/alert-config`
- 返回：告警配置快照（enabled/suppressEnabled/rulesFile/logPaths/pollInterval/startFromEnd）
- `POST /api/alert-config`
- Body：
  ```json
  {
    "enabled": true,
    "suppressEnabled": true,
    "rulesFile": "/etc/gwf/alert-rules.yaml",
    "logPaths": "/var/log/app/error.log",
    "pollInterval": "2s",
    "startFromEnd": true
  }
  ```
- 说明：仅更新内存配置，重启后以配置文件为准。

### 10) 告警规则
- `GET /api/alert-rules`
- `POST /api/alert-rules`
- Body：`{ "rules": { ... } }`
- 说明：保存后写入 `config.runtime.yaml`（可写时）。

### 11) 系统资源面板
- `GET /api/system`
- Query：
  - `mode=lite` 或 `mode=light` → 仅返回概览/指标/分区，不返回进程列表
  - `limit=200` → 限制返回的进程数量，`0` 表示不限制
  - `includeEnv=true` → 返回进程环境变量（默认不返回）
- 返回：`{ systemOverview, systemGauges, systemVolumes, systemProcesses }`
- 说明：需开启 `systemResourceEnabled`，否则返回 403；默认不返回进程环境变量，避免敏感信息暴露。

## 运行时配置更新说明
`/api/config` 会在内部重新创建 watcher / upload pool / runtime state，并迁移历史指标；若新配置启动失败会回滚到旧配置。支持更新 upload_retry_enabled/upload_retry_delays，该接口不会写回 `config.yaml`，也不支持在线切换 `upload_queue_persist_*`（静态项需重启）。
Runtime updates are persisted to `config.runtime.yaml` (best effort).

`/api/alert-config` 仅更新告警配置与轮询状态，不写回 `config.yaml`。
Alert config updates are persisted to `config.runtime.yaml` (best effort).


## 运行态与指标
- 运行态数据保存在内存中（tail/timeline/uploads/chart）。
- 队列统计使用 `QueueLength + InFlight` 作为 backlog。
- 图表点为累计成功/失败 + 当前队列深度（非单次区间计数）。

## 已知限制
- 支持多监控目录（逗号或分号分隔）
- 默认上传队列为内存队列，重启会清空；开启 `upload_queue_persist_enabled` 后会从持久化文件恢复未完成任务。
- 不支持断点续传。
- 已实现钉钉通知与邮件通知，企业微信未接入。
- 目录过大时可能触发系统句柄限制，可通过 `watch_exclude` 跳过大目录或提升系统 `ulimit`。

## 相关文档
- 平台概述：`../docs/overview.md`
- 流程图：`../docs/system-flowchart.md`
- 前后端联动：`../docs/frontend-backend-linkage.md`
- 队列与 worker：`../docs/queue-worker-flow.md`
- 队列持久化运行手册：`../docs/queue-persistence-runbook.md`
- 系统资源面板：`../docs/system-resource-console.md`
- 告警模式：`../docs/alert-mode.md`
- 开发指南：`../docs/dev-guide.md`
- 常见问题：`../docs/faq.md`

## 开发与测试
- 运行测试：`go test ./...`
- 代码格式：`gofmt`，遵循 Go 官方规范。
