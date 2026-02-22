# Basic validation script for phase-0 gates.
# It validates:
# 1) go test ./...
# 2) single-node upload path (manual enqueue)
# 3) notification count delta in dashboard metric cards

param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$WatchDir = "",
  [string]$FileExt = "",
  [int]$WaitTimeoutSec = 90,
  [int]$PollIntervalSec = 2,
  [switch]$SkipGoTest,
  [switch]$RequireNotification,
  [string]$OutputFile = "../reports/basic-e2e-result.json",
  [string]$ReportFile = ""
)

function Ensure-Dir {
  param([string]$Path)
  if (-not [string]::IsNullOrWhiteSpace($Path) -and -not (Test-Path $Path)) {
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
  }
}

function Resolve-OutputPath {
  param([string]$Path)
  if ([System.IO.Path]::IsPathRooted($Path)) {
    return $Path
  }
  return (Join-Path (Get-Location).Path $Path)
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

function Get-NotificationCountFromDashboard {
  param($Dashboard)
  if ($null -eq $Dashboard -or $null -eq $Dashboard.metricCards) {
    return $null
  }
  foreach ($card in $Dashboard.metricCards) {
    $label = [string]$card.label
    # Prefer exact Chinese label, fallback to fuzzy match for compatibility.
    if ($label -eq "通知次数" -or $label -like "*通知*" -or $label -like "*notify*") {
      $num = Parse-DoubleInvariant -Raw ([string]$card.value)
      if ($null -ne $num) {
        return [int]$num
      }
    }
  }
  return $null
}

function Get-FirstWatchDirFromDashboard {
  param($Dashboard)
  if ($null -eq $Dashboard -or $null -eq $Dashboard.heroCopy -or $null -eq $Dashboard.heroCopy.watchDirs) {
    return ""
  }
  foreach ($item in @($Dashboard.heroCopy.watchDirs)) {
    $dir = [string]$item
    if (-not [string]::IsNullOrWhiteSpace($dir)) {
      return $dir.Trim()
    }
  }
  return ""
}

function Get-FirstExtFromDashboard {
  param($Dashboard)
  if ($null -eq $Dashboard -or $null -eq $Dashboard.configSnapshot) {
    return ""
  }
  $raw = [string]$Dashboard.configSnapshot.fileExt
  if ([string]::IsNullOrWhiteSpace($raw)) {
    return ""
  }
  $parts = $raw -split "[,\s;]+" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
  if ($parts.Count -eq 0) {
    return ""
  }
  $first = [string]$parts[0]
  if ($first.StartsWith(".")) {
    return $first
  }
  return "." + $first
}

function New-ResultObject {
  return [pscustomobject]@{
    generatedAt = (Get-Date).ToString("s")
    baseUrl     = ""
    goTest      = [pscustomobject]@{
      skipped  = $false
      ok       = $false
      exitCode = -1
      output   = ""
    }
    upload      = [pscustomobject]@{
      watchDir            = ""
      sampleFile          = ""
      enqueued            = $false
      successDelta        = 0
      failureDelta        = 0
      uploadRecordMatched = $false
      pass                = $false
    }
    notification = [pscustomobject]@{
      required = $false
      before   = $null
      after    = $null
      delta    = $null
      pass     = $false
      note     = ""
    }
    allPassed = $false
    notes     = @()
  }
}

function Write-Report {
  param(
    $Result,
    [string]$Path
  )

  $dir = Split-Path -Parent $Path
  Ensure-Dir $dir

  $summaryText = if ($Result.allPassed) { "PASS" } else { "FAIL" }
  $goTestText = if ($Result.goTest.skipped) { "-" } elseif ($Result.goTest.ok) { "P" } else { "F" }
  $uploadText = if ($Result.upload.pass) { "P" } else { "F" }
  $notifyText = if ($Result.notification.pass) { "P" } else { "F" }

  $lines = @()
  $lines += ("# Basic Validation Report ({0})" -f (Get-Date).ToString("yyyy-MM-dd"))
  $lines += ""
  $lines += ("- GeneratedAt: {0}" -f $Result.generatedAt)
  $lines += ("- BaseUrl: {0}" -f $Result.baseUrl)
  $lines += ("- Summary: {0}" -f $summaryText)
  $lines += ""
  $lines += "## 1. Result"
  $lines += ""
  $lines += "| Item | Result | Details |"
  $lines += "| --- | --- | --- |"
  $lines += ("| go test ./... | {0} | exitCode={1} |" -f $goTestText, $Result.goTest.exitCode)
  $lines += ("| upload path | {0} | successDelta={1}, failureDelta={2}, uploadRecordMatched={3} |" -f $uploadText, $Result.upload.successDelta, $Result.upload.failureDelta, $Result.upload.uploadRecordMatched)
  $lines += ("| notification | {0} | required={1}, before={2}, after={3}, delta={4}, note={5} |" -f $notifyText, $Result.notification.required, $Result.notification.before, $Result.notification.after, $Result.notification.delta, $Result.notification.note)
  $lines += ""
  $lines += "## 2. Sample"
  $lines += ""
  $lines += ("- watchDir: {0}" -f $Result.upload.watchDir)
  $lines += ("- sampleFile: {0}" -f $Result.upload.sampleFile)
  $lines += ""
  $lines += "## 3. Notes"
  foreach ($note in @($Result.notes)) {
    $lines += ("- {0}" -f $note)
  }

  Set-Content -Path $Path -Encoding UTF8 -Value (($lines -join "`r`n") + "`r`n")
}

$result = New-ResultObject
$base = $BaseUrl.TrimEnd("/")
$result.baseUrl = $base
$result.notification.required = [bool]$RequireNotification

$headers = @{}

# 1) go test gate
if ($SkipGoTest) {
  $result.goTest.skipped = $true
  $result.goTest.ok = $true
  $result.goTest.exitCode = 0
  $result.notes += "go test skipped by -SkipGoTest"
} else {
  $testOutput = & go test ./... -count=1 2>&1
  $testExit = $LASTEXITCODE
  $result.goTest.output = ($testOutput | Out-String).Trim()
  $result.goTest.exitCode = $testExit
  $result.goTest.ok = ($testExit -eq 0)
}

# 2) read baseline metrics and dashboard
$metricsEndpoint = "$base/metrics"
$dashboardEndpoint = "$base/api/dashboard?mode=light"
try {
  $metricsBeforeText = (Invoke-WebRequest -Uri $metricsEndpoint -Headers $headers -UseBasicParsing -TimeoutSec 10).Content
  $dashboardBefore = Invoke-RestMethod -Uri $dashboardEndpoint -Headers $headers -Method Get -TimeoutSec 10
} catch {
  Write-Host ("failed to read baseline data: {0}" -f $_.Exception.Message)
  exit 2
}

$uploadSuccessBefore = Read-MetricValue -MetricsText $metricsBeforeText -MetricName "gwf_upload_success_total"
$uploadFailureBefore = Read-MetricValue -MetricsText $metricsBeforeText -MetricName "gwf_upload_failure_total"
$notifyBefore = Get-NotificationCountFromDashboard -Dashboard $dashboardBefore
$result.notification.before = $notifyBefore

$effectiveWatchDir = $WatchDir
if ([string]::IsNullOrWhiteSpace($effectiveWatchDir)) {
  $effectiveWatchDir = Get-FirstWatchDirFromDashboard -Dashboard $dashboardBefore
}
if ([string]::IsNullOrWhiteSpace($effectiveWatchDir)) {
  $result.notes += "watchDir missing in parameter and dashboard"
  Write-Host "watchDir is not configured"
  exit 2
}
if (-not (Test-Path $effectiveWatchDir)) {
  $result.notes += ("watchDir not found: {0}" -f $effectiveWatchDir)
  Write-Host ("watchDir not found: {0}" -f $effectiveWatchDir)
  exit 2
}
$result.upload.watchDir = $effectiveWatchDir

$effectiveExt = $FileExt
if ([string]::IsNullOrWhiteSpace($effectiveExt)) {
  $effectiveExt = Get-FirstExtFromDashboard -Dashboard $dashboardBefore
}
if ([string]::IsNullOrWhiteSpace($effectiveExt)) {
  $effectiveExt = ".log"
}
if (-not $effectiveExt.StartsWith(".")) {
  $effectiveExt = "." + $effectiveExt
}

# 3) create sample file and enqueue manual upload
$smokeDir = Join-Path $effectiveWatchDir "ops-smoke"
Ensure-Dir $smokeDir
$fileName = "basic-e2e-{0}{1}" -f (Get-Date -Format "yyyyMMdd-HHmmss"), $effectiveExt
$sampleFile = Join-Path $smokeDir $fileName
$content = @(
  ("timestamp={0}" -f (Get-Date).ToString("s"))
  "scenario=basic-e2e-upload-notify"
  "message=smoke validation"
) -join "`r`n"
Set-Content -Path $sampleFile -Encoding UTF8 -Value $content
$result.upload.sampleFile = $sampleFile

$body = @{ path = $sampleFile } | ConvertTo-Json
try {
  $manualHeaders = @{ "Content-Type" = "application/json" }
  foreach ($key in $headers.Keys) {
    $manualHeaders[$key] = $headers[$key]
  }
  $manualResp = Invoke-RestMethod -Uri "$base/api/manual-upload" -Method Post -Headers $manualHeaders -Body $body -TimeoutSec 15
  $result.upload.enqueued = [bool]$manualResp.ok
} catch {
  $result.notes += ("manual-upload failed: {0}" -f $_.Exception.Message)
  $result.upload.enqueued = $false
}

if (-not $result.upload.enqueued) {
  $result.upload.pass = $false
  $result.notification.pass = (-not [bool]$RequireNotification)
  $result.allPassed = ($result.goTest.ok -and $result.upload.pass -and $result.notification.pass)

  $outPath = Resolve-OutputPath -Path $OutputFile
  Ensure-Dir (Split-Path -Parent $outPath)
  $result | ConvertTo-Json -Depth 8 | Set-Content -Path $outPath -Encoding UTF8
  if (-not [string]::IsNullOrWhiteSpace($ReportFile)) {
    Write-Report -Result $result -Path (Resolve-OutputPath -Path $ReportFile)
  }
  Write-Host "manual-upload not enqueued"
  exit 3
}

# 4) poll for upload and notification deltas
$deadline = (Get-Date).AddSeconds($WaitTimeoutSec)
$uploadSuccessAfter = $uploadSuccessBefore
$uploadFailureAfter = $uploadFailureBefore
$dashboardAfter = $dashboardBefore

while ((Get-Date) -lt $deadline) {
  Start-Sleep -Seconds $PollIntervalSec
  try {
    $metricsCurrent = (Invoke-WebRequest -Uri $metricsEndpoint -Headers $headers -UseBasicParsing -TimeoutSec 10).Content
    $dashboardAfter = Invoke-RestMethod -Uri $dashboardEndpoint -Headers $headers -Method Get -TimeoutSec 10
  } catch {
    continue
  }

  $uploadSuccessAfter = Read-MetricValue -MetricsText $metricsCurrent -MetricName "gwf_upload_success_total"
  $uploadFailureAfter = Read-MetricValue -MetricsText $metricsCurrent -MetricName "gwf_upload_failure_total"

  $successDelta = 0
  $failureDelta = 0
  if ($null -ne $uploadSuccessBefore -and $null -ne $uploadSuccessAfter) {
    $successDelta = [int]([double]$uploadSuccessAfter - [double]$uploadSuccessBefore)
  }
  if ($null -ne $uploadFailureBefore -and $null -ne $uploadFailureAfter) {
    $failureDelta = [int]([double]$uploadFailureAfter - [double]$uploadFailureBefore)
  }
  if ($successDelta -gt 0 -or $failureDelta -gt 0) {
    break
  }
}

$successFinal = 0
$failureFinal = 0
if ($null -ne $uploadSuccessBefore -and $null -ne $uploadSuccessAfter) {
  $successFinal = [int]([double]$uploadSuccessAfter - [double]$uploadSuccessBefore)
}
if ($null -ne $uploadFailureBefore -and $null -ne $uploadFailureAfter) {
  $failureFinal = [int]([double]$uploadFailureAfter - [double]$uploadFailureBefore)
}
$result.upload.successDelta = $successFinal
$result.upload.failureDelta = $failureFinal

$recordMatched = $false
if ($null -ne $dashboardAfter -and $null -ne $dashboardAfter.uploadRecords) {
  foreach ($rec in @($dashboardAfter.uploadRecords)) {
    $name = [string]$rec.file
    if ($name -eq $fileName) {
      $recordMatched = $true
      break
    }
  }
}
$result.upload.uploadRecordMatched = $recordMatched
$result.upload.pass = ($successFinal -gt 0 -and $failureFinal -eq 0 -and $recordMatched)

$notifyAfter = Get-NotificationCountFromDashboard -Dashboard $dashboardAfter
$result.notification.after = $notifyAfter
if ($null -ne $notifyBefore -and $null -ne $notifyAfter) {
  $result.notification.delta = ([int]$notifyAfter - [int]$notifyBefore)
}

if ([bool]$RequireNotification) {
  $result.notification.pass = ($null -ne $result.notification.delta -and [int]$result.notification.delta -gt 0)
  if (-not $result.notification.pass) {
    $result.notification.note = "notification delta required but not observed"
  }
} else {
  if ($null -ne $result.notification.delta -and [int]$result.notification.delta -gt 0) {
    $result.notification.pass = $true
    $result.notification.note = "notification delta observed"
  } else {
    $result.notification.pass = $true
    $result.notification.note = "notification delta not observed (non-blocking without -RequireNotification)"
  }
}

if (-not $result.upload.pass) {
  $result.notes += "upload gate not met: successDelta>0 && failureDelta=0 && uploadRecordMatched=true"
}

$result.allPassed = ($result.goTest.ok -and $result.upload.pass -and $result.notification.pass)

$outPath = Resolve-OutputPath -Path $OutputFile
Ensure-Dir (Split-Path -Parent $outPath)
$result | ConvertTo-Json -Depth 8 | Set-Content -Path $outPath -Encoding UTF8
Write-Host ("basic validation done: allPassed={0}" -f $result.allPassed)
Write-Host ("result file: {0}" -f $outPath)

if (-not [string]::IsNullOrWhiteSpace($ReportFile)) {
  $reportPath = Resolve-OutputPath -Path $ReportFile
  Write-Report -Result $result -Path $reportPath
  Write-Host ("report file: {0}" -f $reportPath)
}

if (-not $result.allPassed) {
  exit 3
}
exit 0
