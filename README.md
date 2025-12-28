# 通用文件监控与处理平台（File Watch & Processing）

> 面向 SRE/运维的多应用、多目录“文件入云 + 路由 + 自动处理”平台。当前核心为 Go Agent（`go-watch-file`），负责监听/过滤/上传与通知；控制面、路由编排、可视化与分析按路线图演进。旧版 OOM 材料归档于 `legacy/oom/`。

## 目标与场景
- 多主机/多应用：日志、转储、归档/媒体文件自动入云与集中管理  
- 自动处理：上传后可扩展 Webhook/队列等后续流程（当前不使用 Jenkins）  
- 观测与管理：事件时间线、成功率/滞留、告警、日志检索（规划中）  

## 模块与职责（蓝图）
- **Agent 采集**：inotify/轮询，过滤后缀/路径，检测写入完成并推送事件与内容。  
- **控制与配置**：集中配置/下发，多 Agent 注册与分组，目标存储/策略管理。  
- **文件路由与处理**：策略引擎分流到不同桶/队列/动作；支持解压、预处理、脚本/Webhook。  
- **上传传输**：并发控制、重试/断点、进度与错误监控，失败可重传。  
- **存储与索引**：对象存储承载原始文件；元数据/事件/任务落库并可检索报表。  
- **可视化界面**：仪表盘、事件时间线、失败/滞留列表、日志查看（tail/全文检索）。  
- **报警与通知**：失败/滞留/SLA 告警，邮件/钉钉/Webhook，支持重试与抑制。  

## 当前交付（Agent）
- **目录监控**：基于 fsnotify 递归监听，自动发现子目录，按后缀过滤。  
- **写入完成判定**：静默窗口确认写入结束，避免半截文件。  
- **异步上传**：工作池并发 + 队列背压，上传至 S3 兼容存储（AWS/OSS/MinIO/COS）。  
- **通知告警**：企业微信/钉钉机器人推送成功/异常。  
- **路径与安全**：相对路径校验，统一对象 Key/下载 URL 生成，防止目录穿越。  
- **配置管理**：`config.yaml` + `.env` + 环境变量覆盖，严格校验与默认值。  
- **观测（当前）**：上传队列与 worker 统计；控制台概览卡片基于“当日”上传/失败/通知次数。Prometheus 指标规划中。  

## 快速开始（go-watch-file Agent）
1) 环境：Go 1.21+；可访问 S3 兼容存储；可选 企微/钉钉机器人。  
2) 配置  
   ```bash
   cd go-watch-file
   cp .env.example .env
   # 填写 watch_dir、file_ext、S3、通知等
   ```
   关键字段：`watch_dir`、`file_ext`（单后缀）、`silence`/`SILENCE_WINDOW`（写入完成静默窗口，默认 10s，可填 `5s` 等），S3 凭证与 endpoint、通知 Webhook、`UPLOAD_WORKERS`、`UPLOAD_QUEUE_SIZE`。  
3) 运行  
   ```bash
   go build -o bin/file-watch cmd/main.go
   ./bin/file-watch -config config.yaml
   # Ctrl+C 优雅退出，等待队列 drain
   ```
4) 测试：`cd go-watch-file && go test ./...`。  
5) 容器化：`./docker-build.sh` 或自定义 Dockerfile/Helm，挂载 `config.yaml` 与 `.env`。  

配置优先级：环境变量 > `.env` > `config.yaml` 占位符 > 内置默认值。  

## 前后端联调（Console）
1) 启动 `go-watch-file`（默认 API 监听 `:8080`，可用 `API_BIND` 覆盖）。  
2) 前端：`cd console-frontend && npm install && npm run dev`，Vite 将 `/api` 代理到 `http://localhost:8080`；若后端地址不同可设置 `VITE_API_BASE`。  
3) 访问 `http://localhost:5173`，目录树/文件列表/上传记录/图表等数据来自后端 API，切换自动上传或手动上传会实时调用接口并刷新视图。  

## 仓库结构
- `go-watch-file/`：Go Agent 源码、配置模板与脚本。  
- `console-frontend/`：File Watch 控制台前端（React + TS + Vite），基于 `docs/prototype` 原型。  
- `docs/`：概述、流程图、开发指南、FAQ。  
- `legacy/`：旧版 OOM 方案归档。  
- `大纲.md`：平台蓝图与模块说明。  

## 迭代路径（与大纲对齐）
- **MVP**：单目录/单后缀 → 上传 → 通知（当前阶段无 Jenkins 触发）。  
- **多源/多 Agent**：多后缀与忽略规则，Agent 分组与配置中心下发，上传可靠性增强。  
- **可视化与告警**：仪表盘、事件时间线、失败/滞留列表、SLA 告警、日志查看/检索。  
- **编排与路由**：规则/工作流驱动的多桶/多队列分流与处理链路。  
- **分析与报表**：日/周趋势、容量/成本预测、异常检测与报表。  

## 运维与排障
- 日志：默认写入 `logs/`（或按 `LOG_FILE`），`LOG_LEVEL=debug` 便于排查。  
- 常见问题：`docs/faq.md`。  
- 建议：先稳固 Agent（可靠性、观测、配置），再逐步补齐控制面、编排与可视化。  

维护：运维团队；旧版材料参考 `legacy/oom/`。
