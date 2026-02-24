#!/usr/bin/env bash
set -euo pipefail

# 控制台闭环与降级验收（macOS/Linux）
# 核心目标：用固定 API 集合验证值班路径是否可用、降级接口是否保持 200 可读返回。

usage() {
  cat <<'EOF'
用法:
  scripts/ops/console-closure-check.sh \
    --base-url http://localhost:8080 \
    --log-path /abs/path/to/file-monitor.log \
    --output-file ../reports/console-closure-check-2026-02-24.json

参数:
  --base-url URL           API 地址，默认 http://localhost:8082
  --log-path FILE          /api/file-log 使用的日志文件路径（必填）
  --query TEXT             知识推荐 query，默认 回滚演练
  --output-file FILE       输出文件，默认 ../reports/console-closure-check-YYYY-MM-DD.json
  -h, --help               显示帮助
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少依赖命令: $1" >&2
    exit 2
  fi
}

http_code() {
  local method="$1"
  local url="$2"
  local body="${3:-}"
  local out_file="$4"
  if [[ -n "$body" ]]; then
    curl -sS -o "$out_file" -w '%{http_code}' -X "$method" \
      -H 'Content-Type: application/json' \
      -d "$body" \
      "$url"
  else
    curl -sS -o "$out_file" -w '%{http_code}' -X "$method" "$url"
  fi
}

require_cmd curl
require_cmd jq

BASE_URL="http://localhost:8082"
LOG_PATH=""
QUERY="回滚演练"
DATE_TAG="$(date +%F)"
OUTPUT_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)
      BASE_URL="${2:-}"; shift 2 ;;
    --log-path)
      LOG_PATH="${2:-}"; shift 2 ;;
    --query)
      QUERY="${2:-}"; shift 2 ;;
    --output-file)
      OUTPUT_FILE="${2:-}"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 2 ;;
  esac
done

if [[ -z "$LOG_PATH" ]]; then
  echo "--log-path 为必填参数" >&2
  exit 2
fi

if [[ -z "$OUTPUT_FILE" ]]; then
  OUTPUT_FILE="../reports/console-closure-check-${DATE_TAG}.json"
fi
mkdir -p "$(dirname "$OUTPUT_FILE")"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

dash_code="$(http_code GET "${BASE_URL%/}/api/dashboard" "" "$tmp_dir/dashboard.json" || true)"
dash_light_code="$(http_code GET "${BASE_URL%/}/api/dashboard?mode=light" "" "$tmp_dir/dashboard-light.json" || true)"
health_code="$(http_code GET "${BASE_URL%/}/api/health" "" "$tmp_dir/health.json" || true)"

filelog_payload="$(jq -nc --arg p "$LOG_PATH" '{path:$p,mode:"tail",limit:50,caseSensitive:false}')"
filelog_code="$(http_code POST "${BASE_URL%/}/api/file-log" "$filelog_payload" "$tmp_dir/filelog.json" || true)"
alerts_code="$(http_code GET "${BASE_URL%/}/api/alerts" "" "$tmp_dir/alerts.json" || true)"

query_escaped="$(jq -rn --arg q "$QUERY" '$q|@uri')"
kb_reco_code="$(http_code GET "${BASE_URL%/}/api/kb/recommendations?query=${query_escaped}&limit=3" "" "$tmp_dir/kb-reco.json" || true)"

alert_decisions="$(jq -r '.data.decisions | length // 0' "$tmp_dir/alerts.json" 2>/dev/null || echo 0)"
kb_reco_items="$(jq -r '.items | length // 0' "$tmp_dir/kb-reco.json" 2>/dev/null || echo 0)"

jq -n \
  --arg date "$DATE_TAG" \
  --arg baseUrl "$BASE_URL" \
  --argjson dash_code "${dash_code:-0}" \
  --argjson dash_light_code "${dash_light_code:-0}" \
  --argjson health_code "${health_code:-0}" \
  --argjson filelog_code "${filelog_code:-0}" \
  --argjson alerts_code "${alerts_code:-0}" \
  --argjson kb_reco_code "${kb_reco_code:-0}" \
  --argjson alert_decisions "${alert_decisions:-0}" \
  --argjson kb_reco_items "${kb_reco_items:-0}" \
  '{
    date: $date,
    baseUrl: $baseUrl,
    checks: {
      dashboard: {code: $dash_code, pass: ($dash_code == 200)},
      dashboardLight: {code: $dash_light_code, pass: ($dash_light_code == 200)},
      health: {code: $health_code, pass: ($health_code == 200)},
      fileLog: {code: $filelog_code, pass: ($filelog_code == 200)},
      alerts: {code: $alerts_code, decisions: $alert_decisions, pass: ($alerts_code == 200)},
      kbRecommendations: {code: $kb_reco_code, items: $kb_reco_items, pass: ($kb_reco_code == 200)}
    },
    pass: (
      ($dash_code == 200) and ($dash_light_code == 200) and ($health_code == 200) and
      ($filelog_code == 200) and ($alerts_code == 200) and ($kb_reco_code == 200)
    )
  }' >"$OUTPUT_FILE"

echo "验收结果: $OUTPUT_FILE"
jq . "$OUTPUT_FILE"

if [[ "$(jq -r '.pass' "$OUTPUT_FILE")" != "true" ]]; then
  exit 3
fi

exit 0
