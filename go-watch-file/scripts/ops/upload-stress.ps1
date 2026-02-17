# 本脚本用于生成批量测试文件 验证上传队列与背压行为
# 可用于压测和故障演练前置数据准备

param(
  [Parameter(Mandatory = $true)]
  [string]$WatchDir,
  [int]$Count = 1000,
  [int]$IntervalMs = 20,
  [int]$MinBytes = 512,
  [int]$MaxBytes = 4096,
  [string]$Prefix = "stress"
)

if ($Count -le 0) {
  Write-Error "Count must be greater than 0"
  exit 2
}
if ($MinBytes -le 0 -or $MaxBytes -lt $MinBytes) {
  Write-Error "Invalid MinBytes/MaxBytes range"
  exit 2
}

if (-not (Test-Path $WatchDir)) {
  New-Item -ItemType Directory -Path $WatchDir -Force | Out-Null
}

$random = New-Object System.Random
$start = Get-Date

Write-Host "Start generating stress files"
Write-Host "Directory: $WatchDir"
Write-Host "Count: $Count"

for ($i = 1; $i -le $Count; $i++) {
  $size = $random.Next($MinBytes, $MaxBytes + 1)
  $ts = Get-Date -Format "yyyyMMdd_HHmmss_fff"
  $name = "{0}_{1}_{2:D6}.log" -f $Prefix, $ts, $i
  $path = Join-Path $WatchDir $name

  $header = "case=$Prefix index=$i size=$size time=$ts`n"
  $bodySize = [Math]::Max(0, $size - $header.Length)
  $body = ("x" * $bodySize)
  Set-Content -Path $path -Encoding utf8 -Value ($header + $body)

  if ($IntervalMs -gt 0) {
    Start-Sleep -Milliseconds $IntervalMs
  }

  if ($i % 100 -eq 0) {
    Write-Host "Generated $i/$Count"
  }
}

$elapsed = (Get-Date) - $start
Write-Host ("Done generated {0} files in {1:n2}s" -f $Count, $elapsed.TotalSeconds)
exit 0
