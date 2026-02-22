# Upload reliability recap runner.
# Runs queue saturation stress, optional fault window, and emits JSON + Markdown report.

param(
  [string]$BaseUrl = "http://localhost:8082",
  [Parameter(Mandatory = $true)]
  [string]$WatchDir,
  [int]$SaturationCount = 1200,
  [int]$SaturationIntervalMs = 5,
  [int]$FaultCount = 400,
  [int]$FaultIntervalMs = 5,
  [int]$RecoveryCount = 300,
  [int]$RecoveryIntervalMs = 10,
  [int]$MinBytes = 1024,
  [int]$MaxBytes = 8192,
  [int]$StabilizeSeconds = 8,
  [ValidateSet("none", "manual", "command")]
  [string]$FaultMode = "none",
  [string]$FaultStartCommand = "",
  [string]$FaultRecoverCommand = "",
  [int]$FaultObserveSeconds = 60,
  [int]$RecoveryObserveSeconds = 30,
  [switch]$RequireQueueSaturation = $false,
  [switch]$FailOnGate = $false,
  [string]$OutputFile = "../reports/upload-recap-result.json",
  [string]$ReportFile = ""
)

function Ensure-Dir {
  param([string]$Path)
  if (-not [string]::IsNullOrWhiteSpace($Path) -and -not (Test-Path $Path)) {
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
  }
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
  if ([string]::IsNullOrWhiteSpace($MetricsText)) {
    return 0.0
  }
  $escaped = [regex]::Escape($MetricName)
  $pattern = "(?m)^$escaped(?:\{[^\n]*\})?\s+([0-9.eE+\-]+)$"
  $match = [regex]::Match($MetricsText, $pattern)
  if (-not $match.Success) {
    return 0.0
  }
  $value = Parse-DoubleInvariant -Raw $match.Groups[1].Value
  if ($null -eq $value) {
    return 0.0
  }
  return [double]$value
}

function Read-MetricMapByLabel {
  param(
    [string]$MetricsText,
    [string]$MetricName,
    [string]$LabelName
  )
  $result = @{}
  if ([string]::IsNullOrWhiteSpace($MetricsText)) {
    return $result
  }
  $metricEscaped = [regex]::Escape($MetricName)
  $labelEscaped = [regex]::Escape($LabelName)
  $linePattern = "(?m)^$metricEscaped\{([^\n]*)\}\s+([0-9.eE+\-]+)$"
  $matches = [regex]::Matches($MetricsText, $linePattern)
  foreach ($m in $matches) {
    $labelsText = [string]$m.Groups[1].Value
    $valueRaw = [string]$m.Groups[2].Value
    $labelPattern = ('{0}="([^"]+)"' -f $labelEscaped)
    $labelMatch = [regex]::Match($labelsText, $labelPattern)
    if (-not $labelMatch.Success) {
      continue
    }
    $labelValue = [string]$labelMatch.Groups[1].Value
    $parsed = Parse-DoubleInvariant -Raw $valueRaw
    if ($null -eq $parsed) {
      continue
    }
    if ($result.ContainsKey($labelValue)) {
      $result[$labelValue] += [double]$parsed
    } else {
      $result[$labelValue] = [double]$parsed
    }
  }
  return $result
}

function Map-ToPSObject {
  param([hashtable]$InputMap)
  if ($null -eq $InputMap) {
    return [pscustomobject]@{}
  }
  $obj = [ordered]@{}
  foreach ($key in ($InputMap.Keys | Sort-Object)) {
    $obj[$key] = $InputMap[$key]
  }
  return [pscustomobject]$obj
}

function Get-DeltaMap {
  param(
    [hashtable]$Before,
    [hashtable]$After
  )
  $delta = @{}
  $keys = @()
  if ($null -ne $Before) { $keys += $Before.Keys }
  if ($null -ne $After) { $keys += $After.Keys }
  foreach ($key in ($keys | Sort-Object -Unique)) {
    $beforeValue = 0.0
    if ($null -ne $Before -and $Before.ContainsKey($key)) {
      $beforeValue = [double]$Before[$key]
    }
    $afterValue = 0.0
    if ($null -ne $After -and $After.ContainsKey($key)) {
      $afterValue = [double]$After[$key]
    }
    $delta[$key] = ($afterValue - $beforeValue)
  }
  return $delta
}

