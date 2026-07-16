#!/bin/bash
# 工作日北京时间 21:05 跑日线 pierce + reversal（GCP 上 cron 用 UTC 13:05）
set -euo pipefail
DIR="$(cd "$(dirname "$0")/../.." && pwd)"
W="${WORKERS:-10}"
OUT="${OUT_DIR:-$DIR/output/ashare}"
BIN="${SCANNER_BIN:-$DIR/scanner}"

mkdir -p "$OUT" "$DIR/logs"
cd "$DIR"

run_one() {
  local period=$1 pattern=$2
  "$BIN" \
    -p "$period" -pt "$pattern" \
    -workers "$W" -source auto \
    -export md,json -out "$OUT" \
    -mail=true \
    -env "$DIR/.env"
}

run_one day pierce
run_one day reversal
