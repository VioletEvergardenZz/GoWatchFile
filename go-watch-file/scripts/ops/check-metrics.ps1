# 本脚本用于检查指标接口可用性并校验关键指标是否已暴露
# 建议在发布后和日常巡检时执行

param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$OutputFile = ""
)

$endpoint = "{0}/metrics" -f $BaseUrl.TrimEnd("/")
Write-Host "检查指标端点: $endpoint"

try {
  $resp = Invoke-WebRequest -Uri $endpoint -Method Get -TimeoutSec 10
} catch {
  Write-Error "访问指标端点失败: $($_.Exception.Message)"
  exit 2
}

if ($resp.StatusCode -lt 200 -or $resp.StatusCode -ge 300) {
  Write-Error "指标端点状态码异常: $($resp.StatusCode)"
  exit 2
}

$content = [string]$resp.Content
if ([string]::IsNullOrWhiteSpace($content)) {
  Write-Error "指标响应为空"
  exit 2
}

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
  @{ Name = "gwf_kb_ask_citation_ratio"; Pattern = "(?m)^gwf_kb_ask_citation_ratio(\{.*\})?\s+" }
)

$missing = @()
foreach ($metric in $required) {
  if ($content -notmatch $metric.Pattern) {
    $missing += $metric.Name
  }
}

if ($OutputFile -ne "") {
  $dir = Split-Path -Parent $OutputFile
  if ($dir -and -not (Test-Path $dir)) {
    New-Item -Path $dir -ItemType Directory | Out-Null
  }
  Set-Content -Path $OutputFile -Encoding utf8 -Value $content
  Write-Host "已保存指标快照: $OutputFile"
}

if ($missing.Count -gt 0) {
  Write-Error ("缺少关键指标: " + ($missing -join ", "))
  exit 3
}

Write-Host "指标检查通过"
exit 0
