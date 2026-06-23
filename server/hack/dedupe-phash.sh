#!/usr/bin/env bash
set -euo pipefail

BIN="${BIN:-./main}"
LIMIT="${LIMIT:-10000}"
START_ID="${START_ID:-0}"
MAX_ROUNDS="${MAX_ROUNDS:-0}"

round=0
while true; do
  round=$((round + 1))
  output="$("${BIN}" tools -m=content -a1=dedupePHash -startId="${START_ID}" -limit="${LIMIT}" 2>&1)"
  printf '%s\n' "${output}"

  scanned="$(printf '%s\n' "${output}" | sed -nE 's/.*scanned:([0-9]+).*/\1/p' | tail -n 1)"
  last_id="$(printf '%s\n' "${output}" | sed -nE 's/.*lastId:([0-9]+).*/\1/p' | tail -n 1)"
  scanned="${scanned:-0}"
  last_id="${last_id:-0}"

  if [ "${scanned}" -eq 0 ] || [ "${last_id}" -le "${START_ID}" ]; then
    break
  fi
  START_ID="${last_id}"

  if [ "${MAX_ROUNDS}" -gt 0 ] && [ "${round}" -ge "${MAX_ROUNDS}" ]; then
    break
  fi
done