function Invoke-UploadStressPhase {
  param(
    [string]$StressScriptPath,
    [string]$PhaseName,
    [int]$Count,
    [int]$IntervalMs,
    [string]$Prefix,
    [string]$WatchDir,
    [int]$MinBytes,
    [int]$MaxBytes
  )
  if ($Count -le 0) {
    Write-Host ("Skip phase {0} (count<=0)" -f $PhaseName)
    return [pscustomobject]@{
      name      = $PhaseName
      ok        = $true
      exitCode  = 0
      elapsedMs = 0
      output    = "skip (count<=0)"
    }
  }

  Write-Host ("Run phase {0}: count={1}, intervalMs={2}" -f $PhaseName, $Count, $IntervalMs)
  $start = Get-Date
  $args = @(
    "-ExecutionPolicy", "Bypass",
    "-File", $StressScriptPath,
    "-WatchDir", $WatchDir,
    "-Count", ([string]$Count),
    "-IntervalMs", ([string]$IntervalMs),
    "-MinBytes", ([string]$MinBytes),
    "-MaxBytes", ([string]$MaxBytes),
    "-Prefix", $Prefix
  )
  $output = & powershell @args 2>&1
  $exitCode = $LASTEXITCODE
  $elapsedMs = [int64]((Get-Date) - $start).TotalMilliseconds
  $text = ($output | Out-String).Trim()
  if (-not [string]::IsNullOrWhiteSpace($text)) {
    Write-Host $text
  }

  return [pscustomobject]@{
    name      = $PhaseName
    ok        = ($exitCode -eq 0)
    exitCode  = $exitCode
    elapsedMs = $elapsedMs
    output    = $text
  }
}

function Invoke-ShellCommandPhase {
  param(
    [string]$Name,
    [string]$Command
  )
  if ([string]::IsNullOrWhiteSpace($Command)) {
    return [pscustomobject]@{
      name      = $Name
      ok        = $true
      exitCode  = 0
      elapsedMs = 0
      output    = "skip (empty command)"
    }
  }
  Write-Host ("Run command phase: {0}" -f $Name)
  $start = Get-Date
  $output = & powershell -NoProfile -Command $Command 2>&1
  $exitCode = $LASTEXITCODE
  $elapsedMs = [int64]((Get-Date) - $start).TotalMilliseconds
  $text = ($output | Out-String).Trim()
  if (-not [string]::IsNullOrWhiteSpace($text)) {
    Write-Host $text
  }
  return [pscustomobject]@{
    name      = $Name
    ok        = ($exitCode -eq 0)
    exitCode  = $exitCode
    elapsedMs = $elapsedMs
    output    = $text
  }
}

function Collect-Snapshot {
  param(
    [string]$Name,
    [string]$Base,
    [hashtable]$Headers,
    [string]$ReportsDir
  )
  $metricsEndpoint = "{0}/metrics" -f $Base
  $healthEndpoint = "{0}/api/health" -f $Base

  $metricsResp = Invoke-WebRequest -Uri $metricsEndpoint -Method Get -TimeoutSec 15
  $metricsText = [string]$metricsResp.Content
  if ([string]::IsNullOrWhiteSpace($metricsText)) {
    throw ("metrics empty for snapshot {0}" -f $Name)
  }

  $healthResp = Invoke-RestMethod -Uri $healthEndpoint -Method Get -Headers $Headers -TimeoutSec 15
  if ($null -eq $healthResp) {
    throw ("health empty for snapshot {0}" -f $Name)
  }

  $metricsFile = Join-Path $ReportsDir ("metrics-upload-{0}.prom" -f $Name)
  $healthFile = Join-Path $ReportsDir ("health-upload-{0}.json" -f $Name)
  Set-Content -Path $metricsFile -Encoding utf8 -Value $metricsText
  $healthResp | ConvertTo-Json -Depth 8 | Set-Content -Path $healthFile -Encoding utf8

  $failureReasons = Read-MetricMapByLabel -MetricsText $metricsText -MetricName "gwf_upload_failure_reason_total" -LabelName "reason"

  return [pscustomobject]@{
    name        = $Name
    collectedAt = (Get-Date).ToString("s")
    metricsFile = $metricsFile
    healthFile  = $healthFile
    metrics     = [pscustomobject]@{
      fileEventsTotal    = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_file_events_total"
      queueLength        = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_queue_length"
      inFlight           = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_inflight"
      workers            = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_workers"
      queueFullTotal     = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_queue_full_total"
      queueShedTotal     = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_queue_shed_total"
      retryTotal         = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_retry_total"
      uploadSuccessTotal = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_success_total"
      uploadFailureTotal = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_upload_failure_total"
      failureReasons     = Map-ToPSObject -InputMap $failureReasons
    }
    health      = $healthResp
  }
}

