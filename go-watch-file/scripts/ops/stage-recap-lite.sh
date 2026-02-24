#!/usr/bin/env bash
set -euo pipefail

# 阶段轻量复盘（macOS/Linux）：
# 聚合 AI 周度复盘、知识库演练、控制台闭环三份 JSON 结果，输出统一 pass/fail 结论。

usage() {
  cat <<'EOF'
用法:
  scripts/ops/stage-recap-lite.sh \
    --ai-baseline ../reports/ai-baseline-2026-02-24.json \
    --kb-drill ../reports/kb-failure-rollback-drill-2026-02-24.json \
    --console-check ../reports/console-closure-check-2026-02-24.json \
    --output-file ../reports/stage-recap-2026-02-24-lite.json

参数:
  --ai-baseline FILE       AI 基线结果（必填）
  --kb-drill FILE          知识库失败与回滚演练结果（必填）
  --console-check FILE     控制台闭环验收结果（必填）
  --output-file FILE       输出文件，默认 ../reports/stage-recap-YYYY-MM-DD-lite.json
  --backend-tests-pass B   后端门禁是否通过（true/false，默认 true）
  --frontend-build-pass B  前端构建是否通过（true/false，默认 true）
  -h, --help               显示帮助
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少依赖命令: $1" >&2
    exit 2
  fi
}

to_json_bool() {
  local raw="$1"
  case "$raw" in
    true|TRUE|1) echo true ;;
    false|FALSE|0) echo false ;;
    *)
      echo "非法布尔值: $raw (允许 true/false/1/0)" >&2
      exit 2 ;;
  esac
}

require_cmd jq

AI_BASELINE=""
KB_DRILL=""
CONSOLE_CHECK=""
DATE_TAG="$(date +%F)"
OUTPUT_FILE=""
BACKEND_PASS="true"
FRONTEND_PASS="true"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --ai-baseline)
      AI_BASELINE="${2:-}"; shift 2 ;;
    --kb-drill)
      KB_DRILL="${2:-}"; shift 2 ;;
    --console-check)
      CONSOLE_CHECK="${2:-}"; shift 2 ;;
    --output-file)
      OUTPUT_FILE="${2:-}"; shift 2 ;;
    --backend-tests-pass)
      BACKEND_PASS="$(to_json_bool "${2:-}")"; shift 2 ;;
    --frontend-build-pass)
      FRONTEND_PASS="$(to_json_bool "${2:-}")"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 2 ;;
  esac
done

if [[ -z "$AI_BASELINE" || -z "$KB_DRILL" || -z "$CONSOLE_CHECK" ]]; then
  echo "--ai-baseline / --kb-drill / --console-check 均为必填参数" >&2
  exit 2
fi

if [[ ! -f "$AI_BASELINE" || ! -f "$KB_DRILL" || ! -f "$CONSOLE_CHECK" ]]; then
  echo "输入文件不存在，请检查参数路径" >&2
  exit 2
fi

if [[ -z "$OUTPUT_FILE" ]]; then
  OUTPUT_FILE="../reports/stage-recap-${DATE_TAG}-lite.json"
fi
mkdir -p "$(dirname "$OUTPUT_FILE")"

jq -n \
  --arg generatedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --argjson backendPass "$BACKEND_PASS" \
  --argjson frontendPass "$FRONTEND_PASS" \
  --slurpfile ai "$AI_BASELINE" \
  --slurpfile kb "$KB_DRILL" \
  --slurpfile cc "$CONSOLE_CHECK" \
  --arg aiFile "$AI_BASELINE" \
  --arg kbFile "$KB_DRILL" \
  --arg ccFile "$CONSOLE_CHECK" \
  '{
    generatedAt: $generatedAt,
    scope: "A类收口执行周（lite recap）",
    checks: {
      backendTests: {pass: $backendPass},
      frontendBuild: {pass: $frontendPass},
      aiWeeklyBaseline: {pass: ($ai[0].gates.allPassed // false)},
      kbFailureRollbackDrill: {pass: ($kb[0].pass // false)},
      consoleClosure: {pass: ($cc[0].pass // false)}
    },
    pass: (
      $backendPass and $frontendPass and
      ($ai[0].gates.allPassed // false) and
      ($kb[0].pass // false) and
      ($cc[0].pass // false)
    ),
    artifacts: {
      aiBaseline: $aiFile,
      kbDrill: $kbFile,
      consoleClosure: $ccFile
    }
  }' >"$OUTPUT_FILE"

echo "阶段复盘: $OUTPUT_FILE"
jq . "$OUTPUT_FILE"

if [[ "$(jq -r '.pass' "$OUTPUT_FILE")" != "true" ]]; then
  exit 3
fi

exit 0
