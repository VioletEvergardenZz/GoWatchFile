# 文件监控服务（go-watch-file）

一个通用的文件监控与处理服务：递归监听目录、过滤/匹配文件、写入完成后并行上传到阿里云 OSS，支持上传失败重试、钉钉机器人通知与邮件通知，并提供 AI 日志分析。AI 能力是平台主线能力，运行时可按环境开关控制。内置控制台 API 用于目录树/文件列表/上传记录/日志 Tail/检索、系统资源面板与运行时配置。

## 配置说明（控制台优先）

- 运行时字段（watch_dir, file_ext, silence, upload_workers, upload_queue_size, upload_retry_enabled, upload_retry_delays, system_resource_enabled, alert_*）由控制台设置，并持久化到 `config.runtime.yaml`。
- `config.yaml` 保留静态配置（OSS 连接参数/日志/API bind/队列持久化开关），也可被环境变量覆盖。
- 环境变量主要用于密钥、安全与 AI 配置（OSS_AK/OSS_SK, DINGTALK_*, EMAIL_*, API_*, AI_*），并支持可选覆盖 OSS_BUCKET/OSS_ENDPOINT/OSS_REGION/OSS_FORCE_PATH_STYLE/OSS_DISABLE_SSL/UPLOAD_QUEUE_PERSIST_*/UPLOAD_QUEUE_SATURATION_THRESHOLD/UPLOAD_QUEUE_CIRCUIT_BREAKER_ENABLED/UPLOAD_RETRY_MAX_ATTEMPTS。

## 工作方式（按实际代码）
1) fsnotify 递归监听 `watch_dir`（支持多目录 运行中自动发现新建子目录）
2) 按 `file_ext` 过滤目标文件（支持多后缀；为空表示不过滤），并忽略临时后缀（如 `.tmp/.part/.crdownload`）。
3) 文件写入完成判定：在静默窗口内无新写入才入队（默认 10s）。
4) 入队后由 WorkerPool 并发上传至阿里云 OSS，失败按配置重试。
5) 写入状态与上传结果写入运行态（Dashboard/时间线/上传记录）。
6) 钉钉机器人可选通知，支持邮件通知（需配置 SMTP）。

## 快速上手
1) 环境：Go 1.24+（`go.mod` 含 `toolchain go1.24.3`）。
2) 复制并填写环境变量：
   ```bash
   cp .env.example .env
   # 填写密钥（OSS_AK/OSS_SK, DINGTALK_*, EMAIL_*）
   # API 鉴权可选：可设置 API_AUTH_TOKEN 开启鉴权
   # 如需显式关闭可设置 API_AUTH_DISABLED=true
   # 跨域来源建议设置 API_CORS_ORIGINS
   # 可选开启队列落盘：UPLOAD_QUEUE_PERSIST_ENABLED/UPLOAD_QUEUE_PERSIST_FILE
   # 可选调整背压保护：UPLOAD_QUEUE_SATURATION_THRESHOLD/UPLOAD_QUEUE_CIRCUIT_BREAKER_ENABLED
   # 可选调整上传重试上限：UPLOAD_RETRY_MAX_ATTEMPTS
   # 可选开启上传 ETag 校验：UPLOAD_ETAG_VERIFY_ENABLED
   # 如需覆盖 OSS 参数，可设置 OSS_BUCKET/OSS_ENDPOINT/OSS_REGION/OSS_FORCE_PATH_STYLE/OSS_DISABLE_SSL
   # 可选 AI 分析：AI_ENABLED/AI_BASE_URL/AI_API_KEY/AI_MODEL/AI_TIMEOUT/AI_MAX_LINES
   ```
3) 配置文件：密钥字段使用占位符，OSS 连接参数默认在 `config.yaml`，也可用环境变量覆盖。
4) 构建与运行：
   ```bash
   go build -o bin/file-watch cmd/main.go
   ./bin/file-watch -config config.yaml
   ```
5) 停止：`Ctrl + C`，服务会优雅退出并等待队列 drain。

配置优先级：config.yaml -> config.runtime.yaml -> 环境变量覆盖 -> 默认值。
环境变量仅覆盖 OSS / 通知 / API 安全 / AI / 上传静态策略相关字段，`watch_dir`/`file_ext`/`watch_exclude`/`log_level`/`alert_*` 不会被环境变量覆盖。
当 `API_AUTH_TOKEN` 留空（或未配置）时，管理接口默认不校验 token；
如需强制关闭可设置 `API_AUTH_DISABLED=true`，如需开启则设置 `API_AUTH_TOKEN`。
`/api/health` 始终允许匿名访问。

