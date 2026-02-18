# 本脚本用于控制面任务生命周期回放
# 目标：验证 control API 的最小闭环 (register -> pull -> ack -> complete) 并导出指标快照

param(
  [string]$BaseUrl = "http://localhost:8082",
  [Parameter(Mandatory = $true)]
  [string]$Token,
  [int]$AgentCount = 3,
  [int]$TaskCount = 30,
  [string]$OutputFile = "reports/control-replay-result.json",
  [string]$MetricsFile = "reports/metrics-control-replay.prom"
)

# 统一 endpoint 拼接和鉴权头，避免每次请求重复构造
$base = $BaseUrl.TrimEnd("/")
$headers = @{
  "Content-Type" = "application/json"
  "X-API-Token"  = $Token
}

$agentsEndpoint = "{0}/api/control/agents" -f $base
$tasksEndpoint = "{0}/api/control/tasks" -f $base
$pullEndpoint = "{0}/api/control/dispatch/pull" -f $base

# 边界钳制：
# - 防止误传 0/负数导致空回放
# - 防止超大规模压垮本地演练环境（本脚本定位为门禁与回归，而非极限压测）
if ($AgentCount -le 0) { $AgentCount = 1 }
if ($TaskCount -le 0) { $TaskCount = 1 }
if ($AgentCount -gt 20) { $AgentCount = 20 }
if ($TaskCount -gt 500) { $TaskCount = 500 }

Write-Host ("开始控制面回放: agents={0} tasks={1}" -f $AgentCount, $TaskCount)

$agentIds = @()
for ($i = 1; $i -le $AgentCount; $i++) {
  # 使用可重复推导的 agentKey，便于失败后按 key 检索审计日志
  $payload = @{
    agentKey  = "replay-agent-{0}" -f $i
    hostname  = "replay-host-{0}" -f $i
    version   = "replay"
    ip        = "127.0.0.1"
    groupName = "replay"
  } | ConvertTo-Json

  try {
    $resp = Invoke-RestMethod -Uri $agentsEndpoint -Method Post -Headers $headers -Body $payload -TimeoutSec 10
  } catch {
    Write-Error ("注册 agent 失败: idx={0} err={1}" -f $i, $_.Exception.Message)
    exit 2
  }

  if (-not $resp.ok -or -not $resp.agent -or [string]::IsNullOrWhiteSpace([string]$resp.agent.id)) {
    Write-Error ("注册 agent 响应异常: idx={0} resp={1}" -f $i, ($resp | ConvertTo-Json -Depth 6))
    exit 2
  }
  $agentIds += [string]$resp.agent.id
  Write-Host ("注册 agent 成功: {0} key={1}" -f $resp.agent.id, $resp.agent.agentKey)
}

$taskIds = @()
for ($i = 1; $i -le $TaskCount; $i++) {
  # 任务目标路径为“可识别的伪路径”，不依赖真实文件存在，仅用于驱动状态流转
  $fakePath = "D:/logs/gwf/replay-{0}.log" -f $i
  $payload = @{
    type       = "manual_upload"
    target     = $fakePath
    priority   = "normal"
    createdBy  = "replay"
    maxRetries = 1
    payload    = @{
      path = $fakePath
    }
  } | ConvertTo-Json -Depth 6

  try {
    $resp = Invoke-RestMethod -Uri $tasksEndpoint -Method Post -Headers $headers -Body $payload -TimeoutSec 10
  } catch {
    Write-Error ("创建 task 失败: idx={0} err={1}" -f $i, $_.Exception.Message)
    exit 2
  }

  if (-not $resp.ok -or -not $resp.task -or [string]::IsNullOrWhiteSpace([string]$resp.task.id)) {
    Write-Error ("创建 task 响应异常: idx={0} resp={1}" -f $i, ($resp | ConvertTo-Json -Depth 6))
    exit 2
  }
  $taskIds += [string]$resp.task.id
}
Write-Host ("创建任务完成: total={0}" -f $taskIds.Count)

# done 字典用于去重，避免重复 pull 导致同一任务被重复统计
$done = @{}
$success = 0
$failed = 0
$start = Get-Date

