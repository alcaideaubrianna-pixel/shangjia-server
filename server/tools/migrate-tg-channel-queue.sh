#!/usr/bin/env bash
set -euo pipefail

# Migrate unfinished Telegram jobs from the legacy Asynq/Outbox path into the
# per-channel Redis sorted queues.  The script is deliberately dry-run by
# default.  It requires psql and redis-cli; credentials are supplied through
# PGURL/REDIS_URL and are never stored in the repository.

MODE=dry-run
BATCH_SIZE=${BATCH_SIZE:-500}
MAX_BATCHES=${MAX_BATCHES:-0}
PGURL=${PGURL:-}
REDIS_URL=${REDIS_URL:-}
REDIS_HOST=${REDIS_HOST:-}
REDIS_PORT=${REDIS_PORT:-6379}
REDIS_DB=${REDIS_DB:-3}
REDIS_PREFIX=${REDIS_PREFIX:-youban:tg:channel:queue:}
URGENT_PREFIX=${URGENT_PREFIX:-youban:tg:channel:urgent:}
LEGACY_PREFIX=${LEGACY_PREFIX:-youban_publish:tg:channel_pending:}

usage() {
  cat <<'EOF'
Usage: migrate-tg-channel-queue.sh [--dry-run|--execute] [--batch-size N] [--max-batches N]

Environment:
  PGURL       PostgreSQL connection URL (required for --execute)
  REDIS_URL   redis-cli URL (required for --execute)
  BATCH_SIZE  rows per batch (default: 500)
  MAX_BATCHES 0 means unlimited
  LEGACY_PREFIX legacy Redis ZSET prefix
  URGENT_PREFIX  urgent channel ZSET prefix
EOF
}

while (($#)); do
  case "$1" in
    --dry-run) MODE=dry-run ;;
    --execute) MODE=execute ;;
    --batch-size) BATCH_SIZE=$2; shift ;;
    --max-batches) MAX_BATCHES=$2; shift ;;
    --legacy-prefix) LEGACY_PREFIX=$2; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

[[ "$BATCH_SIZE" =~ ^[1-9][0-9]*$ ]] || { echo "BATCH_SIZE must be positive" >&2; exit 2; }
[[ "$MAX_BATCHES" =~ ^[0-9]+$ ]] || { echo "MAX_BATCHES must be non-negative" >&2; exit 2; }
if [[ "$MODE" == execute && ( -z "$PGURL" || -z "$REDIS_URL" ) ]]; then
  echo "PGURL and REDIS_URL are required for --execute" >&2
  exit 2
fi

command -v psql >/dev/null || { echo "psql is required" >&2; exit 127; }
command -v redis-cli >/dev/null || { echo "redis-cli is required" >&2; exit 127; }

redis_cmd() {
  if [[ -n "$REDIS_HOST" ]]; then
    REDISCLI_AUTH="${REDISCLI_AUTH:-}" redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" "$@"
  else
    redis-cli -u "$REDIS_URL" "$@"
  fi
}

if [[ "$MODE" == dry-run ]]; then
  echo "DRY RUN: no Redis or database writes will be performed."
  if [[ -n "$REDIS_URL" ]]; then
    keys=$(redis_cmd --scan --pattern "${LEGACY_PREFIX}*" || true)
    while IFS= read -r key; do
      [[ -n "$key" ]] || continue
      echo "legacy_key=$key members=$(redis_cmd ZCARD "$key")"
    done <<< "$keys"
  fi
  exit 0
fi

