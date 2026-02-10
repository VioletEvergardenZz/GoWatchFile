# 队列持久化 Spike 说明

## 目标
降低“重启后队列丢失”这一核心不确定性，先验证最小可行方案，再决定是否接入主链路

## 本次 Spike 产物
- PoC 实现：`go-watch-file/internal/spike/persistqueue/file_queue.go`
- PoC 测试：`go-watch-file/internal/spike/persistqueue/file_queue_test.go`
- 可运行命令：`go-watch-file/cmd/queue-spike/main.go`

## 方案摘要
- 存储模型：本地 JSON 文件，按数组记录待处理路径
- 写入策略：原子写（临时文件 + rename），避免写半文件
- 操作能力：`enqueue`、`dequeue`、`peek`、`reset`、`removeOne`
- 目标定位：先以 Spike 验证可靠性，再以可开关方式接入主链路

## 当前集成状态
- 已接入上传主链路：入队前落盘、上传成功后确认删除、服务启动时自动恢复
- 默认关闭：通过 `upload_queue_persist_enabled` 或 `UPLOAD_QUEUE_PERSIST_ENABLED` 开启
- 持久化文件：`upload_queue_persist_file` 或 `UPLOAD_QUEUE_PERSIST_FILE`，默认 `logs/upload-queue.json`
- 语义说明：当前提供“至少一次”保障，不保证严格去重
- 损坏降级：若持久化文件不可解析，会自动备份为 `.corrupt-*.bak` 并重建空队列，服务继续启动

## 运行方式
在 `go-watch-file` 目录执行

```bash
# 入队
go run ./cmd/queue-spike -action enqueue -item /tmp/a.log -store logs/queue-spike.json

# 查看
go run ./cmd/queue-spike -action peek -store logs/queue-spike.json

# 出队
go run ./cmd/queue-spike -action dequeue -store logs/queue-spike.json

# 清空
go run ./cmd/queue-spike -action reset -store logs/queue-spike.json
```

## 观察项
- 进程重启后，`peek` 是否还能读取重启前数据
- 反复 `enqueue/dequeue` 后文件是否保持可解析
- 异常中断后是否会产生损坏文件

## 后续建议
- 将 `internal/spike/persistqueue` 迁移到正式目录（如 `internal/persistqueue`），避免“已上线能力仍在 spike 包”造成认知偏差
- 评估去重策略（路径去重 / 内容指纹）以降低重复处理概率
