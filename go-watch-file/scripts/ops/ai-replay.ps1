# 本脚本用于批量回放 AI 日志分析请求
# 在原有降级率统计基础上 增加结构一致性和错误分类覆盖率验收

param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$Token = "",
  [Parameter(Mandatory = $true)]
  [string]$PathsFile,
  [int]$Limit = 200,
  [switch]$CaseSensitive,
  [double]$DegradedRatioTarget = 0.2,
  [double]$StructurePassRatioTarget = 1.0,
  [double]$ErrorClassCoverageTarget = 1.0,
  [switch]$FailOnGate,
  [string]$OutputFile = "reports/ai-replay-result.json"
)

function Add-MapCount {
  param(
    [hashtable]$Map,
    [string]$Key
  )
  if ($null -eq $Map -or [string]::IsNullOrWhiteSpace($Key)) {
    return
  }
  if (-not $Map.ContainsKey($Key)) {
    $Map[$Key] = 0
  }
  $Map[$Key]++
}

function Parse-DoubleInvariant {
  param([string]$Raw)
  $value = 0.0
  if ([double]::TryParse($Raw, [System.Globalization.NumberStyles]::Float, [System.Globalization.CultureInfo]::InvariantCulture, [ref]$value)) {
    return $value
  }
  return $null
}

function Normalize-Severity {
  param([string]$Severity)
  $trimmed = [string]$Severity
  if ($null -eq $trimmed) {
    return "unknown"
  }
  $clean = $trimmed.Trim().ToLowerInvariant()
  if ($clean -eq "low" -or $clean -eq "medium" -or $clean -eq "high") {
    return $clean
  }
  return "unknown"
}

function Convert-MapToObject {
  param([hashtable]$Map)
  $obj = [ordered]@{}
  if ($null -ne $Map) {
    foreach ($key in ($Map.Keys | Sort-Object)) {
      $obj[$key] = $Map[$key]
    }
  }
  return [pscustomobject]$obj
}

function Test-AIAnalysisStructure {
  param($Analysis)

  $issues = @()
  $severity = "unknown"
  $suggestions = @()
  $causes = @()
  $keyErrors = @()
  $confidence = $null

  if ($null -eq $Analysis) {
    $issues += "analysis_missing"
    return [pscustomobject]@{
      ok               = $false
      issues           = $issues
      severity         = "unknown"
      suggestionsCount = 0
      causesCount      = 0
      keyErrorsCount   = 0
      confidence       = $null
    }
  }

  $summary = [string]$Analysis.summary
  if ([string]::IsNullOrWhiteSpace($summary)) {
    $issues += "summary_missing"
  }

  $rawSeverity = [string]$Analysis.severity
  $severity = Normalize-Severity -Severity $rawSeverity
  if ($severity -eq "unknown") {
    $issues += "severity_invalid"
  }

  if ($null -ne $Analysis.suggestions) {
    $suggestions = @($Analysis.suggestions)
  } else {
    $issues += "suggestions_missing"
  }
  if ($suggestions.Count -eq 0) {
    $issues += "suggestions_empty"
  }
  if ($suggestions.Count -gt 3) {
    $issues += "suggestions_over_limit"
  }

  if ($null -ne $Analysis.causes) {
    $causes = @($Analysis.causes)
  } else {
    $issues += "causes_missing"
  }
  if ($causes.Count -gt 3) {
    $issues += "causes_over_limit"
  }

  if ($null -ne $Analysis.keyErrors) {
    $keyErrors = @($Analysis.keyErrors)
  } else {
    $issues += "keyErrors_missing"
  }
  if ($keyErrors.Count -gt 5) {
    $issues += "keyErrors_over_limit"
  }

  $confidenceRaw = $Analysis.confidence
  if ($null -ne $confidenceRaw -and -not [string]::IsNullOrWhiteSpace([string]$confidenceRaw)) {
    $confidence = Parse-DoubleInvariant -Raw ([string]$confidenceRaw)
    if ($null -eq $confidence) {
      $issues += "confidence_invalid"
    } elseif ($confidence -lt 0 -or $confidence -gt 1) {
      $issues += "confidence_out_of_range"
    }
  }

  return [pscustomobject]@{
    ok               = ($issues.Count -eq 0)
    issues           = $issues
    severity         = $severity
    suggestionsCount = $suggestions.Count
    causesCount      = $causes.Count
    keyErrorsCount   = $keyErrors.Count
    confidence       = $confidence
  }
}

