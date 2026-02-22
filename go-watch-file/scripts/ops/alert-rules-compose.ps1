# Compose alert rules from a layered template:
# - defaultRules: baseline rules
# - scenarios: optional scenario rule sets selected by id
# Output is a runtime-compatible alert ruleset payload.

param(
  [string]$TemplateFile = "../deploy/alert/gwf-alert-rules-layered-template.json",
  [string]$Scenarios = "",
  [switch]$IncludeAllScenarios = $false,
  [string]$OutputFile = "../reports/alert-rules-composed.json",
  [switch]$Apply = $false,
  [string]$BaseUrl = "http://localhost:8082"
)

function Ensure-Dir {
  param([string]$Path)
  if (-not [string]::IsNullOrWhiteSpace($Path) -and -not (Test-Path $Path)) {
    New-Item -Path $Path -ItemType Directory -Force | Out-Null
  }
}

function Split-IDs {
  param([string]$Raw)
  if ([string]::IsNullOrWhiteSpace($Raw)) {
    return @()
  }
  $parts = $Raw -split '[,;，；\s]+' | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne "" }
  $seen = @{}
  $out = @()
  foreach ($part in $parts) {
    $key = $part.ToLowerInvariant()
    if ($seen.ContainsKey($key)) {
      continue
    }
    $seen[$key] = $true
    $out += $part
  }
  return $out
}

function Normalize-Rule {
  param(
    [object]$Rule,
    [string]$Source
  )

  if ($null -eq $Rule) {
    throw "null rule from $Source"
  }

  $ruleID = [string]$Rule.id
  $title = [string]$Rule.title
  $level = [string]$Rule.level
  $keywords = @()
  if ($null -ne $Rule.keywords) {
    $keywords = @($Rule.keywords | ForEach-Object { [string]$_ })
  }
  $keywords = @($keywords | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne "" })

  if ($keywords.Count -eq 0) {
    throw "rule '$ruleID' from $Source has empty keywords"
  }
  if ([string]::IsNullOrWhiteSpace($level)) {
    throw "rule '$ruleID' from $Source has empty level"
  }

  $normalized = [ordered]@{
    id       = $ruleID.Trim()
    title    = $title.Trim()
    level    = $level.Trim().ToLowerInvariant()
    keywords = $keywords
  }

  $excludes = @()
  if ($null -ne $Rule.excludes) {
    $excludes = @($Rule.excludes | ForEach-Object { [string]$_ })
    $excludes = @($excludes | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne "" })
    if ($excludes.Count -gt 0) {
      $normalized.excludes = $excludes
    }
  }

  $suppressWindow = [string]$Rule.suppress_window
  if (-not [string]::IsNullOrWhiteSpace($suppressWindow)) {
    $normalized.suppress_window = $suppressWindow.Trim()
  }

  if ($null -ne $Rule.match_case) {
    $normalized.match_case = [bool]$Rule.match_case
  }
  if ($null -ne $Rule.notify) {
    $normalized.notify = [bool]$Rule.notify
  }

  return [pscustomobject]$normalized
}

Write-Host ("Compose alert rules from template: {0}" -f $TemplateFile)

if (-not (Test-Path $TemplateFile)) {
  Write-Error ("template file not found: {0}" -f $TemplateFile)
  exit 2
}

try {
  $raw = Get-Content -Path $TemplateFile -Raw -Encoding UTF8
  $template = $raw | ConvertFrom-Json
} catch {
  Write-Error ("failed to parse template JSON: {0}" -f $_.Exception.Message)
  exit 2
}

$defaultRules = @()
if ($null -ne $template.defaultRules) {
  $defaultRules = @($template.defaultRules)
}
if ($defaultRules.Count -eq 0) {
  Write-Error "template.defaultRules is required and cannot be empty"
  exit 2
}

$scenarioDefs = @()
if ($null -ne $template.scenarios) {
  $scenarioDefs = @($template.scenarios)
}

$selectedScenarioIDs = @()
if ($IncludeAllScenarios) {
  $selectedScenarioIDs = @($scenarioDefs | ForEach-Object { ([string]$_.id).Trim() } | Where-Object { $_ -ne "" })
} else {
  $selectedScenarioIDs = Split-IDs -Raw $Scenarios
}

$scenarioMap = @{}
foreach ($sc in $scenarioDefs) {
  $id = ([string]$sc.id).Trim()
  if ([string]::IsNullOrWhiteSpace($id)) {
    continue
  }
  $scenarioMap[$id.ToLowerInvariant()] = $sc
}

$missingScenarioIDs = @()
$composedRules = @()

foreach ($rule in $defaultRules) {
  try {
    $composedRules += Normalize-Rule -Rule $rule -Source "defaultRules"
  } catch {
    Write-Error ("invalid default rule: {0}" -f $_.Exception.Message)
    exit 2
  }
}

