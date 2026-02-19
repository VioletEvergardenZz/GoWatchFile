# GWF 运维平台

> 面向运维工程师 / SRE / 平台维护者的统一运维平台。  
> 当前版本已从“文件监控上传工具”演进为“控制、监控、决策、知识复用”平台。

## 1. 核心定位

### 1.1 平台主线能力

1. Agent 运行与管理（控制面）
2. 告警监控与决策
3. 系统/主机资源采集
4. 运维知识库
5. 平台化控制台能力

### 1.2 辅助能力

1. 文件监控与上传（输入适配器）
2. 通知通道（钉钉/邮件）
3. AI 增强分析（可开关、可降级）

## 2. 仓库结构与职责

- `go-watch-file/`：后端执行平面与统一 API（控制面、告警、资源采集、知识库、文件适配器）。
- `console-frontend/`：统一运维控制台（主控制台、告警、系统资源、知识库、控制面）。
- `docs/`：平台文档体系与专题手册。
- `reports/`：回放、压测、阶段复盘输出。
- `legacy/`：历史归档资料。

## 3. 快速开始

### 3.1 后端

```bash
cd go-watch-file
cp .env.example .env
go build -o bin/file-watch cmd/main.go
./bin/file-watch -config config.yaml
```

### 3.2 前端

```bash
cd console-frontend
npm install
npm run dev
```

默认前端会把 `/api` 代理到 `http://localhost:8080`。

### 3.3 Docker Compose（可选）

```bash
cp .env.example .env
docker compose up --build -d
```

- 前端：`http://localhost:8081`
- 后端：`http://localhost:8082`

## 4. 文档入口

- 文档中心：`docs/README.md`
- 文档导航：`docs/文档导航.md`
- 平台定位：`docs/01-平台定位/项目定位与主线能力.md`
- 架构说明：`docs/02-架构设计/总体架构说明.md`
- 模块说明：`docs/03-能力模块/功能模块说明.md`
- Roadmap：`docs/04-路线图与计划/Roadmap.md`
- TODO：`docs/04-路线图与计划/TODO.md`

## 5. 当前约束（与代码一致）

- 控制面与知识库目前是单实例本地 SQLite 实现。
- 权限模型当前以 Token 鉴权为主，尚未引入 RBAC。
- 文件采集仍是当前最成熟输入适配器，但不代表平台主线。

## 6. 历史与兼容

- 旧版专题文档仍在 `docs/02-开发运维/`、`docs/03-告警与AI/`、`docs/04-知识库/`、`docs/05-指标与评估/`。
- 新旧映射见 `docs/98-历史兼容/旧文档映射.md`。
