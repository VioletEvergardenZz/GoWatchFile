# 本脚本用于控制面 Agent 在线状态巡检
# 目标：统一“在线判定”口径，输出结构化结果并支持门禁退出码

param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$Token = "",
  [int]$OfflineAfterSec = 45,
  [int]$MaxOfflineAgents = 0,
  [switch]$FailOnOffline = $false,
  [int]$TopLag = 20,
  [string]$OutputFile = "../reports/control-agent-check-result.json",
  [string]$ReportFile = ""
)

function Ensure-Dir {
  param([string]$Path)
  if (-not [string]::IsNullOrWhiteSpace($Path) -and -not (Test-Path $Path)) {
    New-Item -Path $Path -ItemType Directory -Force | Out-Null
  }
}

function Parse-UtcDateTime {
  param([string]$Value)
  $trimmed = [string]$Value
  if ([string]::IsNullOrWhiteSpace($trimmed)) {
    return $null
  }
  try {
    $dto = [DateTimeOffset]::Parse(
      $trimmed,
      [System.Globalization.CultureInfo]::InvariantCulture,
      [System.Globalization.DateTimeStyles]::AssumeUniversal
    )
    return $dto.UtcDateTime
  } catch {
    return $null
  }
}

function Read-MetricValue {
  param(
    [string]$MetricsText,
    [string]$MetricName
  )
  $escapedName = [regex]::Escape($MetricName)
  $numberPattern = "[-+]?[0-9]*\.?[0-9]+(?:[eE][-+]?[0-9]+)?"
  $pattern = "(?m)^${escapedName}(?:\{[^\n]*\})?\s+($numberPattern)\s*$"
  $match = [regex]::Match($MetricsText, $pattern)
  if ($match.Success) {
    return [double]$match.Groups[1].Value
  }
  return $null
}

function Convert-ToAgentClass {
  param(
    [string]$Status,
    [double]$LagSeconds,
    [int]$OfflineAfterSec
  )

  $normalized = ([string]$Status).Trim().ToLower()
  if ($normalized -eq "draining") {
    return [pscustomobject]@{
      Class    = "draining"
      IsOnline = $false
    }
  }
  if ($normalized -eq "offline") {
    return [pscustomobject]@{
      Class    = "offline_flag"
      IsOnline = $false
    }
  }
  if ($LagSeconds -lt 0) {
    return [pscustomobject]@{
      Class    = "online_no_heartbeat"
      IsOnline = $true
    }
  }
  if ($LagSeconds -le $OfflineAfterSec) {
    return [pscustomobject]@{
      Class    = "online"
      IsOnline = $true
    }
  }
  return [pscustomobject]@{
    Class    = "offline_stale"
    IsOnline = $false
  }
}

