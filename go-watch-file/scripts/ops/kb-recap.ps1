# 本脚本用于知识库门禁复盘汇总
# 一次执行完成 hitrate / citation / mttd 三项评估并输出统一 JSON 报告

param(
  [string]$BaseUrl = "http://localhost:8082",
  [Parameter(Mandatory = $true)]
  [string]$Token,
  [string]$SamplesFile = "../docs/04-知识库/知识库命中率样本.json",
  [string]$MttdFile = "../docs/04-知识库/知识库MTTD基线.csv",
  [double]$CitationTarget = 1.0,
  [string]$OutputFile = "../reports/kb-recap-result.json"
)

# 统一封装评估命令调用：
# - 保留标准输出/错误输出，便于失败后直接定位门禁项
# - 不在函数内部抛异常，统一由返回结构和调用方决定退出码
function Invoke-EvalCommand {
  param(
    [string]$Name,
    [string[]]$Args
  )

  Write-Host ("执行 {0}: go {1}" -f $Name, ($Args -join " "))

  $output = & go @Args 2>&1
  $exitCode = $LASTEXITCODE
  # 输出统一转文本，避免后续写 JSON 时出现对象序列化差异
  $text = ($output | Out-String).Trim()

  return [pscustomobject]@{
    name     = $Name
    ok       = ($exitCode -eq 0)
    exitCode = $exitCode
    output   = $text
  }
}

# 先做输入校验，避免进入评估后才在深层命令处失败
if (-not (Test-Path $SamplesFile)) {
  Write-Error "样本文件不存在: $SamplesFile"
  exit 2
}
if (-not (Test-Path $MttdFile)) {
  Write-Error "MTTD 文件不存在: $MttdFile"
  exit 2
}

# 统一规范 base URL，避免拼接 endpoint 时出现双斜杠
$base = $BaseUrl.TrimEnd("/")

# 命中率评估采用更高检索上限，尽量暴露召回不足问题
$hitrate = Invoke-EvalCommand -Name "hitrate" -Args @(
  "run", "./cmd/kb-eval", "hitrate",
  "-base", $base,
  "-token", $Token,
  "-samples", $SamplesFile,
  "-limit", "5"
)

# 引用率评估限制返回条数，重点检查引用命中而非长答案稳定性
$citation = Invoke-EvalCommand -Name "citation" -Args @(
  "run", "./cmd/kb-eval", "citation",
  "-base", $base,
  "-token", $Token,
  "-samples", $SamplesFile,
  "-limit", "3",
  "-target", ([string]$CitationTarget)
)

# MTTD 仅依赖本地输入文件，和在线服务解耦，便于离线复盘
$mttd = Invoke-EvalCommand -Name "mttd" -Args @(
  "run", "./cmd/kb-eval", "mttd",
  "-input", $MttdFile
)

# 汇总结构固定字段，便于阶段脚本和控制台做机器可读解析
$summary = [pscustomobject]@{
  baseUrl         = $base
  samplesFile     = $SamplesFile
  mttdFile        = $MttdFile
  citationTarget  = $CitationTarget
  generatedAt     = (Get-Date).ToString("s")
  hitrate         = $hitrate
  citation        = $citation
  mttd            = $mttd
  allPassed       = ($hitrate.ok -and $citation.ok -and $mttd.ok)
}

# 输出目录不存在则自动创建，减少 CI/本地执行时的前置依赖
$outDir = Split-Path -Parent $OutputFile
if ($outDir -and -not (Test-Path $outDir)) {
  New-Item -ItemType Directory -Path $outDir -Force | Out-Null
}
$summary | ConvertTo-Json -Depth 8 | Set-Content -Path $OutputFile -Encoding utf8

Write-Host ("复盘完成: hitrate={0} citation={1} mttd={2} allPassed={3}" -f $hitrate.ok, $citation.ok, $mttd.ok, $summary.allPassed)
Write-Host "结果文件: $OutputFile"

# 退出码约定：
# 0=全部通过，2=输入参数/文件问题，3=至少一个门禁失败
if (-not $summary.allPassed) {
  exit 3
}
exit 0

