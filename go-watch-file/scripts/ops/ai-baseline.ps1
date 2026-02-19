# 本脚本用于 AI 基线验证（固定样例 + 结构稳定性门禁）
# 支持两种模式：
# 1) 在线模式：调用 ai-replay.ps1 触发真实回放
# 2) 离线模式：读取已有 ai-replay 结果文件做二次验收

param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$Token = "",
  [string]$PathsFile = "../docs/03-告警与AI/AI回放路径清单.txt",
  [int]$Limit = 200,
  [double]$DegradedRatioTarget = 0.2,
  [double]$StructurePassRatioTarget = 1.0,
  [double]$ErrorClassCoverageTarget = 1.0,
  [double]$SummaryPassRatioTarget = 1.0,
  [double]$SeverityPassRatioTarget = 1.0,
  [double]$SuggestionsPassRatioTarget = 1.0,
  [switch]$AutoPrime,
  [string]$PrimeDocsPath = "../docs",
  [string]$PrimeOperator = "ai-baseline",
  [bool]$PrimeApproveImported = $false,
  [string]$PrimeAISamplesDir = "../reports/ai-replay-samples",
  [bool]$PrimeUpdateAlertLogPaths = $true,
  [string]$PrimeOutputFile = "../reports/stage-prime-result.json",
  [string]$FromResultFile = "",
  [string]$ReplayOutputFile = "../reports/ai-replay-result.json",
  [string]$ReportFile = "",
  [string]$OutputFile = "../reports/ai-baseline-result.json"
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
      return [System.IO.Path]::GetFullPath($Candidate)
    }
    return ""
  }
  foreach ($baseDir in $BaseDirs) {
    if ([string]::IsNullOrWhiteSpace($baseDir)) {
      continue
    }
    $tryPath = Join-Path $baseDir $Candidate
    if (Test-Path $tryPath) {
      return [System.IO.Path]::GetFullPath($tryPath)
    }
  }
  if (Test-Path $Candidate) {
    return [System.IO.Path]::GetFullPath($Candidate)
  }
  return ""
}

function Resolve-OutputPath {
  param([string]$Candidate)
  if ([string]::IsNullOrWhiteSpace($Candidate)) {
    return ""
  }
  if ([System.IO.Path]::IsPathRooted($Candidate)) {
    return [System.IO.Path]::GetFullPath($Candidate)
  }
  return [System.IO.Path]::GetFullPath((Join-Path (Get-Location).Path $Candidate))
}

function Parse-DoubleInvariant {
  param([string]$Raw)
  $value = 0.0
  if ([double]::TryParse($Raw, [System.Globalization.NumberStyles]::Float, [System.Globalization.CultureInfo]::InvariantCulture, [ref]$value)) {
    return $value
  }
  return $null
}

function Get-IntFromValue {
  param(
    $Value,
    [int]$Default = 0
  )
  if ($null -eq $Value) {
    return $Default
  }
  $parsed = Parse-DoubleInvariant -Raw ([string]$Value)
  if ($null -eq $parsed) {
    return $Default
  }
  return [int][Math]::Round([double]$parsed, 0)
}

function Get-DoubleFromValue {
  param(
    $Value,
    [double]$Default = 0.0
  )
  if ($null -eq $Value) {
    return $Default
  }
  $parsed = Parse-DoubleInvariant -Raw ([string]$Value)
  if ($null -eq $parsed) {
    return $Default
  }
  return [double]$parsed
}

function Get-PropertyValue {
  param(
    $Object,
    [string]$Name
  )
  if ($null -eq $Object -or [string]::IsNullOrWhiteSpace($Name)) {
    return $null
  }
  $prop = $Object.PSObject.Properties[$Name]
  if ($null -eq $prop) {
    return $null
  }
  return $prop.Value
}

function Has-Property {
  param(
    $Object,
    [string]$Name
  )
  if ($null -eq $Object -or [string]::IsNullOrWhiteSpace($Name)) {
    return $false
  }
  return ($null -ne $Object.PSObject.Properties[$Name])
}

