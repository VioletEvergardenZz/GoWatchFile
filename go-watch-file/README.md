# File Watch Service（go-watch-file）

一个通用的文件监控与处理服务：递归监听目录、过滤/匹配文件、写入完成后并行上传到 S3 兼容存储，并支持钉钉机器人通知与邮件通知。内置控制台 API 用于目录树/文件列表/上传记录/日志 Tail 与运行时配置。

## 工作方式（按实际代码）
1) fsnotify 递归监听 `watch_dir`（支持多目录 运行中自动发现新建子目录）
2) 按 `file_ext` 过滤目标文件（支持多后缀；为空表示不过滤）。
3) 文件写入完成判定：在静默窗口内无新写入才入队（默认 10s）。
4) 入队后由 WorkerPool 并发上传至 S3 兼容存储。
5) 写入状态与上传结果写入运行态（Dashboard/时间线/上传记录）。
6) 钉钉机器人可选通知，支持邮件通知（需配置 SMTP）。

## 快速上手
1) 环境：Go 1.23+（`go.mod` 含 `toolchain go1.24.3`）。
2) 复制并填写环境变量：
   ```bash
   cp .env.example .env
   # 填写 WATCH_DIR、FILE_EXT、S3、DINGTALK 等
   ```
3) 配置文件：`config.yaml` 保留占位符，实际值来自 `.env` 或系统环境变量。
4) 构建与运行：
   ```bash
   go build -o bin/file-watch cmd/main.go
   ./bin/file-watch -config config.yaml
   ```
5) 停止：`Ctrl + C`，服务会优雅退出并等待队列 drain。

配置优先级：系统环境变量 > `.env` > `config.yaml` 占位符 > 内置默认值。

`.env` 读取策略：会尝试加载当前目录的 `.env`，以及配置文件同目录的 `.env`；仅在系统环境未设置时生效。

## 配置详解

### 必填字段
- `watch_dir` (`WATCH_DIR`)：监控目录（支持多目录 逗号或分号分隔）
- `bucket` / `ak` / `sk` / `endpoint` / `region`：S3 访问配置。
- `log_level`：`debug|info|warn|error`。
- `api_bind`：API 监听地址（默认 `:8080`）。

### 可选字段
- `watch_exclude` (`WATCH_EXCLUDE`)：排除目录（逗号/分号分隔），支持目录名或绝对路径，如 `.git,node_modules,/opt/homebrew`。
- `file_ext` (`FILE_EXT`)：后缀过滤，支持多值（逗号/空格分隔，如 `.log,.txt`），为空则不过滤。
- `silence` (`SILENCE_WINDOW`)：写入完成判定窗口，默认 `10s`。
  - 支持写法：`10s` / `10秒` / `10`。
  - `config.yaml` 默认模板未包含此字段，可自行添加。
- `upload_workers` / `upload_queue_size`：上传并发与队列容量。
- `force_path_style` / `disable_ssl`：S3 兼容性开关。
- `dingtalk_webhook` / `dingtalk_secret`：钉钉机器人（可选）。
- `email_host` / `email_port` / `email_user` / `email_pass` / `email_from` / `email_to` / `email_use_tls`：SMTP 邮件通知（与钉钉同内容，可选）。
- `robot_key`：预留字段，当前代码未使用。
- `log_file` / `log_to_std` / `log_show_caller`：日志输出配置。
- `alert_enabled` (`ALERT_ENABLED`)：是否启用告警决策（true/false）。
- `alert_suppress_enabled` (`ALERT_SUPPRESS_ENABLED`)：是否开启告警抑制（默认 true）。
- `alert_rules_file` (`ALERT_RULES_FILE`)：告警规则文件路径（YAML/JSON）。
- `alert_log_paths` (`ALERT_LOG_PATHS`)：日志文件路径列表（逗号/分号/空白分隔）。
- `alert_poll_interval` (`ALERT_POLL_INTERVAL`)：轮询间隔（默认 2s，支持 2s/2秒/2）。
- `alert_start_from_end` (`ALERT_START_FROM_END`)：是否从文件末尾开始追踪（默认 true）。
  - `true` 仅处理新写入日志，忽略历史内容。
  - `false` 启动时从头扫描，可能产生历史告警。

