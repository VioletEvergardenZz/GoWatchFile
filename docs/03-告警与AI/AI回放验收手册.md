# AI 回放与降级验收运行手册

- 更新时间：2026-02-18
- 目标：验证 AI 日志分析在真实样本下的成功率、降级率与错误分类可观测性
- 关联脚本：`go-watch-file/scripts/ops/ai-replay.ps1`

## 1. 输入准备

创建样本文件（UTF-8，每行一个日志路径）：

```text
# 允许注释行
D:\logs\app\service-a.log
D:\logs\app\service-b.log
```

可直接复制 `docs/03-告警与AI/AI回放路径样本.txt` 作为起始模板。
建议将实际回放路径维护在 `docs/03-告警与AI/AI回放路径清单.txt`。

建议样本集覆盖：
- 正常日志
- 高频错误日志
- 大体量日志
- 边界场景日志（包含 timeout/connection reset 等）

## 2. 回放执行

```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/ai-replay.ps1 `
  -BaseUrl http://localhost:8082 `
  -Token $env:API_AUTH_TOKEN `
  -PathsFile ../docs/03-告警与AI/AI回放路径清单.txt `
  -Limit 200 `
  -OutputFile ../reports/ai-replay-result.json
```
若后端未启用鉴权，可省略 `-Token` 参数。

## 3. 输出字段说明

结果文件结构：
- `total`：总样本数
- `success`：非降级样本数
- `degraded`：降级样本数
- `degradedRatio`：降级率
- `errorClass`：按错误分类聚合计数
- `results[]`：单条样本明细（`path/ok/degraded/errorClass/elapsedMs`）

## 4. 验收建议阈值

- `degradedRatio <= 0.20`
- `request_error` 不应持续增长
- `elapsedMs` 中位数保持在业务可接受范围（建议 `< 5000ms`）

若不满足：
1. 先看 `errorClass` Top1
2. 再看 `AI_TIMEOUT`、网络质量、模型限流
3. 必要时降低 `Limit` 或优化日志输入规模

## 5. 指标联动校验

回放前后对比：
- `gwf_ai_log_summary_total{outcome="success"}`
- `gwf_ai_log_summary_total{outcome="degraded"}`
- `gwf_ai_log_summary_retry_total`
- `gwf_ai_log_summary_duration_seconds`

可使用：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ops/check-metrics.ps1 -BaseUrl http://localhost:8082 -OutputFile ../reports/metrics-ai-replay.prom
```

## 6. 与知识库问答引用率联测

当 AI 回放稳定后，执行知识库问答引用率门禁：

```powershell
cd go-watch-file
go run ./cmd/kb-eval citation -base http://localhost:8082 -token "$env:API_AUTH_TOKEN" -samples ../docs/04-知识库/知识库命中率样本.json -limit 3 -target 1.0
```
若后端未启用鉴权，`-token` 参数可省略。

说明：
- 该命令会调用 `/api/kb/ask`
- 若实际引用率低于 `target`，命令将返回非零退出码

