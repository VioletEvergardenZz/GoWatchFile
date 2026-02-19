# AI 回放与降级验收运行手册

- 更新时间：2026-02-19
- 目标：验证 AI 日志分析在真实样本下的降级率、结构一致性、错误分类覆盖率，以及基线结构稳定性（summary/severity/suggestions）
- 关联脚本：`go-watch-file/scripts/ops/ai-replay.ps1`、`go-watch-file/scripts/ops/ai-baseline.ps1`

## 1. 输入准备

1. 准备回放路径文件（UTF-8，每行一个日志路径）：

```text
# 允许注释行
D:\logs\app\service-a.log
D:\logs\app\service-b.log
```

2. 推荐起始模板：
- `docs/03-告警与AI/AI回放路径样本.txt`
- 实际执行清单维护在 `docs/03-告警与AI/AI回放路径清单.txt`

3. 样例集覆盖建议（至少覆盖以下场景）：
- 正常日志（期望 `degraded=false`）
- 网络抖动/超时日志（期望 `errorClass=timeout` 或 `network`）
- 上游 4xx/5xx 日志（期望 `errorClass=upstream_4xx/upstream_5xx`）
- 不可达路径或请求失败（期望 `errorClass=request_error`）
- 大体量日志（验证结构稳定性与耗时）

## 2. 回放执行

```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/ai-replay.ps1 `
  -BaseUrl http://localhost:8082 `
  -Token $env:API_AUTH_TOKEN `
  -PathsFile ../docs/03-告警与AI/AI回放路径清单.txt `
  -Limit 200 `
  -DegradedRatioTarget 0.20 `
  -StructurePassRatioTarget 1.00 `
  -ErrorClassCoverageTarget 1.00 `
  -FailOnGate `
  -OutputFile ../reports/ai-replay-result.json
```

若后端未启用鉴权，可省略 `-Token` 参数。

退出码约定：
- `0`：回放完成（门禁通过，或未启用 `-FailOnGate`）
- `2`：输入参数错误（如路径文件缺失/为空）
- `3`：启用 `-FailOnGate` 且门禁未通过

## 2.1 基线验证执行（summary/severity/suggestions）

在线模式（自动回放并执行基线门禁）：

```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/ai-baseline.ps1 `
  -BaseUrl http://localhost:8082 `
  -Token $env:API_AUTH_TOKEN `
  -PathsFile ../docs/03-告警与AI/AI回放路径清单.txt `
  -SummaryPassRatioTarget 1.00 `
  -SeverityPassRatioTarget 1.00 `
  -SuggestionsPassRatioTarget 1.00 `
  -OutputFile ../reports/ai-baseline-result.json `
  -ReportFile ../docs/05-指标与评估/AI基线验证报告-$(Get-Date -Format yyyy-MM-dd).md
```

离线模式（不访问服务，直接消费既有回放结果）：

```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/ai-baseline.ps1 `
  -FromResultFile ../reports/ai-replay-result.json `
  -OutputFile ../reports/ai-baseline-result.json
```

`ai-baseline.ps1` 退出码约定：
- `0`：门禁通过
- `2`：输入参数错误（如结果文件不存在）
- `3`：门禁失败（包括结构证据不足）

## 3. 参数说明

- `-DegradedRatioTarget`：降级率门禁，默认 `0.2`
- `-StructurePassRatioTarget`：结构一致性通过率门禁，默认 `1.0`
- `-ErrorClassCoverageTarget`：错误分类覆盖率门禁，默认 `1.0`
- `-FailOnGate`：门禁未通过时返回非零退出码（`3`）
- `-CaseSensitive`：回放时全文检索是否区分大小写
- `-Limit`：单样本日志读取上限，默认 `200`

## 4. 输出字段说明

结果文件主字段（`reports/ai-replay-result.json`）：
- `total/success/degraded/requestFailed`
- `degradedRatio`
- `structure.checked/passed/failed/passRatio/issues`
- `severity.low/medium/high/unknown`
- `errorClass`（降级错误分类计数）
- `errorClassCoverage`
- `knownErrorClassRatio`
- `gateTargets.degradedRatio/structurePassRatio/errorClassCoverage`
- `gates.degradedRatioPass/structurePassRatioPass/errorClassCoveragePass/allPassed`
- `results[]`（单样本明细）

单样本明细字段（`results[]`）：
- `path/ok/degraded/errorClass/elapsedMs`
- `structureChecked/structureOK/structureIssues`
- `severity/suggestionsCount/causesCount/keyErrorsCount/confidence`

基线结果主字段（`reports/ai-baseline-result.json`）：
- `replay.total/success/degraded/degradedRatio`
- `analysis.structureEvidenceReady/structureChecked`
- `analysis.summaryPassRatio/severityPassRatio/suggestionsPassRatio`
- `gates.replay.*`、`gates.baseline.*`、`gates.allPassed`
- `notes[]`（缺少 `structure.*` 或 `results[].structureIssues` 时会标记结构证据不足）

## 5. 验收口径

建议将以下 3 项作为强门禁：
1. `degradedRatio <= DegradedRatioTarget`
2. `structure.passRatio >= StructurePassRatioTarget`
3. `errorClassCoverage >= ErrorClassCoverageTarget`

建议观察项：
- `knownErrorClassRatio` 接近 `1.0`（表示降级均被已知分类覆盖）
- `structure.issues` TopN 稳定，不出现持续放大
- `requestFailed` 不持续增长

不达标时排查顺序：
1. 看 `gates` 与 `structure.issues`，先定位是结构问题还是请求问题
2. 看 `errorClass` Top1，区分超时/网络/鉴权/上游错误
3. 再检查 AI 配置（`AI_TIMEOUT`、`AI_BASE_URL`、模型限流）与样本路径可达性

## 6. 与阶段复盘联动

`stage-recap.ps1` 已接入 AI 三项门禁，可直接统一验收：

```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/stage-recap.ps1 `
  -BaseUrl http://localhost:8082 `
  -Token $env:API_AUTH_TOKEN `
  -AIPathsFile ../docs/03-告警与AI/AI回放路径清单.txt `
  -AIDegradedRatioTarget 0.20 `
  -AIStructurePassRatioTarget 1.00 `
  -AIErrorClassCoverageTarget 1.00 `
  -OutputFile ../reports/stage-recap-result.json
```

## 7. 指标联动校验

回放前后对比：
- `gwf_ai_log_summary_total{outcome="success"}`
- `gwf_ai_log_summary_total{outcome="degraded"}`
- `gwf_ai_log_summary_retry_total`
- `gwf_ai_log_summary_duration_seconds`

可使用：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ops/check-metrics.ps1 -BaseUrl http://localhost:8082 -OutputFile ../reports/metrics-ai-replay.prom
```

## 8. 与知识库问答引用率联测

当 AI 回放门禁稳定后，执行知识库问答引用率门禁：

```powershell
cd go-watch-file
go run ./cmd/kb-eval citation -base http://localhost:8082 -token "$env:API_AUTH_TOKEN" -samples ../docs/04-知识库/知识库命中率样本.json -limit 3 -target 1.0
```

若后端未启用鉴权，`-token` 参数可省略。该命令会调用 `/api/kb/ask`，低于目标时返回非零退出码。