### 配置示例（config.yaml）
```yaml
watch_dir: "${WATCH_DIR}"
watch_exclude: "${WATCH_EXCLUDE}"
file_ext: "${FILE_EXT}"
# 可选：silence: "${SILENCE_WINDOW}"

robot_key: "${ROBOT_KEY}"
dingtalk_webhook: "${DINGTALK_WEBHOOK}"
dingtalk_secret: "${DINGTALK_SECRET}"
email_host: "${EMAIL_HOST}"
email_user: "${EMAIL_USER}"
email_pass: "${EMAIL_PASS}"
email_from: "${EMAIL_FROM}"
email_to: "${EMAIL_TO}"
email_port: 587
email_use_tls: true

bucket: "${S3_BUCKET}"
ak: "${S3_AK}"
sk: "${S3_SK}"
endpoint: "${S3_ENDPOINT}"
region: "${S3_REGION}"
force_path_style: false
disable_ssl: false

log_level: "${LOG_LEVEL}"
log_file: "${LOG_FILE}"
log_to_std: true
log_show_caller: false

upload_workers: 3
upload_queue_size: 100
api_bind: "${API_BIND}"

alert_enabled: true
alert_suppress_enabled: true
alert_rules_file: "/etc/gwf/alert-rules.yaml"
alert_log_paths: "/var/log/app/error.log,/var/log/app/worker.error.log"
alert_poll_interval: "2s"
alert_start_from_end: true
```

### 环境变量模板（.env.example）
`.env.example` 已提供模板。需要时可补充 `SILENCE_WINDOW=10s`。

### 告警规则文件示例
参考 `alert-rules.example.yaml`，按需调整关键词、级别与抑制窗口。

## HTTP API（控制台使用）

### 1) 获取仪表盘
- `GET /api/dashboard`
- 返回：`DashboardData`（目录树、文件列表、指标、监控摘要、配置快照等）。

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

### 5) 运行时配置更新
- `POST /api/config`
- Body：
  ```json
  {
    "watchDir": "/path/to/watch",
    "fileExt": ".log",
    "silence": "10s",
    "uploadWorkers": 3,
    "uploadQueueSize": 100
  }
  ```
- 说明：仅更新目录/后缀/静默窗口/并发/队列。S3 与通知配置需改配置文件并重启。

### 6) 健康检查
- `GET /api/health` → `{ "queue": n, "workers": n }`

### 7) 告警决策面板
- `GET /api/alerts`
- 返回：`AlertDashboard`（告警概览、列表、统计、规则摘要、轮询摘要）
- 说明：概览窗口为最近 24 小时，控制台“最新窗口”展示 `overview.window`

### 8) 告警配置
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

## 运行时配置更新说明
`/api/config` 会在内部重新创建 watcher / upload pool / runtime state，并迁移历史指标；若新配置启动失败会回滚到旧配置。该接口不会写回 `config.yaml`。
`/api/alert-config` 仅更新告警配置与轮询状态，不写回 `config.yaml`。

## 运行态与指标
- 运行态数据保存在内存中（tail/timeline/uploads/chart）。
- 队列统计使用 `QueueLength + InFlight` 作为 backlog。
- 图表点为累计成功/失败 + 当前队列深度（非单次区间计数）。

## 已知限制
- 支持多监控目录（逗号或分号分隔）
- 上传队列为内存队列，重启会清空。
- 上传失败没有自动重试（需手动触发）。
- 已实现钉钉通知与邮件通知，企业微信未接入。
- 目录过大时可能触发系统句柄限制，可通过 `watch_exclude` 跳过大目录或提升系统 `ulimit`。

## 相关文档
- 平台概述：`../docs/overview.md`
- 流程图：`../docs/system-flowchart.md`
- 前后端联动：`../docs/frontend-backend-linkage.md`
- 告警模式：`../docs/alert-mode.md`
- 开发指南：`../docs/dev-guide.md`
- 常见问题：`../docs/faq.md`

## 开发与测试
- 运行测试：`go test ./...`
- 代码格式：`gofmt`，遵循 Go 官方规范。
