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
  "gwf_file_events_total",
  "gwf_upload_queue_length",
  "gwf_upload_queue_full_total",
  "gwf_upload_queue_shed_total",
  "gwf_upload_duration_seconds",
  "gwf_ai_log_summary_total",
  "gwf_ai_log_summary_retry_total",
  "gwf_kb_search_hit_ratio",
  "gwf_kb_ask_citation_ratio"
)

$missing = @()
foreach ($metric in $required) {
  if ($content -notmatch "(?m)^$metric(\{.*\})?\s+") {
    $missing += $metric
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
