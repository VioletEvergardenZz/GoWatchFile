# 本脚本用于检查指标接口可用性并校验关键指标是否已暴露
# 建议在发布后和日常巡检时执行

param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$OutputFile = "",
  [switch]$CheckThresholds = $false,
  [switch]$FailOnWarning = $false,
  [double]$QueueLengthWarning = 80,
  [double]$QueueLengthCritical = 95,
  [double]$UploadFailureRatioWarning = 0.05,
  [double]$UploadFailureRatioCritical = 0.15,
  [double]$AIDegradedRatioWarning = 0.20,
  [double]$AIDegradedRatioCritical = 0.40
)

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

function Read-MetricValueByLabel {
  param(
    [string]$MetricsText,
    [string]$MetricName,
    [string]$LabelName,
    [string]$LabelValue
  )

  $escapedName = [regex]::Escape($MetricName)
  $escapedLabelName = [regex]::Escape($LabelName)
  $escapedLabelValue = [regex]::Escape($LabelValue)
  $numberPattern = "[-+]?[0-9]*\.?[0-9]+(?:[eE][-+]?[0-9]+)?"
  $pattern = "(?m)^${escapedName}\{[^\n]*\b${escapedLabelName}=""${escapedLabelValue}""[^\n]*\}\s+($numberPattern)\s*$"
  $match = [regex]::Match($MetricsText, $pattern)
  if ($match.Success) {
    return [double]$match.Groups[1].Value
  }
  return $null
}

function Read-MetricSum {
  param(
    [string]$MetricsText,
    [string]$MetricName
  )

  $escapedName = [regex]::Escape($MetricName)
  $numberPattern = "[-+]?[0-9]*\.?[0-9]+(?:[eE][-+]?[0-9]+)?"
  $pattern = "(?m)^${escapedName}(?:\{[^\n]*\})?\s+($numberPattern)\s*$"
  $matches = [regex]::Matches($MetricsText, $pattern)
  if ($matches.Count -eq 0) {
    return $null
  }
  $sum = 0.0
  foreach ($metricMatch in $matches) {
    $sum += [double]$metricMatch.Groups[1].Value
  }
  return $sum
}

# 先规整 URL 再拼接，避免调用方传入末尾斜杠导致重复 //
$endpoint = "{0}/metrics" -f $BaseUrl.TrimEnd("/")
Write-Host "检查指标端点: $endpoint"

try {
  $resp = Invoke-WebRequest -Uri $endpoint -Method Get -TimeoutSec 10
} catch {
  Write-Error "访问指标端点失败: $($_.Exception.Message)"
  exit 2
}

# 仅接受 2xx，提前拦截 401/500 等常见发布异常
if ($resp.StatusCode -lt 200 -or $resp.StatusCode -ge 300) {
  Write-Error "指标端点状态码异常: $($resp.StatusCode)"
  exit 2
}

$content = [string]$resp.Content
# 返回空文本通常意味着网关/反向代理链路异常，直接判失败
if ([string]::IsNullOrWhiteSpace($content)) {
  Write-Error "指标响应为空"
  exit 2
}

# 关键指标最小集：
# - 覆盖上传、AI、知识库、控制面四条主链路
# - 使用正则而非精确文本，兼容标签变化和样本顺序变化
$required = @(
  @{ Name = "gwf_file_events_total"; Pattern = "(?m)^gwf_file_events_total(\{.*\})?\s+" },
  @{ Name = "gwf_upload_queue_length"; Pattern = "(?m)^gwf_upload_queue_length(\{.*\})?\s+" },
  @{ Name = "gwf_upload_queue_full_total"; Pattern = "(?m)^gwf_upload_queue_full_total(\{.*\})?\s+" },
  @{ Name = "gwf_upload_queue_shed_total"; Pattern = "(?m)^gwf_upload_queue_shed_total(\{.*\})?\s+" },
  @{ Name = "gwf_upload_success_total"; Pattern = "(?m)^gwf_upload_success_total(\{.*\})?\s+" },
  @{ Name = "gwf_upload_failure_total"; Pattern = "(?m)^gwf_upload_failure_total(\{.*\})?\s+" },
  # histogram 指标需要校验 bucket/sum/count 子项，而不是裸指标名
  @{ Name = "gwf_upload_duration_seconds"; Pattern = "(?m)^gwf_upload_duration_seconds_(bucket|sum|count)(\{.*\})?\s+" },
  # ai summary 按 label 输出，零流量时可能仅有 HELP/TYPE，无样本行
  @{ Name = "gwf_ai_log_summary_total"; Pattern = '(?m)^gwf_ai_log_summary_total\{.*outcome="(success|degraded)".*\}\s+' },
  @{ Name = "gwf_ai_log_summary_retry_total"; Pattern = "(?m)^gwf_ai_log_summary_retry_total(\{.*\})?\s+" },
  @{ Name = "gwf_kb_search_hit_ratio"; Pattern = "(?m)^gwf_kb_search_hit_ratio(\{.*\})?\s+" },
  @{ Name = "gwf_kb_ask_citation_ratio"; Pattern = "(?m)^gwf_kb_ask_citation_ratio(\{.*\})?\s+" },
  @{ Name = "gwf_control_agents_total"; Pattern = "(?m)^gwf_control_agents_total(\{.*\})?\s+" },
  @{ Name = "gwf_control_agents_online"; Pattern = "(?m)^gwf_control_agents_online(\{.*\})?\s+" },
  @{ Name = "gwf_control_agent_heartbeat_lag_seconds"; Pattern = "(?m)^gwf_control_agent_heartbeat_lag_seconds(\{.*\})?\s+" },
  @{ Name = "gwf_control_task_backlog"; Pattern = "(?m)^gwf_control_task_backlog(\{.*\})?\s+" },
  @{ Name = "gwf_control_tasks_total"; Pattern = "(?m)^gwf_control_tasks_total(\{.*\})?\s+" },
  @{ Name = "gwf_control_task_timeout_total"; Pattern = "(?m)^gwf_control_task_timeout_total(\{.*\})?\s+" },
  @{ Name = "gwf_control_task_duration_seconds"; Pattern = "(?m)^gwf_control_task_duration_seconds_(bucket|sum|count)(\{.*\})?\s+" }
)

