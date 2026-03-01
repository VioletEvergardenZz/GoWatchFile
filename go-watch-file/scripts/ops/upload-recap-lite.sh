#!/usr/bin/env bash
set -euo pipefail

# 上传链路轻量复盘（macOS/Linux）：
# 1) 采集压测前后 metrics + health 快照
# 2) 执行队列压测
# 3) 输出统一 JSON/Markdown 结果，便于阶段收口归档

usage() {
  cat <<'EOF'
用法:
  scripts/ops/upload-recap-lite.sh \
    --base-url http://localhost:8082 \
    --watch-dir /tmp/gwf-stress \
    --output-file ../reports/upload-recap-2026-03-01.json

参数:
  --base-url URL                 API 地址，默认 http://localhost:8082
  --watch-dir DIR                压测目录（必填）
  --saturation-count N           压测文件数，默认 1200
  --saturation-interval-ms N     压测间隔毫秒，默认 5
  --min-bytes N                  单文件最小字节数，默认 1024
  --max-bytes N                  单文件最大字节数，默认 8192
  --stabilize-seconds N          压测后等待秒数，默认 8
  --max-failure-ratio R          失败率阈值（0~1），默认 0.05
  --max-queue-full-delta N       队列打满增量阈值，默认 0
  --require-queue-saturation     要求观察到队列饱和（queueFull/queueShed 增量 > 0）
  --output-file FILE             输出 JSON 文件，默认 ../reports/upload-recap-YYYY-MM-DD.json
  --report-file FILE             输出 Markdown 文件，默认同目录 upload-recap-YYYY-MM-DD.md
  --allow-gate-fail              门禁失败时也返回 0
  -h, --help                     显示帮助
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少依赖命令: $1" >&2
    exit 2
  fi
}

metric_value() {
  local file="$1"
  local metric="$2"
  awk -v metric="$metric" '
    $1 == metric { value = $2 }
    index($1, metric "{") == 1 { value = $2 }
    END {
      if (value == "") value = 0
      print value + 0
    }
  ' "$file"
}