function Write-MarkdownReport {
  param(
    [string]$FilePath,
    [psobject]$Result
  )

  if ([string]::IsNullOrWhiteSpace($FilePath)) {
    return
  }
  $dir = Split-Path -Parent $FilePath
  Ensure-Dir $dir

  $lines = @()
  $lines += "# 控制面 Agent 在线巡检报告（$(Get-Date -Format yyyy-MM-dd)）"
  $lines += ""
  $lines += "- 生成时间：$($Result.generatedAt)"
  $lines += "- 目标地址：$($Result.baseUrl)"
  $lines += "- 心跳离线阈值：$($Result.offlineAfterSec)s"
  $lines += "- 门禁配置：FailOnOffline=$($Result.gates.failOnOffline) / MaxOfflineAgents=$($Result.gates.maxOfflineAgents)"
  $lines += ""
  $lines += "## 1. 摘要"
  $lines += ""
  $lines += "| 项 | 值 |"
  $lines += "| --- | --- |"
  $lines += "| Agent 总数 | $($Result.summary.total) |"
  $lines += "| 在线数（按心跳判定） | $($Result.summary.online) |"
  $lines += "| 离线总数 | $($Result.summary.offlineTotal) |"
  $lines += "| 其中心跳超时离线 | $($Result.summary.offlineStale) |"
  $lines += "| 其中状态标记离线 | $($Result.summary.offlineFlag) |"
  $lines += "| 维护中 | $($Result.summary.draining) |"
  $lines += "| 无心跳时间（视为在线） | $($Result.summary.onlineNoHeartbeat) |"
  $lines += "| 最大心跳延迟（秒） | $($Result.summary.maxLagSeconds) |"
  $lines += ""

  $lines += "## 2. 离线明细"
  $lines += ""
  if ($Result.offlineAgents -and $Result.offlineAgents.Count -gt 0) {
    $lines += "| Agent | 分类 | 组 | LastSeenAt | Lag(s) |"
    $lines += "| --- | --- | --- | --- | --- |"
    foreach ($item in $Result.offlineAgents) {
      $agentName = if ([string]::IsNullOrWhiteSpace([string]$item.agentKey)) { $item.id } else { $item.agentKey }
      $lines += "| $agentName | $($item.classification) | $($item.groupName) | $($item.lastSeenAt) | $($item.lagSeconds) |"
    }
  } else {
    $lines += "- 无离线 Agent。"
  }
  $lines += ""

  $lines += "## 3. 心跳延迟 Top 列表"
  $lines += ""
  if ($Result.topLagAgents -and $Result.topLagAgents.Count -gt 0) {
    $lines += "| Agent | 分类 | Lag(s) | LastSeenAt |"
    $lines += "| --- | --- | --- | --- |"
    foreach ($item in $Result.topLagAgents) {
      $agentName = if ([string]::IsNullOrWhiteSpace([string]$item.agentKey)) { $item.id } else { $item.agentKey }
      $lines += "| $agentName | $($item.classification) | $($item.lagSeconds) | $($item.lastSeenAt) |"
    }
  } else {
    $lines += "- 无可用心跳数据。"
  }
  $lines += ""

  $lines += "## 4. /metrics 对账"
  $lines += ""
  if ($Result.metrics.available) {
    $lines += "| 指标 | 值 |"
    $lines += "| --- | --- |"
    $lines += "| gwf_control_agents_total | $($Result.metrics.total) |"
    $lines += "| gwf_control_agents_online | $($Result.metrics.online) |"
    $lines += "| gwf_control_agent_heartbeat_lag_seconds | $($Result.metrics.heartbeatLagSeconds) |"
    $lines += "| 与巡检结果一致 | $($Result.metrics.matched) |"
  } else {
    $lines += "- 未获取到 /metrics，对账跳过。"
  }

  Set-Content -Path $FilePath -Encoding UTF8 -Value ($lines -join "`r`n")
}

$base = $BaseUrl.TrimEnd("/")
$headers = @{}
if (-not [string]::IsNullOrWhiteSpace($Token)) {
  $headers["X-API-Token"] = $Token.Trim()
}

$agentsEndpoint = "{0}/api/control/agents" -f $base
$metricsEndpoint = "{0}/metrics" -f $base

Write-Host ("开始 Agent 在线巡检: base={0} offlineAfter={1}s" -f $base, $OfflineAfterSec)

try {
  $agentResp = Invoke-RestMethod -Uri $agentsEndpoint -Method Get -Headers $headers -TimeoutSec 15
} catch {
  Write-Error ("获取控制面 Agent 列表失败: {0}" -f $_.Exception.Message)
  exit 2
}

if (-not $agentResp.ok) {
  Write-Error ("控制面 Agent 接口返回失败: {0}" -f ($agentResp | ConvertTo-Json -Depth 6))
  exit 2
}

$items = @()
if ($null -ne $agentResp.items) {
  $items = @($agentResp.items)
}

$now = (Get-Date).ToUniversalTime()
$rows = @()
foreach ($item in $items) {
  $lastSeenRaw = [string]$item.lastSeenAt
  $lastSeenUtc = Parse-UtcDateTime -Value $lastSeenRaw
  $lagSeconds = -1.0
  if ($null -ne $lastSeenUtc) {
    $lag = $now - $lastSeenUtc
    if ($lag.TotalSeconds -lt 0) {
      $lag = [TimeSpan]::Zero
    }
    $lagSeconds = [Math]::Round($lag.TotalSeconds, 3)
  }
  $judge = Convert-ToAgentClass -Status ([string]$item.status) -LagSeconds $lagSeconds -OfflineAfterSec $OfflineAfterSec
  $rows += [pscustomobject]@{
    id             = [string]$item.id
    agentKey       = [string]$item.agentKey
    groupName      = [string]$item.groupName
    status         = [string]$item.status
    classification = $judge.Class
    isOnline       = [bool]$judge.IsOnline
    lastSeenAt     = $lastSeenRaw
    lagSeconds     = $(if ($lagSeconds -lt 0) { $null } else { $lagSeconds })
    heartbeatCount = [int64]$item.heartbeatCount
  }
}

$total = $rows.Count
$online = @($rows | Where-Object { $_.isOnline }).Count
$draining = @($rows | Where-Object { $_.classification -eq "draining" }).Count
$offlineFlag = @($rows | Where-Object { $_.classification -eq "offline_flag" }).Count
$offlineStale = @($rows | Where-Object { $_.classification -eq "offline_stale" }).Count
$onlineNoHeartbeat = @($rows | Where-Object { $_.classification -eq "online_no_heartbeat" }).Count
$offlineTotal = $offlineFlag + $offlineStale

