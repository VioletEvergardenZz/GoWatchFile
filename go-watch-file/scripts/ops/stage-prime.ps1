# 鏈剼鏈敤浜庨樁娈靛鐩樺墠缃噯澶囥€?# 鑷姩瀹屾垚鐭ヨ瘑搴撳鍏ュ彂甯冧笌 AI 鍥炴斁鏍锋湰鍑嗗锛屽噺灏戞墜宸ョ幆澧冨亸宸€?
param(
  [string]$BaseUrl = "http://localhost:8082",
  [string]$DocsPath = "../docs",
  [string]$Operator = "stage-prime",
  [string]$ApproveImported = "true",
  [string]$AIPathsFile = "../reports/ai-replay-paths-prime.txt",
  [string]$AISamplesDir = "../reports/ai-replay-samples",
  [string]$UpdateAlertLogPaths = "true",
  [string]$OutputFile = "../reports/stage-prime-result.json"
)

function Ensure-Dir {
  param([string]$Path)
  if (-not [string]::IsNullOrWhiteSpace($Path) -and -not (Test-Path $Path)) {
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
  }
}

function Resolve-AnyPath {
  param([string]$Candidate)
  if ([string]::IsNullOrWhiteSpace($Candidate)) {
    return ""
  }
  if ([System.IO.Path]::IsPathRooted($Candidate)) {
    return $Candidate
  }
  return (Join-Path (Get-Location).Path $Candidate)
}

function Parse-BoolValue {
  param(
    $Raw,
    [bool]$Default = $false
  )
  if ($Raw -is [bool]) {
    return [bool]$Raw
  }
  $text = ([string]$Raw).Trim().ToLower()
  switch -Regex ($text) {
    "^(1|true|yes|y|on)$" { return $true }
    "^(0|false|no|n|off)$" { return $false }
    default { return $Default }
  }
}

function Split-LogPaths {
  param([string]$Raw)
  if ([string]::IsNullOrWhiteSpace($Raw)) {
    return @()
  }
  $parts = $Raw -split "[,;`r`n`t ]+"
  $out = @()
  foreach ($part in $parts) {
    $trimmed = $part.Trim()
    if (-not [string]::IsNullOrWhiteSpace($trimmed)) {
      $out += $trimmed
    }
  }
  return $out
}

function Merge-UniquePaths {
  param(
    [string[]]$First,
    [string[]]$Second
  )
  $set = New-Object System.Collections.Generic.HashSet[string]([System.StringComparer]::OrdinalIgnoreCase)
  $out = @()
  foreach ($item in @($First + $Second)) {
    if ([string]::IsNullOrWhiteSpace($item)) {
      continue
    }
    $normalized = [System.IO.Path]::GetFullPath($item)
    if ($set.Add($normalized)) {
      $out += $normalized
    }
  }
  return $out
}

function Invoke-JsonApi {
  param(
    [string]$Method,
    [string]$Url,
    $Body
  )
  $headers = @{
    "Content-Type" = "application/json"
  }
  if ($null -eq $Body) {
    return Invoke-RestMethod -Uri $Url -Method $Method -Headers $headers -TimeoutSec 20
  }
  $json = $Body | ConvertTo-Json -Depth 8
  return Invoke-RestMethod -Uri $Url -Method $Method -Headers $headers -Body $json -TimeoutSec 20
}

$base = $BaseUrl.TrimEnd("/")
$approveImportedFlag = Parse-BoolValue -Raw $ApproveImported -Default $true
$updateAlertLogPathsFlag = Parse-BoolValue -Raw $UpdateAlertLogPaths -Default $true
$docsAbs = [System.IO.Path]::GetFullPath((Resolve-AnyPath -Candidate $DocsPath))
$samplesAbs = [System.IO.Path]::GetFullPath((Resolve-AnyPath -Candidate $AISamplesDir))
$pathsFileAbs = [System.IO.Path]::GetFullPath((Resolve-AnyPath -Candidate $AIPathsFile))

$result = [ordered]@{
  generatedAt = (Get-Date).ToString("s")
  baseUrl     = $base
  operator    = $Operator
  ai          = [ordered]@{
    pathsFile      = $pathsFileAbs
    samplesDir     = $samplesAbs
    sampleCount    = 0
    alertConfigSet = $false
    logPaths       = @()
  }
  kb          = [ordered]@{
    docsPath       = $docsAbs
    importOk       = $false
    imported       = 0
    updated        = 0
    skipped        = 0
    approved       = 0
    approveScanned = 0
  }
  warnings    = @()
  allPassed   = $true
}

