# 本脚本用于检查指标接口可用性并校验关键指标是否已暴露
# 建议在发布后和日常巡检时执行

param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$OutputFile = ""
)

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
  # histogram 指标需要校验 bucket/sum/count 子项，而不是裸指标名
  @{ Name = "gwf_upload_duration_seconds"; Pattern = "(?m)^gwf_upload_duration_seconds_(bucket|sum|count)(\{.*\})?\s+" },
  # ai summary 按 label 输出，零流量时可能仅有 HELP/TYPE，无样本行
  @{ Name = "gwf_ai_log_summary_total"; Pattern = "(?m)(^gwf_ai_log_summary_total(\{.*\})?\s+|^# TYPE gwf_ai_log_summary_total counter$)" },
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

Write-Host "指标检查通过"
exit 0

