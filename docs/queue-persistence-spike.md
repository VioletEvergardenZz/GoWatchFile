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
- 操作能力：`enqueue`、`dequeue`、`peek`、`reset`
- 目标定位：仅用于验证可靠性行为，不直接替换当前生产队列

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

## 下一步决策建议
- 若 PoC 行为稳定，下一步评估接入上传主链路的改造点：
  - 入队时落盘
  - 上传成功后出队
  - 启动时恢复队列
  - 队列文件损坏时的降级与告警