`.env` 读取策略：会尝试加载当前目录的 `.env`，以及配置文件同目录的 `.env`；仅在系统环境未设置时生效。

## 配置详解

### 必填字段
- `watch_dir`：监控目录（支持多目录 逗号或分号分隔，可为空由控制台设置）
- `bucket` / `ak` / `sk` / `endpoint` / `region`：OSS 访问配置。
- `log_level`：`debug|info|warn|error`。
- `api_bind`：API 监听地址（默认 `:8080`）。
- `api_auth_token`：管理接口鉴权令牌（可选，建议从 `API_AUTH_TOKEN` 注入；留空则关闭鉴权）。
- `api_cors_origins`：允许跨域来源列表（逗号分隔，可填 `*`）。
  - 当 API 鉴权关闭（`API_AUTH_TOKEN` 为空或 `API_AUTH_DISABLED=true`）且该字段为空时，默认允许所有来源。
  - 当 API 鉴权开启且该字段为空时，会启用本地开发兜底策略：允许 `localhost/127.0.0.1/::1` 与同主机来源。

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
- `upload_retry_max_attempts`：上传最大尝试次数（含首次，默认 `4`）。
- `upload_etag_verify_enabled`：是否在上传成功后校验 OSS ETag（默认 false，开启后会比对本地 MD5 与 ETag）。
- `upload_queue_saturation_threshold`：队列饱和阈值（`0~1`，默认 `0.9`）。
- `upload_queue_circuit_breaker_enabled`：队列熔断限流开关（默认 `true`）。
- `system_resource_enabled`：系统资源面板开关（默认 false，开启后 `/api/system` 可用）。
- `force_path_style` / `disable_ssl`：OSS 地址开关。
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
ak: "${OSS_AK}"
sk: "${OSS_SK}"
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
upload_queue_saturation_threshold: 0.9
upload_queue_circuit_breaker_enabled: true
upload_retry_enabled: true
upload_retry_delays: "1s,2s,5s"
upload_retry_max_attempts: 4
upload_etag_verify_enabled: false
api_bind: ":8080"
system_resource_enabled: false

alert_enabled: false
alert_suppress_enabled: true
alert_rules_file: ""
alert_log_paths: ""
alert_poll_interval: "2s"
alert_start_from_end: true

