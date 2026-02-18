# 本脚本用于阶段回归一键复盘
# 顺序执行 metrics 巡检、控制面回放、知识库复盘，并汇总输出统一 JSON 报告

param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$Token = "",
  [int]$AgentCount = 3,
  [int]$TaskCount = 30,
  [string]$AIPathsFile = "../docs/03-告警与AI/AI回放路径清单.txt",
  [int]$AILimit = 200,
  [double]$AIDegradedRatioTarget = 0.2,
  [string]$SamplesFile = "../docs/04-知识库/知识库命中率样本.json",
  [string]$MttdFile = "../docs/04-知识库/知识库MTTD基线.csv",
  [double]$CitationTarget = 1.0,
  [string]$ReportsDir = "../reports",
  [switch]$AutoPrime,
  [string]$PrimeDocsPath = "../docs",
  [string]$PrimeOperator = "stage-recap",
  [bool]$PrimeApproveImported = $true,
  [string]$PrimeAISamplesDir = "../reports/ai-replay-samples",
  [bool]$PrimeUpdateAlertLogPaths = $true,
  [string]$PrimeOutputFile = "../reports/stage-prime-result.json",
  [switch]$SkipAIReplay,
  [switch]$SkipControlReplay,
  [switch]$SkipKBRecap,
  [string]$OutputFile = "../reports/stage-recap-result.json"
)

function Ensure-Dir {
  param([string]$Path)
  if (-not [string]::IsNullOrWhiteSpace($Path) -and -not (Test-Path $Path)) {
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
  }
}

function Resolve-OutputPath {
  param(
    [string]$BaseDir,
    [string]$FileName
  )
  return Join-Path $BaseDir $FileName
}

function Quote-Arg {
  param([string]$Value)
  if ($null -eq $Value) {
    return '""'
  }
  if ($Value -match '[\s"]') {
    return '"' + ($Value -replace '"', '\"') + '"'
  }
  return $Value
}

function Invoke-Stage {
  param(
    [string]$Name,
    [string]$ScriptPath,
    [string[]]$Arguments
  )

  $start = Get-Date
  Write-Host ("开始阶段: {0}" -f $Name)
  if (-not (Test-Path $ScriptPath)) {
    Write-Warning ("阶段脚本不存在: {0}" -f $ScriptPath)
    return [pscustomobject]@{
      name      = $Name
      ok        = $false
      exitCode  = 2
      elapsedMs = 0
      error     = "script not found"
    }
  }

  $argsList = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $ScriptPath) + $Arguments
  $argLine = [string]::Join(" ", ($argsList | ForEach-Object { Quote-Arg $_ }))
  $stdoutPath = [System.IO.Path]::GetTempFileName()
  $stderrPath = [System.IO.Path]::GetTempFileName()
  $exitCode = 2
  $outputText = ""
  try {
    $proc = Start-Process -FilePath "powershell" -ArgumentList $argLine -NoNewWindow -Wait -PassThru -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath
    if ($null -ne $proc) {
      $exitCode = $proc.ExitCode
    }
    $stdoutText = ""
    $stderrText = ""
    if (Test-Path $stdoutPath) {
      $stdoutRaw = Get-Content -Raw -Encoding UTF8 -ErrorAction SilentlyContinue $stdoutPath
      if ($null -ne $stdoutRaw) {
        $stdoutText = ([string]$stdoutRaw).Trim()
      }
    }
    if (Test-Path $stderrPath) {
      $stderrRaw = Get-Content -Raw -Encoding UTF8 -ErrorAction SilentlyContinue $stderrPath
      if ($null -ne $stderrRaw) {
        $stderrText = ([string]$stderrRaw).Trim()
      }
    }
    $parts = @()
    if (-not [string]::IsNullOrWhiteSpace($stdoutText)) { $parts += $stdoutText }
    if (-not [string]::IsNullOrWhiteSpace($stderrText)) { $parts += $stderrText }
    $outputText = ($parts -join [Environment]::NewLine).Trim()
  } finally {
    Remove-Item -ErrorAction SilentlyContinue $stdoutPath, $stderrPath
  }
  if (-not [string]::IsNullOrWhiteSpace($outputText)) {
    Write-Host $outputText
  }

  $elapsedMs = [int64]((Get-Date) - $start).TotalMilliseconds
  $ok = ($exitCode -eq 0)
  Write-Host ("阶段结束: {0} ok={1} exitCode={2} elapsedMs={3}" -f $Name, $ok, $exitCode, $elapsedMs)
  return [pscustomobject]@{
    name      = $Name
    ok        = $ok
    exitCode  = $exitCode
    elapsedMs = $elapsedMs
    error     = ""
    output    = $outputText
  }
}

