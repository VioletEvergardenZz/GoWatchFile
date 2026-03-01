#!/usr/bin/env bash
set -euo pipefail

# 生成批量测试文件，用于验证上传队列与背压行为（macOS/Linux）。

usage() {
  cat <<'EOF'
用法:
  scripts/ops/upload-stress.sh \
    --watch-dir /tmp/gwf-stress \
    --count 1000 \
    --interval-ms 20

参数:
  --watch-dir DIR      压测目录（必填）
  --count N            文件数量，默认 1000
  --interval-ms N      每个文件的间隔毫秒，默认 20
  --min-bytes N        单文件最小大小，默认 512
  --max-bytes N        单文件最大大小，默认 4096
  --prefix TEXT        文件名前缀，默认 stress
  -h, --help           显示帮助
EOF
}

sleep_ms() {
  local ms="$1"
  if [[ "$ms" -le 0 ]]; then
    return 0
  fi
  local seconds
  seconds="$(awk -v v="$ms" 'BEGIN { printf "%.3f", v / 1000.0 }')"
  sleep "$seconds"
}

WATCH_DIR=""
COUNT=1000
INTERVAL_MS=20
MIN_BYTES=512
MAX_BYTES=4096
PREFIX="stress"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --watch-dir)
      WATCH_DIR="${2:-}"; shift 2 ;;
    --count)
      COUNT="${2:-}"; shift 2 ;;
    --interval-ms)
      INTERVAL_MS="${2:-}"; shift 2 ;;
    --min-bytes)
      MIN_BYTES="${2:-}"; shift 2 ;;
    --max-bytes)
      MAX_BYTES="${2:-}"; shift 2 ;;
    --prefix)
      PREFIX="${2:-}"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 2 ;;
  esac
done

if [[ -z "$WATCH_DIR" ]]; then
  echo "--watch-dir 为必填参数" >&2
  exit 2
fi
if [[ "$COUNT" -le 0 ]]; then
  echo "count 必须大于 0" >&2
  exit 2
fi
if [[ "$MIN_BYTES" -le 0 || "$MAX_BYTES" -lt "$MIN_BYTES" ]]; then
  echo "min/max 字节范围非法" >&2
  exit 2
fi

mkdir -p "$WATCH_DIR"

echo "Start generating stress files"
echo "Directory: $WATCH_DIR"
echo "Count: $COUNT"

start_ts="$(date +%s)"
run_id="${start_ts}_$$"

for ((i = 1; i <= COUNT; i++)); do
  size=$((MIN_BYTES + RANDOM % (MAX_BYTES - MIN_BYTES + 1)))
  ts="$(date +%Y%m%d_%H%M%S)"
  # 追加 run_id，避免连续两轮压测在同秒启动时发生同名覆盖。
  filename="$(printf '%s_%s_%s_%06d.log' "$PREFIX" "$run_id" "$ts" "$i")"
  filepath="$WATCH_DIR/$filename"

  header="$(printf 'case=%s index=%d size=%d time=%s\n' "$PREFIX" "$i" "$size" "$ts")"
  header_len=${#header}
  body_size=$((size - header_len))
  if [[ "$body_size" -lt 0 ]]; then
    body_size=0
  fi

  printf '%s' "$header" >"$filepath"
  if [[ "$body_size" -gt 0 ]]; then
    head -c "$body_size" /dev/zero | tr '\0' 'x' >>"$filepath"
  fi

  sleep_ms "$INTERVAL_MS"

  if ((i % 100 == 0)); then
    echo "Generated $i/$COUNT"
  fi
done

end_ts="$(date +%s)"
elapsed=$((end_ts - start_ts))
echo "Done generated $COUNT files in ${elapsed}s"