failure_reasons_json() {
  local file="$1"
  local lines
  lines="$(
    grep -E '^gwf_upload_failure_reason_total\{' "$file" 2>/dev/null \
      | sed -E 's/^gwf_upload_failure_reason_total\{.*reason="([^"]+)".*\}[[:space:]]+([0-9.eE+-]+).*$/\1\t\2/' \
      || true
  )"

  if [[ -z "$lines" ]]; then
    echo '{}'
    return 0
  fi

  printf '%s\n' "$lines" \
    | jq -Rn '
        [inputs | select(length > 0) | split("\t") | {k: .[0], v: (.[1] | tonumber)}]
        | reduce .[] as $item ({}; .[$item.k] = ((.[$item.k] // 0) + $item.v))
      '
}

collect_snapshot() {
  local name="$1"
  local out_dir="$2"

  local metrics_file="${out_dir}/metrics-upload-${name}.prom"
  local health_file="${out_dir}/health-upload-${name}.json"

  curl -sS "${BASE_URL%/}/metrics" -o "$metrics_file"
  curl -sS "${BASE_URL%/}/api/health" -o "$health_file"

  local failure_reasons
  failure_reasons="$(failure_reasons_json "$metrics_file")"

  jq -n \
    --arg name "$name" \
    --arg collectedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg metricsFile "$metrics_file" \
    --arg healthFile "$health_file" \
    --argjson fileEventsTotal "$(metric_value "$metrics_file" "gwf_file_events_total")" \
    --argjson queueLength "$(metric_value "$metrics_file" "gwf_upload_queue_length")" \
    --argjson inFlight "$(metric_value "$metrics_file" "gwf_upload_inflight")" \
    --argjson workers "$(metric_value "$metrics_file" "gwf_upload_workers")" \
    --argjson queueFullTotal "$(metric_value "$metrics_file" "gwf_upload_queue_full_total")" \
    --argjson queueShedTotal "$(metric_value "$metrics_file" "gwf_upload_queue_shed_total")" \
    --argjson retryTotal "$(metric_value "$metrics_file" "gwf_upload_retry_total")" \
    --argjson uploadSuccessTotal "$(metric_value "$metrics_file" "gwf_upload_success_total")" \
    --argjson uploadFailureTotal "$(metric_value "$metrics_file" "gwf_upload_failure_total")" \
    --argjson failureReasons "$failure_reasons" \
    --slurpfile health "$health_file" \
    '{
      name: $name,
      collectedAt: $collectedAt,
      metricsFile: $metricsFile,
      healthFile: $healthFile,
      metrics: {
        fileEventsTotal: $fileEventsTotal,
        queueLength: $queueLength,
        inFlight: $inFlight,
        workers: $workers,
        queueFullTotal: $queueFullTotal,
        queueShedTotal: $queueShedTotal,
        retryTotal: $retryTotal,
        uploadSuccessTotal: $uploadSuccessTotal,
        uploadFailureTotal: $uploadFailureTotal,
        failureReasons: $failureReasons
      },
      health: ($health[0] // {})
    }'
}

BASE_URL="http://localhost:8082"
WATCH_DIR=""
SATURATION_COUNT=1200
SATURATION_INTERVAL_MS=5
MIN_BYTES=1024
MAX_BYTES=8192
STABILIZE_SECONDS=8
MAX_FAILURE_RATIO=0.05
MAX_QUEUE_FULL_DELTA=0
REQUIRE_QUEUE_SATURATION="false"
ALLOW_GATE_FAIL=0
DATE_TAG="$(date +%F)"
OUTPUT_FILE=""
REPORT_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)
      BASE_URL="${2:-}"; shift 2 ;;
    --watch-dir)
      WATCH_DIR="${2:-}"; shift 2 ;;
    --saturation-count)
      SATURATION_COUNT="${2:-}"; shift 2 ;;
    --saturation-interval-ms)
      SATURATION_INTERVAL_MS="${2:-}"; shift 2 ;;
    --min-bytes)
      MIN_BYTES="${2:-}"; shift 2 ;;
    --max-bytes)
      MAX_BYTES="${2:-}"; shift 2 ;;
    --stabilize-seconds)
      STABILIZE_SECONDS="${2:-}"; shift 2 ;;
    --max-failure-ratio)
      MAX_FAILURE_RATIO="${2:-}"; shift 2 ;;
    --max-queue-full-delta)
      MAX_QUEUE_FULL_DELTA="${2:-}"; shift 2 ;;
    --require-queue-saturation)
      REQUIRE_QUEUE_SATURATION="true"; shift ;;
    --output-file)
      OUTPUT_FILE="${2:-}"; shift 2 ;;
    --report-file)
      REPORT_FILE="${2:-}"; shift 2 ;;
    --allow-gate-fail)
      ALLOW_GATE_FAIL=1; shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 2 ;;
  esac
done

require_cmd jq
require_cmd curl
require_cmd awk

if [[ -z "$WATCH_DIR" ]]; then
  echo "--watch-dir 为必填参数" >&2
  exit 2
fi
if [[ "$SATURATION_COUNT" -le 0 ]]; then
  echo "saturation-count 必须大于 0" >&2
  exit 2
fi
if [[ "$MIN_BYTES" -le 0 || "$MAX_BYTES" -lt "$MIN_BYTES" ]]; then
  echo "min/max 字节范围非法" >&2
  exit 2
fi

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
STRESS_SCRIPT="${SCRIPT_DIR}/upload-stress.sh"
if [[ ! -x "$STRESS_SCRIPT" ]]; then
  echo "缺少可执行脚本: $STRESS_SCRIPT" >&2
  exit 2
fi

if [[ -z "$OUTPUT_FILE" ]]; then
  OUTPUT_FILE="../reports/upload-recap-${DATE_TAG}.json"
fi
OUT_DIR="$(dirname "$OUTPUT_FILE")"
mkdir -p "$OUT_DIR"

if [[ -z "$REPORT_FILE" ]]; then
  REPORT_FILE="${OUT_DIR}/upload-recap-${DATE_TAG}.md"