if (-not (Test-Path $PathsFile)) {
  Write-Error "路径文件不存在: $PathsFile"
  exit 2
}

$paths = Get-Content -Encoding utf8 $PathsFile | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne "" -and -not $_.StartsWith("#") }
if ($paths.Count -eq 0) {
  Write-Error "路径文件为空"
  exit 2
}

$endpoint = "{0}/api/ai/log-summary" -f $BaseUrl.TrimEnd("/")
$headers = @{
  "Content-Type" = "application/json"
}
if (-not [string]::IsNullOrWhiteSpace($Token)) {
  $headers["X-API-Token"] = $Token.Trim()
}

$allowedErrorClassSet = @{}
@("timeout", "network", "rate_limit", "auth", "upstream_5xx", "upstream_4xx", "parse_error", "request_error", "unknown") | ForEach-Object {
  $allowedErrorClassSet[$_] = $true
}

$rows = @()
$degraded = 0
$success = 0
$requestFailed = 0

$errorClassMap = @{}
$structureIssueMap = @{}
$severityMap = @{
  "low" = 0
  "medium" = 0
  "high" = 0
  "unknown" = 0
}

$structureChecked = 0
$structurePassed = 0
$structureFailed = 0

$classifiedDegraded = 0
$knownErrorClassDegraded = 0

for ($i = 0; $i -lt $paths.Count; $i++) {
  $path = $paths[$i]
  $payload = @{
    path          = $path
    mode          = "tail"
    limit         = $Limit
    caseSensitive = [bool]$CaseSensitive
  } | ConvertTo-Json

  try {
    $resp = Invoke-RestMethod -Uri $endpoint -Method Post -Headers $headers -Body $payload -TimeoutSec 30
  } catch {
    $requestFailed++
    $degraded++
    $classifiedDegraded++
    $knownErrorClassDegraded++
    Add-MapCount -Map $errorClassMap -Key "request_error"
    $rows += [pscustomobject]@{
      path             = $path
      ok               = $false
      degraded         = $true
      errorClass       = "request_error"
      elapsedMs        = 0
      structureChecked = $false
      structureOK      = $false
      structureIssues  = @("request_error")
      severity         = "unknown"
      suggestionsCount = 0
      causesCount      = 0
      keyErrorsCount   = 0
      confidence       = $null
    }
    Write-Host ("[{0}/{1}] 请求失败: {2}" -f ($i + 1), $paths.Count, $path)
    continue
  }

  $isDegraded = $false
  $errorClass = ""
  $elapsedMs = 0
  if ($null -ne $resp.meta) {
    $isDegraded = [bool]$resp.meta.degraded
    $errorClass = [string]$resp.meta.errorClass
    $elapsedMs = [int64]$resp.meta.elapsedMs
  }

  $validation = Test-AIAnalysisStructure -Analysis $resp.analysis
  $structureChecked++
  if ([bool]$validation.ok) {
    $structurePassed++
  } else {
    $structureFailed++
    foreach ($issue in $validation.issues) {
      Add-MapCount -Map $structureIssueMap -Key ([string]$issue)
    }
  }

  $severity = [string]$validation.severity
  if (-not $severityMap.ContainsKey($severity)) {
    $severity = "unknown"
  }
  $severityMap[$severity]++

  if ($isDegraded) {
    $degraded++
    if ([string]::IsNullOrWhiteSpace($errorClass)) {
      $errorClass = "unknown"
    }
    $classifiedDegraded++
    if ($allowedErrorClassSet.ContainsKey($errorClass)) {
      $knownErrorClassDegraded++
    }
    Add-MapCount -Map $errorClassMap -Key $errorClass
  } else {
    $success++
  }

  $rows += [pscustomobject]@{
    path             = $path
    ok               = [bool]$resp.ok
    degraded         = $isDegraded
    errorClass       = $errorClass
    elapsedMs        = $elapsedMs
    structureChecked = $true
    structureOK      = [bool]$validation.ok
    structureIssues  = @($validation.issues)
    severity         = $severity
    suggestionsCount = [int]$validation.suggestionsCount
    causesCount      = [int]$validation.causesCount
    keyErrorsCount   = [int]$validation.keyErrorsCount
    confidence       = $validation.confidence
  }
  Write-Host ("[{0}/{1}] 完成: {2} degraded={3} structure={4}" -f ($i + 1), $paths.Count, $path, $isDegraded, [bool]$validation.ok)
}