$opsDir = $PSScriptRoot
$base = $BaseUrl.TrimEnd("/")
$tokenArgs = @()
if (-not [string]::IsNullOrWhiteSpace($Token)) {
  $tokenArgs = @("-Token", $Token.Trim())
}

$stagePrimeScript = Join-Path $opsDir "stage-prime.ps1"
$checkMetricsScript = Join-Path $opsDir "check-metrics.ps1"
$aiReplayScript = Join-Path $opsDir "ai-replay.ps1"
$controlReplayScript = Join-Path $opsDir "control-replay.ps1"
$kbRecapScript = Join-Path $opsDir "kb-recap.ps1"

if (-not (Test-Path $stagePrimeScript) -and $AutoPrime) {
  Write-Error "缺少脚本: stage-prime.ps1"
  exit 2
}
if (-not (Test-Path $checkMetricsScript)) {
  Write-Error "缺少脚本: check-metrics.ps1"
  exit 2
}
if (-not (Test-Path $aiReplayScript) -and -not $SkipAIReplay) {
  Write-Error "缺少脚本: ai-replay.ps1"
  exit 2
}
if (-not (Test-Path $controlReplayScript) -and -not $SkipControlReplay) {
  Write-Error "缺少脚本: control-replay.ps1"
  exit 2
}
if (-not (Test-Path $kbRecapScript) -and -not $SkipKBRecap) {
  Write-Error "缺少脚本: kb-recap.ps1"
  exit 2
}

Ensure-Dir $ReportsDir
$primeOutput = $PrimeOutputFile
$metricsOutput = Resolve-OutputPath -BaseDir $ReportsDir -FileName "metrics-stage.prom"
$aiOutput = Resolve-OutputPath -BaseDir $ReportsDir -FileName "ai-replay-result.json"
$controlOutput = Resolve-OutputPath -BaseDir $ReportsDir -FileName "control-replay-result.json"
$controlMetricsOutput = Resolve-OutputPath -BaseDir $ReportsDir -FileName "metrics-control-replay.prom"
$kbOutput = Resolve-OutputPath -BaseDir $ReportsDir -FileName "kb-recap-result.json"

$stages = @()

if ($AutoPrime) {
  $primeArgs = @(
    "-BaseUrl", $base,
    "-DocsPath", $PrimeDocsPath,
    "-Operator", $PrimeOperator,
    "-ApproveImported", ([string]$PrimeApproveImported),
    "-AIPathsFile", $AIPathsFile,
    "-AISamplesDir", $PrimeAISamplesDir,
    "-UpdateAlertLogPaths", ([string]$PrimeUpdateAlertLogPaths),
    "-OutputFile", $primeOutput
  )
  if ($tokenArgs.Count -gt 0) {
    $primeArgs += $tokenArgs
  }
  $primeStage = Invoke-Stage -Name "stage-prime" -ScriptPath $stagePrimeScript -Arguments $primeArgs
  $stages += $primeStage
}

$metricsStage = Invoke-Stage -Name "metrics-check" -ScriptPath $checkMetricsScript -Arguments @(
  "-BaseUrl", $base,
  "-OutputFile", $metricsOutput
)
$stages += $metricsStage

if (-not $SkipAIReplay) {
  $aiArgs = @(
    "-BaseUrl", $base,
    "-PathsFile", $AIPathsFile,
    "-Limit", ([string]$AILimit),
    "-OutputFile", $aiOutput
  )
  if ($tokenArgs.Count -gt 0) {
    $aiArgs += $tokenArgs
  }
  $aiStage = Invoke-Stage -Name "ai-replay" -ScriptPath $aiReplayScript -Arguments $aiArgs
  $stages += $aiStage
}

if (-not $SkipControlReplay) {
  $controlArgs = @(
    "-BaseUrl", $base,
    "-AgentCount", ([string]$AgentCount),
    "-TaskCount", ([string]$TaskCount),
    "-OutputFile", $controlOutput,
    "-MetricsFile", $controlMetricsOutput
  )
  if ($tokenArgs.Count -gt 0) {
    $controlArgs += $tokenArgs
  }
  $controlStage = Invoke-Stage -Name "control-replay" -ScriptPath $controlReplayScript -Arguments $controlArgs
  $stages += $controlStage
}

