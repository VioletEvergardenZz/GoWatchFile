#!/usr/bin/env bash
set -euo pipefail

# AI 周度复盘（macOS/Linux）：
# 1) 批量调用 /api/ai/log-summary
# 2) 生成 ai-replay-YYYY-MM-DD.json
# 3) 基于回放结果生成 ai-baseline-YYYY-MM-DD.json
#
# 设计边界：
# - 仅依赖 curl + jq，避免 PowerShell 依赖
# - 请求失败/解析失败按降级样本计入，确保批处理不中断
# - 默认启用门禁失败退出（exit 3），便于 CI/CD 直接使用

usage() {
  cat <<'EOF'
用法:
  scripts/ops/ai-weekly-recap.sh \
    --base-url http://localhost:8080 \
    --paths-file ../reports/ai-replay-paths-2026-02-24.txt \
    --date 2026-02-24

参数:
  --base-url URL           API 地址，默认 http://localhost:8082
  --paths-file FILE        路径清单（必填，每行一个日志绝对路径）
  --date YYYY-MM-DD        产物日期标签，默认当天
  --out-dir DIR            输出目录，默认 ../reports
  --limit N                每条日志最大读取行数，默认 200
  --degraded-target R      降级率阈值，默认 0.20
  --structure-target R     结构通过率阈值，默认 1.00
  --errorclass-target R    错误分类覆盖率阈值，默认 1.00
  --allow-gate-fail        门禁失败时也返回 0
  -h, --help               显示帮助
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少依赖命令: $1" >&2
    exit 2
  fi
}

trim_line() {
  local s="$1"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf '%s' "$s"
}

BASE_URL="http://localhost:8082"
PATHS_FILE=""
DATE_TAG="$(date +%F)"
OUT_DIR="../reports"
LIMIT=200
DEGRADED_TARGET="0.20"
STRUCTURE_TARGET="1.00"
ERRORCLASS_TARGET="1.00"
FAIL_ON_GATE=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)
      BASE_URL="${2:-}"; shift 2 ;;
    --paths-file)
      PATHS_FILE="${2:-}"; shift 2 ;;
    --date)
      DATE_TAG="${2:-}"; shift 2 ;;
    --out-dir)
      OUT_DIR="${2:-}"; shift 2 ;;
    --limit)
      LIMIT="${2:-}"; shift 2 ;;
    --degraded-target)
      DEGRADED_TARGET="${2:-}"; shift 2 ;;
    --structure-target)
      STRUCTURE_TARGET="${2:-}"; shift 2 ;;
    --errorclass-target)
      ERRORCLASS_TARGET="${2:-}"; shift 2 ;;
    --allow-gate-fail)
      FAIL_ON_GATE=0; shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 2 ;;
  esac
done

require_cmd curl
require_cmd jq

if [[ -z "$PATHS_FILE" ]]; then
  echo "--paths-file 为必填参数" >&2
  exit 2
fi
if [[ ! -f "$PATHS_FILE" ]]; then
  echo "路径清单不存在: $PATHS_FILE" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"

ROWS_FILE="${OUT_DIR}/ai-replay-${DATE_TAG}.rows.jsonl"
REPLAY_FILE="${OUT_DIR}/ai-replay-${DATE_TAG}.json"
BASELINE_FILE="${OUT_DIR}/ai-baseline-${DATE_TAG}.json"

: >"$ROWS_FILE"

row_filter_file="$(mktemp)"
agg_filter_file="$(mktemp)"
baseline_filter_file="$(mktemp)"
trap 'rm -f "$row_filter_file" "$agg_filter_file" "$baseline_filter_file"' EXIT

