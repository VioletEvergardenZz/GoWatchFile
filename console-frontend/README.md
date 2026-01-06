# File Watch 控制台（前端）

React + TypeScript + Vite 单页应用，用于展示 Go 后端的仪表盘与操作入口（目录树/文件列表/上传记录/文件内容/配置更新）。

## 功能点（与当前代码一致）
- 目录树与自动上传开关（目录级联动子目录）。
- 文件列表筛选（自动/手动）与搜索。
- 上传记录与失败概览。
- 队列趋势与监控摘要图表。
- 文件内容（实时 Tail + 关键词检索，轮询 `/api/file-log`）。
- 运行时配置更新（`watchDir/fileExt/silence/workers/queue`）。

## 快速开始
```bash
cd console-frontend
npm install
npm run dev
```

默认通过 Vite 将 `/api` 代理到 `http://localhost:8080`。若后端地址不同可设置 `VITE_API_BASE`。

## 数据刷新策略
- 仪表盘：每 3 秒刷新一次（`DASHBOARD_POLL_MS=3000`）。
- 文件内容：实时 Tail 每 2 秒拉取一次（`LOG_POLL_MS=2000`），关键词检索为按需触发。
- 目录树与文件列表仅在首次加载或手动操作后全量刷新，避免频繁扫描。

## 目录结构
- `src/App.tsx`：主界面与交互逻辑。
- `src/types.ts`：与后端 `DashboardData` 对齐的类型定义。
- `src/mockData.ts`：默认占位数据；API 未连通时用于初始渲染。
- `src/App.css` / `src/index.css`：主题与布局样式。

## 接口对接说明
后端 API 由 `go-watch-file` 提供：
- `GET /api/dashboard`：仪表盘聚合数据。
- `POST /api/auto-upload`：切换自动上传。
- `POST /api/manual-upload`：手动触发上传。
- `POST /api/file-log`：读取文件内容（Tail/关键词检索）。
- `POST /api/config`：运行时更新配置。

字段与数据结构详见 `docs/state-types-visual.md`。
