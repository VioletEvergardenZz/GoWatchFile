# 运维事件分析与知识决策平台（GWF）

> 面向 SRE/运维的事件分析与决策平台。当前版本聚焦告警决策、AI 分析与知识复用闭环；文件监控上传能力作为可选采集器保留并持续维护。

## 平台定位（当前口径）
- 主线能力：告警决策闭环 + AI 分析闭环 + 知识库复用闭环。
- 输入能力：文件监控上传是当前已落地的输入适配器之一，不再是唯一主线。
- 演进策略：后续可根据新需求调整输入侧重点，但保持“可观测、可追溯、可回滚”原则不变。

## 当前能力
- 文件输入适配器：递归监控目录、自动发现新增子目录（fsnotify）。
- 文件输入治理：多后缀过滤、临时文件后缀过滤、写入完成判定（silence window）。
- 文件上传链路：worker pool 并发上传 OSS（默认内存队列，可选持久化恢复）。
- 上传可靠性：失败重试、饱和阈值、熔断限流（可配置）。
- 钉钉机器人/邮件通知（可选）。
- 告警决策：日志轮询、规则匹配、抑制/升级、告警概览与决策列表（规则由控制台维护并写入 `config.runtime.yaml`）。
- 控制台 API：仪表盘、目录树、文件列表、自动上传开关、手动上传、文件 Tail/检索、运行时配置、告警面板与配置、系统资源面板。
- Prometheus 指标接口：`/metrics`（事件速率、队列长度、上传耗时、错误分类、AI/知识库命中率）。
- 控制台 API 支持可选 Token 鉴权与 CORS 白名单（`/api/health` 始终匿名可访问）。
- 当 `API_AUTH_TOKEN` 为空、占位符（`${API_AUTH_TOKEN}`）或 `API_AUTH_DISABLED=true` 时，管理接口默认不校验 token。
- 仪表盘接口在运行态未就绪时会返回降级数据（`200`），避免前端直接出现 `500`。
- 路径安全：相对路径校验、防止目录穿越、对象 Key 归一化。
- 控制台前端：目录树、上传历史、队列趋势、Tail/检索、运行时配置、告警控制台、系统资源控制台。
- AI 日志分析（可选，需配置 AI_* 并启用）。
- AI 能力定位：作为平台主线能力持续建设，当前以“可配置开关 + 人工确认”为默认策略。

## 快速开始

### 后端（go-watch-file）
1) 环境：Go 1.24+（`go.mod` 含 `toolchain go1.24.3`，支持自动下载）。
2) 配置：
   ```bash
   cd go-watch-file
   cp .env.example .env
   # 填写密钥相关变量（OSS_AK/OSS_SK、DINGTALK_*、EMAIL_*）
   # API 鉴权可选：可设置 API_AUTH_TOKEN 开启鉴权
   # 如需显式关闭可设置 API_AUTH_DISABLED=true
   # 跨域来源建议设置 API_CORS_ORIGINS
   # 可选开启队列落盘：UPLOAD_QUEUE_PERSIST_ENABLED/UPLOAD_QUEUE_PERSIST_FILE
   # 如需覆盖 OSS 参数，可设置 OSS_BUCKET/OSS_ENDPOINT/OSS_REGION/OSS_FORCE_PATH_STYLE/OSS_DISABLE_SSL
   # 可选 AI 分析：AI_ENABLED/AI_BASE_URL/AI_API_KEY/AI_MODEL/AI_TIMEOUT/AI_MAX_LINES
   ```
   `config.yaml` 在密钥字段使用环境变量占位符，OSS 连接参数默认在配置文件，也可用环境变量覆盖；队列持久化开关属于静态配置（需重启）；其他配置通过控制台设置并持久化到 `config.runtime.yaml`。
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
若后端启用鉴权，请在控制台顶部输入 API Token（支持会话或本地保存，可随时清除）。
若后端未启用鉴权，可直接空 token 使用控制台。

### Docker Compose（可选）
```bash
cp .env.example .env
mkdir -p data/watch data/logs
# 如需固定监控目录 可将 go-watch-file/config.yaml 的 watch_dir 改为 /data/gwf/watch
# 前端镜像不注入鉴权口令；仅在后端启用鉴权时，启动后在控制台页面输入 API Token
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
- `AGENTS.md`：仓库协作与代理执行规范。

## 文档入口
- 总入口：`docs/文档导航.md`
- 总览规划：`docs/01-总览规划/`
- 开发运维：`docs/02-开发运维/`
- 告警与 AI：`docs/03-告警与AI/`
- 知识库：`docs/04-知识库/`
- 指标与评估：`docs/05-指标与评估/`
- 架构附录：`docs/99-架构附录/`

## 现阶段限制（与代码一致）
- 支持多监控目录（`watch_dir` 可用逗号或分号分隔），多后缀已支持。
- 默认上传队列在内存中，重启会清空；开启 `upload_queue_persist_enabled` 后可恢复未完成任务，持久化文件损坏时会自动备份并降级为空队列；不支持断点续传。
- 通知渠道有限：钉钉/邮件，企业微信等仅保留配置字段。
- 控制面为本地 API + 前端，不包含多 Agent 管理与编排。
- 系统资源面板默认关闭，需要在控制台开启后才能访问 `/api/system`。

如需了解规划内容，参考 `todo.md` 与 `大纲.md`。

维护：运维团队；旧版材料参考 `legacy/`。

