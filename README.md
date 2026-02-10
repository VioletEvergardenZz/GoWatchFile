# 通用文件监控与处理平台（Go Watch File / GWF）

> 面向 SRE/运维的本地文件监控与入云平台。当前版本聚焦 Go Agent + 本地 API + 控制台；路由/编排/多 Agent 等能力在路线图中。

## 当前能力
- 递归监控目录，自动发现新增子目录（fsnotify）。
- 多后缀过滤，可为空表示全量目录。
- 临时文件后缀过滤（如 `.tmp/.part/.crdownload`）。
- 写入完成判定（silence window，默认 10s，支持 `10s` / `10秒` / `10`）。
- worker pool 并发上传到 S3 兼容存储（默认内存队列，可选持久化恢复）。
- 上传失败重试（可开关，可配置重试间隔）。
- 钉钉机器人/邮件通知（可选）。
- 告警决策：日志轮询、规则匹配、抑制/升级、告警概览与决策列表（规则由控制台维护并写入 `config.runtime.yaml`）。
- 控制台 API：仪表盘、目录树、文件列表、自动上传开关、手动上传、文件 Tail/检索、运行时配置、告警面板与配置、系统资源面板。
- 控制台 API 支持 Token 鉴权（`/api/health` 例外）与 CORS 白名单。
- 路径安全：相对路径校验、防止目录穿越、对象 Key 归一化。
- 控制台前端：目录树、上传历史、队列趋势、Tail/检索、运行时配置、告警控制台、系统资源控制台。
- AI 日志分析（可选，需配置 AI_* 并启用）。

## 快速开始

### 后端（go-watch-file）
1) 环境：Go 1.23+（`go.mod` 含 `toolchain go1.24.3`，支持自动下载）。
2) 配置：
   ```bash
   cd go-watch-file
   cp .env.example .env
   # 填写密钥相关变量（S3_AK/S3_SK、DINGTALK_*、EMAIL_*）
   # 填写 API 安全变量（API_AUTH_TOKEN、API_CORS_ORIGINS）
   # 可选开启队列落盘：UPLOAD_QUEUE_PERSIST_ENABLED/UPLOAD_QUEUE_PERSIST_FILE
   # 如需覆盖 S3 参数，可设置 S3_BUCKET/S3_ENDPOINT/S3_REGION/S3_FORCE_PATH_STYLE/S3_DISABLE_SSL
   # 可选 AI 分析：AI_ENABLED/AI_BASE_URL/AI_API_KEY/AI_MODEL/AI_TIMEOUT/AI_MAX_LINES
   ```
   `config.yaml` 在密钥字段使用环境变量占位符，S3 连接参数默认在配置文件，也可用环境变量覆盖；队列持久化开关属于静态配置（需重启）；其他配置通过控制台设置并持久化到 `config.runtime.yaml`。
3) 启动：
   ```bash
   go build -o bin/file-watch cmd/main.go
   ./bin/file-watch -config config.yaml
   ```

### 前端（console-frontend）
```bash
cd console-frontend
npm install
npm run dev
```

默认通过 Vite 将 `/api` 代理到 `http://localhost:8080`。若后端地址不同，可设置 `VITE_API_BASE`。
如后端开启鉴权，可在前端环境变量中设置 `VITE_API_TOKEN`。

### Docker Compose（可选）
```bash
cp .env.example .env
mkdir -p data/watch data/logs
# 如需固定监控目录 可将 go-watch-file/config.yaml 的 watch_dir 改为 /data/gwf/watch
# 前端镜像会在构建时注入 API_AUTH_TOKEN 作为 VITE_API_TOKEN
docker compose up --build -d
```

访问地址：
- 后端 API：`http://localhost:8082`
- 前端控制台：`http://localhost:8081`

停止：
```bash
docker compose down
```

## 仓库结构
- `go-watch-file/`：Go Agent 源码、配置模板与脚本。
- `console-frontend/`：控制台前端（React + TS + Vite）。
- `docs/`：概述、流程图、开发指南、FAQ、数据结构说明。
- `legacy/`：旧版 OOM 方案归档。
- `todo.md` / `大纲.md`：路线图与蓝图说明。

## 文档入口
- 平台概述：`docs/overview.md`
- 开发指南：`docs/dev-guide.md`
- 前后端联动：`docs/frontend-backend-linkage.md`
- 队列与 worker：`docs/queue-worker-flow.md`
- 队列持久化运行手册：`docs/queue-persistence-runbook.md`
- DTO 结构：`docs/state-types-visual.md`
- 告警模式：`docs/alert-mode.md`
- 告警控制台：`docs/alert-console.md`
- FAQ：`docs/faq.md`

## 现阶段限制（与代码一致）
- 支持多监控目录（`watch_dir` 可用逗号或分号分隔），多后缀已支持。
- 默认上传队列在内存中，重启会清空；开启 `upload_queue_persist_enabled` 后可恢复未完成任务，持久化文件损坏时会自动备份并降级为空队列；不支持断点续传。
- 通知渠道有限：钉钉/邮件，企业微信等仅保留配置字段。
- 控制面为本地 API + 前端，不包含多 Agent 管理与编排。
- 系统资源面板默认关闭，需要在控制台开启后才能访问 `/api/system`。

如需了解规划内容，参考 `todo.md` 与 `大纲.md`。

维护：运维团队；旧版材料参考 `legacy/`。
