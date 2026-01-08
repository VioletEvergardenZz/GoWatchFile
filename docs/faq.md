# 常见问题（FAQ）

## 1. 文件创建后没有触发上传
- 确认 `watch_dir` 存在且 Agent 有权限。
- 确认文件后缀与 `file_ext` 匹配（不区分大小写）。
- 写入完成判定需要等待静默窗口（默认约 10 秒）。
- 控制台中该目录是否关闭了“自动上传”。

## 2. 支持多个后缀吗？
支持多个后缀（如 `.log,.txt`），逗号或空格分隔。

## 3. 上传队列提示满了或任务堆积
- `UPLOAD_QUEUE_SIZE` 是容量上限，不是当前数量。
- 队列满会返回 `upload queue full`，建议增大 `UPLOAD_QUEUE_SIZE` 或提高 `UPLOAD_WORKERS`。
- 查看控制台“队列深度/monitor”判断 backlog。

## 4. 上传失败或 Endpoint 报错
- `S3_ENDPOINT` 可带协议或不带协议（如 `https://s3.example.com`）。
- MinIO 场景请设置 `S3_FORCE_PATH_STYLE=true`，必要时 `S3_DISABLE_SSL=true`。
- 检查 `S3_AK/S3_SK/S3_BUCKET` 是否有效。

## 5. 自动上传关闭后如何上传？
可在控制台选择文件，点击“手动上传/触发上传”；系统会为该文件执行一次上传。

## 6. 文件 Tail 报错或无内容
- `/api/file-log` 仅支持文本文件（遇到二进制会报错）。
- 只返回最后 512KB / 500 行。
- `path` 必须位于 `watch_dir` 配置的任一目录下，否则会被拒绝。

## 7. 目录很多时监控不完整
- fsnotify 依赖系统文件句柄，目录数量过多可能触发 “too many open files”。
- 日志中会提示监控降级，可考虑提高系统 `ulimit` 或缩小监控范围。
- 可通过 `watch_exclude` 或 `WATCH_EXCLUDE` 排除 `.git` 等大目录。
