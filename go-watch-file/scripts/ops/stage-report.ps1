# 本脚本用于把阶段复盘 JSON 渲染为中文 Markdown 报告
# 目标是减少手工整理步骤 让“实测复盘结果”可以一键落档

param(
  [string]$RecapFile = "../reports/stage-recap-result.json",
  [string]$MetricsFile = "",
  [string]$OutputFile = "",
  [string]$Operator = "Codex",
  [string]$Environment = "dev-like (local)",
  [string]$Version = ""
)

function Ensure-Dir {
  param([string]$Path)
  if (-not [string]::IsNullOrWhiteSpace($Path) -and -not (Test-Path $Path)) {
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
  }
}

function Resolve-ExistingPath {
  param(
    [string]$Candidate,
    [string[]]$BaseDirs
  )
  if ([string]::IsNullOrWhiteSpace($Candidate)) {
    return ""
  }
  if ([System.IO.Path]::IsPathRooted($Candidate)) {
    if (Test-Path $Candidate) {
      return (Resolve-Path $Candidate).Path
    }
    return ""
  }
  foreach ($baseDir in $BaseDirs) {
    if ([string]::IsNullOrWhiteSpace($baseDir)) {
      continue
    }
    $tryPath = Join-Path $baseDir $Candidate
    if (Test-Path $tryPath) {
      return (Resolve-Path $tryPath).Path
    }
  }
  if (Test-Path $Candidate) {
    return (Resolve-Path $Candidate).Path
  }
  return ""
}

function Parse-DoubleInvariant {
  param([string]$Raw)
  $value = 0.0
  if ([double]::TryParse($Raw, [System.Globalization.NumberStyles]::Float, [System.Globalization.CultureInfo]::InvariantCulture, [ref]$value)) {
    return $value
  }
  return $null
}

function Read-MetricValue {
  param(
    [string]$MetricsText,
    [string]$MetricName
  )
  if ([string]::IsNullOrWhiteSpace($MetricsText) -or [string]::IsNullOrWhiteSpace($MetricName)) {
    return $null
  }
  $escaped = [regex]::Escape($MetricName)
  $pattern = "(?m)^$escaped(?:\{[^\n]*\})?\s+([0-9.eE+\-]+)$"
  $match = [regex]::Match($MetricsText, $pattern)
  if (-not $match.Success) {
    return $null
  }
  return Parse-DoubleInvariant -Raw $match.Groups[1].Value
}

function Read-MetricValueByLabel {
  param(
    [string]$MetricsText,
    [string]$MetricName,
    [string]$LabelName,
    [string]$LabelValue
  )
  if ([string]::IsNullOrWhiteSpace($MetricsText)) {
    return $null
  }
  $metricEscaped = [regex]::Escape($MetricName)
  $labelNameEscaped = [regex]::Escape($LabelName)
  $labelValueEscaped = [regex]::Escape($LabelValue)
  $pattern = "(?m)^$metricEscaped\{[^\n]*$labelNameEscaped=""$labelValueEscaped""[^\n]*\}\s+([0-9.eE+\-]+)$"
  $match = [regex]::Match($MetricsText, $pattern)
  if (-not $match.Success) {
    return $null
  }
  return Parse-DoubleInvariant -Raw $match.Groups[1].Value
}

function Read-MetricSum {
  param(
    [string]$MetricsText,
    [string]$MetricName
  )
  if ([string]::IsNullOrWhiteSpace($MetricsText)) {
    return $null
  }
  $metricEscaped = [regex]::Escape($MetricName)
  $pattern = "(?m)^$metricEscaped(?:\{[^\n]*\})?\s+([0-9.eE+\-]+)$"
  $matches = [regex]::Matches($MetricsText, $pattern)
  if ($matches.Count -eq 0) {
    return $null
  }
  $sum = 0.0
  foreach ($match in $matches) {
    $value = Parse-DoubleInvariant -Raw $match.Groups[1].Value
    if ($null -ne $value) {
      $sum += $value
    }
  }
  return $sum
}