ai_enabled: false
ai_base_url: "${AI_BASE_URL}"
ai_api_key: "${AI_API_KEY}"
ai_model: "${AI_MODEL}"
ai_timeout: "90s"
ai_max_lines: 200
```


### 环境变量模板（.env.example）
`.env.example` 已提供模板，可按需补充 `OSS_BUCKET/OSS_ENDPOINT/OSS_REGION/OSS_FORCE_PATH_STYLE/OSS_DISABLE_SSL` 与 AI 相关变量。
如需关闭 API 鉴权，可保持 `API_AUTH_TOKEN` 为空，或显式设置 `API_AUTH_DISABLED=true`。

### 告警规则文件示例
参考 `alert-rules.example.yaml`，按需调整关键词、级别与抑制窗口。

## HTTP API（控制台使用）

### 1) 获取仪表盘
- `GET /api/dashboard`
- 返回：`DashboardData`（目录树、文件列表、指标、监控摘要、配置快照等）。
- 全量仪表盘默认使用约 2 秒短缓存，减少目录高频扫描。
- 当运行态未就绪时，接口会返回降级结构（`200`），避免控制台直接出现 `500`。
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
- 路径范围：`path` 支持 `watch_dir` 下的文件，也支持 `alert_log_paths` 中声明的日志路径（便于直接分析后端报错日志）。
- AI 超时或异常时会自动压缩输入并重试；若仍失败则降级返回规则摘要（`meta.degraded=true`）。

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
- 说明：仅更新目录/后缀/静默窗口/并发/队列/重试参数/系统资源开关。OSS 与通知配置需改配置文件并重启。

### 7) 健康检查
- `GET /api/health`
- 返回示例：
  ```json
  {
    "queue": 0,
    "workers": 3,
    "inFlight": 0,
    "queueFullTotal": 0,
    "queueShedTotal": 0,
    "retryTotal": 0,
    "uploadFailureTotal": 0,
    "failureReasons": [],
    "persistQueue": {
      "enabled": false,
      "storeFile": "",
      "recoveredTotal": 0,
      "corruptFallbackTotal": 0,
      "persistWriteFailureTotal": 0
    }
  }
  ```

### 12) Prometheus 指标
- `GET /metrics`
- 说明：返回 Prometheus 文本暴露格式，可直接被采集器抓取。
- 关键指标：上传队列、上传耗时、失败原因、AI 请求结果、知识库检索/问答命中率与评审延迟。
- 告警规则模板：`deploy/prometheus/gwf-alert-rules.yaml`（覆盖队列高位、上传失败率、AI 降级率）。

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
- 说明：更新运行时配置并尽力写入 `config.runtime.yaml`（best effort）。

### 10) 告警规则
- `GET /api/alert-rules`
- `POST /api/alert-rules`
- Body：`{ "rules": { ... } }`
- 说明：保存后写入 `config.runtime.yaml`（可写时）。

### 11) 系统资源面板
- `GET /api/system`
- `POST /api/system/terminate`
- Body：`{"pid": 12345, "force": false}`
- 说明：按 PID 发送终止指令，默认 `TERM`，超时会回退 `KILL`；`force=true` 直接 `KILL`。
- 返回：`{ ok: true, result: { pid, name, command, signal, forced } }`
- 查询参数：
  - `mode=lite` 或 `mode=light` → 仅返回概览/指标/分区，不返回进程列表
  - `limit=200` → 限制返回的进程数量，`0` 表示不限制
  - `includeEnv=true` → 返回进程环境变量（默认不返回）
- 返回：`{ systemOverview, systemGauges, systemVolumes, systemProcesses }`
- 说明：需开启 `systemResourceEnabled`，否则返回 403；默认不返回进程环境变量，避免敏感信息暴露。

### 13) 控制面 MVP（SQLite 持久化）
- Agent：
  - `POST /api/control/agents`（注册）
  - `GET /api/control/agents`（列表）
  - `GET /api/control/agents/{id}`（详情）
  - `POST /api/control/agents/{id}/heartbeat`（心跳）
  - `POST /api/control/agents/{id}/drain`（摘流）
- 分发：
  - `POST /api/control/dispatch/pull`（Agent 拉取任务，MVP 为轮询模式）
- 审计：
  - `GET /api/control/audit`（按资源/操作人/动作筛选审计日志）
  - 查询参数：
    - `resourceType`（如 `task` / `agent`）
    - `resourceId`（资源 ID）
    - `operator`（操作人）
    - `action`（动作名）
    - `from`（开始时间，支持 `RFC3339` 或 `YYYY-MM-DDTHH:mm`）
    - `to`（结束时间，支持 `RFC3339` 或 `YYYY-MM-DDTHH:mm`）
    - `limit`（默认 200，最大 2000）
- 任务：
  - `POST /api/control/tasks`（创建任务）
  - `GET /api/control/tasks`（任务列表）
  - `GET /api/control/tasks/{id}`（任务详情）
  - `POST /api/control/tasks/{id}/cancel`（取消）
  - `POST /api/control/tasks/{id}/retry`（重试）
  - `POST /api/control/tasks/{id}/ack`（ack 并进入 running）
  - `POST /api/control/tasks/{id}/progress`（进度心跳，刷新 running 的 updatedAt）
  - `POST /api/control/tasks/{id}/complete`（完成并进入 success/failed）
  - `GET /api/control/tasks/{id}/events`（任务事件列表）
- 说明：
  - 当前为 MVP 实现，默认落盘到 `data/control/control.db`，重启后可恢复。
  - 可通过环境变量 `CONTROL_DATA_DIR` 指定存储目录。
  - 若存储初始化失败，会降级为内存模式继续提供接口能力。

## 运行时配置更新说明
`/api/config` 会在内部重新创建 watcher / upload pool / runtime state，并迁移历史指标；若新配置启动失败会回滚到旧配置。支持更新 upload_retry_enabled/upload_retry_delays，该接口不会写回 `config.yaml`，也不支持在线切换 `upload_queue_persist_*` / `upload_queue_saturation_threshold` / `upload_queue_circuit_breaker_enabled` / `upload_retry_max_attempts` / `upload_etag_verify_enabled`（静态项需重启）。
运行时更新会尽力持久化到 `config.runtime.yaml`。

`/api/alert-config` 仅更新告警配置与轮询状态，不写回 `config.yaml`。
告警配置更新会尽力持久化到 `config.runtime.yaml`。


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

## 运维评估命令
命中率评估：
```bash
cd go-watch-file
go run ./cmd/kb-eval hitrate -base http://localhost:8082 -token "$API_AUTH_TOKEN" -samples ../docs/04-知识库/知识库命中率样本.json
```

问答引用率评估：
```bash
cd go-watch-file
go run ./cmd/kb-eval citation -base http://localhost:8082 -token "$API_AUTH_TOKEN" -samples ../docs/04-知识库/知识库命中率样本.json -limit 3 -target 1.0
```

MTTD 对比评估：
```bash
cd go-watch-file
go run ./cmd/kb-eval mttd -input ../docs/04-知识库/知识库MTTD基线.csv
```

知识库复盘汇总：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/kb-recap.ps1 -BaseUrl http://localhost:8082 -Token $env:API_AUTH_TOKEN -SamplesFile ../docs/04-知识库/知识库命中率样本.json -MttdFile ../docs/04-知识库/知识库MTTD基线.csv -CitationTarget 1.0 -HitrateTarget 0.8 -MttdDropTarget 0.2 -OutputFile ../reports/kb-recap-result.json -ReportFile ../docs/05-指标与评估/知识库命中率与引用率阶段复盘改进清单-$(Get-Date -Format yyyy-MM-dd).md
```
若后端未启用鉴权，上述命令可省略 `-Token` 参数；`kb-eval` 的 `-token` 参数同样可省略。  
离线复盘可直接消费既有结果文件并重建改进清单：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/kb-recap.ps1 -FromResultFile ../reports/kb-recap-result.json -OutputFile ../reports/kb-recap-result.json -ReportFile ../docs/05-指标与评估/知识库命中率与引用率阶段复盘改进清单-$(Get-Date -Format yyyy-MM-dd).md
```

## 可靠性演练脚本
基础验证（go test + 单机上传 + 通知观测）：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/basic-e2e.ps1 -BaseUrl http://localhost:8082 -Token $env:API_AUTH_TOKEN -WaitTimeoutSec 90 -RequireNotification -OutputFile ../reports/basic-e2e-result.json -ReportFile ../docs/05-指标与评估/基础验证报告-$(Get-Date -Format yyyy-MM-dd).md
```
- 可选 `-WatchDir` 指定验证目录；不传时默认从仪表盘读取 `heroCopy.watchDirs[0]`。
- 若当前环境未配置通知渠道，可去掉 `-RequireNotification`，脚本将把“未观测到通知增量”视为非阻断。

