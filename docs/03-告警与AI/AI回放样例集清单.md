# AI 回放样例集清单

> 文档状态：兼容保留

- 更新时间：2026-02-19
- 用途：为 `ai-replay.ps1` 提供可持续维护的样例集口径，统一“场景 -> 路径 -> 预期结果”。

## 1. 维护规则

1. 本清单用于维护“样例元数据”，实际回放路径仍写入 `AI回放路径清单.txt`。
2. 每条样例必须标注 `场景`、`预期 errorClass`、`预期是否降级`。
3. 样例文件路径建议固定，避免每次回放因路径变更导致噪音。
4. 若新增错误分类，需同步更新：
- `go-watch-file/scripts/ops/ai-replay.ps1` 的允许分类集合
- `docs/03-告警与AI/AI回放验收手册.md` 验收口径

## 2. 场景矩阵模板

| 样例ID | 场景 | 日志路径 | 预期是否降级 | 预期 errorClass | 说明 |
| --- | --- | --- | --- | --- | --- |
| S01 | 正常请求 | `D:\logs\gwf\backend-info.log` | 否 | `""` | 应返回结构完整的 AI 结果 |
| S02 | AI 超时 | `D:\logs\gwf\backend-timeout.log` | 是 | `timeout` | 验证超时分类与降级行为 |
| S03 | 网络抖动 | `D:\logs\gwf\backend-network.log` | 是 | `network` | 验证连接异常分类 |
| S04 | 上游 5xx | `D:\logs\gwf\backend-upstream-5xx.log` | 是 | `upstream_5xx` | 验证上游服务异常 |
| S05 | 无效路径 | `D:\logs\gwf\not-found.log` | 是 | `request_error` | 验证请求失败兜底分类 |
| S06 | 大体量日志 | `D:\logs\gwf\backend-large.log` | 否 | `""` | 验证结构一致性与耗时 |

## 3. 最低覆盖要求

1. 至少 1 条非降级样例。
2. 至少 1 条 `request_error` 降级样例。
3. 至少 1 条网络/超时类降级样例（`timeout` 或 `network`）。
4. 至少 1 条上游错误样例（`upstream_4xx` 或 `upstream_5xx`）。
5. 每次执行前，确认 `AI回放路径清单.txt` 与本清单中的样例路径一致。

## 4. 执行建议

1. 按本清单更新 `AI回放路径清单.txt`。
2. 执行 `ai-replay.ps1` 并开启 `-FailOnGate`。
3. 将输出 `reports/ai-replay-result.json` 与本清单对照，复核：
- `structure.passRatio`
- `errorClassCoverage`
- `results[].structureIssues`
- `results[].errorClass`
