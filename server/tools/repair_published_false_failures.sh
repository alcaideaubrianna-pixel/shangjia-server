#!/usr/bin/env bash
set -euo pipefail
DATABASE_URL="${DATABASE_URL:?DATABASE_URL is required}"
BATCH_SIZE="${BATCH_SIZE:-500}"
SLEEP_SECONDS="${BATCH_SLEEP_SECONDS:-1}"
ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
while true; do
  result="$(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -At -v batch_size="$BATCH_SIZE" -f "$ROOT_DIR/repair_published_false_failures.sql")"
  echo "$result"
  changed="$(printf '%s\n' "$result" | awk -F'|' '{sum += $1} END {print sum+0}')"
  [ "$changed" -eq 0 ] && break
  sleep "$SLEEP_SECONDS"
done
echo "published false-failure repair complete"