指标巡检：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/check-metrics.ps1 -BaseUrl http://localhost:8082 -OutputFile ../reports/metrics.prom
```
阈值巡检（队列长度/上传失败率/AI 降级率）：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/check-metrics.ps1 -BaseUrl http://localhost:8082 -CheckThresholds -OutputFile ../reports/metrics.prom
```
说明：
- `-CheckThresholds` 开启阈值计算。
- `-FailOnWarning` 可将 Warning 也作为失败退出（便于 CI 门禁）。
- 退出码：`4=Critical 阈值触发`，`5=Warning 阈值触发且启用 -FailOnWarning`。

上传压测文件生成：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/upload-stress.ps1 -WatchDir D:\tmp\gwf-stress -Count 1000 -IntervalMs 20
```

上传链路压测与故障注入复盘（输出 JSON + Markdown）：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/upload-recap.ps1 -BaseUrl http://localhost:8082 -WatchDir D:\tmp\gwf-stress -FaultMode manual -SaturationCount 1200 -FaultCount 400 -RecoveryCount 300 -OutputFile ../reports/upload-recap-result.json -ReportFile ../docs/05-指标与评估/上传链路压测与故障注入报告-$(Get-Date -Format yyyy-MM-dd).md
```
`FaultMode` 支持：
- `none`：仅队列压测
- `manual`：手工执行故障注入/恢复
- `command`：通过 `-FaultStartCommand/-FaultRecoverCommand` 自动执行注入/恢复