$lagRows = @($rows | Where-Object { $null -ne $_.lagSeconds } | Sort-Object -Property @{ Expression = { [double]$_.lagSeconds }; Descending = $true }, id)
$topLagRows = @()
if ($TopLag -gt 0) {
  $topLagRows = @($lagRows | Select-Object -First $TopLag)
}
$maxLagSeconds = 0.0
if ($lagRows.Count -gt 0) {
  $maxLagSeconds = [double]$lagRows[0].lagSeconds
}

$metricsSummary = [ordered]@{
  available           = $false
  total               = $null
  online              = $null
  heartbeatLagSeconds = $null
  matched             = $null
}

try {
  $metricsResp = Invoke-WebRequest -Uri $metricsEndpoint -Method Get -Headers $headers -TimeoutSec 15
  if ($metricsResp.StatusCode -ge 200 -and $metricsResp.StatusCode -lt 300) {
    $metricsText = [string]$metricsResp.Content
    if (-not [string]::IsNullOrWhiteSpace($metricsText)) {
      $metricsSummary.available = $true
      $metricsTotal = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_control_agents_total"
      $metricsOnline = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_control_agents_online"
      $metricsLag = Read-MetricValue -MetricsText $metricsText -MetricName "gwf_control_agent_heartbeat_lag_seconds"
      if ($metricsTotal -ne $null) { $metricsSummary.total = [int][Math]::Round($metricsTotal, 0) }
      if ($metricsOnline -ne $null) { $metricsSummary.online = [int][Math]::Round($metricsOnline, 0) }
      if ($metricsLag -ne $null) { $metricsSummary.heartbeatLagSeconds = [Math]::Round($metricsLag, 3) }
      if ($metricsSummary.total -ne $null -and $metricsSummary.online -ne $null) {
        $metricsSummary.matched = (($metricsSummary.total -eq $total) -and ($metricsSummary.online -eq $online))
      }
    }
  }
} catch {
  Write-Warning ("读取 /metrics 失败，跳过对账: {0}" -f $_.Exception.Message)
}

$result = [pscustomobject]@{
  generatedAt      = (Get-Date).ToString("s")
  baseUrl          = $base
  offlineAfterSec  = $OfflineAfterSec
  gates            = [pscustomobject]@{
    failOnOffline    = [bool]$FailOnOffline
    maxOfflineAgents = $MaxOfflineAgents
  }
  summary          = [pscustomobject]@{
    total             = $total
    online            = $online
    offlineTotal      = $offlineTotal
    offlineStale      = $offlineStale
    offlineFlag       = $offlineFlag
    draining          = $draining
    onlineNoHeartbeat = $onlineNoHeartbeat
    maxLagSeconds     = [Math]::Round($maxLagSeconds, 3)
  }
  metrics          = [pscustomobject]$metricsSummary
  offlineAgents    = @($rows | Where-Object { $_.classification -eq "offline_stale" -or $_.classification -eq "offline_flag" })
  topLagAgents     = $topLagRows
  agents           = $rows
}

$outputDir = Split-Path -Parent $OutputFile
Ensure-Dir $outputDir
$result | ConvertTo-Json -Depth 10 | Set-Content -Path $OutputFile -Encoding UTF8
Write-Host ("已输出巡检结果: {0}" -f $OutputFile)

if (-not [string]::IsNullOrWhiteSpace($ReportFile)) {
  Write-MarkdownReport -FilePath $ReportFile -Result $result
  Write-Host ("已输出巡检报告: {0}" -f $ReportFile)
}

Write-Host ("巡检摘要: total={0} online={1} offline={2}(stale={3}, flag={4}) draining={5} maxLag={6}s" -f `
  $total, $online, $offlineTotal, $offlineStale, $offlineFlag, $draining, ([Math]::Round($maxLagSeconds, 3)))

if ($metricsSummary.available -and $metricsSummary.matched -eq $false) {
  Write-Warning ("Agent 在线统计与 /metrics 不一致: local(total={0},online={1}) metrics(total={2},online={3})" -f `
    $total, $online, $metricsSummary.total, $metricsSummary.online)
}

# 退出码约定：
# 0=巡检通过，2=接口请求失败，4=离线超阈值且启用 FailOnOffline
if ($FailOnOffline -and $offlineTotal -gt $MaxOfflineAgents) {
  Write-Error ("离线 Agent 数超阈值: offline={0} > maxOfflineAgents={1}" -f $offlineTotal, $MaxOfflineAgents)
  exit 4
}

exit 0


