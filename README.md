# 通用文件监控与处理平台（Go Watch File / GWF）

> 面向 SRE/运维的本地文件监控与入云平台。当前版本聚焦 Go Agent + 本地 API + 控制台；路由/编排/多 Agent 等能力在路线图中。

## 当前能力
- 递归监控目录，自动发现新增子目录（fsnotify）。
- 多后缀过滤，可为空表示全量目录。
- 写入完成判定（silence window，默认 10s，支持 `10s` / `10秒` / `10`）。
- 内存队列 + worker pool 并发上传到 S3 兼容存储。
- 钉钉机器人通知（可选）。
- 控制台 API：仪表盘、目录树、文件列表、自动上传开关、手动上传、文件 Tail、运行时配置更新。
- 路径安全：相对路径校验、防止目录穿越、对象 Key 归一化。
- 控制台前端：目录树、上传历史、队列趋势、Tail 查看、运行时配置。

## 快速开始

### 后端（go-watch-file）
1) 环境：Go 1.23+（`go.mod` 含 `toolchain go1.24.3`，支持自动下载）。
2) 配置：
   ```bash
   cd go-watch-file
   cp .env.example .env
   # 填写 WATCH_DIR、FILE_EXT、S3、DINGTALK 等
   ```
   `config.yaml` 使用环境变量占位符，实际值来自 `.env` 或系统环境变量。
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
- DTO 结构：`docs/state-types-visual.md`
- FAQ：`docs/faq.md`

## 现阶段限制（与代码一致）
- 单一监控目录（`watch_dir`），多后缀已支持。
- 上传队列在内存中，重启会清空；没有自动重试与断点续传。
- 仅支持钉钉通知，企业微信等仅保留配置字段。
- 控制面为本地 API + 前端，不包含多 Agent 管理与编排。

如需了解规划内容，参考 `todo.md` 与 `大纲.md`。

维护：运维团队；旧版材料参考 `legacy/`。
