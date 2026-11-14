#!/usr/bin/env bash
set -euo pipefail
MODE=dry-run
BATCH_SIZE=${BATCH_SIZE:-500}
PGURL=${PGURL:-}
REDIS_URL=${REDIS_URL:-}
LEGACY_PREFIX=${LEGACY_PREFIX:-youban_publish:tg:channel_pending:}
REDIS_PREFIX=${REDIS_PREFIX:-youban:tg:channel:queue:}
while (($#)); do
  case "$1" in
    --dry-run) MODE=dry-run ;;
    --execute) MODE=execute ;;
    --batch-size) BATCH_SIZE=$2; shift ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done
[[ "$BATCH_SIZE" =~ ^[1-9][0-9]*$ ]] || exit 2
[[ -n "$REDIS_URL" ]] || { echo "REDIS_URL required" >&2; exit 2; }
command -v redis-cli >/dev/null || exit 127
if [[ "$MODE" == execute ]]; then
  [[ -n "$PGURL" ]] || { echo "PGURL required" >&2; exit 2; }
  command -v psql >/dev/null || exit 127
fi
keys=$(redis-cli -u "$REDIS_URL" --scan --pattern "${LEGACY_PREFIX}*")
total=0
while IFS= read -r old_key; do
  [[ -n "$old_key" ]] || continue
  suffix=${old_key#"$LEGACY_PREFIX"}; channel=${suffix#*:}
  [[ "$suffix" == *:* && "$channel" =~ ^[0-9]+$ ]] || continue
  new_key="${REDIS_PREFIX}${channel}"
  members=$(redis-cli -u "$REDIS_URL" --raw ZRANGE "$old_key" 0 -1 WITHSCORES)
  count=0
  while IFS= read -r job_id && IFS= read -r score; do
    [[ "$job_id" =~ ^[0-9]+$ && -n "$score" ]] || continue
    ((count+=1))
    if [[ "$MODE" == execute ]]; then
      redis-cli -u "$REDIS_URL" --raw ZADD "$new_key" "$score" "$job_id" >/dev/null
      psql "$PGURL" -X -qAt -v ON_ERROR_STOP=1 -c "UPDATE hg_youban_publish_tg_job SET dispatch_status='queued', queue_name='channel_queue', updated_at=CURRENT_TIMESTAMP WHERE id=${job_id} AND status IN ('pending','failed_retry','unknown') AND COALESCE(asynq_task_id,'')='';" >/dev/null
    fi
  done <<< "$members"
  total=$((total+count)); echo "mode=$MODE old_key=$old_key new_key=$new_key members=$count" >&2
done <<< "$keys"
echo "legacy migration complete total=$total mode=$MODE" >&2
