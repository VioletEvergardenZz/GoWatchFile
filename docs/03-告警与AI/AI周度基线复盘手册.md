# AI 周度基线复盘手册

> 文档状态：兼容保留  
> 更新时间：2026-02-24  
> 适用范围：`go-watch-file/scripts/ops/ai-weekly-recap.sh`（优先）、
> `go-watch-file/scripts/ops/ai-replay.ps1` / `go-watch-file/scripts/ops/ai-baseline.ps1`（兼容）

## 1. 目标

将 AI 回放结果从“单次可跑”固化为“周度可对比”，重点关注：

1. 降级率是否稳定在门禁范围内
2. 结构一致性是否出现回退
3. 错误分类覆盖率是否出现异常波动

## 2. 周度节奏

建议固定每周同一天执行一次全量回放，建议节奏：

1. 执行 AI 回放 + 基线（`ai-weekly-recap.sh`）
2. 如需兼容旧流程，再分别执行 `ai-replay.ps1` / `ai-baseline.ps1`
3. 填写周度复盘模板并归档

## 3. 执行前准备

1. 确认样例路径清单已按环境更新：`docs/03-告警与AI/AI回放路径清单.txt`
2. 确认 AI 端点可用，避免把“环境不可达”误判为模型质量下降
3. 确认本周产物命名（建议使用日期后缀）

## 4. 标准执行命令

### 4.1 macOS / Linux（推荐）

```bash
cd go-watch-file
DATE_TAG="$(date +%F)"
bash scripts/ops/ai-weekly-recap.sh \
  --base-url http://localhost:8080 \
  --paths-file ../docs/03-告警与AI/AI回放路径清单.txt \
  --date "$DATE_TAG" \
  --out-dir ../reports
```

输出产物：
- `reports/ai-replay-YYYY-MM-DD.json`
- `reports/ai-baseline-YYYY-MM-DD.json`

### 4.2 PowerShell（兼容）

```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/ai-replay.ps1 `
  -BaseUrl http://localhost:8082 `
  -PathsFile ../docs/03-告警与AI/AI回放路径清单.txt `
  -DegradedRatioTarget 0.20 `
  -StructurePassRatioTarget 1.00 `
  -ErrorClassCoverageTarget 1.00 `
  -FailOnGate `
  -OutputFile ../reports/ai-replay-$(Get-Date -Format yyyy-MM-dd).json

powershell -ExecutionPolicy Bypass -File scripts/ops/ai-baseline.ps1 `
  -FromResultFile ../reports/ai-replay-$(Get-Date -Format yyyy-MM-dd).json `
  -SummaryPassRatioTarget 1.00 `
  -SeverityPassRatioTarget 1.00 `
  -SuggestionsPassRatioTarget 1.00 `
  -OutputFile ../reports/ai-baseline-$(Get-Date -Format yyyy-MM-dd).json
```

## 5. 周度对比口径

本周与上周至少对比以下字段：

1. `replay.degradedRatio`
2. `analysis.summaryPassRatio`
3. `analysis.severityPassRatio`
4. `analysis.suggestionsPassRatio`
5. `gates.allPassed`

若出现以下任一情况，需标记为“质量回退”：

- 本周 `degradedRatio` 高于上周且超过门禁阈值
- 任一结构通过率从 `1.00` 下降
- `gates.allPassed` 从 `true` 变为 `false`

## 6. 复盘输出

1. 产物文件：
- `reports/ai-replay-YYYY-MM-DD.json`
- `reports/ai-baseline-YYYY-MM-DD.json`

2. 周报文档：
- `docs/05-指标与评估/AI周度基线复盘-YYYY-MM-DD.md`
- 建议基于模板：`docs/05-指标与评估/AI周度基线复盘模板-v1.md`

## 7. 与阶段门禁关系

周度复盘通过后，建议将最新结果纳入阶段总复盘：

```bash
cd go-watch-file
bash scripts/ops/stage-recap-lite.sh \
  --ai-baseline ../reports/ai-baseline-$(date +%F).json \
  --kb-drill ../reports/kb-failure-rollback-drill-$(date +%F).json \
  --console-check ../reports/console-closure-check-$(date +%F).json \
  --output-file ../reports/stage-recap-$(date +%F)-lite.json
```