function Build-Delta {
  param(
    $BeforeSnapshot,
    $AfterSnapshot
  )
  if ($null -eq $BeforeSnapshot -or $null -eq $AfterSnapshot) {
    return $null
  }

  $before = $BeforeSnapshot.metrics
  $after = $AfterSnapshot.metrics

  $successDelta = [double]$after.uploadSuccessTotal - [double]$before.uploadSuccessTotal
  $failureDelta = [double]$after.uploadFailureTotal - [double]$before.uploadFailureTotal
  $trafficDelta = $successDelta + $failureDelta
  $failureRatioPct = $null
  if ($trafficDelta -gt 0) {
    $failureRatioPct = ($failureDelta / $trafficDelta) * 100.0
  }

  $beforeReasons = @{}
  if ($null -ne $before.failureReasons) {
    foreach ($p in $before.failureReasons.PSObject.Properties) {
      $beforeReasons[$p.Name] = [double]$p.Value
    }
  }
  $afterReasons = @{}
  if ($null -ne $after.failureReasons) {
    foreach ($p in $after.failureReasons.PSObject.Properties) {
      $afterReasons[$p.Name] = [double]$p.Value
    }
  }
  $failureReasonDelta = Get-DeltaMap -Before $beforeReasons -After $afterReasons

  return [pscustomobject]@{
    fileEventsDelta    = [double]$after.fileEventsTotal - [double]$before.fileEventsTotal
    queueFullDelta     = [double]$after.queueFullTotal - [double]$before.queueFullTotal
    queueShedDelta     = [double]$after.queueShedTotal - [double]$before.queueShedTotal
    retryDelta         = [double]$after.retryTotal - [double]$before.retryTotal
    uploadSuccessDelta = $successDelta
    uploadFailureDelta = $failureDelta
    trafficDelta       = $trafficDelta
    failureRatioPct    = $failureRatioPct
    queueLengthEnd     = [double]$after.queueLength
    inFlightEnd        = [double]$after.inFlight
    failureReasonDelta = Map-ToPSObject -InputMap $failureReasonDelta
  }
}

function Format-Number {
  param(
    $Value,
    [int]$Digits = 2
  )
  if ($null -eq $Value) {
    return "N/A"
  }
  return [string]::Format([System.Globalization.CultureInfo]::InvariantCulture, "{0:F$Digits}", [double]$Value)
}

if ($SaturationCount -le 0) {
  Write-Error "SaturationCount must be > 0."
  exit 2
}
if ($MinBytes -le 0 -or $MaxBytes -lt $MinBytes) {
  Write-Error "MinBytes/MaxBytes is invalid."
  exit 2
}
if ($FaultMode -eq "command" -and [string]::IsNullOrWhiteSpace($FaultStartCommand)) {
  Write-Error "FaultStartCommand is required when FaultMode=command."
  exit 2
}

$base = $BaseUrl.TrimEnd("/")
$headers = @{}

$scriptDir = (Resolve-Path $PSScriptRoot).Path
$stressScript = Join-Path $scriptDir "upload-stress.ps1"
if (-not (Test-Path $stressScript)) {
  Write-Error ("Missing script: {0}" -f $stressScript)
  exit 2
}

$outputDir = Split-Path -Parent $OutputFile
if ([string]::IsNullOrWhiteSpace($outputDir)) {
  $outputDir = "."
}
Ensure-Dir $outputDir

if ([string]::IsNullOrWhiteSpace($ReportFile)) {
  $dateText = (Get-Date).ToString("yyyy-MM-dd")
  $ReportFile = Join-Path $outputDir ("upload-recap-{0}.md" -f $dateText)
}
$reportDir = Split-Path -Parent $ReportFile
Ensure-Dir $reportDir

$phaseResults = @()
$snapshots = [ordered]@{}