$loadedScenarioTitles = @()
foreach ($scenarioID in $selectedScenarioIDs) {
  $lookup = $scenarioID.ToLowerInvariant()
  if (-not $scenarioMap.ContainsKey($lookup)) {
    $missingScenarioIDs += $scenarioID
    continue
  }
  $scenario = $scenarioMap[$lookup]
  $scenarioTitle = ([string]$scenario.title).Trim()
  if ([string]::IsNullOrWhiteSpace($scenarioTitle)) {
    $scenarioTitle = $scenarioID
  }
  $loadedScenarioTitles += $scenarioTitle
  $rules = @()
  if ($null -ne $scenario.rules) {
    $rules = @($scenario.rules)
  }
  foreach ($rule in $rules) {
    try {
      $composedRules += Normalize-Rule -Rule $rule -Source ("scenario:" + $scenarioID)
    } catch {
      Write-Error ("invalid scenario rule ({0}): {1}" -f $scenarioID, $_.Exception.Message)
      exit 2
    }
  }
}

if ($missingScenarioIDs.Count -gt 0) {
  Write-Error ("unknown scenarios: {0}" -f ($missingScenarioIDs -join ", "))
  exit 3
}

$levelAllow = @("ignore", "business", "system", "fatal")
$ruleIDSeen = @{}
for ($i = 0; $i -lt $composedRules.Count; $i++) {
  $rule = $composedRules[$i]
  $ruleID = [string]$rule.id
  if ([string]::IsNullOrWhiteSpace($ruleID)) {
    $ruleID = "rule-{0}" -f ($i + 1)
    $rule.id = $ruleID
  }
  $ruleID = $ruleID.Trim()
  $rule.id = $ruleID
  $key = $ruleID.ToLowerInvariant()
  if ($ruleIDSeen.ContainsKey($key)) {
    Write-Error ("duplicate rule id: {0}" -f $ruleID)
    exit 2
  }
  $ruleIDSeen[$key] = $true
  if ([string]::IsNullOrWhiteSpace([string]$rule.title)) {
    $rule.title = $ruleID
  }
  if (-not ($levelAllow -contains ([string]$rule.level))) {
    Write-Error ("invalid rule level '{0}' in rule '{1}'" -f $rule.level, $ruleID)
    exit 2
  }
}

$version = 1
if ($null -ne $template.version -and [int]$template.version -gt 0) {
  $version = [int]$template.version
}

$defaults = $template.defaults
if ($null -eq $defaults) {
  $defaults = [ordered]@{
    suppress_window = "5m"
    match_case      = $false
  }
}

$escalation = $template.escalation
if ($null -eq $escalation) {
  $escalation = [ordered]@{
    enabled         = $false
    level           = "fatal"
    window          = "5m"
    threshold       = 0
    suppress_window = "5m"
    rule_id         = "system_spike"
    title           = "系统异常激增"
    message         = "系统异常达到阈值"
  }
}

$ruleset = [ordered]@{
  version    = $version
  defaults   = $defaults
  escalation = $escalation
  rules      = $composedRules
}

$outDir = Split-Path -Parent $OutputFile
Ensure-Dir $outDir
$ruleset | ConvertTo-Json -Depth 20 | Set-Content -Path $OutputFile -Encoding UTF8

Write-Host ("Composed ruleset: total={0} default={1} scenario={2}" -f `
  $composedRules.Count, $defaultRules.Count, ($composedRules.Count - $defaultRules.Count))
if ($loadedScenarioTitles.Count -gt 0) {
  Write-Host ("Scenarios: {0}" -f ($loadedScenarioTitles -join ", "))
} else {
  Write-Host "Scenarios: none"
}
Write-Host ("Output: {0}" -f $OutputFile)

if ($Apply) {
  $base = $BaseUrl.TrimEnd("/")
  $url = "{0}/api/alert-rules" -f $base
  $headers = @{
    "Content-Type" = "application/json"
  }
  $body = @{
    rules = $ruleset
  } | ConvertTo-Json -Depth 20

  try {
    $resp = Invoke-RestMethod -Uri $url -Method Post -Headers $headers -Body $body -TimeoutSec 20
    if ($null -eq $resp -or -not $resp.ok) {
      Write-Error ("apply failed: {0}" -f ($resp | ConvertTo-Json -Depth 8))
      exit 4
    }
    Write-Host ("Applied to API: {0}" -f $url)
  } catch {
    Write-Error ("apply failed: {0}" -f $_.Exception.Message)
    exit 4
  }
}

# Exit code:
# 0 = success
# 2 = template/compose validation failed
# 3 = unknown scenario id
# 4 = apply API failed
exit 0