function Get-FirstLine {
  param([string]$Text)
  if ([string]::IsNullOrWhiteSpace($Text)) {
    return ""
  }
  return (($Text -split "`r?`n")[0]).Trim()
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

function Get-StructureIssuesFromRow {
  param($Row)
  if ($null -eq $Row -or -not (Has-Property -Object $Row -Name "structureIssues")) {
    return @()
  }
  if ($null -eq $Row.structureIssues) {
    return @()
  }
  return @($Row.structureIssues)
}

function Get-IssueCountFromRows {
  param(
    [array]$Rows,
    [string[]]$IssueNames
  )
  $count = 0
  foreach ($row in @($Rows)) {
    $issues = Get-StructureIssuesFromRow -Row $row
    if ($issues.Count -eq 0) {
      continue
    }
    $hit = $false
    foreach ($issue in $issues) {
      $text = ([string]$issue).Trim()
      foreach ($issueName in $IssueNames) {
        if ($text -eq $issueName) {
          $hit = $true
          break
        }
      }
      if ($hit) {
        break
      }
    }
    if ($hit) {
      $count++
    }
  }
  return $count
}

function Get-IssueCountFromMap {
  param(
    $MapObject,
    [string[]]$IssueNames
  )
  if ($null -eq $MapObject) {
    return 0
  }
  $sum = 0
  foreach ($issueName in $IssueNames) {
    if (-not (Has-Property -Object $MapObject -Name $issueName)) {
      continue
    }
    $sum += Get-IntFromValue -Value (Get-PropertyValue -Object $MapObject -Name $issueName)
  }
  return $sum
}

function Get-IssueCount {
  param(
    [array]$Rows,
    $IssuesMap,
    [string[]]$IssueNames
  )
  $rowsWithStructure = 0
  foreach ($row in @($Rows)) {
    if (Has-Property -Object $row -Name "structureIssues") {
      $rowsWithStructure++
      break
    }
  }
  if ($rowsWithStructure -gt 0) {
    return Get-IssueCountFromRows -Rows $Rows -IssueNames $IssueNames
  }
  return Get-IssueCountFromMap -MapObject $IssuesMap -IssueNames $IssueNames
}

function Invoke-PowerShellScript {
  param(
    [string]$ScriptPath,
    [string[]]$Arguments
  )
  if (-not (Test-Path $ScriptPath)) {
    return [pscustomobject]@{
      ok       = $false
      exitCode = 2
      output   = ("script not found: {0}" -f $ScriptPath)
    }
  }
  $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $ScriptPath @Arguments 2>&1
  $exitCode = $LASTEXITCODE
  $text = ($output | Out-String).Trim()
  return [pscustomobject]@{
    ok       = ($exitCode -eq 0)
    exitCode = $exitCode
    output   = $text
  }
}

function Write-BaselineReport {
  param(
    $Result,
    [string]$Path
  )

  $dir = Split-Path -Parent $Path
  Ensure-Dir $dir

  $summaryText = if ($Result.gates.allPassed) { "PASS" } else { "FAIL" }
  $lines = @()
  $lines += ("# AI 基线验证报告（{0}）" -f (Get-Date).ToString("yyyy-MM-dd"))
  $lines += ""
  $lines += ("- 生成时间：{0}" -f $Result.generatedAt)
  $lines += ("- 模式：{0}" -f $Result.mode)
  $lines += ("- BaseUrl：{0}" -f $Result.baseUrl)
  $lines += ("- AI 回放结果：{0}" -f $Result.artifacts.aiReplayResult)
  $lines += ("- 总结论：{0}" -f $summaryText)
  $lines += ""
  $lines += "## 1. 门禁结果"
  $lines += ""
  $lines += "| 项目 | 当前值 | 目标值 | 结果 |"
  $lines += "| --- | --- | --- | --- |"
  $lines += ("| 降级率 | {0} | <= {1} | {2} |" -f (Format-Percent -Ratio $Result.replay.degradedRatio), (Format-Percent -Ratio $Result.targets.degradedRatio), (To-PF -Pass $Result.gates.replay.degradedRatioPass))
  $lines += ("| 结构通过率 | {0} | >= {1} | {2} |" -f (Format-Percent -Ratio $Result.replay.structurePassRatio), (Format-Percent -Ratio $Result.targets.structurePassRatio), (To-PF -Pass $Result.gates.replay.structurePassRatioPass))
  $lines += ("| 错误分类覆盖率 | {0} | >= {1} | {2} |" -f (Format-Percent -Ratio $Result.replay.errorClassCoverage), (Format-Percent -Ratio $Result.targets.errorClassCoverage), (To-PF -Pass $Result.gates.replay.errorClassCoveragePass))
  $lines += ("| summary 通过率 | {0} | >= {1} | {2} |" -f (Format-Percent -Ratio $Result.analysis.summaryPassRatio), (Format-Percent -Ratio $Result.targets.summaryPassRatio), (To-PF -Pass $Result.gates.baseline.summaryPass))
  $lines += ("| severity 通过率 | {0} | >= {1} | {2} |" -f (Format-Percent -Ratio $Result.analysis.severityPassRatio), (Format-Percent -Ratio $Result.targets.severityPassRatio), (To-PF -Pass $Result.gates.baseline.severityPass))
  $lines += ("| suggestions 通过率 | {0} | >= {1} | {2} |" -f (Format-Percent -Ratio $Result.analysis.suggestionsPassRatio), (Format-Percent -Ratio $Result.targets.suggestionsPassRatio), (To-PF -Pass $Result.gates.baseline.suggestionsPass))
  $lines += ""
  $lines += "## 2. 样本统计"
  $lines += ""
  $lines += ("- total: {0}" -f $Result.replay.total)
  $lines += ("- success: {0}" -f $Result.replay.success)
  $lines += ("- degraded: {0}" -f $Result.replay.degraded)
  $lines += ("- structureChecked: {0}" -f $Result.analysis.structureChecked)
  $lines += ("- summaryIssueCount: {0}" -f $Result.analysis.summaryIssueCount)
  $lines += ("- severityIssueCount: {0}" -f $Result.analysis.severityIssueCount)
  $lines += ("- suggestionsIssueCount: {0}" -f $Result.analysis.suggestionsIssueCount)
  $lines += ""
  $lines += "## 3. 执行摘要"
  $lines += ""
  $lines += ("- replay exitCode: {0}" -f $Result.execution.replayExitCode)
  $lines += ("- replay output: {0}" -f (Get-FirstLine -Text ([string]$Result.execution.replayOutput)))
  foreach ($note in @($Result.notes)) {
    $lines += ("- note: {0}" -f $note)
  }

  Set-Content -Path $Path -Encoding UTF8 -Value (($lines -join "`r`n") + "`r`n")
}

$scriptDir = (Resolve-Path $PSScriptRoot).Path
$goWatchDir = (Resolve-Path (Join-Path $scriptDir "..\..")).Path
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..\..\..")).Path
$currentDir = (Get-Location).Path
$base = $BaseUrl.TrimEnd("/")

$lookupDirs = @($currentDir, $goWatchDir, $repoRoot, $scriptDir)
$outputPath = Resolve-OutputPath -Candidate $OutputFile
if ([string]::IsNullOrWhiteSpace($outputPath)) {
  Write-Error "无效 OutputFile"
  exit 2
}
Ensure-Dir (Split-Path -Parent $outputPath)

$mode = if ([string]::IsNullOrWhiteSpace($FromResultFile)) { "live" } else { "offline" }
$notes = New-Object System.Collections.Generic.List[string]
$aiReplayResultPath = ""
$aiReplayResult = $null
$replayExitCode = 0
$replayOutput = ""
$primeExitCode = 0
$primeOutput = ""
$primeResult = $null
$pathsFileForOutput = $PathsFile

$aiReplayScript = Join-Path $scriptDir "ai-replay.ps1"
$stagePrimeScript = Join-Path $scriptDir "stage-prime.ps1"

if (-not (Test-Path $aiReplayScript) -and $mode -eq "live") {
  Write-Error "缺少脚本: ai-replay.ps1"
  exit 2
}
if ($AutoPrime -and $mode -eq "live" -and -not (Test-Path $stagePrimeScript)) {
  Write-Error "缺少脚本: stage-prime.ps1"
  exit 2
}

if ($mode -eq "offline") {
  $resolvedResult = Resolve-ExistingPath -Candidate $FromResultFile -BaseDirs $lookupDirs
  if ([string]::IsNullOrWhiteSpace($resolvedResult)) {
    Write-Error ("未找到回放结果文件: {0}" -f $FromResultFile)
    exit 2
  }
  $aiReplayResultPath = $resolvedResult
  $notes.Add("offline mode enabled via -FromResultFile")
} else {
  $aiReplayResultPath = Resolve-OutputPath -Candidate $ReplayOutputFile
  Ensure-Dir (Split-Path -Parent $aiReplayResultPath)

  if ($AutoPrime) {
    $primeOutputPath = Resolve-OutputPath -Candidate $PrimeOutputFile
    Ensure-Dir (Split-Path -Parent $primeOutputPath)
    $pathsForPrime = Resolve-OutputPath -Candidate $PathsFile
    $pathsFileForOutput = $pathsForPrime

    $primeRun = Invoke-PowerShellScript -ScriptPath $stagePrimeScript -Arguments @(
      "-BaseUrl", $base,
      "-DocsPath", $PrimeDocsPath,
      "-Operator", $PrimeOperator,
      "-ApproveImported", ([string]$PrimeApproveImported),
      "-AIPathsFile", $pathsForPrime,
      "-AISamplesDir", $PrimeAISamplesDir,
      "-UpdateAlertLogPaths", ([string]$PrimeUpdateAlertLogPaths),
      "-OutputFile", $primeOutputPath
    ) + $(if (-not [string]::IsNullOrWhiteSpace($Token)) { @("-Token", $Token.Trim()) } else { @() })

    $primeExitCode = $primeRun.exitCode
    $primeOutput = $primeRun.output
    if (-not $primeRun.ok) {
      $notes.Add(("stage-prime failed: exitCode={0}" -f $primeRun.exitCode))
    }
    if (Test-Path $primeOutputPath) {
      try {
        $primeResult = Get-Content -Raw -Encoding UTF8 $primeOutputPath | ConvertFrom-Json
      } catch {
        $notes.Add(("failed to parse stage-prime output: {0}" -f $_.Exception.Message))
      }
    }
  } else {
    $resolvedPaths = Resolve-ExistingPath -Candidate $PathsFile -BaseDirs $lookupDirs
    if ([string]::IsNullOrWhiteSpace($resolvedPaths)) {
      Write-Error ("未找到 PathsFile: {0}" -f $PathsFile)
      exit 2
    }
    $pathsFileForOutput = $resolvedPaths
  }

  $aiRunArgs = @(
    "-BaseUrl", $base,
    "-PathsFile", $pathsFileForOutput,
    "-Limit", ([string]$Limit),
    "-DegradedRatioTarget", ([string]$DegradedRatioTarget),
    "-StructurePassRatioTarget", ([string]$StructurePassRatioTarget),
    "-ErrorClassCoverageTarget", ([string]$ErrorClassCoverageTarget),
    "-OutputFile", $aiReplayResultPath
  )
  if (-not [string]::IsNullOrWhiteSpace($Token)) {
    $aiRunArgs += @("-Token", $Token.Trim())
  }
  $aiRun = Invoke-PowerShellScript -ScriptPath $aiReplayScript -Arguments $aiRunArgs
  $replayExitCode = $aiRun.exitCode
  $replayOutput = $aiRun.output
}

if (-not (Test-Path $aiReplayResultPath)) {
  Write-Error ("未找到 AI 回放结果文件: {0}" -f $aiReplayResultPath)
  exit 2
}

$rawReplay = Get-Content -Raw -Encoding UTF8 $aiReplayResultPath
if ([string]::IsNullOrWhiteSpace($rawReplay)) {
  Write-Error ("AI 回放结果文件为空: {0}" -f $aiReplayResultPath)
  exit 2
}
try {
  $aiReplayResult = $rawReplay | ConvertFrom-Json
} catch {
  Write-Error ("解析 AI 回放结果失败: {0}" -f $_.Exception.Message)
  exit 2
}

$rows = @()
if ($null -ne $aiReplayResult.results) {
  $rows = @($aiReplayResult.results)
}

$total = Get-IntFromValue -Value (Get-PropertyValue -Object $aiReplayResult -Name "total") -Default $rows.Count
$success = Get-IntFromValue -Value (Get-PropertyValue -Object $aiReplayResult -Name "success")
$degraded = Get-IntFromValue -Value (Get-PropertyValue -Object $aiReplayResult -Name "degraded")
$degradedRatio = Get-DoubleFromValue -Value (Get-PropertyValue -Object $aiReplayResult -Name "degradedRatio")
if ($total -gt 0 -and $degradedRatio -eq 0.0 -and $degraded -gt 0) {
  $degradedRatio = [Math]::Round(([double]$degraded / [double]$total), 4)
}

$structureObj = Get-PropertyValue -Object $aiReplayResult -Name "structure"
$structureIssuesMap = $null
$structureChecked = 0
$structurePassRatio = 0.0
if ($null -ne $structureObj) {
  $structureChecked = Get-IntFromValue -Value (Get-PropertyValue -Object $structureObj -Name "checked")
  $structurePassRatio = Get-DoubleFromValue -Value (Get-PropertyValue -Object $structureObj -Name "passRatio")
  $structureIssuesMap = Get-PropertyValue -Object $structureObj -Name "issues"
}
if ($structureChecked -le 0) {
  if ($rows.Count -gt 0) {
    $structureChecked = $rows.Count
  } elseif ($total -gt 0) {
    $structureChecked = $total
  }
}

$hasRowStructureInfo = $false
foreach ($row in $rows) {
  if ((Has-Property -Object $row -Name "structureIssues") -or (Has-Property -Object $row -Name "structureChecked")) {
    $hasRowStructureInfo = $true
    break
  }
}
$hasStructureRatio = ($null -ne $structureObj -and (Has-Property -Object $structureObj -Name "passRatio"))
$hasStructureEvidence = ($hasRowStructureInfo -or $hasStructureRatio)
if (-not $hasStructureEvidence) {
  $notes.Add("ai replay result missing structure evidence (structure.* or results[].structureIssues)")
}

$summaryIssueCount = Get-IssueCount -Rows $rows -IssuesMap $structureIssuesMap -IssueNames @("summary_missing")
$severityIssueCount = Get-IssueCount -Rows $rows -IssuesMap $structureIssuesMap -IssueNames @("severity_invalid")
$suggestionsIssueCount = Get-IssueCount -Rows $rows -IssuesMap $structureIssuesMap -IssueNames @("suggestions_missing", "suggestions_empty", "suggestions_over_limit")

if ($structureChecked -le 0) {
  $notes.Add("structureChecked is 0, cannot verify summary/severity/suggestions stability")
}

$summaryPassRatio = 0.0
$severityPassRatio = 0.0
$suggestionsPassRatio = 0.0
if ($structureChecked -gt 0) {
  $summaryFailCount = [Math]::Min($summaryIssueCount, $structureChecked)
  $severityFailCount = [Math]::Min($severityIssueCount, $structureChecked)
  $suggestionsFailCount = [Math]::Min($suggestionsIssueCount, $structureChecked)
  $summaryPassRatio = [Math]::Round((1.0 - ($summaryFailCount / [double]$structureChecked)), 4)
  $severityPassRatio = [Math]::Round((1.0 - ($severityFailCount / [double]$structureChecked)), 4)
  $suggestionsPassRatio = [Math]::Round((1.0 - ($suggestionsFailCount / [double]$structureChecked)), 4)
}

$errorClassCoverage = Get-DoubleFromValue -Value (Get-PropertyValue -Object $aiReplayResult -Name "errorClassCoverage")
if ($errorClassCoverage -eq 0.0 -and $degraded -eq 0) {
  $errorClassCoverage = 1.0
}

$replayGates = Get-PropertyValue -Object $aiReplayResult -Name "gates"
$replayDegradedPass = $false
$replayStructurePass = $false
$replayErrorClassPass = $false
if ($null -ne $replayGates) {
  $replayDegradedPass = [bool](Get-PropertyValue -Object $replayGates -Name "degradedRatioPass")
  $replayStructurePass = [bool](Get-PropertyValue -Object $replayGates -Name "structurePassRatioPass")
  $replayErrorClassPass = [bool](Get-PropertyValue -Object $replayGates -Name "errorClassCoveragePass")
} else {
  $replayDegradedPass = ($degradedRatio -le $DegradedRatioTarget)
  $replayStructurePass = ($structurePassRatio -ge $StructurePassRatioTarget)
  $replayErrorClassPass = ($errorClassCoverage -ge $ErrorClassCoverageTarget)
}
$replayAllPassed = ($replayDegradedPass -and $replayStructurePass -and $replayErrorClassPass)

$summaryPass = ($hasStructureEvidence -and $summaryPassRatio -ge $SummaryPassRatioTarget)
$severityPass = ($hasStructureEvidence -and $severityPassRatio -ge $SeverityPassRatioTarget)
$suggestionsPass = ($hasStructureEvidence -and $suggestionsPassRatio -ge $SuggestionsPassRatioTarget)
$baselineAllPassed = ($summaryPass -and $severityPass -and $suggestionsPass)

$totalPass = ($total -gt 0)
if (-not $totalPass) {
  $notes.Add("ai replay total is 0")
}

$allPassed = ($replayAllPassed -and $baselineAllPassed -and $totalPass)

$summary = [pscustomobject]@{
  generatedAt = (Get-Date).ToString("s")
  mode        = $mode
  baseUrl     = $base
  artifacts   = [pscustomobject]@{
    pathsFile      = $pathsFileForOutput
    aiReplayResult = $aiReplayResultPath
    primeResult    = $(if ($AutoPrime -and $mode -eq "live") { Resolve-OutputPath -Candidate $PrimeOutputFile } else { "" })
  }
  replay      = [pscustomobject]@{
    total              = $total
    success            = $success
    degraded           = $degraded
    degradedRatio      = $degradedRatio
    structurePassRatio = $structurePassRatio
    errorClassCoverage = $errorClassCoverage
  }
  analysis    = [pscustomobject]@{
    structureEvidenceReady = $hasStructureEvidence
    structureChecked       = $structureChecked
    summaryIssueCount      = $summaryIssueCount
    severityIssueCount     = $severityIssueCount
    suggestionsIssueCount  = $suggestionsIssueCount
    summaryPassRatio       = $summaryPassRatio
    severityPassRatio      = $severityPassRatio
    suggestionsPassRatio   = $suggestionsPassRatio
  }
  targets     = [pscustomobject]@{
    degradedRatio        = $DegradedRatioTarget
    structurePassRatio   = $StructurePassRatioTarget
    errorClassCoverage   = $ErrorClassCoverageTarget
    summaryPassRatio     = $SummaryPassRatioTarget
    severityPassRatio    = $SeverityPassRatioTarget
    suggestionsPassRatio = $SuggestionsPassRatioTarget
  }
  execution   = [pscustomobject]@{
    primeExitCode  = $primeExitCode
    primeOutput    = $primeOutput
    replayExitCode = $replayExitCode
    replayOutput   = $replayOutput
  }
  gates       = [pscustomobject]@{
    replay = [pscustomobject]@{
      degradedRatioPass      = $replayDegradedPass
      structurePassRatioPass = $replayStructurePass
      errorClassCoveragePass = $replayErrorClassPass
      allPassed              = $replayAllPassed
    }
    baseline = [pscustomobject]@{
      summaryPass     = $summaryPass
      severityPass    = $severityPass
      suggestionsPass = $suggestionsPass
      allPassed       = $baselineAllPassed
    }
    totalPass = $totalPass
    allPassed = $allPassed
  }
  notes       = @($notes)
  prime       = $primeResult
}

$summary | ConvertTo-Json -Depth 12 | Set-Content -Path $outputPath -Encoding UTF8
Write-Host ("AI baseline validation done: allPassed={0}" -f $allPassed)
Write-Host ("result file: {0}" -f $outputPath)

if (-not [string]::IsNullOrWhiteSpace($ReportFile)) {
  $reportPath = Resolve-OutputPath -Candidate $ReportFile
  Write-BaselineReport -Result $summary -Path $reportPath
  Write-Host ("report file: {0}" -f $reportPath)
}

if (-not $allPassed) {
  exit 3
}
exit 0