if (-not $SkipKBRecap) {
  $kbArgs = @(
    "-BaseUrl", $base,
    "-SamplesFile", $SamplesFile,
    "-MttdFile", $MttdFile,
    "-CitationTarget", ([string]$CitationTarget),
    "-OutputFile", $kbOutput
  )
  if ($tokenArgs.Count -gt 0) {
    $kbArgs += $tokenArgs
  }
  $kbStage = Invoke-Stage -Name "kb-recap" -ScriptPath $kbRecapScript -Arguments $kbArgs
  $stages += $kbStage
}

$primeResult = $null
if ($AutoPrime -and (Test-Path $primeOutput)) {
  try {
    $primeResult = Get-Content -Raw -Encoding UTF8 $primeOutput | ConvertFrom-Json
  } catch {
    Write-Warning ("解析阶段预备结果失败: {0}" -f $_.Exception.Message)
  }
}

$aiResult = $null
if (Test-Path $aiOutput) {
  try {
    $aiResult = Get-Content -Raw -Encoding UTF8 $aiOutput | ConvertFrom-Json
  } catch {
    Write-Warning ("解析 AI 回放结果失败: {0}" -f $_.Exception.Message)
  }
}

if ($null -ne $aiResult -and $null -ne $aiResult.degradedRatio) {
  $actualRatio = [double]$aiResult.degradedRatio
  if ($actualRatio -gt $AIDegradedRatioTarget) {
    $actualPct = [Math]::Round($actualRatio * 100, 2)
    $targetPct = [Math]::Round($AIDegradedRatioTarget * 100, 2)
    $gateMessage = "degraded ratio ${actualPct}% exceeds target ${targetPct}%"
    foreach ($stage in $stages) {
      if ([string]$stage.name -eq "ai-replay") {
        $stage.ok = $false
        $stage.exitCode = 3
        $stage.error = $gateMessage
        if ([string]::IsNullOrWhiteSpace([string]$stage.output)) {
          $stage.output = $gateMessage
        } else {
          $stage.output = ([string]$stage.output + [Environment]::NewLine + $gateMessage).Trim()
        }
        break
      }
    }
  }
}

$controlResult = $null
if (Test-Path $controlOutput) {
  try {
    $controlResult = Get-Content -Raw -Encoding UTF8 $controlOutput | ConvertFrom-Json
  } catch {
    Write-Warning ("解析控制面回放结果失败: {0}" -f $_.Exception.Message)
  }
}

$kbResult = $null
if (Test-Path $kbOutput) {
  try {
    $kbResult = Get-Content -Raw -Encoding UTF8 $kbOutput | ConvertFrom-Json
  } catch {
    Write-Warning ("解析知识库复盘结果失败: {0}" -f $_.Exception.Message)
  }
}

$allPassed = $true
foreach ($stage in $stages) {
  if (-not $stage.ok) {
    $allPassed = $false
    break
  }
}

$report = [pscustomobject]@{
  generatedAt = (Get-Date).ToString("s")
  baseUrl     = $base
  gateTargets = [pscustomobject]@{
    aiDegradedRatio = $AIDegradedRatioTarget
  }
  allPassed   = $allPassed
  stages      = $stages
  artifacts   = [pscustomobject]@{
    primeResult          = $(if ($AutoPrime) { $primeOutput } else { "" })
    metricsSnapshot      = $metricsOutput
    aiReplayResult       = $(if ($SkipAIReplay) { "" } else { $aiOutput })
    controlReplayResult  = $(if ($SkipControlReplay) { "" } else { $controlOutput })
    controlMetrics       = $(if ($SkipControlReplay) { "" } else { $controlMetricsOutput })
    kbRecapResult        = $(if ($SkipKBRecap) { "" } else { $kbOutput })
  }
  prime         = $primeResult
  aiReplay      = $aiResult
  controlReplay = $controlResult
  kbRecap       = $kbResult
}

$outputDir = Split-Path -Parent $OutputFile
Ensure-Dir $outputDir
$report | ConvertTo-Json -Depth 12 | Set-Content -Path $OutputFile -Encoding UTF8

Write-Host ("阶段复盘完成 allPassed={0}" -f $allPassed)
Write-Host ("汇总文件: {0}" -f $OutputFile)

if (-not $allPassed) {
  exit 3
}
exit 0

