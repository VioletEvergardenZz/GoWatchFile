# 本脚本用于知识库门禁复盘汇总
# 一次执行完成 hitrate / citation / mttd 三项评估 并输出“复盘 + 改进清单”

param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$SamplesFile = "../docs/04-知识库/知识库命中率样本.json",
  [string]$MttdFile = "../docs/04-知识库/知识库MTTD基线.csv",
  [double]$CitationTarget = 1.0,
  [double]$HitrateTarget = 0.8,
  [double]$MttdDropTarget = 0.2,
  [string]$FromResultFile = "",
  [string]$ReportFile = "",
  [string]$OutputFile = "../reports/kb-recap-result.json"
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

function Parse-PercentRatioFromOutput {
  param(
    [string]$OutputText,
    [string]$Pattern
  )
  if ([string]::IsNullOrWhiteSpace($OutputText)) {
    return $null
  }
  $match = [regex]::Match($OutputText, $Pattern, [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
  if (-not $match.Success) {
    return $null
  }
  $percent = Parse-DoubleInvariant -Raw $match.Groups[1].Value
  if ($null -eq $percent) {
    return $null
  }
  return [Math]::Round(([double]$percent / 100.0), 4)
}

function Format-Percent {
  param(
    $Ratio,
    [int]$Digits = 2
  )
  if ($null -eq $Ratio) {
    return "N/A"
  }
  return [string]::Format([System.Globalization.CultureInfo]::InvariantCulture, "{0:F$Digits}%", ([double]$Ratio * 100.0))
}

function To-PF {
  param($Pass)
  if ($null -eq $Pass) {
    return "-"
  }
  if ([bool]$Pass) {
    return "P"
  }
  return "F"
}

function Get-FirstLine {
  param([string]$Text)
  if ([string]::IsNullOrWhiteSpace($Text)) {
    return ""
  }
  return (($Text -split "`r?`n")[0]).Trim()
}

function New-ImprovementItem {
  param(
    [string]$Priority,
    [string]$Title,
    [string]$Action,
    [string]$Owner,
    [string]$Eta,
    [string]$Evidence
  )
  return [pscustomobject]@{
    priority = $Priority
    title    = $Title
    action   = $Action
    owner    = $Owner
    eta      = $Eta
    evidence = $Evidence
  }
}

function Build-KBAnalysis {
  param(
    $HitrateResult,
    $CitationResult,
    $MttdResult,
    [double]$HitrateTarget,
    [double]$CitationTarget,
    [double]$MttdDropTarget
  )

  $hitrateRatio = Parse-PercentRatioFromOutput -OutputText ([string]$HitrateResult.output) -Pattern "Hitrate:\s+\d+/\d+\s+=\s+([0-9.]+)%"
  $citationRatio = Parse-PercentRatioFromOutput -OutputText ([string]$CitationResult.output) -Pattern "Citation ratio:\s+\d+/\d+\s+=\s+([0-9.]+)%"
  $mttdDropRatio = Parse-PercentRatioFromOutput -OutputText ([string]$MttdResult.output) -Pattern "Drop ratio\s*:\s*([0-9.]+)%"

  $hitratePass = ($null -ne $hitrateRatio -and $hitrateRatio -ge $HitrateTarget)
  $citationPass = ($null -ne $citationRatio -and $citationRatio -ge $CitationTarget)
  $mttdDropPass = ($null -ne $mttdDropRatio -and $mttdDropRatio -ge $MttdDropTarget)

  $improvements = New-Object System.Collections.Generic.List[object]

  if (-not [bool]$HitrateResult.ok) {
    $improvements.Add((New-ImprovementItem -Priority "P0" -Title "检索命中率评估失败" -Action "检查 /api/kb/search 可用性与样本文件路径后重跑 hitrate" -Owner "知识库平台" -Eta "T+0" -Evidence (Get-FirstLine -Text ([string]$HitrateResult.output))))
  } elseif ($null -eq $hitrateRatio) {
    $improvements.Add((New-ImprovementItem -Priority "P1" -Title "检索命中率结果不可解析" -Action "统一 kb-eval hitrate 输出格式，确保能提取命中率百分比并落档" -Owner "知识库平台" -Eta "T+1" -Evidence "未匹配到 Hitrate 百分比字段"))
  } elseif ($hitrateRatio -lt $HitrateTarget) {
    $gap = [Math]::Round(($HitrateTarget - $hitrateRatio) * 100, 2)
    $improvements.Add((New-ImprovementItem -Priority "P1" -Title "检索命中率未达标" -Action ("补齐 FAQ/同义词关键词和标签治理，优先处理高频 miss 主题（差距 {0}%）" -f $gap) -Owner "知识库运营" -Eta "T+3" -Evidence ("current={0}, target={1}" -f (Format-Percent -Ratio $hitrateRatio), (Format-Percent -Ratio $HitrateTarget))))
  }

  if (-not [bool]$CitationResult.ok) {
    $improvements.Add((New-ImprovementItem -Priority "P0" -Title "问答引用率评估失败" -Action "检查 /api/kb/ask 可用性、文档发布状态与引用提取逻辑后重跑 citation" -Owner "知识库平台" -Eta "T+0" -Evidence (Get-FirstLine -Text ([string]$CitationResult.output))))
  } elseif ($null -eq $citationRatio) {
    $improvements.Add((New-ImprovementItem -Priority "P1" -Title "问答引用率结果不可解析" -Action "统一 kb-eval citation 输出格式，确保能提取引用率百分比并落档" -Owner "知识库平台" -Eta "T+1" -Evidence "未匹配到 Citation ratio 百分比字段"))
  } elseif ($citationRatio -lt $CitationTarget) {
    $gap = [Math]::Round(($CitationTarget - $citationRatio) * 100, 2)
    $improvements.Add((New-ImprovementItem -Priority "P1" -Title "问答引用率未达标" -Action ("补齐可引用正文段落与条目发布覆盖，优先修复无 citation 的样本（差距 {0}%）" -f $gap) -Owner "知识库运营" -Eta "T+2" -Evidence ("current={0}, target={1}" -f (Format-Percent -Ratio $citationRatio), (Format-Percent -Ratio $CitationTarget))))
  }

  if (-not [bool]$MttdResult.ok) {
    $improvements.Add((New-ImprovementItem -Priority "P1" -Title "MTTD 对比评估失败" -Action "检查 MTTD 输入文件格式与样本完整性，修复后重跑 mttd" -Owner "知识库平台" -Eta "T+1" -Evidence (Get-FirstLine -Text ([string]$MttdResult.output))))
  } elseif ($null -eq $mttdDropRatio) {
    $improvements.Add((New-ImprovementItem -Priority "P2" -Title "MTTD 下降比例不可解析" -Action "统一 kb-eval mttd 输出格式，确保可提取 Drop ratio 并用于趋势分析" -Owner "知识库平台" -Eta "T+2" -Evidence "未匹配到 Drop ratio 百分比字段"))
  } elseif ($mttdDropRatio -lt $MttdDropTarget) {
    $gap = [Math]::Round(($MttdDropTarget - $mttdDropRatio) * 100, 2)
    $improvements.Add((New-ImprovementItem -Priority "P2" -Title "MTTD 下降收益不足" -Action ("补齐告警推荐映射与排障步骤模板，提升首轮定位效率（差距 {0}%）" -f $gap) -Owner "知识库运营" -Eta "T+5" -Evidence ("current={0}, target={1}" -f (Format-Percent -Ratio $mttdDropRatio), (Format-Percent -Ratio $MttdDropTarget))))
  }

  if ($improvements.Count -eq 0) {
    $improvements.Add((New-ImprovementItem -Priority "P3" -Title "门禁通过 进入持续优化" -Action "维持周度抽样复盘，新增样本优先覆盖新上线功能与高频告警主题" -Owner "知识库运营" -Eta "持续" -Evidence "hitrate/citation/mttd 当前均达标"))
  }

  return [pscustomobject]@{
    targets      = [pscustomobject]@{
      hitrate      = $HitrateTarget
      citation     = $CitationTarget
      mttdDrop     = $MttdDropTarget
    }
    metrics      = [pscustomobject]@{
      hitrate      = $hitrateRatio
      citation     = $citationRatio
      mttdDrop     = $mttdDropRatio
    }
    gates        = [pscustomobject]@{
      hitratePass  = $hitratePass
      citationPass = $citationPass
      mttdDropPass = $mttdDropPass
      allPassed    = ($hitratePass -and $citationPass -and $mttdDropPass)
    }
    improvements = $improvements
  }
}

function Write-RecapReport {
  param(
    $Result,
    [string]$Path
  )

  $reportDir = Split-Path -Parent $Path
  Ensure-Dir $reportDir

  $analysis = $Result.analysis
  $metrics = $analysis.metrics
  $targets = $analysis.targets
  $gates = $analysis.gates
  $commandPassedText = if ($Result.commandPassed) { "通过" } else { "失败" }
  $metricGateText = if ($gates.allPassed) { "通过" } else { "失败" }
  $allPassedText = if ($Result.allPassed) { "通过" } else { "未通过" }

  $lines = @()
  $lines += ("# 知识库命中率/引用率阶段复盘与改进清单（{0}）" -f (Get-Date).ToString("yyyy-MM-dd"))
  $lines += ""
  $lines += ("- 生成时间：{0}" -f $Result.generatedAt)
  $lines += ('- BaseUrl：`{0}`' -f $Result.baseUrl)
  $lines += ('- 样本文件：`{0}`' -f $Result.samplesFile)
  $lines += ('- MTTD 文件：`{0}`' -f $Result.mttdFile)
  $lines += ('- 命令执行门禁：`{0}`' -f $commandPassedText)
  $lines += ('- 指标门禁：`{0}`' -f $metricGateText)
  $lines += ('- 总结论：`{0}`' -f $allPassedText)
  $lines += ""
  $lines += "## 1. 指标结果"
  $lines += ""
  $lines += "| 指标 | 目标 | 当前 | 门禁 |"
  $lines += "| --- | --- | --- | --- |"
  $lines += ("| 检索命中率 | >= {0} | {1} | {2} |" -f (Format-Percent -Ratio $targets.hitrate), (Format-Percent -Ratio $metrics.hitrate), (To-PF -Pass $gates.hitratePass))
  $lines += ("| 问答引用率 | >= {0} | {1} | {2} |" -f (Format-Percent -Ratio $targets.citation), (Format-Percent -Ratio $metrics.citation), (To-PF -Pass $gates.citationPass))
  $lines += ("| MTTD 下降比例 | >= {0} | {1} | {2} |" -f (Format-Percent -Ratio $targets.mttdDrop), (Format-Percent -Ratio $metrics.mttdDrop), (To-PF -Pass $gates.mttdDropPass))
  $lines += ""
  $lines += "## 2. 改进清单"
  $lines += ""
  $lines += "| 优先级 | 主题 | 动作 | Owner | 时限 | 证据 |"
  $lines += "| --- | --- | --- | --- | --- | --- |"
  foreach ($item in $analysis.improvements) {
    $evidence = ([string]$item.evidence) -replace "\|", "/"
    $lines += ("| {0} | {1} | {2} | {3} | {4} | {5} |" -f $item.priority, $item.title, $item.action, $item.owner, $item.eta, $evidence)
  }
  $lines += ""
  $lines += "## 3. 评估输出摘要"
  $lines += ""
  $lines += ("- hitrate: {0}" -f (Get-FirstLine -Text ([string]$Result.hitrate.output)))
  $lines += ("- citation: {0}" -f (Get-FirstLine -Text ([string]$Result.citation.output)))
  $lines += ("- mttd: {0}" -f (Get-FirstLine -Text ([string]$Result.mttd.output)))

  $text = ($lines -join "`r`n") + "`r`n"
  Set-Content -Path $Path -Encoding UTF8 -Value $text
}

# 统一封装评估命令调用：
# - 保留标准输出/错误输出，便于失败后直接定位门禁项
# - 不在函数内部抛异常，统一由返回结构和调用方决定退出码
function Invoke-EvalCommand {
  param(
    [string]$Name,
    [string[]]$CommandArgs
  )

  Write-Host ("执行 {0}: go {1}" -f $Name, ($CommandArgs -join " "))

  $output = & go @CommandArgs 2>&1
  $exitCode = $LASTEXITCODE
  # 输出统一转文本，避免后续写 JSON 时出现对象序列化差异
  $text = ($output | Out-String).Trim()

  return [pscustomobject]@{
    name     = $Name
    ok       = ($exitCode -eq 0)
    exitCode = $exitCode
    output   = $text
  }
}

$scriptDir = (Resolve-Path $PSScriptRoot).Path
$goWatchDir = (Resolve-Path (Join-Path $scriptDir "..\..")).Path
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..\..\..")).Path
$currentDir = (Get-Location).Path

# 统一规范 base URL，避免拼接 endpoint 时出现双斜杠
$base = $BaseUrl.TrimEnd("/")

$hitrate = $null
$citation = $null
$mttd = $null
$samplesForOutput = $SamplesFile
$mttdForOutput = $MttdFile
$baseForOutput = $base
$citationTargetForOutput = $CitationTarget
$generatedAt = (Get-Date).ToString("s")

if (-not [string]::IsNullOrWhiteSpace($FromResultFile)) {
  $sourcePath = Resolve-ExistingPath -Candidate $FromResultFile -BaseDirs @($currentDir, $goWatchDir, $repoRoot, $scriptDir)
  if ([string]::IsNullOrWhiteSpace($sourcePath)) {
    Write-Error "未找到复盘结果文件: $FromResultFile"
    exit 2
  }
  $raw = Get-Content -Raw -Encoding UTF8 $sourcePath
  if ([string]::IsNullOrWhiteSpace($raw)) {
    Write-Error "复盘结果文件为空: $sourcePath"
    exit 2
  }
  try {
    $source = $raw | ConvertFrom-Json
  } catch {
    Write-Error ("解析复盘结果文件失败: {0}" -f $_.Exception.Message)
    exit 2
  }

  if ($null -eq $source.hitrate -or $null -eq $source.citation -or $null -eq $source.mttd) {
    Write-Error "复盘结果缺少 hitrate/citation/mttd 字段"
    exit 2
  }

  $hitrate = $source.hitrate
  $citation = $source.citation
  $mttd = $source.mttd
  if ($null -ne $source.samplesFile -and -not [string]::IsNullOrWhiteSpace([string]$source.samplesFile)) {
    $samplesForOutput = [string]$source.samplesFile
  }
  if ($null -ne $source.mttdFile -and -not [string]::IsNullOrWhiteSpace([string]$source.mttdFile)) {
    $mttdForOutput = [string]$source.mttdFile
  }
  if ($null -ne $source.baseUrl -and -not [string]::IsNullOrWhiteSpace([string]$source.baseUrl)) {
    $baseForOutput = [string]$source.baseUrl
  }
  if ($null -ne $source.citationTarget) {
    $citationTargetForOutput = [double]$source.citationTarget
  }
  if ($null -ne $source.generatedAt -and -not [string]::IsNullOrWhiteSpace([string]$source.generatedAt)) {
    $generatedAt = [string]$source.generatedAt
  }
} else {
  $samplesPath = Resolve-ExistingPath -Candidate $SamplesFile -BaseDirs @($currentDir, $goWatchDir, $repoRoot, $scriptDir)
  if ([string]::IsNullOrWhiteSpace($samplesPath)) {
    Write-Error "样本文件不存在: $SamplesFile"
    exit 2
  }
  $mttdPath = Resolve-ExistingPath -Candidate $MttdFile -BaseDirs @($currentDir, $goWatchDir, $repoRoot, $scriptDir)
  if ([string]::IsNullOrWhiteSpace($mttdPath)) {
    Write-Error "MTTD 文件不存在: $MttdFile"
    exit 2
  }

  $samplesForOutput = $samplesPath
  $mttdForOutput = $mttdPath

  $hitrateArgs = @(
    "run", "./cmd/kb-eval", "hitrate",
    "-base", $base,
    "-samples", $samplesPath,
    "-limit", "5"
  )
  $citationArgs = @(
    "run", "./cmd/kb-eval", "citation",
    "-base", $base,
    "-samples", $samplesPath,
    "-limit", "3",
    "-target", ([string]$CitationTarget)
  )

  # 命中率评估采用更高检索上限，尽量暴露召回不足问题
  $hitrate = Invoke-EvalCommand -Name "hitrate" -CommandArgs $hitrateArgs

  # 引用率评估限制返回条数，重点检查引用命中而非长答案稳定性
  $citation = Invoke-EvalCommand -Name "citation" -CommandArgs $citationArgs

  # MTTD 仅依赖本地输入文件，和在线服务解耦，便于离线复盘
  $mttd = Invoke-EvalCommand -Name "mttd" -CommandArgs @(
    "run", "./cmd/kb-eval", "mttd",
    "-input", $mttdPath
  )
}

$analysis = Build-KBAnalysis -HitrateResult $hitrate -CitationResult $citation -MttdResult $mttd -HitrateTarget $HitrateTarget -CitationTarget $citationTargetForOutput -MttdDropTarget $MttdDropTarget
$commandPassed = ([bool]$hitrate.ok -and [bool]$citation.ok -and [bool]$mttd.ok)
$allPassed = ($commandPassed -and [bool]$analysis.gates.allPassed)

# 汇总结构固定字段，便于阶段脚本和控制台做机器可读解析
$summary = [pscustomobject]@{
  baseUrl         = $baseForOutput
  samplesFile     = $samplesForOutput
  mttdFile        = $mttdForOutput
  citationTarget  = $citationTargetForOutput
  hitrateTarget   = $HitrateTarget
  mttdDropTarget  = $MttdDropTarget
  generatedAt     = $generatedAt
  hitrate         = $hitrate
  citation        = $citation
  mttd            = $mttd
  commandPassed   = $commandPassed
  allPassed       = $allPassed
  analysis        = $analysis
}

# 输出目录不存在则自动创建，减少 CI/本地执行时的前置依赖
$outDir = Split-Path -Parent $OutputFile
Ensure-Dir $outDir
$summary | ConvertTo-Json -Depth 10 | Set-Content -Path $OutputFile -Encoding utf8

if (-not [string]::IsNullOrWhiteSpace($ReportFile)) {
  Write-RecapReport -Result $summary -Path $ReportFile
  Write-Host ("改进清单报告: {0}" -f $ReportFile)
}

Write-Host ("复盘完成: commandPassed={0} metricGatePassed={1} allPassed={2}" -f $commandPassed, $analysis.gates.allPassed, $summary.allPassed)
Write-Host ("结果文件: {0}" -f $OutputFile)

# 退出码约定：
# 0=全部通过，2=输入参数/文件问题，3=至少一个门禁失败
if (-not $summary.allPassed) {
  exit 3
}
exit 0