try {
  Write-Host "Collect baseline snapshot..."
  $snapshots["baseline"] = Collect-Snapshot -Name "before" -Base $base -Headers $headers -ReportsDir $outputDir

  $phaseResults += Invoke-UploadStressPhase -StressScriptPath $stressScript -PhaseName "queue-saturation" -Count $SaturationCount -IntervalMs $SaturationIntervalMs -Prefix "queue_saturation" -WatchDir $WatchDir -MinBytes $MinBytes -MaxBytes $MaxBytes
  Start-Sleep -Seconds $StabilizeSeconds
  $snapshots["afterSaturation"] = Collect-Snapshot -Name "after-saturation" -Base $base -Headers $headers -ReportsDir $outputDir

  if ($FaultMode -eq "manual") {
    Write-Host ("Manual fault window starts. Inject OSS/network fault within {0}s." -f $FaultObserveSeconds)
    $phaseResults += [pscustomobject]@{
      name      = "fault-manual-window"
      ok        = $true
      exitCode  = 0
      elapsedMs = 0
      output    = ("manual window {0}s" -f $FaultObserveSeconds)
    }
    $phaseResults += Invoke-UploadStressPhase -StressScriptPath $stressScript -PhaseName "fault-traffic" -Count $FaultCount -IntervalMs $FaultIntervalMs -Prefix "fault_window" -WatchDir $WatchDir -MinBytes $MinBytes -MaxBytes $MaxBytes
    Start-Sleep -Seconds $FaultObserveSeconds
    $snapshots["afterFault"] = Collect-Snapshot -Name "after-fault" -Base $base -Headers $headers -ReportsDir $outputDir
    Write-Host ("Run recovery action and wait {0}s..." -f $RecoveryObserveSeconds)
    Start-Sleep -Seconds $RecoveryObserveSeconds
    $phaseResults += Invoke-UploadStressPhase -StressScriptPath $stressScript -PhaseName "recovery-traffic" -Count $RecoveryCount -IntervalMs $RecoveryIntervalMs -Prefix "recovery_window" -WatchDir $WatchDir -MinBytes $MinBytes -MaxBytes $MaxBytes
    Start-Sleep -Seconds $StabilizeSeconds
    $snapshots["afterRecovery"] = Collect-Snapshot -Name "after-recovery" -Base $base -Headers $headers -ReportsDir $outputDir
  } elseif ($FaultMode -eq "command") {
    $faultStartPhase = Invoke-ShellCommandPhase -Name "fault-start-command" -Command $FaultStartCommand
    $phaseResults += $faultStartPhase
    if (-not $faultStartPhase.ok) {
      throw "fault start command failed"
    }
    $phaseResults += Invoke-UploadStressPhase -StressScriptPath $stressScript -PhaseName "fault-traffic" -Count $FaultCount -IntervalMs $FaultIntervalMs -Prefix "fault_window" -WatchDir $WatchDir -MinBytes $MinBytes -MaxBytes $MaxBytes
    Start-Sleep -Seconds $FaultObserveSeconds
    $snapshots["afterFault"] = Collect-Snapshot -Name "after-fault" -Base $base -Headers $headers -ReportsDir $outputDir

    if (-not [string]::IsNullOrWhiteSpace($FaultRecoverCommand)) {
      $faultRecoverPhase = Invoke-ShellCommandPhase -Name "fault-recover-command" -Command $FaultRecoverCommand
      $phaseResults += $faultRecoverPhase
      if (-not $faultRecoverPhase.ok) {
        throw "fault recover command failed"
      }
    } else {
      $phaseResults += [pscustomobject]@{
        name      = "fault-recover-command"
        ok        = $true
        exitCode  = 0
        elapsedMs = 0
        output    = "skip (empty recover command)"
      }
    }

    Start-Sleep -Seconds $RecoveryObserveSeconds
    $phaseResults += Invoke-UploadStressPhase -StressScriptPath $stressScript -PhaseName "recovery-traffic" -Count $RecoveryCount -IntervalMs $RecoveryIntervalMs -Prefix "recovery_window" -WatchDir $WatchDir -MinBytes $MinBytes -MaxBytes $MaxBytes
    Start-Sleep -Seconds $StabilizeSeconds
    $snapshots["afterRecovery"] = Collect-Snapshot -Name "after-recovery" -Base $base -Headers $headers -ReportsDir $outputDir
  }
} catch {
  Write-Error ("Execution failed: {0}" -f $_.Exception.Message)
  exit 2
}

$baseline = $snapshots["baseline"]
$afterSaturation = $snapshots["afterSaturation"]
$afterFault = $null
if ($snapshots.Contains("afterFault")) {
  $afterFault = $snapshots["afterFault"]
}
$afterRecovery = $null
if ($snapshots.Contains("afterRecovery")) {
  $afterRecovery = $snapshots["afterRecovery"]
}

