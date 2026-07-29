#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:5999}"
TOKEN="${TOKEN:-}"
SOURCE_ID="${SOURCE_ID:-0}"
ACCOUNT_ID="${ACCOUNT_ID:-0}"
LIMIT="${LIMIT:-30}"
CONCURRENCY="${CONCURRENCY:-4}"
ROUNDS="${ROUNDS:-1}"
INCLUDE_CACHED="${INCLUDE_CACHED:-false}"
OUTPUT="${OUTPUT:-./tg-media-benchmark-$(date +%Y%m%d-%H%M%S).json}"

usage() {
  cat <<'USAGE'
TG media benchmark

Environment variables:
  BASE_URL          server address, default http://127.0.0.1:5999
  TOKEN             Bearer token, required
  SOURCE_ID         collect source id, default 0 (all sources)
  ACCOUNT_ID        super-admin only: publish account id, default 0 (current account)
  LIMIT            samples per round, default 30, max 200
  CONCURRENCY      download concurrency, default 4, max 32
  ROUNDS           rounds, default 1
  INCLUDE_CACHED   include cached media, default false
  OUTPUT           output JSON path

Example:
  TOKEN='Bearer eyJ...' SOURCE_ID=9 LIMIT=30 CONCURRENCY=4 \
    ./server/tools/tg_media_benchmark.sh
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ -z "$TOKEN" ]]; then
  echo "TOKEN is required" >&2
  usage >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 2
fi

mkdir -p "$(dirname "$OUTPUT")"
rounds_file="$(mktemp)"
trap 'rm -f "$rounds_file"' EXIT

for ((round = 1; round <= ROUNDS; round++)); do
  query="limit=${LIMIT}&concurrency=${CONCURRENCY}&includeCached=${INCLUDE_CACHED}"
  if [[ "$ACCOUNT_ID" != "0" ]]; then
    query="${query}&accountId=${ACCOUNT_ID}"
  fi
  if [[ "$SOURCE_ID" != "0" ]]; then
    query="${query}&sourceId=${SOURCE_ID}"
  fi

  echo "round ${round}/${ROUNDS}: limit=${LIMIT} concurrency=${CONCURRENCY} source=${SOURCE_ID} cached=${INCLUDE_CACHED}"
  response_file="$(mktemp)"
  trap 'rm -f "$response_file"' EXIT
  curl --fail-with-body --silent --show-error --max-time 1800 \
    -H "Authorization: ${TOKEN}" \
    "${BASE_URL}/api/youban_publish/publish/collect/material/benchmark?${query}" > "$response_file"

  if [[ "$(jq -r '.code // -1' "$response_file")" != "0" ]]; then
    jq . "$response_file" >&2
    exit 1
  fi

  jq --argjson round "$round" '.data + {round: $round}' "$response_file" >> "$rounds_file"
  jq -r '
    .data |
    "total=\(.total) started=\(.started) ok=\(.succeeded) failed=\(.failed) " +
    "total_ms=\(.totalDurationMs) p50_ms=\(.p50DurationMs) p95_ms=\(.p95DurationMs) " +
    "throughput_mbps=\(.throughputMbps)"
  ' "$response_file"
  rm -f "$response_file"
  trap - EXIT
done

jq -s '.' "$rounds_file" > "$OUTPUT"

echo "results: ${OUTPUT}"
