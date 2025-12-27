# File Watch Service

一个通用的文件监控与处理服务：监听目录、过滤/匹配文件、并行上传到 S3 兼容存储，并通过企业微信/钉钉机器人通知。适用于日志归档、落地文件入云、模型产物分发等场景。

## 能力概览
- 目录监听：递归监控指定目录，基于后缀过滤目标文件。
- 上传流水线：S3 兼容存储上传，工作池并发 + 队列限流，防止积压。
- 通知告警：企业微信 / 钉钉机器人推送结果或异常。
- 路径规范：统一的相对路径与对象 Key 生成，避免路径穿越和重复。
- 配置管理：YAML + 环境变量覆盖，内置默认值与严格校验，支持 `.env`。

## 快速上手
1) 复制并填写环境变量  
   ```bash
   cp .env.example .env
   # 按需填充存储凭证与通知配置
   ```
2) 配置监控目录与文件类型  
   编辑 `config.yaml`，主要关心：
   - `watch_dir`: 需要监控的根目录
   - `file_ext`: 目标文件后缀（仅支持单一后缀，如 `.log` / `.txt` / `.zip`）
   其他字段保持 `${ENV}` 占位符，用 `.env` 或真实环境变量注入。
3) 运行
   ```bash
   go build -o file-watch cmd/main.go
   ./file-watch -config config.yaml
   ```
4) 停止：终端 `Ctrl + C`，服务会优雅退出。

环境变量优先级：真实环境变量 > `.env` > `config.yaml` 占位符 > 内置默认值。

## 配置与环境变量
`config.yaml` 仅保留占位符；实际值放在 `.env` 或外部环境：
```yaml
watch_dir: "${WATCH_DIR}"
file_ext: "${FILE_EXT}"
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
S3_BUCKET=your-bucket
S3_AK=your-ak
S3_SK=your-sk
S3_ENDPOINT=s3.example.com
S3_REGION=us-east-1
LOG_LEVEL=info
LOG_FILE=logs/file-watch.log
LOG_TO_STD=true
LOG_SHOW_CALLER=false
UPLOAD_WORKERS=3
UPLOAD_QUEUE_SIZE=100
API_BIND=:8080
```

关键校验与默认策略：
- `watch_dir` 必须存在且为目录；`file_ext` 必须以 `.` 开头。
- 日志级别仅允许 `debug|info|warn|error`。
- 上传并发/队列为容错默认（3/100），可被环境变量覆盖。
- API 默认监听 `:8080`，用于控制台/调试数据查询，可用 `API_BIND` 覆盖。

## 架构与模块
- `watcher/`：文件系统监听，过滤后缀，推送事件到队列。
- `upload/`：工作池消费队列并上传至 S3 兼容存储。
- `pathutil/`：路径归一化、相对路径校验、对象 Key/下载 URL 生成。
- `wechat/` / `dingtalk/`：机器人通知发送。
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