$deltaSaturation = Build-Delta -BeforeSnapshot $baseline -AfterSnapshot $afterSaturation
$deltaFault = Build-Delta -BeforeSnapshot $afterSaturation -AfterSnapshot $afterFault
$deltaRecovery = Build-Delta -BeforeSnapshot $afterFault -AfterSnapshot $afterRecovery

$uploadTrafficObserved = ($null -ne $deltaSaturation -and [double]$deltaSaturation.trafficDelta -gt 0)
$queueSaturationObserved = ($null -ne $deltaSaturation -and ([double]$deltaSaturation.queueShedDelta -gt 0 -or [double]$deltaSaturation.queueFullDelta -gt 0))
$faultFailureObserved = $null
$recoverySuccessObserved = $null
if ($FaultMode -ne "none") {
  $faultFailureObserved = ($null -ne $deltaFault -and ([double]$deltaFault.uploadFailureDelta -gt 0 -or [double]$deltaFault.retryDelta -gt 0))
  $recoverySuccessObserved = ($null -ne $deltaRecovery -and [double]$deltaRecovery.uploadSuccessDelta -gt 0)
}

$allPassed = $uploadTrafficObserved
if ($RequireQueueSaturation) {
  $allPassed = ($allPassed -and $queueSaturationObserved)
}
if ($FaultMode -ne "none") {
  $allPassed = ($allPassed -and [bool]$faultFailureObserved -and [bool]$recoverySuccessObserved)
}

$summary = [pscustomobject]@{
  generatedAt = (Get-Date).ToString("s")
  baseUrl     = $base
  watchDir    = $WatchDir
  stressConfig = [pscustomobject]@{
    saturation = [pscustomobject]@{ count = $SaturationCount; intervalMs = $SaturationIntervalMs }
    fault      = [pscustomobject]@{ count = $FaultCount; intervalMs = $FaultIntervalMs }
    recovery   = [pscustomobject]@{ count = $RecoveryCount; intervalMs = $RecoveryIntervalMs }
    bytes      = [pscustomobject]@{ min = $MinBytes; max = $MaxBytes }
  }
  fault = [pscustomobject]@{
    mode               = $FaultMode
    observeSeconds     = $FaultObserveSeconds
    recoveryObserveSec = $RecoveryObserveSeconds
    hasRecoverCommand  = (-not [string]::IsNullOrWhiteSpace($FaultRecoverCommand))
  }
  checks = [pscustomobject]@{
    uploadTrafficObserved   = $uploadTrafficObserved
    queueSaturationObserved = $queueSaturationObserved
    faultFailureObserved    = $faultFailureObserved
    recoverySuccessObserved = $recoverySuccessObserved
    requireQueueSaturation  = [bool]$RequireQueueSaturation
  }
  allPassed = $allPassed
  phases    = $phaseResults
  snapshots = [pscustomobject]@{
    baseline        = $baseline
    afterSaturation = $afterSaturation
    afterFault      = $afterFault
    afterRecovery   = $afterRecovery
  }
  deltas = [pscustomobject]@{
    saturation = $deltaSaturation
    fault      = $deltaFault
    recovery   = $deltaRecovery
  }
}

$summary | ConvertTo-Json -Depth 12 | Set-Content -Path $OutputFile -Encoding utf8

