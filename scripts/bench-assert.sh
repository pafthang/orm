#!/usr/bin/env bash
set -euo pipefail

BASELINE_FILE="${BASELINE_FILE:-perf/baseline.tsv}"
TMP_OUT="$(mktemp)"
trap 'rm -f "$TMP_OUT"' EXIT

if [[ ! -f "$BASELINE_FILE" ]]; then
  echo "baseline file not found: $BASELINE_FILE"
  exit 1
fi

go test -run '^$' -bench '^BenchmarkCompare' -benchtime=1x ./... > "$TMP_OUT"

status=0
while IFS=$'\t' read -r name max_ns; do
  [[ -z "${name}" || "${name}" =~ ^# ]] && continue
  current="$(awk -v n="$name" '$1 ~ n {print $(NF-1)}' "$TMP_OUT" | head -n1)"
  if [[ -z "$current" ]]; then
    echo "[bench] missing benchmark in output: $name"
    status=1
    continue
  fi
  if (( current > max_ns )); then
    echo "[bench] FAIL $name current=${current}ns baseline=${max_ns}ns"
    status=1
  else
    echo "[bench] OK   $name current=${current}ns baseline=${max_ns}ns"
  fi
done < "$BASELINE_FILE"

exit $status
