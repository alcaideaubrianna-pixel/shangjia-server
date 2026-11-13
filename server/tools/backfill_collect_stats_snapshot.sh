#!/usr/bin/env bash
set -euo pipefail

# 慢速、可重复回填；默认每个账户之间休眠，避免与线上采集任务争用 IO。
# DATABASE_URL 必填；可选 BATCH_SLEEP_SECONDS（默认 1）。
ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
DATABASE_URL="${DATABASE_URL:?DATABASE_URL is required}"
SLEEP_SECONDS="${BATCH_SLEEP_SECONDS:-1}"
TENANT_ID="${1:-0}"
ACCOUNT_ID="${2:-0}"

mapfile -t scopes < <(psql "$DATABASE_URL" -At -F $'\t' -c "
SELECT DISTINCT tenant_id, account_id
FROM hg_youban_publish_collect_source
WHERE deleted_at IS NULL AND account_id > 0
  AND (${TENANT_ID} = 0 OR tenant_id = ${TENANT_ID})
  AND (${ACCOUNT_ID} = 0 OR account_id = ${ACCOUNT_ID})
ORDER BY tenant_id, account_id;")

total="${#scopes[@]}"
current=0
for scope in "${scopes[@]}"; do
  IFS=$'\t' read -r tenant account <<< "$scope"
  current=$((current + 1))
  echo "[$current/$total] tenant=$tenant account=$account"
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -v tenant_id="$tenant" -v account_id="$account" \
    -f "$ROOT_DIR/backfill_collect_stats_snapshot.sql" >/dev/null
  sleep "$SLEEP_SECONDS"
done

echo "backfill complete: $total scopes"
