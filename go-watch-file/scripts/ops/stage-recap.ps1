# 本脚本用于阶段回归一键复盘
# 顺序执行 metrics 巡检、控制面回放、知识库复盘，并汇总输出统一 JSON 报告

param(
  [string]$BaseUrl = "http://localhost:8082",
  [Parameter(Mandatory = $true)]
  [string]$Token,
  [int]$AgentCount = 3,
  [int]$TaskCount = 30,
  [string]$SamplesFile = "../docs/04-知识库/知识库命中率样本.json",
  [string]$MttdFile = "../docs/04-知识库/知识库MTTD基线.csv",
  [double]$CitationTarget = 1.0,
  [string]$ReportsDir = "../reports",
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

$checkMetricsScript = Join-Path $opsDir "check-metrics.ps1"
$controlReplayScript = Join-Path $opsDir "control-replay.ps1"
$kbRecapScript = Join-Path $opsDir "kb-recap.ps1"

if (-not (Test-Path $checkMetricsScript)) {
  Write-Error "缺少脚本: check-metrics.ps1"
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
$metricsOutput = Resolve-OutputPath -BaseDir $ReportsDir -FileName "metrics-stage.prom"
$controlOutput = Resolve-OutputPath -BaseDir $ReportsDir -FileName "control-replay-result.json"
$controlMetricsOutput = Resolve-OutputPath -BaseDir $ReportsDir -FileName "metrics-control-replay.prom"
$kbOutput = Resolve-OutputPath -BaseDir $ReportsDir -FileName "kb-recap-result.json"

$stages = @()

$metricsStage = Invoke-Stage -Name "metrics-check" -ScriptPath $checkMetricsScript -Arguments @(
  "-BaseUrl", $base,
  "-OutputFile", $metricsOutput
)
$stages += $metricsStage

if (-not $SkipControlReplay) {
  $controlStage = Invoke-Stage -Name "control-replay" -ScriptPath $controlReplayScript -Arguments @(
    "-BaseUrl", $base,
    "-Token", $Token,
    "-AgentCount", ([string]$AgentCount),
    "-TaskCount", ([string]$TaskCount),
    "-OutputFile", $controlOutput,
    "-MetricsFile", $controlMetricsOutput
  )
  $stages += $controlStage
}

if (-not $SkipKBRecap) {
  $kbStage = Invoke-Stage -Name "kb-recap" -ScriptPath $kbRecapScript -Arguments @(
    "-BaseUrl", $base,
    "-Token", $Token,
    "-SamplesFile", $SamplesFile,
    "-MttdFile", $MttdFile,
    "-CitationTarget", ([string]$CitationTarget),
    "-OutputFile", $kbOutput
  )
  $stages += $kbStage
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
  allPassed   = $allPassed
  stages      = $stages
  artifacts   = [pscustomobject]@{
    metricsSnapshot      = $metricsOutput
    controlReplayResult  = $(if ($SkipControlReplay) { "" } else { $controlOutput })
    controlMetrics       = $(if ($SkipControlReplay) { "" } else { $controlMetricsOutput })
    kbRecapResult        = $(if ($SkipKBRecap) { "" } else { $kbOutput })
  }
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

