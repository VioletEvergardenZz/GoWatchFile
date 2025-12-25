# 常见问题（FAQ）

## 1. 文件创建后没有触发上传
- 确认 `watch_dir` 存在且 Agent 有权限。
- 确认文件后缀与 `file_ext` 完全一致（区分大小写）。
- 写入完成判定需要等待约 10 秒静默窗口。

## 2. 支持多个后缀吗？
- 当前仅支持单一后缀（如 `.log`）。多后缀与忽略规则属于后续版本规划。

## 3. Jenkins 初始化失败
- 检查 `JENKINS_HOST`、`JENKINS_USER`、`JENKINS_PASSWORD` 是否可用。
- 确认 Jenkins 可从 Agent 机器访问（网络/认证/证书）。

## 4. 上传失败或 Endpoint 校验报错
- `S3_ENDPOINT` 可写域名或带协议的地址（如 `https://s3.example.com`）。
- MinIO 等场景请设置 `S3_FORCE_PATH_STYLE=true`。

## 5. 通知不生效
- 企业微信：检查 `ROBOT_KEY` 是否有效。
- 钉钉：确认 `DINGTALK_WEBHOOK`/`DINGTALK_SECRET` 配置正确且允许当前 IP。

## 6. 日志过少/排查困难
- 设置 `LOG_LEVEL=debug`，并确认 `LOG_TO_STD=true` 或 `LOG_FILE` 可写。

## 7. 上传压力过大
- 调整 `UPLOAD_WORKERS` 与 `UPLOAD_QUEUE_SIZE`，并观察队列堆积情况。

## 8. Windows 环境路径问题
- `watch_dir` 需填写系统真实目录；对象 Key 会自动归一化为 `/` 分隔。