cat >"$row_filter_file" <<'JQ'
def normalize_severity:
  (. // "" | ascii_downcase) as $v
  | if ($v=="low" or $v=="medium" or $v=="high") then $v else "unknown" end;

def arr_or_null:
  if . == null then null
  elif type=="array" then .
  else [.] end;

. as $resp
| ($resp.analysis // null) as $a
| ($a == null) as $analysisMissing
| (if ($a != null and ($a|has("suggestions"))) then ($a.suggestions|arr_or_null) else null end) as $suggestions
| (if ($a != null and ($a|has("causes"))) then ($a.causes|arr_or_null) else null end) as $causes
| (if ($a != null and ($a|has("keyErrors"))) then ($a.keyErrors|arr_or_null) else null end) as $keyErrors
| ($a.confidence // null) as $confidence
| [
    (if $analysisMissing then "analysis_missing" else empty end),
    (if (($a.summary // "") | length) == 0 then "summary_missing" else empty end),
    (if (($a.severity|normalize_severity) == "unknown") then "severity_invalid" else empty end),
    (if $suggestions == null then "suggestions_missing" else empty end),
    (if ($suggestions != null and ($suggestions|length)==0) then "suggestions_empty" else empty end),
    (if ($suggestions != null and ($suggestions|length)>3) then "suggestions_over_limit" else empty end),
    (if $causes == null then "causes_missing" else empty end),
    (if ($causes != null and ($causes|length)>3) then "causes_over_limit" else empty end),
    (if $keyErrors == null then "keyErrors_missing" else empty end),
    (if ($keyErrors != null and ($keyErrors|length)>5) then "keyErrors_over_limit" else empty end),
    (if ($confidence != null and ($confidence|type)!="number") then "confidence_invalid" else empty end),
    (if ($confidence != null and ($confidence|type)=="number" and ($confidence < 0 or $confidence > 1)) then "confidence_out_of_range" else empty end)
  ] as $issues
| {
    path: $log_path,
    ok: ($resp.ok // false),
    degraded: ($resp.meta.degraded // false),
    errorClass: ($resp.meta.errorClass // ""),
    elapsedMs: ($resp.meta.elapsedMs // 0),
    structureChecked: true,
    structureOK: (($issues|length) == 0),
    structureIssues: $issues,
    severity: ($a.severity|normalize_severity),
    suggestionsCount: (if $suggestions==null then 0 else ($suggestions|length) end),
    causesCount: (if $causes==null then 0 else ($causes|length) end),
    keyErrorsCount: (if $keyErrors==null then 0 else ($keyErrors|length) end),
    confidence: $confidence,
    requestFailed: false
  }
| if .degraded == true and ((.errorClass|length) == 0) then .errorClass = "unknown" else . end
JQ

cat >"$agg_filter_file" <<'JQ'
def ratio4($num; $den):
  if $den == 0 then 0
  else (($num / $den * 10000) | floor) / 10000
  end;

def map_count($arr):
  reduce $arr[] as $k ({}; .[$k] = ((.[$k] // 0) + 1));

def allowed_error_class:
  ["timeout","network","rate_limit","auth","upstream_5xx","upstream_4xx","parse_error","request_error","unknown"];

def is_allowed_error_class($v):
  (allowed_error_class | index($v)) != null;

. as $rows
| ($rows | length) as $total
| ($rows | map(select(.degraded == true)) | length) as $degraded
| ($rows | map(select(.requestFailed == true)) | length) as $requestFailed
| ($rows | map(select(.degraded != true)) | length) as $success
| ($rows | map(select(.structureChecked == true)) | length) as $structureChecked
| ($rows | map(select(.structureChecked == true and .structureOK == true)) | length) as $structurePassed
| ($structureChecked - $structurePassed) as $structureFailed
| ($rows | map(select(.degraded == true) | (.errorClass // "" | if length == 0 then "unknown" else . end))) as $degradedErrorClasses
| ($degradedErrorClasses | length) as $classifiedDegraded
| ($degradedErrorClasses | map(select(is_allowed_error_class(.)))) as $knownErrorClasses
| ($knownErrorClasses | length) as $knownErrorClassDegraded
| ($rows | map(select(.structureChecked == true) | .severity)) as $severityRows
| (ratio4($degraded; $total)) as $degradedRatio
| (ratio4($structurePassed; $structureChecked)) as $structurePassRatio
| (if $degraded == 0 then 1 else ratio4($classifiedDegraded; $degraded) end) as $errorClassCoverage
| (if $degraded == 0 then 1 else ratio4($knownErrorClassDegraded; $degraded) end) as $knownErrorClassRatio
| {
    total: $total,
    success: $success,
    degraded: $degraded,
    requestFailed: $requestFailed,
    degradedRatio: $degradedRatio,
    structure: {
      checked: $structureChecked,
      passed: $structurePassed,
      failed: $structureFailed,
      passRatio: $structurePassRatio,
      issues: map_count([$rows[] | select(.structureChecked == true and .structureOK != true) | .structureIssues[]?])
    },
    severity: {
      low: ($severityRows | map(select(. == "low")) | length),
      medium: ($severityRows | map(select(. == "medium")) | length),
      high: ($severityRows | map(select(. == "high")) | length),
      unknown: ($severityRows | map(select(. == "unknown")) | length)
    },
    errorClass: map_count($degradedErrorClasses),
    errorClassCoverage: $errorClassCoverage,
    knownErrorClassRatio: $knownErrorClassRatio,
    gateTargets: {
      degradedRatio: $degradedTarget,
      structurePassRatio: $structureTarget,
      errorClassCoverage: $errorclassTarget
    },
    gates: {
      degradedRatioPass: ($degradedRatio <= $degradedTarget),
      structurePassRatioPass: ($structurePassRatio >= $structureTarget),
      errorClassCoveragePass: ($errorClassCoverage >= $errorclassTarget),
      allPassed: false
    },
    results: $rows
  }
| .gates.allPassed = (.gates.degradedRatioPass and .gates.structurePassRatioPass and .gates.errorClassCoveragePass)
JQ

cat >"$baseline_filter_file" <<'JQ'
def ratio4($num; $den):
  if $den == 0 then 0
  else (($num / $den * 10000) | floor) / 10000
  end;

. as $r
| ($r.structure // {}) as $s
| ($s.issues // {}) as $issues
| (($s.checked // ($r.results|length) // $r.total // 0)) as $checked
| ($checked | tonumber) as $structureChecked
| ($issues.summary_missing // 0) as $summaryIssueCount
| ($issues.severity_invalid // 0) as $severityIssueCount
| (($issues.suggestions_missing // 0) + ($issues.suggestions_empty // 0) + ($issues.suggestions_over_limit // 0)) as $suggestionsIssueCount
| (if $structureChecked > 0 then ratio4(($structureChecked - (if $summaryIssueCount > $structureChecked then $structureChecked else $summaryIssueCount end)); $structureChecked) else 0 end) as $summaryPassRatio
| (if $structureChecked > 0 then ratio4(($structureChecked - (if $severityIssueCount > $structureChecked then $structureChecked else $severityIssueCount end)); $structureChecked) else 0 end) as $severityPassRatio
| (if $structureChecked > 0 then ratio4(($structureChecked - (if $suggestionsIssueCount > $structureChecked then $structureChecked else $suggestionsIssueCount end)); $structureChecked) else 0 end) as $suggestionsPassRatio
| (($r.total // 0) > 0) as $totalPass
| (($r.gates // {}) ) as $replayGates
| ($replayGates.degradedRatioPass // (($r.degradedRatio // 0) <= 0.2)) as $replayDegradedPass
| ($replayGates.structurePassRatioPass // (($s.passRatio // 0) >= 1)) as $replayStructurePass
| ($replayGates.errorClassCoveragePass // (($r.errorClassCoverage // 0) >= 1)) as $replayErrorClassPass
| (($replayDegradedPass and $replayStructurePass and $replayErrorClassPass)) as $replayAllPassed
| (($summaryPassRatio >= 1) and ($severityPassRatio >= 1) and ($suggestionsPassRatio >= 1)) as $baselineAllPassed
| {
    generatedAt: $generatedAt,
    mode: "offline",
    baseUrl: $baseUrl,
    artifacts: {
      pathsFile: $pathsFile,
      aiReplayResult: $replayFile,
      primeResult: ""
    },
    replay: {
      total: ($r.total // 0),
      success: ($r.success // 0),
      degraded: ($r.degraded // 0),
      degradedRatio: ($r.degradedRatio // 0),
      structurePassRatio: ($s.passRatio // 0),
      errorClassCoverage: ($r.errorClassCoverage // 1)
    },
    analysis: {
      structureEvidenceReady: true,
      structureChecked: $structureChecked,
      summaryIssueCount: $summaryIssueCount,
      severityIssueCount: $severityIssueCount,
      suggestionsIssueCount: $suggestionsIssueCount,
      summaryPassRatio: $summaryPassRatio,
      severityPassRatio: $severityPassRatio,
      suggestionsPassRatio: $suggestionsPassRatio
    },
    targets: {
      degradedRatio: 0.2,
      structurePassRatio: 1,
      errorClassCoverage: 1,
      summaryPassRatio: 1,
      severityPassRatio: 1,
      suggestionsPassRatio: 1
    },
    execution: {
      primeExitCode: 0,
      primeOutput: "",
      replayExitCode: 0,
      replayOutput: ""
    },
    gates: {
      replay: {
        degradedRatioPass: $replayDegradedPass,
        structurePassRatioPass: $replayStructurePass,
        errorClassCoveragePass: $replayErrorClassPass,
        allPassed: $replayAllPassed
      },
      baseline: {
        summaryPass: ($summaryPassRatio >= 1),
        severityPass: ($severityPassRatio >= 1),
        suggestionsPass: ($suggestionsPassRatio >= 1),
        allPassed: $baselineAllPassed
      },
      totalPass: $totalPass,
      allPassed: ($replayAllPassed and $baselineAllPassed and $totalPass)
    },
    notes: [],
    prime: null
  }
JQ

declare -a LOG_PATHS=()
while IFS= read -r raw; do
  line="$(trim_line "$raw")"
  [[ -z "$line" ]] && continue
  [[ "$line" == \#* ]] && continue
  LOG_PATHS+=("$line")
done <"$PATHS_FILE"

if [[ "${#LOG_PATHS[@]}" -eq 0 ]]; then
  echo "路径清单为空: $PATHS_FILE" >&2
  exit 2
fi

echo "AI 周度回放开始: total=${#LOG_PATHS[@]} baseUrl=$BASE_URL"

for log_path in "${LOG_PATHS[@]}"; do
  echo "replay => $log_path"
  payload="$(jq -nc --arg p "$log_path" --argjson l "$LIMIT" '{path:$p,mode:"tail",limit:$l,caseSensitive:false}')"
  tmp_resp="$(mktemp)"
  http_code="000"
  if http_code="$(curl -sS -m 60 -o "$tmp_resp" -w '%{http_code}' \
    -X POST "${BASE_URL%/}/api/ai/log-summary" \
    -H 'Content-Type: application/json' \
    -d "$payload")"; then
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
      if ! jq -c --arg log_path "$log_path" -f "$row_filter_file" "$tmp_resp" >>"$ROWS_FILE"; then
        # 解析失败归类为 parse_error，避免单条坏样本中断整批复盘
        jq -nc --arg log_path "$log_path" --argjson code "$http_code" '{
          path:$log_path,
          ok:false,
          degraded:true,
          errorClass:"parse_error",
          elapsedMs:0,
          structureChecked:false,
          structureOK:false,
          structureIssues:["parse_error"],
          severity:"unknown",
          suggestionsCount:0,
          causesCount:0,
          keyErrorsCount:0,
          confidence:null,
          requestFailed:false,
          note:("parse_failed_http_" + ($code|tostring))
        }' >>"$ROWS_FILE"
      fi
    else
      jq -nc --arg log_path "$log_path" --argjson code "$http_code" '{
        path:$log_path,
        ok:false,
        degraded:true,
        errorClass:"request_error",
        elapsedMs:0,
        structureChecked:false,
        structureOK:false,
        structureIssues:["request_error"],
        severity:"unknown",
        suggestionsCount:0,
        causesCount:0,
        keyErrorsCount:0,
        confidence:null,
        requestFailed:true,
        note:("http_status_" + ($code|tostring))
      }' >>"$ROWS_FILE"
    fi
  else
    jq -nc --arg log_path "$log_path" '{
      path:$log_path,
      ok:false,
      degraded:true,
      errorClass:"request_error",
      elapsedMs:0,
      structureChecked:false,
      structureOK:false,
      structureIssues:["request_error"],
      severity:"unknown",
      suggestionsCount:0,
      causesCount:0,
      keyErrorsCount:0,
      confidence:null,
      requestFailed:true
    }' >>"$ROWS_FILE"
  fi
  rm -f "$tmp_resp"
done

jq -s \
  --argjson degradedTarget "$DEGRADED_TARGET" \
  --argjson structureTarget "$STRUCTURE_TARGET" \
  --argjson errorclassTarget "$ERRORCLASS_TARGET" \
  -f "$agg_filter_file" \
  "$ROWS_FILE" >"$REPLAY_FILE"

jq \
  --arg generatedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg baseUrl "$BASE_URL" \
  --arg pathsFile "$PATHS_FILE" \
  --arg replayFile "$REPLAY_FILE" \
  -f "$baseline_filter_file" \
  "$REPLAY_FILE" >"$BASELINE_FILE"

echo "回放结果: $REPLAY_FILE"
echo "基线结果: $BASELINE_FILE"
jq '{total,success,degraded,degradedRatio,gates}' "$REPLAY_FILE"
jq '{analysis,gates}' "$BASELINE_FILE"

all_passed="$(jq -r '.gates.allPassed' "$BASELINE_FILE")"
if [[ "$FAIL_ON_GATE" -eq 1 && "$all_passed" != "true" ]]; then
  exit 3
fi

exit 0
