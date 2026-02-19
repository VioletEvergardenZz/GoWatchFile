# 统一运维控制台（console-frontend）

`console-frontend` 是 GWF 平台的统一操作入口，负责把后端能力组织成可执行的运维界面，而不是只做文件上传看板。

## 在平台中的定位

- 承载多视图控制台：
  - 主控制台（文件与运行态总览）
  - 告警控制台
  - 系统资源控制台
  - 运维知识库控制台
  - 控制面控制台（Agent/Task/Audit）
- 统一对接 `go-watch-file` 提供的 API。
- 提供值班可用的实时刷新、筛选和处置入口。

## 主要能力（与当前代码一致）

- 仪表盘：目录树、上传记录、队列趋势、文件检索、运行时配置。
- 告警：告警概览、决策列表、规则与配置管理、知识推荐联动。
- 系统资源：CPU/内存/磁盘/进程视图与进程终止操作。
- 知识库：条目管理、检索、问答、审核与回滚流程。
- 控制面：Agent 状态、任务队列、任务事件、审计日志、取消/重试操作。

## 快速开始

```bash
cd console-frontend
npm install
npm run dev
```

默认通过 Vite 将 `/api` 代理到 `http://localhost:8080`。  
若后端地址不同，可设置 `VITE_API_BASE`。

## 鉴权

- 若后端启用 `API_AUTH_TOKEN`，可在控制台顶部输入 Token。
- 若后端关闭鉴权（`API_AUTH_TOKEN` 为空或 `API_AUTH_DISABLED=true`），可直接访问。

## 目录结构

- `src/App.tsx`：应用入口与视图路由。
- `src/OriginalConsole.tsx`：主控制台编排。
- `src/AlertConsole.tsx`：告警控制台。
- `src/SystemConsole.tsx`：系统资源控制台。
- `src/KnowledgeConsole.tsx`：知识库控制台。
- `src/ControlConsole.tsx`：控制面控制台。
- `src/console/dashboardApi.ts`：前后端 API 封装。
- `src/types.ts`：统一类型定义。

## 对应文档

- 文档中心：`../docs/README.md`
- 平台定位：`../docs/01-平台定位/项目定位与主线能力.md`
- 总体架构：`../docs/02-架构设计/总体架构说明.md`
- 模块说明：`../docs/03-能力模块/功能模块说明.md`
