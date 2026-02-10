# 上传队列持久化运行手册

## 1. 目标
- 避免服务重启导致上传队列任务全部丢失
- 在不引入外部中间件的前提下，提供最小可用的“至少一次”保障
- 出现持久化文件损坏时，服务仍可继续启动并恢复可用

## 2. 当前实现
- 持久化实现：`go-watch-file/internal/persistqueue/file_queue.go`
- 核心测试：`go-watch-file/internal/persistqueue/file_queue_test.go`
- 运维命令：`go-watch-file/cmd/queue-admin/main.go`

## 3. 配置项
- `upload_queue_persist_enabled`
  - 含义：是否开启上传队列持久化
  - 默认：`false`
  - 生效方式：静态配置，修改后需要重启服务
- `upload_queue_persist_file`
  - 含义：持久化队列文件路径
  - 默认：`logs/upload-queue.json`
  - 生效方式：静态配置，修改后需要重启服务

对应环境变量：
- `UPLOAD_QUEUE_PERSIST_ENABLED`
- `UPLOAD_QUEUE_PERSIST_FILE`

## 4. 运行语义
- 入队前先落盘：任务先写入持久化文件，再写入内存队列
- 成功后确认：上传成功后从持久化队列删除对应任务
- 启动时恢复：服务启动时会把持久化文件中的任务恢复到内存队列
- 语义级别：至少一次（At-Least-Once），不保证严格去重

## 5. 损坏降级策略
- 如果持久化文件不可解析：
  - 自动备份损坏文件为：`<原文件>.corrupt-时间戳.bak`
  - 自动重建空队列文件
  - 服务继续启动，避免因为坏文件导致不可用
- 同时记录错误日志，便于排查根因

## 6. 运维命令（queue-admin）
在 `go-watch-file` 目录执行：

```bash
# 入队
go run ./cmd/queue-admin -action enqueue -item /tmp/a.log -store logs/upload-queue.json

# 查看
go run ./cmd/queue-admin -action peek -store logs/upload-queue.json

# 出队
go run ./cmd/queue-admin -action dequeue -store logs/upload-queue.json

# 清空
go run ./cmd/queue-admin -action reset -store logs/upload-queue.json
```

### 6.1 健康观察（/api/health）
- `persistQueue.enabled`：是否开启持久化队列
- `persistQueue.storeFile`：当前持久化文件路径
- `persistQueue.recoveredTotal`：累计恢复到内存队列的任务数
- `persistQueue.corruptFallbackTotal`：累计损坏降级次数
- `persistQueue.persistWriteFailureTotal`：累计持久化写失败次数

## 7. 现场核对清单
- 重启前入队的任务，重启后是否可恢复
- 连续执行 `enqueue/dequeue` 后，队列文件是否可持续解析
- 人工写入损坏 JSON 后，是否生成 `.corrupt-*.bak` 备份且服务仍可启动
- 日志中是否包含损坏降级记录

## 8. 当前边界
- 不做严格去重，极端故障场景可能重复处理同一路径
- 不支持断点续传
- 不支持跨实例共享同一持久化文件（单实例假设）