# 轮询上限与任务量相关，避免后端异常时脚本无限等待
$maxLoops = [Math]::Max(200, $TaskCount * 20)
for ($loop = 1; $loop -le $maxLoops; $loop++) {
  foreach ($agentId in $agentIds) {
    if ($done.Count -ge $TaskCount) { break }

    $pullPayload = @{
      agentId  = $agentId
      maxTasks = 1
    } | ConvertTo-Json

    try {
      $pull = Invoke-RestMethod -Uri $pullEndpoint -Method Post -Headers $headers -Body $pullPayload -TimeoutSec 10
    } catch {
      Write-Error ("pull 失败: agent={0} err={1}" -f $agentId, $_.Exception.Message)
      exit 2
    }

    if (-not $pull.ok) { continue }
    if (-not $pull.items -or $pull.items.Count -eq 0) { continue }

    foreach ($item in $pull.items) {
      $taskId = [string]$item.id
      if ([string]::IsNullOrWhiteSpace($taskId)) { continue }
      if ($done.ContainsKey($taskId)) { continue }

      $ackEndpoint = "{0}/api/control/tasks/{1}/ack" -f $base, $taskId
      $completeEndpoint = "{0}/api/control/tasks/{1}/complete" -f $base, $taskId

      # ack -> complete 按最小闭环顺序执行，确保覆盖任务状态机主路径
      try {
        $ackPayload = @{ agentId = $agentId } | ConvertTo-Json
        $ackResp = Invoke-RestMethod -Uri $ackEndpoint -Method Post -Headers $headers -Body $ackPayload -TimeoutSec 10
      } catch {
        Write-Error ("ack 失败: task={0} agent={1} err={2}" -f $taskId, $agentId, $_.Exception.Message)
        exit 2
      }

      try {
        $completePayload = @{ agentId = $agentId; status = "success"; message = "ok" } | ConvertTo-Json
        $completeResp = Invoke-RestMethod -Uri $completeEndpoint -Method Post -Headers $headers -Body $completePayload -TimeoutSec 10
      } catch {
        Write-Error ("complete 失败: task={0} agent={1} err={2}" -f $taskId, $agentId, $_.Exception.Message)
        exit 2
      }

      if ($completeResp.ok -and $completeResp.task -and [string]$completeResp.task.status -eq "success") {
        $success++
      } else {
        $failed++
      }
      $done[$taskId] = $true
      Write-Host ("完成任务: {0} agent={1} done={2}/{3}" -f $taskId, $agentId, $done.Count, $TaskCount)
    }
  }
  if ($done.Count -ge $TaskCount) { break }
  # 短暂退避，避免空拉取时对 dispatch 接口形成无意义高频压力
  Start-Sleep -Milliseconds 50
}

$elapsedMs = [int64]((Get-Date) - $start).TotalMilliseconds

if ($done.Count -lt $TaskCount) {
  Write-Error ("回放未完成: done={0}/{1}，请检查 control /metrics 与后端日志" -f $done.Count, $TaskCount)
}

if ($MetricsFile -ne "") {
  # metrics 快照为“尽力而为”策略：回放结果优先，抓取失败仅告警不覆盖主退出逻辑
  $metricsEndpoint = "{0}/metrics" -f $base
  try {
    $resp = Invoke-WebRequest -Uri $metricsEndpoint -Method Get -TimeoutSec 10
    $content = [string]$resp.Content
    $dir = Split-Path -Parent $MetricsFile
    if ($dir -and -not (Test-Path $dir)) {
      New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
    Set-Content -Path $MetricsFile -Encoding utf8 -Value $content
    Write-Host "已保存 metrics 快照: $MetricsFile"
  } catch {
    Write-Warning ("保存 metrics 失败: {0}" -f $_.Exception.Message)
  }
}

$summary = [pscustomobject]@{
  agents    = $agentIds
  tasks     = $taskIds
  done      = $done.Count
  total     = $TaskCount
  success   = $success
  failed    = $failed
  elapsedMs = $elapsedMs
}

$outDir = Split-Path -Parent $OutputFile
if ($outDir -and -not (Test-Path $outDir)) {
  New-Item -ItemType Directory -Path $outDir -Force | Out-Null
}
$summary | ConvertTo-Json -Depth 6 | Set-Content -Path $OutputFile -Encoding utf8

Write-Host ("回放完成 done={0}/{1} success={2} failed={3} elapsedMs={4}" -f $done.Count, $TaskCount, $success, $failed, $elapsedMs)
Write-Host "结果文件: $OutputFile"

# 退出码约定：
# 0=回放闭环完成，2=接口调用或响应结构异常，3=在轮询上限内未完成全部任务
if ($done.Count -lt $TaskCount) {
  exit 3
}
exit 0