$markdown = @()
$markdown += ("# Upload Reliability Recap ({0})" -f (Get-Date -Format "yyyy-MM-dd"))
$markdown += ""
$markdown += ("- generatedAt: {0}" -f $summary.generatedAt)
$markdown += ("- baseUrl: {0}" -f $base)
$markdown += ("- watchDir: {0}" -f $WatchDir)
$markdown += ("- faultMode: {0}" -f $FaultMode)
$markdown += ("- outputJson: {0}" -f $OutputFile)
$markdown += ""
$markdown += "## 1. Phase Results"
$markdown += ""
$markdown += "| Phase | Result | ExitCode | Remark |"
$markdown += "| --- | --- | --- | --- |"
foreach ($phase in $phaseResults) {
  $phaseResult = if ([bool]$phase.ok) { "P" } else { "F" }
  $remark = ([string]$phase.output -replace "\|", "/")
  if ($remark.Length -gt 120) {
    $remark = $remark.Substring(0, 120) + "..."
  }
  $markdown += ("| {0} | {1} | {2} | {3} |" -f $phase.name, $phaseResult, $phase.exitCode, $remark)
}
$markdown += ""
$markdown += "## 2. Delta Metrics"
$markdown += ""
$markdown += "| Window | fileEventsDelta | queueFullDelta | queueShedDelta | retryDelta | successDelta | failureDelta | failureRatio |"
$markdown += "| --- | --- | --- | --- | --- | --- | --- | --- |"
if ($null -ne $deltaSaturation) {
  $markdown += ("| saturation | {0} | {1} | {2} | {3} | {4} | {5} | {6}% |" -f (Format-Number -Value $deltaSaturation.fileEventsDelta -Digits 0), (Format-Number -Value $deltaSaturation.queueFullDelta -Digits 0), (Format-Number -Value $deltaSaturation.queueShedDelta -Digits 0), (Format-Number -Value $deltaSaturation.retryDelta -Digits 0), (Format-Number -Value $deltaSaturation.uploadSuccessDelta -Digits 0), (Format-Number -Value $deltaSaturation.uploadFailureDelta -Digits 0), (Format-Number -Value $deltaSaturation.failureRatioPct -Digits 2))
}
if ($null -ne $deltaFault) {
  $markdown += ("| fault | {0} | {1} | {2} | {3} | {4} | {5} | {6}% |" -f (Format-Number -Value $deltaFault.fileEventsDelta -Digits 0), (Format-Number -Value $deltaFault.queueFullDelta -Digits 0), (Format-Number -Value $deltaFault.queueShedDelta -Digits 0), (Format-Number -Value $deltaFault.retryDelta -Digits 0), (Format-Number -Value $deltaFault.uploadSuccessDelta -Digits 0), (Format-Number -Value $deltaFault.uploadFailureDelta -Digits 0), (Format-Number -Value $deltaFault.failureRatioPct -Digits 2))
}
if ($null -ne $deltaRecovery) {
  $markdown += ("| recovery | {0} | {1} | {2} | {3} | {4} | {5} | {6}% |" -f (Format-Number -Value $deltaRecovery.fileEventsDelta -Digits 0), (Format-Number -Value $deltaRecovery.queueFullDelta -Digits 0), (Format-Number -Value $deltaRecovery.queueShedDelta -Digits 0), (Format-Number -Value $deltaRecovery.retryDelta -Digits 0), (Format-Number -Value $deltaRecovery.uploadSuccessDelta -Digits 0), (Format-Number -Value $deltaRecovery.uploadFailureDelta -Digits 0), (Format-Number -Value $deltaRecovery.failureRatioPct -Digits 2))
}
$markdown += ""
$markdown += "## 3. Checks"
$markdown += ""
$markdown += ("- uploadTrafficObserved: {0}" -f $uploadTrafficObserved)
$markdown += ("- queueSaturationObserved: {0} (require={1})" -f $queueSaturationObserved, [bool]$RequireQueueSaturation)
if ($FaultMode -ne "none") {
  $markdown += ("- faultFailureObserved: {0}" -f $faultFailureObserved)
  $markdown += ("- recoverySuccessObserved: {0}" -f $recoverySuccessObserved)
}
$markdown += ("- allPassed: {0}" -f $allPassed)
$markdown += ""
$markdown += "## 4. Snapshot Files"
$markdown += ""
$markdown += ("- before metrics: {0}" -f $baseline.metricsFile)
$markdown += ("- after saturation metrics: {0}" -f $afterSaturation.metricsFile)
if ($null -ne $afterFault) {
  $markdown += ("- after fault metrics: {0}" -f $afterFault.metricsFile)
}
if ($null -ne $afterRecovery) {
  $markdown += ("- after recovery metrics: {0}" -f $afterRecovery.metricsFile)
}

$lineBreak = [Environment]::NewLine
Set-Content -Path $ReportFile -Encoding utf8 -Value (($markdown -join $lineBreak) + $lineBreak)

Write-Host ("Upload recap completed. allPassed={0}" -f $allPassed)
Write-Host ("JSON: {0}" -f $OutputFile)
Write-Host ("Markdown: {0}" -f $ReportFile)

# exit code:
# 0: executed successfully
# 2: execution failure
# 3: gate failure when FailOnGate is enabled
if ($FailOnGate -and -not $allPassed) {
  exit 3
}
exit 0
