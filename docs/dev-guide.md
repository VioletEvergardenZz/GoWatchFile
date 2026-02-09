# 开发指南

## 环境准备
- Go 1.23+（`go.mod` 含 `toolchain go1.24.3`）。
- Node.js 18+（前端开发）。
- S3 兼容对象存储（本地可用 MinIO/OSS/COS）。
- 钉钉机器人/SMTP 邮件（可选）。

## 本地启动（后端）
```bash
cd go-watch-file
cp .env.example .env
# config.yaml 放非密钥配置 密钥放在 .env
# watch_dir/告警配置建议在控制台配置并持久化

go build -o bin/file-watch cmd/main.go
./bin/file-watch -config config.yaml
```

### 配置要点
- `watch_dir` 为空时由控制台配置；如已设置则必须存在且为目录。
- `watch_dir` 支持多目录（逗号或分号分隔）。
- `file_ext` 支持多后缀（逗号或空格分隔），可留空表示不过滤。
- 临时文件后缀会被忽略（如 `.tmp/.part/.crdownload`）。
- `silence` 默认 `10s`，支持 `10s` / `10秒` / `10`。
- `upload_retry_enabled`/`upload_retry_delays` 可控制上传重试开关与间隔（默认 `1s,2s,5s`）。
- `S3_ENDPOINT` 可带协议或不带协议（如 `https://s3.example.com` 或 `s3.example.com`）。
- `S3_FORCE_PATH_STYLE=true` 适配 MinIO 等场景。
- `system_resource_enabled` 默认 `false`，需在控制台开启后才能访问 `/api/system`。

### 环境变量覆盖范围
- 仅会覆盖 S3 / 通知 / AI 相关字段（`S3_*`、`DINGTALK_*`、`EMAIL_*`、`ROBOT_KEY`、`AI_*`）。
- `watch_dir` / `file_ext` / `watch_exclude` / `log_level` / `alert_*` 不会被环境变量覆盖。

### 告警模式配置要点
- 告警规则与日志路径可在控制台配置，并在可写时持久化到 `config.runtime.yaml`。
- `alert_rules` 只在 `config.runtime.yaml` 维护，规则更新需点击“保存规则”。
- `alert_start_from_end=true` 表示只处理新写入日志，避免历史告警。
- `alert_suppress_enabled=false` 可关闭抑制，所有命中都会发送通知。

### 本地 MinIO 示例（示意）
- `S3_ENDPOINT=127.0.0.1:9000`
- `S3_FORCE_PATH_STYLE=true`
- `S3_DISABLE_SSL=true`

## 启动前端
```bash
cd console-frontend
npm install
npm run dev
```

前端默认通过 Vite 代理 `/api` 到 `http://localhost:8080`；若后端地址不同可设置 `VITE_API_BASE`。

## Docker Compose 启动（可选）
```bash
cp .env.example .env
mkdir -p data/watch data/logs
# 如需固定监控目录 可将 go-watch-file/config.yaml 的 watch_dir 改为 /data/gwf/watch
docker compose up --build -d
```

访问地址：
- 后端 API：`http://localhost:8080`
- 前端控制台：`http://localhost:8081`

停止：
```bash
docker compose down
```

## 运行时配置更新
控制台保存配置会调用 `/api/config`，仅更新：
- `watchDir` / `fileExt` / `silence`
- `uploadWorkers` / `uploadQueueSize`
- `uploadRetryEnabled` / `uploadRetryDelays`
- `systemResourceEnabled`

S3 连接参数可在 `config.yaml` 或 `.env` 中设置，密钥配置在 `.env`，变更后需重启后端。

告警配置通过 `/api/alert-config` 更新，实时生效，且在可写时持久化到 `config.runtime.yaml`。
告警规则通过 `/api/alert-rules` 保存并写入 `config.runtime.yaml`。

## 文件内容读取与检索
- `POST /api/file-log` 读取文件尾部（最多 512KB / 500 行）。
- 传入 `query` 可进行全文检索（最多 2000 行），支持 `limit` 与 `caseSensitive`。

## AI 日志分析
- `POST /api/ai/log-summary` 触发 AI 总结（需启用 `ai_enabled` 并配置 `AI_*`）。

## 仪表盘轻量刷新
- `GET /api/dashboard?mode=light` 或 `mode=lite` 返回不含目录树与文件列表的轻量数据，适合高频轮询。

## 运行测试
```bash
cd go-watch-file
go test ./...
```

## 常见开发验证
- 新建文件后等待静默窗口结束，再观察上传与通知。
- 通过 `/api/dashboard` 验证目录树、上传记录与队列趋势。
- 通过 `/api/alerts` 验证告警概览与决策列表。
- `LOG_LEVEL=debug` 便于追踪 watcher 与 queue 行为。
