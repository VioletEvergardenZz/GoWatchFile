# AI 周度基线复盘模板 v1

> 文档状态：模板  
> 校准日期：2026-02-24  
> 口径说明：模板仅定义周度复盘格式，不单独定义平台能力边界

- 复盘日期：
- 执行人：
- 环境：
- AI 样例清单：`docs/03-告警与AI/AI回放路径清单.txt`

## 1. 执行产物

| 项目 | 文件 | 结果 |
| --- | --- | --- |
| 本周回放结果 | `reports/ai-replay-YYYY-MM-DD.json` |  |
| 本周基线结果 | `reports/ai-baseline-YYYY-MM-DD.json` |  |
| 上周基线结果 | `reports/ai-baseline-YYYY-MM-DD.json` |  |

## 2. 指标对比（本周 vs 上周）

| 指标 | 本周 | 上周 | 变化 | 门禁 |
| --- | --- | --- | --- | --- |
| `replay.degradedRatio` |  |  |  | `<= 0.20` |
| `analysis.summaryPassRatio` |  |  |  | `= 1.00` |
| `analysis.severityPassRatio` |  |  |  | `= 1.00` |
| `analysis.suggestionsPassRatio` |  |  |  | `= 1.00` |
| `gates.allPassed` |  |  |  | `true` |

## 3. 异常分类对比

| errorClass | 本周计数 | 上周计数 | 说明 |
| --- | --- | --- | --- |
| timeout |  |  |  |
| network |  |  |  |
| upstream_4xx |  |  |  |
| upstream_5xx |  |  |  |
| request_error |  |  |  |

## 4. 结论

- 本周是否通过门禁：`是/否`
- 是否存在质量回退：`是/否`
- 结论摘要：

## 5. 风险与动作

1. 风险：
2. 风险：
3. 下周动作：