AI 回放：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/ai-replay.ps1 -BaseUrl http://localhost:8082 -Token $env:API_AUTH_TOKEN -PathsFile ../docs/03-告警与AI/AI回放路径清单.txt -DegradedRatioTarget 0.20 -StructurePassRatioTarget 1.00 -ErrorClassCoverageTarget 1.00 -FailOnGate -OutputFile ../reports/ai-replay-result.json
```
可直接维护 `../docs/03-告警与AI/AI回放路径清单.txt`，样例场景矩阵参考 `../docs/03-告警与AI/AI回放样例集清单.md`。

AI 基线验证（固定样例回放，聚焦 `summary/severity/suggestions` 结构稳定性）：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/ai-baseline.ps1 -BaseUrl http://localhost:8082 -Token $env:API_AUTH_TOKEN -PathsFile ../docs/03-告警与AI/AI回放路径清单.txt -SummaryPassRatioTarget 1.00 -SeverityPassRatioTarget 1.00 -SuggestionsPassRatioTarget 1.00 -OutputFile ../reports/ai-baseline-result.json -ReportFile ../docs/05-指标与评估/AI基线验证报告-$(Get-Date -Format yyyy-MM-dd).md
```
离线复核模式（不访问服务，直接消费既有回放结果）：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/ai-baseline.ps1 -FromResultFile ../reports/ai-replay-result.json -OutputFile ../reports/ai-baseline-result.json
```
若回放结果缺少 `structure.*` 或 `results[].structureIssues`，脚本会判定“结构证据不足”并返回失败，避免旧口径结果误通过。

阶段预备（自动导入并发布知识库文档 + 生成 AI 回放样本并登记告警日志路径）：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/stage-prime.ps1 -BaseUrl http://localhost:8082 -Token $env:API_AUTH_TOKEN -DocsPath ../docs -Operator stage-recap -AIPathsFile ../reports/ai-replay-paths-prime.txt -OutputFile ../reports/stage-prime-result.json
```

控制面回放：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/control-replay.ps1 -BaseUrl http://localhost:8082 -Token $env:API_AUTH_TOKEN -AgentCount 3 -TaskCount 30 -OutputFile ../reports/control-replay-result.json -MetricsFile ../reports/metrics-control-replay.prom
```

阶段一键复盘（metrics + ai + control + kb，默认全量）：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/stage-recap.ps1 -BaseUrl http://localhost:8082 -Token $env:API_AUTH_TOKEN -AIPathsFile ../docs/03-告警与AI/AI回放路径清单.txt -AILimit 200 -AIDegradedRatioTarget 0.20 -AIStructurePassRatioTarget 1.00 -AIErrorClassCoverageTarget 1.00 -KBHitrateTarget 0.8 -CitationTarget 1.0 -KBMttdDropTarget 0.2 -OutputFile ../reports/stage-recap-result.json
```
按需跳过阶段可增加 `-SkipAIReplay` / `-SkipControlReplay` / `-SkipKBRecap`。
本地演练环境建议增加 `-AutoPrime`，自动补齐知识库与 AI 回放前置条件：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/stage-recap.ps1 -BaseUrl http://localhost:8082 -Token $env:API_AUTH_TOKEN -AutoPrime -AIPathsFile ../reports/ai-replay-paths-prime.txt -OutputFile ../reports/stage-recap-result.json
```

阶段报告自动生成（根据 stage-recap 结果渲染 Markdown）：
```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/stage-report.ps1 -RecapFile ../reports/stage-recap-result.json -OutputFile ../docs/05-指标与评估/阶段回归报告-$(Get-Date -Format yyyy-MM-dd).md -Operator "your-name" -Environment "dev-like (local)"
```
若后端未启用鉴权，`ai-replay.ps1` / `ai-baseline.ps1` / `stage-prime.ps1` / `control-replay.ps1` / `stage-recap.ps1` 均可省略 `-Token` 参数。

## 相关文档
- 文档导航：`../docs/文档导航.md`
- 平台总览与计划：`../docs/01-总览规划/`
- 开发与运维：`../docs/02-开发运维/`
- 告警与 AI：`../docs/03-告警与AI/`
- 知识库：`../docs/04-知识库/`
- 指标与评估：`../docs/05-指标与评估/`
- 架构附录：`../docs/99-架构附录/`

## 开发与测试
- 运行测试：`go test ./...`
- 代码格式：`gofmt`，遵循 Go 官方规范。


