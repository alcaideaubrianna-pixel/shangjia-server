#!/usr/bin/env bash
set -euo pipefail
MODE=dry-run
BATCH_SIZE=${BATCH_SIZE:-500}
MAX_BATCHES=${MAX_BATCHES:-0}
PGURL=${PGURL:-}
REDIS_URL=${REDIS_URL:-}
LEGACY_PREFIX=${LEGACY_PREFIX:-youban_publish:tg:channel_pending:}
REDIS_PREFIX=${REDIS_PREFIX:-youban:tg:channel:queue:}
while (($#)); do
  case "$1" in
    --dry-run) MODE=dry-run ;;
    --execute) MODE=execute ;;
    --batch-size) BATCH_SIZE=$2; shift ;;
    --max-batches) MAX_BATCHES=$2; shift ;;
    --help|-h) echo 'Usage: migrate-tg-channel-queue-legacy.sh [--dry-run|--execute] [--batch-size N] [--max-batches N]'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done
[[ "$BATCH_SIZE" =~ ^[1-9][0-9]*$ ]] || exit 2
[[ "$MAX_BATCHES" =~ ^[0-9]+$ ]] || exit 2
[[ -n "$REDIS_URL" ]] || { echo "REDIS_URL required" >&2; exit 2; }
command -v redis-cli >/dev/null || exit 127
if [[ "$MODE" == execute ]]; then
  [[ -n "$PGURL" ]] || { echo "PGURL required" >&2; exit 2; }
  command -v psql >/dev/null || exit 127
fi
keys=$(redis-cli -u "$REDIS_URL" --scan --pattern "${LEGACY_PREFIX}*")
total=0
batch=0
while IFS= read -r old_key; do
  if (( MAX_BATCHES > 0 && batch >= MAX_BATCHES )); then break; fi
  [[ -n "$old_key" ]] || continue
  suffix=${old_key#"$LEGACY_PREFIX"}; channel=${suffix##*:}
  [[ "$channel" =~ ^[0-9]+$ ]] || continue
  new_key="${REDIS_PREFIX}${channel}"
  members=$(redis-cli -u "$REDIS_URL" --raw ZRANGE "$old_key" 0 -1 WITHSCORES)
  count=0
  while IFS= read -r job_id && IFS= read -r score; do
    [[ "$job_id" =~ ^[0-9]+$ && -n "$score" ]] || continue
    ((count+=1))
    if [[ "$MODE" == execute ]]; then
      redis-cli -u "$REDIS_URL" --raw ZADD "$new_key" "$score" "$job_id" >/dev/null
      psql "$PGURL" -X -qAt -v ON_ERROR_STOP=1 -v job_id="$job_id" <<'SQL' >/dev/null
WITH claimed AS (
  UPDATE hg_youban_publish_tg_job
  SET dispatch_status='queued', queue_name='channel_queue', updated_at=CURRENT_TIMESTAMP
  WHERE id = :'job_id'
    AND status IN ('pending','failed_retry','unknown')
    AND COALESCE(asynq_task_id,'')=''
  RETURNING id
)
UPDATE hg_youban_publish_tg_outbox o
SET status='delivered', delivered_at=COALESCE(delivered_at,CURRENT_TIMESTAMP),
    lease_owner='', lease_expires_at=NULL, updated_at=CURRENT_TIMESTAMP
WHERE o.job_id IN (SELECT id FROM claimed)
  AND o.status IN ('pending','processing');
SQL
    fi
  done <<< "$members"
  total=$((total+count)); batch=$((batch+1)); echo "mode=$MODE old_key=$old_key new_key=$new_key members=$count batch=$batch" >&2
done <<< "$keys"
echo "legacy migration complete total=$total mode=$MODE" >&2
