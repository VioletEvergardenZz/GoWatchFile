# 本脚本用于批量回放 AI 日志分析请求 统计降级率和错误分类
# 输入文件每行一个日志路径

param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$Token = "",
  [Parameter(Mandatory = $true)]
  [string]$PathsFile,
  [int]$Limit = 200,
  [switch]$CaseSensitive,
  [string]$OutputFile = "reports/ai-replay-result.json"
)

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

$rows = @()
$degraded = 0
$errorClassMap = @{}
$success = 0

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
    $rows += [pscustomobject]@{
      path       = $path
      ok         = $false
      degraded   = $true
      errorClass = "request_error"
      elapsedMs  = 0
    }
    $degraded++
    if (-not $errorClassMap.ContainsKey("request_error")) { $errorClassMap["request_error"] = 0 }
    $errorClassMap["request_error"]++
    Write-Host ("[{0}/{1}] 请求失败: {2}" -f ($i + 1), $paths.Count, $path)
    continue
  }

  $isDegraded = $false
  $errorClass = ""
  $elapsedMs = 0
  if ($resp.meta) {
    $isDegraded = [bool]$resp.meta.degraded
    $errorClass = [string]$resp.meta.errorClass
    $elapsedMs = [int64]$resp.meta.elapsedMs
  }
  if ($isDegraded) {
    $degraded++
    if ([string]::IsNullOrWhiteSpace($errorClass)) { $errorClass = "unknown" }
    if (-not $errorClassMap.ContainsKey($errorClass)) { $errorClassMap[$errorClass] = 0 }
    $errorClassMap[$errorClass]++
  } else {
    $success++
  }

  $rows += [pscustomobject]@{
    path       = $path
    ok         = [bool]$resp.ok
    degraded   = $isDegraded
    errorClass = $errorClass
    elapsedMs  = $elapsedMs
  }
  Write-Host ("[{0}/{1}] 完成: {2} degraded={3}" -f ($i + 1), $paths.Count, $path, $isDegraded)
}

$summary = [pscustomobject]@{
  total         = $paths.Count
  success       = $success
  degraded      = $degraded
  degradedRatio = if ($paths.Count -eq 0) { 0 } else { [Math]::Round($degraded / $paths.Count, 4) }
  errorClass    = $errorClassMap
  results       = $rows
}

$outputDir = Split-Path -Parent $OutputFile
if ($outputDir -and -not (Test-Path $outputDir)) {
  New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
}
$summary | ConvertTo-Json -Depth 6 | Set-Content -Path $OutputFile -Encoding utf8

Write-Host ("回放完成 total={0} success={1} degraded={2} ratio={3}" -f $paths.Count, $success, $degraded, $summary.degradedRatio)
Write-Host "结果文件: $OutputFile"
exit 0
