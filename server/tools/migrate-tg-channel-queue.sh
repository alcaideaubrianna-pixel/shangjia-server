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
REDIS_PREFIX=${REDIS_PREFIX:-youban:tg:channel:queue:}

usage() {
  cat <<'EOF'
Usage: migrate-tg-channel-queue.sh [--dry-run|--execute] [--batch-size N] [--max-batches N]

Environment:
  PGURL       PostgreSQL connection URL (required for --execute)
  REDIS_URL   redis-cli URL (required for --execute)
  BATCH_SIZE  rows per batch (default: 500)
  MAX_BATCHES 0 means unlimited
EOF
}

while (($#)); do
  case "$1" in
    --dry-run) MODE=dry-run ;;
    --execute) MODE=execute ;;
    --batch-size) BATCH_SIZE=$2; shift ;;
    --max-batches) MAX_BATCHES=$2; shift ;;
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

if [[ "$MODE" == dry-run ]]; then
  echo "DRY RUN: no Redis or database writes will be performed."
  exit 0
fi

command -v psql >/dev/null || { echo "psql is required" >&2; exit 127; }
command -v redis-cli >/dev/null || { echo "redis-cli is required" >&2; exit 127; }

batch=0
total=0
while :; do
  if (( MAX_BATCHES > 0 && batch >= MAX_BATCHES )); then break; fi
  rows=$(psql "$PGURL" -X -qAt -F $'\t' -v ON_ERROR_STOP=1 \
    -v limit="$BATCH_SIZE" <<'SQL'
WITH candidates AS MATERIALIZED (
  SELECT j.id, j.channel_id, j.priority,
         EXTRACT(EPOCH FROM COALESCE(j.created_at, CURRENT_TIMESTAMP))::bigint AS created_epoch
  FROM hg_youban_publish_tg_job j
  WHERE j.channel_id > 0
    AND j.status IN ('pending','failed_retry','unknown')
    AND COALESCE(j.dispatch_status,'') IN ('','idle','queued')
    AND COALESCE(j.asynq_task_id,'') = ''
  ORDER BY j.updated_at NULLS FIRST, j.id
  LIMIT :limit
)
SELECT id, channel_id, priority, created_epoch FROM candidates;
SQL
  )

  [[ -n "$rows" ]] || break
  count=0
  while IFS=$'\t' read -r job_id channel_id priority created_epoch; do
    [[ -n "$job_id" ]] || continue
    # Lower score wins: higher priority first, then older jobs first.
    score=$(( (1000000 - priority) * 1000000000000 + created_epoch ))
    key="${REDIS_PREFIX}${channel_id}"
    redis-cli -u "$REDIS_URL" --raw ZADD "$key" "$score" "$job_id" >/dev/null
    # The conditional update is the idempotency/ownership boundary.  If a
    # worker changed the job while we were writing Redis, leave it untouched.
    psql "$PGURL" -X -qAt -v ON_ERROR_STOP=1 \
      -v job_id="$job_id" <<'SQL' >/dev/null
UPDATE hg_youban_publish_tg_job
SET dispatch_status='queued', queue_name='channel_queue', updated_at=CURRENT_TIMESTAMP
WHERE id=:job_id
  AND status IN ('pending','failed_retry','unknown')
  AND COALESCE(dispatch_status,'') IN ('','idle','queued')
  AND COALESCE(asynq_task_id,'')='';
SQL
    psql "$PGURL" -X -qAt -v ON_ERROR_STOP=1 \
      -v job_id="$job_id" <<'SQL' >/dev/null
UPDATE hg_youban_publish_tg_outbox
SET status='delivered', delivered_at=COALESCE(delivered_at,CURRENT_TIMESTAMP),
    lease_owner='', lease_expires_at=NULL, updated_at=CURRENT_TIMESTAMP
WHERE job_id=:job_id AND status IN ('pending','processing');
SQL
    ((count+=1))
  done <<< "$rows"
  ((total+=count))
  ((batch+=1))
  echo "migrated batch=$batch count=$count total=$total" >&2
  (( count < BATCH_SIZE )) && break
done
echo "migration complete total=$total" >&2
