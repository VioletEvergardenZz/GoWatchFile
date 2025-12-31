# File Watch Service

一个通用的文件监控与处理服务：递归监听目录、过滤/匹配文件、写入完成后并行上传到 S3 兼容存储，并通过钉钉机器人通知。内置控制台 API 用于目录树/文件列表/上传记录/日志 Tail 与运行时配置。

## 能力概览
- 目录监听：fsnotify 递归监控，自动发现新建子目录，后缀可留空表示不过滤。
- 写入完成判定：静默窗口（silence）避免半截文件。
- 上传流水线：工作池并发 + 队列背压，防止积压。
- 通知告警：钉钉机器人推送上传结果（webhook/secret 可选，`robot_key` 预留）。
- 控制台 API：Dashboard、自动上传开关、手动上传、文件日志 Tail、运行时配置更新。
- 配置管理：YAML + `.env` + 环境变量覆盖，内置默认值与严格校验。

## 快速上手
1) 环境：Go 1.23+；可访问 S3 兼容存储；可选钉钉机器人。  
2) 复制并填写环境变量  
   ```bash
   cp .env.example .env
   # 按需填充存储凭证与通知配置
   ```
3) 配置监控目录与文件类型  
   编辑 `config.yaml`，主要关心：  
   - `watch_dir`: 需要监控的根目录（必填）  
   - `file_ext`: 目标文件后缀（可留空表示不过滤，如 `.log` / `.txt` / `.zip`）  
   - `silence`: 写入完成静默窗口（默认 10s，可写 `5s` / `10秒`）  
   其他字段保持 `${ENV}` 占位符，用 `.env` 或真实环境变量注入。  
4) 运行  
   ```bash
   go build -o file-watch cmd/main.go
   ./file-watch -config config.yaml
   ```
5) 停止：终端 `Ctrl + C`，服务会优雅退出并等待队列 drain。  

环境变量优先级：真实环境变量 > `.env` > `config.yaml` 占位符 > 内置默认值。

## 配置与环境变量
`config.yaml` 仅保留占位符；实际值放在 `.env` 或外部环境：
```yaml
watch_dir: "${WATCH_DIR}"
file_ext: "${FILE_EXT}"
silence: "${SILENCE_WINDOW}"
robot_key: "${ROBOT_KEY}"
dingtalk_webhook: "${DINGTALK_WEBHOOK}"
dingtalk_secret: "${DINGTALK_SECRET}"
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
```

`.env.example` 提供模板（复制为 `.env` 后填写）：
```
WATCH_DIR=/path/to/watch
FILE_EXT=.log
SILENCE_WINDOW=10s
DINGTALK_WEBHOOK=https://oapi.dingtalk.com/robot/send?access_token=your-token
DINGTALK_SECRET=your-dingtalk-secret
S3_BUCKET=your-bucket
S3_AK=your-ak
S3_SK=your-sk
S3_ENDPOINT=s3.example.com
S3_REGION=us-east-1
S3_FORCE_PATH_STYLE=false
S3_DISABLE_SSL=false
LOG_LEVEL=info
LOG_FILE=logs/file-watch.log
LOG_TO_STD=true
LOG_SHOW_CALLER=false
UPLOAD_WORKERS=3
UPLOAD_QUEUE_SIZE=100
API_BIND=:8080
```

关键校验与默认策略：
- `watch_dir` 必须存在且为目录；`file_ext` 可为空但不为空时必须以 `.` 开头。
- 日志级别仅允许 `debug|info|warn|error`。
- 上传并发/队列默认值为 3/100，可被环境变量覆盖。
- API 默认监听 `:8080`，可用 `API_BIND` 覆盖。

## HTTP API
- `GET /api/dashboard`：控制台聚合数据。  
- `POST /api/auto-upload`：切换自动上传（`{ "path": "...", "enabled": true }`，支持目录）。  
- `POST /api/manual-upload`：手动触发一次上传（`{ "path": "..." }`）。  
- `POST /api/file-log`：读取文件尾部日志（仅文本，最多 512KB/500 行）。  
- `POST /api/config`：更新运行时配置（`watchDir`/`fileExt`/`silence`/`uploadWorkers`/`uploadQueueSize`）。  
- `GET /api/health`：返回队列与 worker 数（`{ "queue": n, "workers": n }`）。  

说明：  
- `path` 必须位于 `watch_dir` 下，建议使用控制台返回的完整路径。  
- `/api/config` 仅更新运行时参数，S3 与通知配置仍需改配置文件并重启。  

## 架构与模块
- `api/`：控制台 API 服务。  
- `service/`：业务协调层（监控/上传/通知/状态）。  
- `state/`：运行态数据与 Dashboard DTO。  
- `watcher/`：文件系统监听与静默窗口处理。  
- `upload/`：工作池消费队列并上传至 S3。  
- `s3/`：S3 兼容存储客户端封装。  
- `pathutil/`：路径归一化、相对路径校验、对象 Key/下载 URL 生成。  
- `dingtalk/`：钉钉机器人通知。  
- `config/`：配置加载（YAML + env）、默认值填充、强校验。  
- `logger/`：级别化日志，支持文件/标准输出。  

## 设计要点
- 配置加载分层：读取 → 环境变量覆盖 → 默认值 → 强校验，避免启动时潜在配置缺失。  
- 三态布尔处理：日志输出等布尔值通过指针区分“未配置 / false / true”。  
- 安全路径：相对路径解析防止目录穿越，生成的 S3 Key 统一去除前导 `/`。  
- 并发与背压：上传工作池 + 队列大小可调，避免流量突增时阻塞监控线程。  

## 相关文档
- 平台概述：`../docs/overview.md`
- 流程图：`../docs/system-flowchart.md`
- 开发指南：`../docs/dev-guide.md`
- 常见问题：`../docs/faq.md`

## 开发与测试
- 运行测试：`go test ./...`
- 代码格式：`gofmt`，遵循 Go 官方规范。