$total = [int]$paths.Count
$degradedRatio = if ($total -eq 0) { 0.0 } else { [Math]::Round($degraded / $total, 4) }
$structurePassRatio = if ($structureChecked -eq 0) { 0.0 } else { [Math]::Round($structurePassed / $structureChecked, 4) }
$errorClassCoverage = if ($degraded -eq 0) { 1.0 } else { [Math]::Round($classifiedDegraded / $degraded, 4) }
$knownErrorClassRatio = if ($degraded -eq 0) { 1.0 } else { [Math]::Round($knownErrorClassDegraded / $degraded, 4) }

$degradedGatePass = ($degradedRatio -le $DegradedRatioTarget)
$structureGatePass = ($structurePassRatio -ge $StructurePassRatioTarget)
$errorClassCoveragePass = ($errorClassCoverage -ge $ErrorClassCoverageTarget)
$allPassed = ($degradedGatePass -and $structureGatePass -and $errorClassCoveragePass)

$summary = [pscustomobject]@{
  total               = $total
  success             = $success
  degraded            = $degraded
  requestFailed       = $requestFailed
  degradedRatio       = $degradedRatio
  structure           = [pscustomobject]@{
    checked   = $structureChecked
    passed    = $structurePassed
    failed    = $structureFailed
    passRatio = $structurePassRatio
    issues    = $(Convert-MapToObject -Map $structureIssueMap)
  }
  severity            = $(Convert-MapToObject -Map $severityMap)
  errorClass          = $(Convert-MapToObject -Map $errorClassMap)
  errorClassCoverage  = $errorClassCoverage
  knownErrorClassRatio = $knownErrorClassRatio
  gateTargets         = [pscustomobject]@{
    degradedRatio      = $DegradedRatioTarget
    structurePassRatio = $StructurePassRatioTarget
    errorClassCoverage = $ErrorClassCoverageTarget
  }
  gates               = [pscustomobject]@{
    degradedRatioPass      = $degradedGatePass
    structurePassRatioPass = $structureGatePass
    errorClassCoveragePass = $errorClassCoveragePass
    allPassed              = $allPassed
  }
  results             = $rows
}

$outputDir = Split-Path -Parent $OutputFile
if ($outputDir -and -not (Test-Path $outputDir)) {
  New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
}
$summary | ConvertTo-Json -Depth 10 | Set-Content -Path $OutputFile -Encoding utf8

Write-Host ("回放完成 total={0} success={1} degraded={2} ratio={3}" -f $total, $success, $degraded, $degradedRatio)
Write-Host ("结构校验 checked={0} passed={1} failed={2} ratio={3}" -f $structureChecked, $structurePassed, $structureFailed, $structurePassRatio)
Write-Host ("错误分类覆盖率={0} knownRatio={1}" -f $errorClassCoverage, $knownErrorClassRatio)
Write-Host ("门禁结果 allPassed={0}" -f $allPassed)
Write-Host ("结果文件: {0}" -f $OutputFile)

# exit code:
# 0 = 完成
# 2 = 输入参数错误
# 3 = 启用 FailOnGate 且门禁失败
if ($FailOnGate -and -not $allPassed) {
  exit 3
}
exit 0
