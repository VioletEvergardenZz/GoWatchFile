# 通用文件监控管理系统（File Watch & Processing）

> 将原来的 Java 堆转储分析方案演进为**通用文件监控与处理管道**。当前核心是 Go Agent（`go-watch-file`）：递归监听目录、按后缀过滤、判定写入完成后异步上传到 S3 兼容存储，并可触发 Jenkins 任务与企业微信/钉钉通知。后续会补充控制面、策略编排和 Web 控制台，支持更多文件类型与处理场景。

## 能力概览
- **目录监控**：基于 fsnotify 递归监听，自动发现新建子目录，按指定后缀过滤目标文件。  
- **写入完成判定**：10s 静默窗口确认文件写入结束，避免上传半截文件。  
- **异步上传**：可配置的并发 + 队列背压（默认 3 worker / 100 队列），上传至 S3 兼容存储（AWS/MinIO/OSS/COS）。  
- **动作触发**：上传后触发 Jenkins Job（参数：`DOWNLOAD_FILE`、`APP`、`FILE_NAME`），用于后续加工/分析；可扩展为 Webhook 或自定义处理器。  
- **通知告警**：企业微信/钉钉机器人推送上传结果或异常。  
- **路径规范**：严格的相对路径校验与对象 Key 生成，防止目录穿越，自动生成下载 URL。  
- **配置管理**：`config.yaml` + `.env` + 环境变量覆盖，内置默认值与严格校验，便于不同环境落地。  

典型场景：日志归档、业务落地文件入云、ETL 入口、模型产物推送、图片/文档收集等。

## 仓库结构
- `go-watch-file/`：Go Agent，提供监听、上传、Jenkins 触发与通知能力（当前核心）。  
- `jenkins-job/`：Jenkins Job 示例（可按需改造成通用处理流水线）。  
- `ai-analysis/`：示例型 Python 处理模块，可演化为自定义处理器模板。  
- `server-ui-prototype.html`：早期 Web 管理界面原型，可用于后续控制台设计参考。  
- `system-flowchart.md`：旧版系统流程图，可在重构时参考结构化思路。  

## 快速开始（Agent）
1) 准备环境  
   - Go 1.21+，网络可访问 S3 兼容存储。  
   - Jenkins 账户（如需触发），企业微信/钉钉机器人（如需通知）。  
2) 配置  
   ```bash
   cd go-watch-file
   cp .env.example .env
   # 填写 watch_dir、file_ext、S3/账号、Jenkins/通知等
   ```
   核心字段：
   - `watch_dir`：要监听的根目录，必须存在。  
   - `file_ext`：目标后缀，如 `.log` / `.txt` / `.hprof`。  
   - 存储：`S3_BUCKET`、`S3_AK`、`S3_SK`、`S3_ENDPOINT`、`S3_REGION`、`S3_FORCE_PATH_STYLE`、`S3_DISABLE_SSL`。  
   - Jenkins：`JENKINS_HOST`、`JENKINS_USER`、`JENKINS_PASSWORD`、`JENKINS_JOB`。  
   - 通知：`ROBOT_KEY`（企微）、`DINGTALK_WEBHOOK`、`DINGTALK_SECRET`。  
   - 日志与并发：`LOG_LEVEL`、`LOG_FILE`、`LOG_TO_STD`、`UPLOAD_WORKERS`、`UPLOAD_QUEUE_SIZE`。  
3) 运行  
   ```bash
   go build -o bin/file-watch cmd/main.go
   ./bin/file-watch -config config.yaml
   # Ctrl+C 可优雅退出，确保队列 drain
   ```
4) 可选：容器化  
   ```bash
   ./docker-build.sh
   # 或自定义 Dockerfile/Helm，挂载 config.yaml 与 .env
   ```

配置优先级：环境变量 > `.env` > `config.yaml` 占位符 > 内置默认值。  

## 工作流
1. 监听 `watch_dir`（递归），捕获满足 `file_ext` 的写入/创建事件。  
2. 进入 10s 静默检测，确认文件写入完成。  
3. 投递到上传队列；worker 并发上传至 S3 兼容存储，生成下载链接。  
4. 触发 Jenkins Job（可替换为 Webhook/自定义处理器），透传下载链接、应用名、文件名。  
5. 通过企业微信/钉钉发送结果通知；失败会记录日志。  
6. 队列与 worker 可按需调整以应对突发流量。  

## 运维与排障
- 日志：默认 `logs/`（或按 `LOG_FILE` 配置），支持标准输出，`LOG_LEVEL=debug` 便于排查。  
- 测试：`cd go-watch-file && go test ./...`。  
- 观测：当前提供队列长度与 worker 数（`UploadStats`）；后续将补充 Prometheus 指标。  
- 常见问题：  
  - 未触发上传：确认 `watch_dir` 存在且后缀匹配；观察 10s 静默窗口是否结束。  
  - 上传失败：检查 S3 访问与凭证；确认 `S3_ENDPOINT`、`S3_REGION`、`force_path_style`。  
  - Jenkins 未执行：核对 Jenkins 账号、Job 名与网络连通性。  
  - 通知未达：检查机器人配置与出网权限。  

## 路线图（vNext）
1) Agent 强化  
   - 队列/worker 指标暴露（Prometheus），限流与告警阈值。  
   - 断点续传/失败重试策略，文件完整性校验（size/etag）。  
   - 更灵活的文件匹配（多后缀、路径规则、忽略列表）。  
2) 控制面 & API  
   - 统一的文件事件/任务模型（FileSource、FileEvent、Task、Action）。  
   - REST/gRPC API：文件状态查询、重试、手动触发、Agent 心跳。  
   - 基础存储：事件/任务/通知流水落库（MySQL/PostgreSQL/SQLite）。  
3) 处理链路与插件  
   - 动作类型扩展：HTTP Webhook、消息队列、脚本执行、本地/远端处理器。  
   - 规则编排：按路径/标签/大小/来源选择处理链路；重试与补偿策略。  
4) Web 控制台  
   - 任务列表、事件时间线、告警面板、Agent 状态、配置管理。  
   - 访问控制与审计。  
5) 发布与部署  
   - Docker/Helm/Compose 样例；一键启动开发环境。  
   - 配置模板与最佳实践（多环境、出网/离线场景）。  

## 历史与状态
- 旧版定位：Java 堆转储自动化分析（MAT + AI），相关文档仍保留在 `java-heapdump-automation-system.md` 供参考。  
- 现阶段：以通用文件监控/上传/触发为核心，逐步演进为可扩展的文件处理平台。  
- 建议先稳定 Agent（go-watch-file），再补齐控制面与可观测性。  

—

维护：运维团队（计划重构为 vNext 通用版）
更新时间：2025-12-25