batch=0
total=0
while :; do
  if (( MAX_BATCHES > 0 && batch >= MAX_BATCHES )); then break; fi
  rows=$(psql "$PGURL" -X -qAt -F $'\t' -v ON_ERROR_STOP=1 \
    -v limit="$BATCH_SIZE" <<'SQL'
WITH candidates AS MATERIALIZED (
  SELECT j.id, j.channel_id, j.priority,
         EXTRACT(EPOCH FROM COALESCE(j.created_at, CURRENT_TIMESTAMP))::bigint AS created_epoch,
         COALESCE(j.operation_no,'') AS operation_no
  FROM hg_youban_publish_tg_job j
  WHERE j.channel_id > 0
    AND j.status IN ('pending','failed_retry','unknown')
    AND COALESCE(j.dispatch_status,'') IN ('','idle','queued')
  ORDER BY j.updated_at NULLS FIRST, j.id
  LIMIT :limit
)
SELECT id, channel_id, priority, created_epoch, operation_no FROM candidates;
SQL
  )

  [[ -n "$rows" ]] || break
  count=0
  update_ids=()
  while IFS=$'\t' read -r job_id channel_id priority created_epoch operation_no; do
    [[ -n "$job_id" ]] || continue
    # Keep this encoding in sync with telegramChannelQueueScore: lane * 2e12
    # + epoch milliseconds. The old encoding used a 1e15 lane scale and
    # overflowed into the next lane at current Unix timestamps.
    if (( priority <= 1 )); then rank=0
    elif (( priority <= 10 )); then rank=1
    elif (( priority <= 50 )); then rank=2
    elif (( priority <= 90 )); then rank=3
    else rank=4
    fi
    score=$(( rank * 2000000000000 + created_epoch * 1000 + job_id % 1000 ))
    key="${REDIS_PREFIX}${channel_id}"
    if [[ "$operation_no" =~ ^(bot_publish|profile|edit|batchtext|full_push): || "$operation_no" == interactive:* ]]; then
      key="${URGENT_PREFIX}${channel_id}"
    fi
    printf '%s\t%s\t%s\n' "$key" "$score" "$job_id"
    update_ids+=("$job_id")
    ((count+=1))
  done <<< "$rows" > /tmp/tg-channel-zadd-batch.$$
  while IFS=$'\t' read -r key score job_id; do
    redis_cmd --raw ZADD "$key" "$score" "$job_id" >/dev/null
    channel_id="${key##*:}"
    redis_cmd --raw SADD youban:tg:channel:active "$channel_id" >/dev/null
    if [[ "$key" == "${URGENT_PREFIX}${channel_id}" ]]; then
      redis_cmd --raw ZREM "${REDIS_PREFIX}${channel_id}" "$job_id" >/dev/null
    else
      redis_cmd --raw ZREM "${URGENT_PREFIX}${channel_id}" "$job_id" >/dev/null
    fi
  done < /tmp/tg-channel-zadd-batch.$$
  rm -f /tmp/tg-channel-zadd-batch.$$
  if ((${#update_ids[@]} > 0)); then
    ids=$(IFS=,; echo "${update_ids[*]}")
    psql "$PGURL" -X -qAt -v ON_ERROR_STOP=1 -v ids="$ids" <<'SQL' >/dev/null
UPDATE hg_youban_publish_tg_job
SET dispatch_status='queued', queue_name='channel_queue', updated_at=CURRENT_TIMESTAMP
WHERE id = ANY(string_to_array(:'ids', ',')::bigint[])
  AND status IN ('pending','failed_retry','unknown')
  AND COALESCE(dispatch_status,'') IN ('','idle','queued');
UPDATE hg_youban_publish_tg_outbox
SET status='delivered', delivered_at=COALESCE(delivered_at,CURRENT_TIMESTAMP),
    lease_owner='', lease_expires_at=NULL, updated_at=CURRENT_TIMESTAMP
WHERE job_id = ANY(string_to_array(:'ids', ',')::bigint[])
  AND status IN ('pending','processing');
SQL
  fi
  ((total+=count))
  ((batch+=1))
  echo "migrated batch=$batch count=$count total=$total" >&2
  (( count < BATCH_SIZE )) && break
done
echo "migration complete total=$total" >&2

# Legacy namespace migration is intentionally a separate explicit pass.
# Invoke tools/migrate-tg-channel-queue-legacy.sh after stopping old consumers.