function Parse-PercentFromText {
  param(
    [string]$Text,
    [string]$Pattern
  )
  if ([string]::IsNullOrWhiteSpace($Text) -or [string]::IsNullOrWhiteSpace($Pattern)) {
    return $null
  }
  $match = [regex]::Match($Text, $Pattern, [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
  if (-not $match.Success) {
    return $null
  }
  return Parse-DoubleInvariant -Raw $match.Groups[1].Value
}

function Format-Decimal {
  param(
    [double]$Value,
    [int]$Digits = 2
  )
  return [string]::Format([System.Globalization.CultureInfo]::InvariantCulture, "{0:F$Digits}", $Value)
}

function Format-NullableNumber {
  param(
    $Value,
    [int]$Digits = 2,
    [string]$Suffix = ""
  )
  if ($null -eq $Value) {
    return "N/A"
  }
  return "$(Format-Decimal -Value ([double]$Value) -Digits $Digits)$Suffix"
}

function To-PF {
  param(
    $Pass
  )
  if ($null -eq $Pass) {
    return "-"
  }
  if ([bool]$Pass) {
    return "P"
  }
  return "F"
}

function Get-Stage {
  param(
    $Recap,
    [string]$Name
  )
  if ($null -eq $Recap -or $null -eq $Recap.stages) {
    return $null
  }
  foreach ($item in $Recap.stages) {
    if ([string]$item.name -eq $Name) {
      return $item
    }
  }
  return $null
}

function Get-FirstLine {
  param([string]$Text)
  if ([string]::IsNullOrWhiteSpace($Text)) {
    return ""
  }
  $line = ($Text -split "`r?`n")[0]
  return ($line -replace "\|", "/").Trim()
}

$scriptDir = (Resolve-Path $PSScriptRoot).Path
$goWatchDir = (Resolve-Path (Join-Path $scriptDir "..\..")).Path
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..\..\..")).Path
$currentDir = (Get-Location).Path

$recapPath = Resolve-ExistingPath -Candidate $RecapFile -BaseDirs @($currentDir, $goWatchDir, $repoRoot, $scriptDir)
if ([string]::IsNullOrWhiteSpace($recapPath)) {
  Write-Error "未找到阶段复盘结果文件: $RecapFile"
  exit 2
}

$recapRaw = Get-Content -Raw -Encoding UTF8 $recapPath
if ([string]::IsNullOrWhiteSpace($recapRaw)) {
  Write-Error "阶段复盘结果文件为空: $recapPath"
  exit 2
}

try {
  $recap = $recapRaw | ConvertFrom-Json
} catch {
  Write-Error ("解析阶段复盘结果失败: {0}" -f $_.Exception.Message)
  exit 2
}

$generatedDate = Get-Date
if ($null -ne $recap.generatedAt -and -not [string]::IsNullOrWhiteSpace([string]$recap.generatedAt)) {
  $parsedDate = Get-Date
  if ([datetime]::TryParse([string]$recap.generatedAt, [ref]$parsedDate)) {
    $generatedDate = $parsedDate
  }
}
$reportDate = $generatedDate.ToString("yyyy-MM-dd")

if ([string]::IsNullOrWhiteSpace($OutputFile)) {
  $OutputFile = Join-Path $repoRoot ("docs/05-指标与评估/阶段回归报告-{0}.md" -f $reportDate)
}
$outputDir = Split-Path -Parent $OutputFile
Ensure-Dir $outputDir

if ([string]::IsNullOrWhiteSpace($Version)) {
  try {
    $versionRaw = (& git -C $repoRoot rev-parse --short HEAD 2>$null | Out-String).Trim()
    if (-not [string]::IsNullOrWhiteSpace($versionRaw)) {
      $Version = $versionRaw
    }
  } catch {
    $Version = ""
  }
}
if ([string]::IsNullOrWhiteSpace($Version)) {
  $Version = "unknown"
}

$metricsCandidate = $MetricsFile
if ([string]::IsNullOrWhiteSpace($metricsCandidate) -and $null -ne $recap.artifacts) {
  $metricsCandidate = [string]$recap.artifacts.metricsSnapshot
}
$recapDir = Split-Path -Parent $recapPath
$metricsPath = Resolve-ExistingPath -Candidate $metricsCandidate -BaseDirs @($currentDir, $goWatchDir, $repoRoot, $recapDir, $scriptDir)
$metricsText = ""
if (-not [string]::IsNullOrWhiteSpace($metricsPath)) {
  $metricsText = Get-Content -Raw -Encoding UTF8 $metricsPath
}

$uploadQueueFullTotal = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_queue_full_total"
$uploadSuccessTotal = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_success_total"
$uploadFailureTotal = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_failure_total"
$uploadFailureRatePct = $null
if ($null -ne $uploadSuccessTotal -and $null -ne $uploadFailureTotal) {
  $uploadDenominator = [double]$uploadSuccessTotal + [double]$uploadFailureTotal
  if ($uploadDenominator -gt 0) {
    $uploadFailureRatePct = ([double]$uploadFailureTotal / $uploadDenominator) * 100
  }
}

$aiSuccess = Read-MetricValueByLabel -MetricsText $metricsText -MetricName "gwf_ai_log_summary_total" -LabelName "outcome" -LabelValue "success"
$aiDegraded = Read-MetricValueByLabel -MetricsText $metricsText -MetricName "gwf_ai_log_summary_total" -LabelName "outcome" -LabelValue "degraded"
$aiTotal = Read-MetricSum -MetricsText $metricsText -MetricName "gwf_ai_log_summary_total"
$aiTargetRatio = 0.2
if ($null -ne $recap.gateTargets -and $null -ne $recap.gateTargets.aiDegradedRatio) {
  $aiTargetRatio = [double]$recap.gateTargets.aiDegradedRatio
}
$aiTargetPct = $aiTargetRatio * 100
$aiStructureTargetRatio = 1.0
if ($null -ne $recap.gateTargets -and $null -ne $recap.gateTargets.aiStructurePassRatio) {
  $aiStructureTargetRatio = [double]$recap.gateTargets.aiStructurePassRatio
}
$aiStructureTargetPct = $aiStructureTargetRatio * 100
$aiErrorClassCoverageTargetRatio = 1.0
if ($null -ne $recap.gateTargets -and $null -ne $recap.gateTargets.aiErrorClassCoverage) {
  $aiErrorClassCoverageTargetRatio = [double]$recap.gateTargets.aiErrorClassCoverage
}
$aiErrorClassCoverageTargetPct = $aiErrorClassCoverageTargetRatio * 100
$aiDegradedRatioPct = $null
if ($null -ne $aiTotal -and $aiTotal -gt 0 -and $null -ne $aiDegraded) {
  $aiDegradedRatioPct = ([double]$aiDegraded / [double]$aiTotal) * 100
}
$aiStructurePassRatioPct = $null
$aiErrorClassCoveragePct = $null
$aiStructureChecked = $null
$aiStructureFailed = $null

if ($null -ne $recap.aiReplay) {
  if ($null -ne $recap.aiReplay.total) {
    $aiTotal = [double]$recap.aiReplay.total
  }
  if ($null -ne $recap.aiReplay.success) {
    $aiSuccess = [double]$recap.aiReplay.success
  }
  if ($null -ne $recap.aiReplay.degraded) {
    $aiDegraded = [double]$recap.aiReplay.degraded
  }
  if ($null -ne $recap.aiReplay.degradedRatio) {
    $aiDegradedRatioPct = [double]$recap.aiReplay.degradedRatio * 100
  }
  if ($null -ne $recap.aiReplay.structure) {
    if ($null -ne $recap.aiReplay.structure.passRatio) {
      $aiStructurePassRatioPct = [double]$recap.aiReplay.structure.passRatio * 100
    }
    if ($null -ne $recap.aiReplay.structure.checked) {
      $aiStructureChecked = [double]$recap.aiReplay.structure.checked
    }
    if ($null -ne $recap.aiReplay.structure.failed) {
      $aiStructureFailed = [double]$recap.aiReplay.structure.failed
    }
  }
  if ($null -ne $recap.aiReplay.errorClassCoverage) {
    $aiErrorClassCoveragePct = [double]$recap.aiReplay.errorClassCoverage * 100
  }
}

$kbSearchHitRatio = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_kb_search_hit_ratio"
$kbAskCitationRatio = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_kb_ask_citation_ratio"
$kbHitratePct = if ($null -ne $kbSearchHitRatio) { [double]$kbSearchHitRatio * 100 } else { $null }
$kbCitationPct = if ($null -ne $kbAskCitationRatio) { [double]$kbAskCitationRatio * 100 } else { $null }
$kbHitrateTargetPct = 70.0
$kbCitationTargetPct = 95.0
$kbMttdDropTargetPct = 20.0
if ($null -ne $recap.gateTargets) {
  if ($null -ne $recap.gateTargets.kbHitrate) {
    $kbHitrateTargetPct = [double]$recap.gateTargets.kbHitrate * 100
  }
  if ($null -ne $recap.gateTargets.kbCitation) {
    $kbCitationTargetPct = [double]$recap.gateTargets.kbCitation * 100
  }
  if ($null -ne $recap.gateTargets.kbMttdDrop) {
    $kbMttdDropTargetPct = [double]$recap.gateTargets.kbMttdDrop * 100
  }
}

$controlOnline = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_control_agents_online"
$controlBacklog = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_control_task_backlog"
$controlTimeoutTotal = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_control_task_timeout_total"

if ($null -ne $recap.kbRecap) {
  if ($null -ne $recap.kbRecap.hitrate -and $null -ne $recap.kbRecap.hitrate.output) {
    $parsedHitratePct = Parse-PercentFromText -Text ([string]$recap.kbRecap.hitrate.output) -Pattern "Hitrate:\s+\d+/\d+\s+=\s+([0-9.]+)%"
    if ($null -ne $parsedHitratePct) {
      $kbHitratePct = $parsedHitratePct
    }
  }
  if ($null -ne $recap.kbRecap.citation -and $null -ne $recap.kbRecap.citation.output) {
    $parsedCitationPct = Parse-PercentFromText -Text ([string]$recap.kbRecap.citation.output) -Pattern "Citation ratio:\s+\d+/\d+\s+=\s+([0-9.]+)%"
    if ($null -ne $parsedCitationPct) {
      $kbCitationPct = $parsedCitationPct
    }
  }
  if ($null -ne $recap.kbRecap.mttd -and $null -ne $recap.kbRecap.mttd.output) {
    $mttdDropPct = Parse-PercentFromText -Text ([string]$recap.kbRecap.mttd.output) -Pattern "Drop ratio\s*:\s*([0-9.]+)%"
  } else {
    $mttdDropPct = $null
  }
} else {
  $mttdDropPct = $null
}

$uploadQueuePass = $null
if ($null -ne $uploadQueueFullTotal) {
  $uploadQueuePass = ([double]$uploadQueueFullTotal -le 0)
}
$uploadFailurePass = $null
if ($null -ne $uploadFailureRatePct) {
  $uploadFailurePass = ([double]$uploadFailureRatePct -lt 5)
}
$aiPass = $null
if ($null -ne $aiDegradedRatioPct) {
  $aiPass = ([double]$aiDegradedRatioPct -le $aiTargetPct)
}
$aiStructurePass = $null
if ($null -ne $aiStructurePassRatioPct) {
  $aiStructurePass = ([double]$aiStructurePassRatioPct -ge $aiStructureTargetPct)
}
$aiErrorClassCoveragePass = $null
if ($null -ne $aiErrorClassCoveragePct) {
  $aiErrorClassCoveragePass = ([double]$aiErrorClassCoveragePct -ge $aiErrorClassCoverageTargetPct)
}
$kbHitratePass = $null
if ($null -ne $kbHitratePct) {
  $kbHitratePass = ([double]$kbHitratePct -ge $kbHitrateTargetPct)
}
$kbCitationPass = $null
if ($null -ne $kbCitationPct) {
  $kbCitationPass = ([double]$kbCitationPct -ge $kbCitationTargetPct)
}
$mttdDropPass = $null
if ($null -ne $mttdDropPct) {
  $mttdDropPass = ([double]$mttdDropPct -ge $kbMttdDropTargetPct)
}
$controlOnlinePass = $null
if ($null -ne $controlOnline) {
  $controlOnlinePass = ([double]$controlOnline -ge 1)
}
$controlBacklogPass = $null
if ($null -ne $controlBacklog) {
  $controlBacklogPass = ([double]$controlBacklog -le 50)
}
$controlTimeoutPass = $null
if ($null -ne $controlTimeoutTotal) {
  $controlTimeoutPass = ([double]$controlTimeoutTotal -eq 0)
}

$aiTargetText = ("<= {0}%" -f (Format-Decimal -Value ([double]$aiTargetPct) -Digits 2))
$aiStructureTargetText = (">= {0}%" -f (Format-Decimal -Value ([double]$aiStructureTargetPct) -Digits 2))
$aiErrorClassCoverageTargetText = (">= {0}%" -f (Format-Decimal -Value ([double]$aiErrorClassCoverageTargetPct) -Digits 2))
$kbHitrateTargetText = (">= {0}%" -f (Format-Decimal -Value ([double]$kbHitrateTargetPct) -Digits 2))
$kbCitationTargetText = (">= {0}%" -f (Format-Decimal -Value ([double]$kbCitationTargetPct) -Digits 2))
$kbMttdDropTargetText = (">= {0}%" -f (Format-Decimal -Value ([double]$kbMttdDropTargetPct) -Digits 2))

$primeStage = Get-Stage -Recap $recap -Name "stage-prime"
$metricsStage = Get-Stage -Recap $recap -Name "metrics-check"
$aiStage = Get-Stage -Recap $recap -Name "ai-replay"
$controlStage = Get-Stage -Recap $recap -Name "control-replay"
$kbStage = Get-Stage -Recap $recap -Name "kb-recap"

$primeStagePF = if ($null -eq $primeStage) { "-" } else { To-PF -Pass $primeStage.ok }
$primeStageRemark = if ($null -ne $recap.prime) {
  "kb imported=$($recap.prime.kb.imported), updated=$($recap.prime.kb.updated), approved=$($recap.prime.kb.approved); ai samples=$($recap.prime.ai.sampleCount)"
} elseif ($null -eq $primeStage) {
  "未执行"
} else {
  "exitCode=$($primeStage.exitCode), elapsedMs=$($primeStage.elapsedMs)"
}
$metricsStagePF = if ($null -eq $metricsStage) { "-" } else { To-PF -Pass $metricsStage.ok }
$metricsStageRemark = if ($null -eq $metricsStage) { "未执行" } else { "exitCode=$($metricsStage.exitCode), elapsedMs=$($metricsStage.elapsedMs)" }
$aiStagePF = if ($null -eq $aiStage) { "-" } else { To-PF -Pass $aiStage.ok }
$aiStageRemark = if ($null -ne $recap.aiReplay) {
  $degradedRatioText = if ($null -ne $recap.aiReplay.degradedRatio) { [Math]::Round([double]$recap.aiReplay.degradedRatio * 100, 2) } else { "N/A" }
  $structureRatioText = if ($null -ne $recap.aiReplay.structure -and $null -ne $recap.aiReplay.structure.passRatio) { [Math]::Round([double]$recap.aiReplay.structure.passRatio * 100, 2) } else { "N/A" }
  $errorCoverageText = if ($null -ne $recap.aiReplay.errorClassCoverage) { [Math]::Round([double]$recap.aiReplay.errorClassCoverage * 100, 2) } else { "N/A" }
  "total=$($recap.aiReplay.total), success=$($recap.aiReplay.success), degraded=$($recap.aiReplay.degraded), degradedRatio=$degradedRatioText%, structureRatio=$structureRatioText%, errorClassCoverage=$errorCoverageText%"
} elseif ($null -eq $aiStage) {
  "未执行"
} else {
  "exitCode=$($aiStage.exitCode), elapsedMs=$($aiStage.elapsedMs)"
}
$controlStagePF = if ($null -eq $controlStage) { "-" } else { To-PF -Pass $controlStage.ok }
$controlStageRemark = if ($null -ne $recap.controlReplay) {
  "done=$($recap.controlReplay.done)/$($recap.controlReplay.total), success=$($recap.controlReplay.success), failed=$($recap.controlReplay.failed)"
} else {
  "未执行"
}
$kbHitratePF = if ($null -ne $recap.kbRecap -and $null -ne $recap.kbRecap.hitrate) { To-PF -Pass $recap.kbRecap.hitrate.ok } else { "-" }
$kbHitrateRemark = if ($null -ne $kbHitratePct) { "hitrate=$(Format-NullableNumber -Value $kbHitratePct -Digits 2 -Suffix '%')" } else { "未执行" }
$kbCitationPF = if ($null -ne $recap.kbRecap -and $null -ne $recap.kbRecap.citation) { To-PF -Pass $recap.kbRecap.citation.ok } else { "-" }
$kbCitationRemark = if ($null -ne $kbCitationPct) { "citation=$(Format-NullableNumber -Value $kbCitationPct -Digits 2 -Suffix '%')" } else { "未执行" }
$kbMttdPF = if ($null -ne $recap.kbRecap -and $null -ne $recap.kbRecap.mttd) { To-PF -Pass $recap.kbRecap.mttd.ok } else { "-" }
$kbMttdRemark = if ($null -ne $mttdDropPct) { "drop=$(Format-NullableNumber -Value $mttdDropPct -Digits 2 -Suffix '%')" } else { "未执行" }

$executionRows = @()
$executionRows += "| 后端测试 | cd go-watch-file && go test ./... | - | 可选接入 stage-recap 扩展阶段 |"
$executionRows += "| 前端构建 | cd console-frontend && npm run build | - | 可选接入 stage-recap 扩展阶段 |"
$executionRows += ("| 阶段预备 | stage-prime.ps1 | {0} | {1} |" -f $primeStagePF, $primeStageRemark)
$executionRows += ("| 指标巡检 | check-metrics.ps1 | {0} | {1} |" -f $metricsStagePF, $metricsStageRemark)
$executionRows += "| 上传压测 | upload-stress.ps1 | - | 建议单独执行并写入备注 |"
$executionRows += ("| AI 回放 | ai-replay.ps1 | {0} | {1} |" -f $aiStagePF, $aiStageRemark)
$executionRows += ("| 控制面回放 | control-replay.ps1 | {0} | {1} |" -f $controlStagePF, $controlStageRemark)
$executionRows += "| 控制面审计查询 | GET /api/control/audit | - | 建议通过控制台筛选或接口抽样验证 |"
$executionRows += ("| 检索命中率 | kb-eval hitrate | {0} | {1} |" -f $kbHitratePF, $kbHitrateRemark)
$executionRows += ("| 问答引用率 | kb-eval citation | {0} | {1} |" -f $kbCitationPF, $kbCitationRemark)
$executionRows += ("| MTTD 对比 | kb-eval mttd | {0} | {1} |" -f $kbMttdPF, $kbMttdRemark)

$metricRows = @()
$metricRows += "| gwf_upload_queue_full_total 增量 | 越低越好 | $(Format-NullableNumber -Value $uploadQueueFullTotal -Digits 0) | $(To-PF -Pass $uploadQueuePass) |"
$metricRows += "| 上传失败率（10m） | < 5% | $(Format-NullableNumber -Value $uploadFailureRatePct -Digits 2 -Suffix '%') | $(To-PF -Pass $uploadFailurePass) |"
$metricRows += "| AI 降级率 | $aiTargetText | $(Format-NullableNumber -Value $aiDegradedRatioPct -Digits 2 -Suffix '%') | $(To-PF -Pass $aiPass) |"
$metricRows += "| AI 结构一致性通过率 | $aiStructureTargetText | $(Format-NullableNumber -Value $aiStructurePassRatioPct -Digits 2 -Suffix '%') | $(To-PF -Pass $aiStructurePass) |"
$metricRows += "| AI 错误分类覆盖率 | $aiErrorClassCoverageTargetText | $(Format-NullableNumber -Value $aiErrorClassCoveragePct -Digits 2 -Suffix '%') | $(To-PF -Pass $aiErrorClassCoveragePass) |"
$metricRows += "| 知识检索命中率 | $kbHitrateTargetText | $(Format-NullableNumber -Value $kbHitratePct -Digits 2 -Suffix '%') | $(To-PF -Pass $kbHitratePass) |"
$metricRows += "| 问答引用率 | $kbCitationTargetText | $(Format-NullableNumber -Value $kbCitationPct -Digits 2 -Suffix '%') | $(To-PF -Pass $kbCitationPass) |"
$metricRows += "| MTTD 下降比例 | $kbMttdDropTargetText | $(Format-NullableNumber -Value $mttdDropPct -Digits 2 -Suffix '%') | $(To-PF -Pass $mttdDropPass) |"
$metricRows += "| 控制面在线 Agent 数 | >= 1 | $(Format-NullableNumber -Value $controlOnline -Digits 0) | $(To-PF -Pass $controlOnlinePass) |"
$metricRows += "| 控制面 backlog | <= 50（参考阈值） | $(Format-NullableNumber -Value $controlBacklog -Digits 0) | $(To-PF -Pass $controlBacklogPass) |"
$metricRows += "| 控制面任务超时增量（10m） | = 0 | $(Format-NullableNumber -Value $controlTimeoutTotal -Digits 0) | $(To-PF -Pass $controlTimeoutPass) |"

$failureRows = @()
if ($null -ne $recap.stages) {
  foreach ($stage in $recap.stages) {
    if (-not [bool]$stage.ok) {
      $firstLine = Get-FirstLine -Text ([string]$stage.output)
      if ([string]::IsNullOrWhiteSpace($firstLine)) {
        $firstLine = "详见复盘输出"
      }
      $failureRows += "| $($stage.name) | 阶段执行失败 | exitCode=$($stage.exitCode)，$firstLine | 检查依赖服务与脚本参数后重跑 | 平台 | 待定 |"
    }
  }
}
if ($failureRows.Count -eq 0) {
  $failureRows += "| - | - | - | - | - | - |"
}

$riskLines = @()
if (-not [bool]$recap.allPassed) {
  $failedNames = @()
  if ($null -ne $recap.stages) {
    foreach ($stage in $recap.stages) {
      if (-not [bool]$stage.ok) {
        $failedNames += [string]$stage.name
      }
    }
  }
  if ($failedNames.Count -gt 0) {
    $riskLines += "1. 风险：阶段失败项未闭环（$($failedNames -join '、')）"
    $riskLines += "2. 待办：按失败阶段输出定位环境/配置问题并重跑 stage-recap.ps1"
    $riskLines += "3. 待办：重跑成功后使用 stage-report.ps1 重新生成报告并归档"
  }
} else {
  $riskLines += "1. 风险：当前阶段脚本通过，但仍需关注真实业务流量下的波动"
  $riskLines += "2. 待办：保持日常巡检并按阈值草案持续校准"
  $riskLines += "3. 待办：将本报告结论同步到里程碑进展文档"
}

$allowNext = if ([bool]$recap.allPassed) { "是" } else { "否" }
$summary = if ([bool]$recap.allPassed) {
  "阶段关键门禁通过，可进入下一阶段"
} else {
  "仍有阶段门禁未通过，需先完成失败项闭环"
}

$markdown = @()
$markdown += "# 阶段回归报告（$reportDate）"
$markdown += ""
$markdown += "- 报告日期：$reportDate"
$markdown += "- 执行人：$Operator"
$markdown += ('- 环境：`{0}`' -f $Environment)
$markdown += ('- 版本：`{0}`' -f $Version)
$markdown += ('- 复盘结果文件：`{0}`' -f $RecapFile)
if (-not [string]::IsNullOrWhiteSpace($metricsPath)) {
  $markdown += ('- 指标快照：`{0}`' -f $metricsPath)
} else {
  $markdown += '- 指标快照：`未找到`'
}
$markdown += ""
$markdown += "## 1. 目标与范围"
$markdown += ""
$markdown += "- 本轮目标：完成阶段脚本回放并自动生成可归档报告"
$markdown += "- 覆盖链路："
$markdown += "  - 上传链路"
$markdown += "  - AI 日志分析"
$markdown += "  - 知识库检索与问答"
$markdown += "  - 告警推荐"
$markdown += "  - 控制面任务分发与回放"
$markdown += ""
$markdown += "## 2. 执行清单"
$markdown += ""
$markdown += "| 项目 | 命令/入口 | 结果（P/F） | 备注 |"
$markdown += "| --- | --- | --- | --- |"
$markdown += $executionRows
$markdown += ""
$markdown += "## 3. 关键指标结果"
$markdown += ""
$markdown += "| 指标 | 目标 | 本次结果 | 是否达标 |"
$markdown += "| --- | --- | --- | --- |"
$markdown += $metricRows
$markdown += ""
$markdown += "补充："
$markdown += "- AI 总请求：$(Format-NullableNumber -Value $aiTotal -Digits 0)，成功：$(Format-NullableNumber -Value $aiSuccess -Digits 0)，降级：$(Format-NullableNumber -Value $aiDegraded -Digits 0)"
$markdown += "- AI 结构校验：checked=$(Format-NullableNumber -Value $aiStructureChecked -Digits 0)，failed=$(Format-NullableNumber -Value $aiStructureFailed -Digits 0)"
$markdown += "- MTTD 下降比例：$(Format-NullableNumber -Value $mttdDropPct -Digits 2 -Suffix '%')"
$markdown += ""
$markdown += "## 4. 失败样例与根因"
$markdown += ""
$markdown += "| 场景 | 现象 | 根因 | 修复动作 | Owner | 截止日期 |"
$markdown += "| --- | --- | --- | --- | --- | --- |"
$markdown += $failureRows
$markdown += ""
$markdown += "## 5. 风险与待办"
$markdown += ""
$markdown += $riskLines
$markdown += ""
$markdown += "## 6. 结论"
$markdown += ""
$markdown += ('- 是否允许进入下一阶段：`{0}`' -f $allowNext)
$markdown += "- 结论摘要：$summary"

$markdownText = ($markdown -join "`r`n") + "`r`n"
Set-Content -Path $OutputFile -Encoding UTF8 -Value $markdownText

Write-Host ("报告生成完成: {0}" -f $OutputFile)
exit 0
