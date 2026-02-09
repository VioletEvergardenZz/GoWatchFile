# 常见问题（FAQ）

## 1. 文件创建后没有触发上传
- 确认 `watch_dir` 存在且 Agent 有权限。
- 确认文件后缀与 `file_ext` 匹配（不区分大小写）。
- 写入完成判定需要等待静默窗口（默认约 10 秒）。
- 控制台中该目录是否关闭了“自动上传”。

## 2. 支持多个后缀吗？
支持多个后缀（如 `.log,.txt`），逗号或空格分隔。

## 3. 临时文件没有上传
- 内置忽略临时后缀（如 `.tmp/.part/.crdownload/.download/.swp`）。
- 这是为了避免“未写完的文件”被提前上传。

## 4. 上传队列提示满了或任务堆积
- `upload_queue_size` 是容量上限，不是当前数量。
- 队列满会返回 `upload queue full`，建议增大 `upload_queue_size` 或提高 `upload_workers`。
- 查看控制台“队列深度/monitor”判断 backlog。

## 5. 上传失败会自动重试吗？
- 默认开启重试，可通过 `upload_retry_enabled` / `upload_retry_delays` 调整。
- 若关闭重试，可在控制台触发“手动上传”进行补传。

## 6. 上传失败或 Endpoint 报错
- `S3_ENDPOINT` 可带协议或不带协议（如 `https://s3.example.com`）。
- MinIO 场景请设置 `S3_FORCE_PATH_STYLE=true`，必要时 `S3_DISABLE_SSL=true`。
- 检查 `S3_AK/S3_SK/S3_BUCKET` 是否有效。

## 7. 自动上传关闭后如何上传？
可在控制台选择文件，点击“手动上传/触发上传”；系统会为该文件执行一次上传。

## 8. 文件 Tail 报错或无内容
- `/api/file-log` 仅支持文本文件（遇到二进制会报错）。
- Tail 只返回最后 512KB / 500 行；传入 `query` 会进入检索模式（默认最多 2000 行）。
- `path` 必须位于 `watch_dir` 配置的任一目录下，否则会被拒绝。

## 9. 目录很多时监控不完整
- fsnotify 依赖系统文件句柄，目录数量过多可能触发 “too many open files”。
- 日志中会提示监控降级，可考虑提高系统 `ulimit` 或缩小监控范围。
- 可通过 `watch_exclude` 排除 `.git` 等大目录。

## 10. 告警控制台提示“告警未启用”
- 确认 `alert_enabled=true`，并在控制台配置了规则与日志路径。
- 规则未点击“保存规则”时不会生效，刷新会恢复为上次保存的规则。

## 11. 告警没有历史数据
- 检查 `alert_start_from_end` 是否为 `true`，该配置会忽略历史日志，仅处理新写入内容。
- 如需回溯历史日志，请改为 `false` 并留意可能产生大量历史告警。

## 11.1 删除规则后刷新又回来了
- 删除只是前端内存变更，需点击“保存规则”才会写入 `config.runtime.yaml`。
- “刷新规则”会从 `config.runtime.yaml` 重新加载，覆盖未保存修改。

## 12. “最新窗口”是什么意思
- 告警概览统计最近 24 小时的告警决策，控制台“最新窗口”展示该统计窗口文案。

## 13. 系统资源控制台提示“未启用”或请求失败
- `/api/system` 需要先在控制台开启 `systemResourceEnabled`。
- 未开启时接口会返回 403，启用后再刷新页面。
