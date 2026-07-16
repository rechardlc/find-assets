#!/bin/bash
# 周五北京时间 21:20 跑周线 pierce + reversal（GCP 上 cron 用 UTC 13:20）
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

run_one week pierce
run_one week reversal