try {
  Ensure-Dir $samplesAbs
  $sampleFiles = @(
    [System.IO.Path]::Combine($samplesAbs, "backend-error.log"),
    [System.IO.Path]::Combine($samplesAbs, "upload-worker.log"),
    [System.IO.Path]::Combine($samplesAbs, "alert-engine.log")
  )
  $now = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss")
  $backendContent = @(
    "$now ERROR api: connection timeout to upstream service",
    "$now WARN api: fallback to cached response",
    "$now INFO api: request trace completed"
  ) -join [Environment]::NewLine
  $uploadContent = @(
    "$now ERROR upload: oss endpoint unreachable",
    "$now WARN upload: retry attempt=1",
    "$now INFO upload: queue length=12"
  ) -join [Environment]::NewLine
  $alertContent = @(
    "$now ERROR alert: rule evaluation failed",
    "$now WARN alert: suppress window active",
    "$now INFO alert: next poll in 2s"
  ) -join [Environment]::NewLine
  $contents = @($backendContent, $uploadContent, $alertContent)

  for ($i = 0; $i -lt $sampleFiles.Count; $i++) {
    Set-Content -Path $sampleFiles[$i] -Encoding UTF8 -Value $contents[$i]
  }

  $pathsDir = Split-Path -Parent $pathsFileAbs
  Ensure-Dir $pathsDir
  Set-Content -Path $pathsFileAbs -Encoding UTF8 -Value ($sampleFiles -join [Environment]::NewLine)

  $result.ai.sampleCount = $sampleFiles.Count
  $result.ai.logPaths = $sampleFiles
  Write-Host ("AI 鍥炴斁鏍锋湰宸茬敓鎴? {0}" -f $pathsFileAbs)

  if ($updateAlertLogPathsFlag) {
    $cfgResp = Invoke-JsonApi -Method "Get" -Url ("{0}/api/alert-config" -f $base) -Body $null
    $cfg = $null
    if ($null -ne $cfgResp -and $cfgResp.ok -and $null -ne $cfgResp.config) {
      $cfg = $cfgResp.config
    } else {
      throw "璇诲彇 alert-config 澶辫触"
    }
    $existing = Split-LogPaths -Raw ([string]$cfg.logPaths)
    $merged = Merge-UniquePaths -First $existing -Second $sampleFiles
    $payload = [ordered]@{
      enabled         = [bool]$cfg.enabled
      suppressEnabled = [bool]$cfg.suppressEnabled
      rulesFile       = [string]$cfg.rulesFile
      logPaths        = ($merged -join ";")
      pollInterval    = [string]$cfg.pollInterval
      startFromEnd    = [bool]$cfg.startFromEnd
    }
    $updateResp = Invoke-JsonApi -Method "Post" -Url ("{0}/api/alert-config" -f $base) -Body $payload
    if ($null -eq $updateResp -or -not $updateResp.ok) {
      throw "鏇存柊 alert-config 鐨?logPaths 澶辫触"
    }
    $result.ai.alertConfigSet = $true
    Write-Host ("鍛婅鏃ュ織璺緞宸叉洿鏂帮紝鏂板鏍锋湰鏂囦欢: {0}" -f $sampleFiles.Count)
  }
} catch {
  $msg = ("AI 鏍锋湰鍑嗗澶辫触: {0}" -f $_.Exception.Message)
  Write-Warning $msg
  $result.warnings += $msg
  $result.allPassed = $false
}

try {
  if (-not (Test-Path $docsAbs)) {
    throw ("DocsPath 涓嶅瓨鍦? {0}" -f $docsAbs)
  }
  $importPayload = [ordered]@{
    path     = $docsAbs
    operator = $Operator
  }
  $importResp = Invoke-JsonApi -Method "Post" -Url ("{0}/api/kb/import/docs" -f $base) -Body $importPayload
  if ($null -eq $importResp -or -not $importResp.ok -or $null -eq $importResp.result) {
    throw "知识库导入失败"
  }
  $result.kb.importOk = $true
  $result.kb.imported = [int]$importResp.result.imported
  $result.kb.updated = [int]$importResp.result.updated
  $result.kb.skipped = [int]$importResp.result.skipped
  Write-Host ("鐭ヨ瘑搴撳鍏ュ畬鎴? imported={0} updated={1} skipped={2}" -f $result.kb.imported, $result.kb.updated, $result.kb.skipped)

  if ($approveImportedFlag) {
    $approved = 0
    $scanned = 0
    for ($round = 0; $round -lt 20; $round++) {
      $listUrl = "{0}/api/kb/articles?status=draft`&page=1`&pageSize=200" -f $base
      $listResp = Invoke-JsonApi -Method "Get" -Url $listUrl -Body $null
      if ($null -eq $listResp -or -not $listResp.ok -or $null -eq $listResp.items -or $listResp.items.Count -eq 0) {
        break
      }
      $matched = @()
      foreach ($item in $listResp.items) {
        $scanned++
        $createdBy = [string]$item.createdBy
        $updatedBy = [string]$item.updatedBy
        if ($createdBy -eq $Operator -or $updatedBy -eq $Operator) {
          $matched += $item
        }
      }
      if ($matched.Count -eq 0) {
        break
      }
      foreach ($item in $matched) {
        $approveUrl = "{0}/api/kb/articles/{1}/approve" -f $base, [string]$item.id
        $approvePayload = [ordered]@{
          operator = $Operator
          comment  = "auto approve imported docs for stage recap"
        }
        $approveResp = Invoke-JsonApi -Method "Post" -Url $approveUrl -Body $approvePayload
        if ($null -ne $approveResp -and $approveResp.ok) {
          $approved++
        }
      }
    }
    $result.kb.approved = $approved
    $result.kb.approveScanned = $scanned
    Write-Host ("鐭ヨ瘑搴撹嚜鍔ㄥ彂甯冨畬鎴? approved={0}" -f $approved)
  }
} catch {
  $msg = ("鐭ヨ瘑搴撳噯澶囧け璐? {0}" -f $_.Exception.Message)
  Write-Warning $msg
  $result.warnings += $msg
  $result.allPassed = $false
}

$outDir = Split-Path -Parent $OutputFile
Ensure-Dir $outDir
$result | ConvertTo-Json -Depth 10 | Set-Content -Path $OutputFile -Encoding UTF8

Write-Host ("闃舵棰勫瀹屾垚 allPassed={0}" -f $result.allPassed)
Write-Host ("缁撴灉鏂囦欢: {0}" -f $OutputFile)

if (-not [bool]$result.allPassed) {
  exit 3
}
exit 0
