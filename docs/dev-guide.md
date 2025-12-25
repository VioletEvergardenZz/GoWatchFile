# 开发指南

## 环境准备
- Go 1.21+。
- S3 兼容对象存储（本地可用 MinIO）。
- 企业微信/钉钉机器人（可选）。

## 本地启动
```bash
cd go-watch-file
cp .env.example .env
# 根据注释填写 WATCH_DIR、FILE_EXT、S3、通知等配置

# 构建并运行
go build -o bin/file-watch cmd/main.go
./bin/file-watch -config config.yaml
```

## 配置要点
- `watch_dir` 必须存在且为目录。
- `file_ext` 仅支持单一后缀（如 `.log` / `.txt` / `.zip`）。
- 配置优先级：环境变量 > `.env` > `config.yaml` 占位符 > 默认值。

## 运行测试
```bash
cd go-watch-file
go test ./...
```

## 单机端到端验证（上传 + 通知）
1) 准备对象存储
- 本地 MinIO 示例：创建 Bucket，并记录 `S3_ENDPOINT`/`S3_BUCKET`。
- 若使用 MinIO：`S3_FORCE_PATH_STYLE=true`，必要时 `S3_DISABLE_SSL=true`。

2) 启动 Agent
- `WATCH_DIR` 指向本地测试目录（确保可写）。

3) 写入测试文件
```bash
# 在 watch_dir 下创建匹配后缀的文件
cp sample.log <watch_dir>/app-a/sample.log
```

4) 验证结果
- 日志出现“文件写入完成 / 上传成功”。
- 对象存储中出现对应对象 Key。
- 若配置机器人，收到通知消息。
