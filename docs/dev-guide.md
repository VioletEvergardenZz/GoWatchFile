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
# Configure config.yaml for non-secret settings; secrets live in .env; watch_dir/alert config are set in the console.

go build -o bin/file-watch cmd/main.go
./bin/file-watch -config config.yaml
```

### 配置要点
- `watch_dir` 为空时由控制台配置；如已设置则必须存在且为目录。
- `watch_dir` 支持多目录（逗号或分号分隔）。
- `file_ext` 支持多后缀（逗号或空格分隔），可留空表示不过滤。
- `silence`/`SILENCE_WINDOW` 默认 `10s`，支持 `10s` / `10秒` / `10`。
- `S3_ENDPOINT` 可带协议或不带协议（如 `https://s3.example.com` 或 `s3.example.com`）。
- `S3_FORCE_PATH_STYLE=true` 适配 MinIO 等场景。

### 告警模式配置要点
- alert rules/log paths are configured in the console and persisted to `config.runtime.yaml` when possible.
- `alert_start_from_end=true` 表示只处理新写入日志，避免历史告警。
- `alert_suppress_enabled=false` 可关闭抑制，所有命中都会发送通知。
- 规则文件支持热加载，修改后在下次轮询生效。

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

## 运行时配置更新
控制台保存配置会调用 `/api/config`，仅更新：
- `watchDir` / `fileExt` / `silence`
- `uploadWorkers` / `uploadQueueSize`

S3 连接参数可在 `config.yaml` 或 `.env` 中设置，密钥配置在 `.env`，变更后需重启后端。

告警配置通过 `/api/alert-config` 更新，实时生效，且在可写时持久化到 `config.runtime.yaml`。

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