fi
mkdir -p "$(dirname "$REPORT_FILE")"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Collect baseline snapshot..."
collect_snapshot "before" "$OUT_DIR" >"${TMP_DIR}/baseline.json"

echo "Run saturation traffic..."
bash "$STRESS_SCRIPT" \
  --watch-dir "$WATCH_DIR" \
  --count "$SATURATION_COUNT" \
  --interval-ms "$SATURATION_INTERVAL_MS" \
  --min-bytes "$MIN_BYTES" \
  --max-bytes "$MAX_BYTES" \
  --prefix "queue_saturation"

sleep "$STABILIZE_SECONDS"

echo "Collect post-saturation snapshot..."
collect_snapshot "after-saturation" "$OUT_DIR" >"${TMP_DIR}/after_saturation.json"

jq -n \
  --arg generatedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg baseUrl "${BASE_URL%/}" \
  --arg watchDir "$WATCH_DIR" \
  --argjson saturationCount "$SATURATION_COUNT" \
  --argjson saturationIntervalMs "$SATURATION_INTERVAL_MS" \
  --argjson minBytes "$MIN_BYTES" \
  --argjson maxBytes "$MAX_BYTES" \
  --argjson stabilizeSeconds "$STABILIZE_SECONDS" \
  --argjson maxFailureRatio "$MAX_FAILURE_RATIO" \
  --argjson maxQueueFullDelta "$MAX_QUEUE_FULL_DELTA" \
  --argjson requireQueueSaturation "$REQUIRE_QUEUE_SATURATION" \
  --slurpfile baseline "${TMP_DIR}/baseline.json" \
  --slurpfile after "${TMP_DIR}/after_saturation.json" \
  '
  def obj_diff($before; $after):
    ([$before, $after] | map(keys) | add | unique) as $keys
    | reduce $keys[] as $k ({}; .[$k] = (($after[$k] // 0) - ($before[$k] // 0)));

  ($baseline[0]) as $b
  | ($after[0]) as $a
  | (($a.metrics.uploadSuccessTotal // 0) - ($b.metrics.uploadSuccessTotal // 0)) as $successDelta
  | (($a.metrics.uploadFailureTotal // 0) - ($b.metrics.uploadFailureTotal // 0)) as $failureDelta
  | ($successDelta + $failureDelta) as $trafficDelta
  | (if $trafficDelta > 0 then ($failureDelta / $trafficDelta) else null end) as $failureRatio
  | (($a.metrics.queueFullTotal // 0) - ($b.metrics.queueFullTotal // 0)) as $queueFullDelta
  | (($a.metrics.queueShedTotal // 0) - ($b.metrics.queueShedTotal // 0)) as $queueShedDelta
  | (($a.metrics.retryTotal // 0) - ($b.metrics.retryTotal // 0)) as $retryDelta
  | {
      generatedAt: $generatedAt,
      scope: "upload recap lite (saturation only)",
      baseUrl: $baseUrl,
      watchDir: $watchDir,
      stressConfig: {
        saturation: {count: $saturationCount, intervalMs: $saturationIntervalMs},
        bytes: {min: $minBytes, max: $maxBytes},
        stabilizeSeconds: $stabilizeSeconds
      },
      gateTargets: {
        maxFailureRatio: $maxFailureRatio,
        maxQueueFullDelta: $maxQueueFullDelta,
        requireQueueSaturation: $requireQueueSaturation
      },
      snapshots: {
        baseline: $b,
        afterSaturation: $a
      },
      deltas: {
        saturation: {
          fileEventsDelta: (($a.metrics.fileEventsTotal // 0) - ($b.metrics.fileEventsTotal // 0)),
          queueFullDelta: $queueFullDelta,
          queueShedDelta: $queueShedDelta,
          retryDelta: $retryDelta,
          uploadSuccessDelta: $successDelta,
          uploadFailureDelta: $failureDelta,
          trafficDelta: $trafficDelta,
          failureRatio: $failureRatio,
          failureRatioPct: (if $failureRatio == null then null else ($failureRatio * 100) end),
          queueLengthEnd: ($a.metrics.queueLength // 0),
          inFlightEnd: ($a.metrics.inFlight // 0),
          failureReasonDelta: obj_diff(($b.metrics.failureReasons // {}); ($a.metrics.failureReasons // {}))
        }
      }
    }
  | .checks = {
      uploadTrafficObserved: (.deltas.saturation.trafficDelta > 0),
      queueSaturationObserved: ((.deltas.saturation.queueShedDelta > 0) or (.deltas.saturation.queueFullDelta > 0)),
      queueFullPass: (.deltas.saturation.queueFullDelta <= .gateTargets.maxQueueFullDelta),
      failureRatioPass: (
        if .deltas.saturation.failureRatio == null then false
        else (.deltas.saturation.failureRatio <= .gateTargets.maxFailureRatio)
        end
      )
    }
  | .allPassed = (
      .checks.uploadTrafficObserved
      and .checks.queueFullPass
      and .checks.failureRatioPass
      and (
        if .gateTargets.requireQueueSaturation
        then .checks.queueSaturationObserved
        else true
        end
      )
    )
  ' >"$OUTPUT_FILE"

ALL_PASSED="$(jq -r '.allPassed' "$OUTPUT_FILE")"
UPLOAD_TRAFFIC="$(jq -r '.checks.uploadTrafficObserved' "$OUTPUT_FILE")"
QUEUE_SATURATION="$(jq -r '.checks.queueSaturationObserved' "$OUTPUT_FILE")"
QUEUE_FULL_DELTA="$(jq -r '.deltas.saturation.queueFullDelta' "$OUTPUT_FILE")"
QUEUE_SHED_DELTA="$(jq -r '.deltas.saturation.queueShedDelta' "$OUTPUT_FILE")"
RETRY_DELTA="$(jq -r '.deltas.saturation.retryDelta' "$OUTPUT_FILE")"
SUCCESS_DELTA="$(jq -r '.deltas.saturation.uploadSuccessDelta' "$OUTPUT_FILE")"
FAILURE_DELTA="$(jq -r '.deltas.saturation.uploadFailureDelta' "$OUTPUT_FILE")"
FAILURE_RATIO_PCT="$(jq -r '.deltas.saturation.failureRatioPct // "N/A"' "$OUTPUT_FILE")"
FAILURE_RATIO_PASS="$(jq -r '.checks.failureRatioPass' "$OUTPUT_FILE")"
QUEUE_FULL_PASS="$(jq -r '.checks.queueFullPass' "$OUTPUT_FILE")"

cat >"$REPORT_FILE" <<EOF
# Upload Reliability Recap Lite (${DATE_TAG})

- generatedAt: $(jq -r '.generatedAt' "$OUTPUT_FILE")
- baseUrl: ${BASE_URL%/}
- watchDir: ${WATCH_DIR}
- outputJson: ${OUTPUT_FILE}
- allPassed: ${ALL_PASSED}

## 1. Checks

- uploadTrafficObserved: ${UPLOAD_TRAFFIC}
- queueSaturationObserved: ${QUEUE_SATURATION}
- queueFullPass: ${QUEUE_FULL_PASS}
- failureRatioPass: ${FAILURE_RATIO_PASS}

## 2. Delta Metrics

| Window | queueFullDelta | queueShedDelta | retryDelta | successDelta | failureDelta | failureRatioPct |
| --- | --- | --- | --- | --- | --- | --- |
| saturation | ${QUEUE_FULL_DELTA} | ${QUEUE_SHED_DELTA} | ${RETRY_DELTA} | ${SUCCESS_DELTA} | ${FAILURE_DELTA} | ${FAILURE_RATIO_PCT} |
EOF

echo "Upload recap lite completed. allPassed=${ALL_PASSED}"
echo "JSON: ${OUTPUT_FILE}"
echo "Markdown: ${REPORT_FILE}"

if [[ "$ALL_PASSED" != "true" && "$ALLOW_GATE_FAIL" -ne 1 ]]; then
  exit 3
fi

exit 0