$missing = @()
foreach ($metric in $required) {
  if ($content -notmatch $metric.Pattern) {
    $missing += $metric.Name
  }
}

# 可选写出原始快照，便于复盘“为什么缺指标”
if ($OutputFile -ne "") {
  $dir = Split-Path -Parent $OutputFile
  if ($dir -and -not (Test-Path $dir)) {
    New-Item -Path $dir -ItemType Directory | Out-Null
  }
  Set-Content -Path $OutputFile -Encoding utf8 -Value $content
  Write-Host "已保存指标快照: $OutputFile"
}

# 退出码约定：0=通过，2=端点访问/响应异常，3=缺少关键指标
if ($missing.Count -gt 0) {
  Write-Error ("缺少关键指标: " + ($missing -join ", "))
  exit 3
}

if ($CheckThresholds) {
  $findings = @()

  $queueLength = Read-MetricValue -MetricsText $content -MetricName "gwf_upload_queue_length"
  if ($queueLength -ne $null) {
    if ($queueLength -ge $QueueLengthCritical) {
      $findings += [PSCustomObject]@{
        Severity = "critical"
        Name     = "queue_length"
        Value    = [math]::Round($queueLength, 4)
        Message  = "上传队列长度超过 Critical 阈值（$QueueLengthCritical）"
      }
    } elseif ($queueLength -ge $QueueLengthWarning) {
      $findings += [PSCustomObject]@{
        Severity = "warning"
        Name     = "queue_length"
        Value    = [math]::Round($queueLength, 4)
        Message  = "上传队列长度超过 Warning 阈值（$QueueLengthWarning）"
      }
    }
  }

  $uploadSuccess = Read-MetricValue -MetricsText $content -MetricName "gwf_upload_success_total"
  $uploadFailure = Read-MetricValue -MetricsText $content -MetricName "gwf_upload_failure_total"
  $uploadTotal = $uploadSuccess + $uploadFailure
  $uploadFailureRatio = 0.0
  if ($uploadTotal -gt 0) {
    $uploadFailureRatio = $uploadFailure / $uploadTotal
  }

  if ($uploadFailureRatio -ge $UploadFailureRatioCritical) {
    $findings += [PSCustomObject]@{
      Severity = "critical"
      Name     = "upload_failure_ratio"
      Value    = [math]::Round($uploadFailureRatio, 6)
      Message  = "上传失败率超过 Critical 阈值（$UploadFailureRatioCritical）"
    }
  } elseif ($uploadFailureRatio -ge $UploadFailureRatioWarning) {
    $findings += [PSCustomObject]@{
      Severity = "warning"
      Name     = "upload_failure_ratio"
      Value    = [math]::Round($uploadFailureRatio, 6)
      Message  = "上传失败率超过 Warning 阈值（$UploadFailureRatioWarning）"
    }
  }

  $aiDegraded = Read-MetricValueByLabel -MetricsText $content -MetricName "gwf_ai_log_summary_total" -LabelName "outcome" -LabelValue "degraded"
  $aiTotal = Read-MetricSum -MetricsText $content -MetricName "gwf_ai_log_summary_total"
  $aiDegradedRatio = 0.0
  if ($aiTotal -gt 0) {
    $aiDegradedRatio = $aiDegraded / $aiTotal
  }

  if ($aiDegradedRatio -ge $AIDegradedRatioCritical) {
    $findings += [PSCustomObject]@{
      Severity = "critical"
      Name     = "ai_degraded_ratio"
      Value    = [math]::Round($aiDegradedRatio, 6)
      Message  = "AI 降级率超过 Critical 阈值（$AIDegradedRatioCritical）"
    }
  } elseif ($aiDegradedRatio -ge $AIDegradedRatioWarning) {
    $findings += [PSCustomObject]@{
      Severity = "warning"
      Name     = "ai_degraded_ratio"
      Value    = [math]::Round($aiDegradedRatio, 6)
      Message  = "AI 降级率超过 Warning 阈值（$AIDegradedRatioWarning）"
    }
  }

  Write-Host ("阈值快照: queue_length={0}, upload_failure_ratio={1:P2}, ai_degraded_ratio={2:P2}" -f `
    [math]::Round($queueLength, 4), `
    $uploadFailureRatio, `
    $aiDegradedRatio)

  $criticalFindings = @($findings | Where-Object { $_.Severity -eq "critical" })
  $warningFindings = @($findings | Where-Object { $_.Severity -eq "warning" })

  if ($criticalFindings.Count -gt 0) {
    $detail = ($criticalFindings | ForEach-Object { "{0}={1} ({2})" -f $_.Name, $_.Value, $_.Message }) -join "; "
    Write-Error ("阈值告警（Critical）: " + $detail)
    exit 4
  }

  if ($warningFindings.Count -gt 0) {
    $detail = ($warningFindings | ForEach-Object { "{0}={1} ({2})" -f $_.Name, $_.Value, $_.Message }) -join "; "
    if ($FailOnWarning) {
      Write-Error ("阈值告警（Warning）: " + $detail)
      exit 5
    }
    Write-Warning ("阈值告警（Warning）: " + $detail)
  }
}

Write-Host "指标检查通过"
exit 0

