#!/usr/bin/env bash
set -euo pipefail

# 清理数据库已经终态化、但仍残留在频道 Redis ZSET 中的旧投影。
# 默认 dry-run；只允许按 job_id 做 ZREM，不修改 PostgreSQL 业务状态。
#
# 用法：
#   PGURL='...' REDIS_HOST=... REDIS_DB=3 REDISCLI_AUTH='...' \
#     ./tools/cleanup-stale-channel-queue.sh --dry-run
#   ... ./tools/cleanup-stale-channel-queue.sh --execute

MODE=dry-run
BATCH_SIZE=${BATCH_SIZE:-500}
MAX_ROWS=${MAX_ROWS:-0}
REDIS_HOST=${REDIS_HOST:-}
REDIS_PORT=${REDIS_PORT:-6379}
REDIS_DB=${REDIS_DB:-3}
REDIS_URL=${REDIS_URL:-}
PGURL=${PGURL:-}
START_AFTER_ID=${START_AFTER_ID:-0}

usage() {
  sed -n '1,18p' "$0"
}

while (($#)); do
  case "$1" in
    --dry-run) MODE=dry-run ;;
    --execute) MODE=execute ;;
    --batch-size) BATCH_SIZE=$2; shift ;;
    --max-rows) MAX_ROWS=$2; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

[[ "$BATCH_SIZE" =~ ^[1-9][0-9]*$ ]] || { echo "BATCH_SIZE must be positive" >&2; exit 2; }
[[ "$MAX_ROWS" =~ ^[0-9]+$ ]] || { echo "MAX_ROWS must be non-negative" >&2; exit 2; }
[[ "$START_AFTER_ID" =~ ^[0-9]+$ ]] || { echo "START_AFTER_ID must be non-negative" >&2; exit 2; }
[[ -n "$PGURL" ]] || { echo "PGURL is required" >&2; exit 2; }
command -v psql >/dev/null || { echo "psql is required" >&2; exit 127; }
command -v redis-cli >/dev/null || { echo "redis-cli is required" >&2; exit 127; }
if [[ "$MODE" == execute && -z "${REDISCLI_AUTH:-}" && -z "$REDIS_URL" ]]; then
  echo "REDISCLI_AUTH or REDIS_URL is required for --execute" >&2
  exit 2
fi

redis_cmd() {
  if [[ -n "$REDIS_HOST" ]]; then
    REDISCLI_AUTH="${REDISCLI_AUTH:-}" redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" "$@"
  else
    redis-cli -u "$REDIS_URL" -n "$REDIS_DB" "$@"
  fi
}

count_sql="SELECT count(*) FROM hg_youban_publish_tg_job WHERE status='superseded' AND dispatch_status='done' AND send_phase='skipped_stale_cycle';"
candidate_sql="SELECT id, channel_id FROM hg_youban_publish_tg_job WHERE id > :last_id AND status='superseded' AND dispatch_status='done' AND send_phase='skipped_stale_cycle' ORDER BY id LIMIT :batch_size;"

total=$(psql "$PGURL" -X -qAt -v ON_ERROR_STOP=1 -c "$count_sql")
echo "目标终态投影=${total} batch=${BATCH_SIZE} mode=${MODE}"
[[ "$MODE" == dry-run ]] && exit 0
[[ "$total" != 0 ]] || { echo "没有需要清理的任务"; exit 0; }

# 单机互斥；中断后重新执行仍安全，ZREM 本身幂等。
lock_dir="${TMPDIR:-/tmp}/youban-stale-channel-cleanup.lock"
mkdir "$lock_dir" 2>/dev/null || { echo "已有清理进程持锁：$lock_dir" >&2; exit 1; }
trap 'rmdir "$lock_dir"' EXIT
last_id=$START_AFTER_ID
processed=0
removed=0
while :; do
  limit=$BATCH_SIZE
  if (( MAX_ROWS > 0 && MAX_ROWS - processed < limit )); then
    limit=$((MAX_ROWS - processed))
  fi
  (( limit > 0 )) || break

  candidate_sql="SELECT id, channel_id FROM hg_youban_publish_tg_job WHERE id > ${last_id} AND status='superseded' AND dispatch_status='done' AND send_phase='skipped_stale_cycle' ORDER BY id LIMIT ${limit};"
  rows=$(psql "$PGURL" -X -qAt -F $'\t' -v ON_ERROR_STOP=1 -c "$candidate_sql")
  [[ -n "$rows" ]] || break

  batch_count=0
  while IFS=$'\t' read -r job_id channel_id; do
    [[ "$job_id" =~ ^[0-9]+$ && "$channel_id" =~ ^[0-9]+$ && "$channel_id" -gt 0 ]] || {
      echo "数据库返回非法任务标识，停止执行" >&2; exit 1;
    }
    last_id=$job_id
    ((batch_count+=1))
  done <<< "$rows"

  grouped=$(awk '{ids[$2]=ids[$2]" "$1} END {for (channel in ids) print channel,ids[channel]}' <<< "$rows")
  while read -r channel_id ids; do
    read -r -a members <<< "$ids"
    for prefix in youban:tg:channel:queue: youban:tg:channel:urgent: youban:tg:channel:history: youban_publish:tg:channel_pending:; do
      removed_here=$(redis_cmd --raw ZREM "${prefix}${channel_id}" "${members[@]}")
      [[ "$removed_here" =~ ^[0-9]+$ ]] || {
        echo "Redis 清理失败，停止执行 channel=${channel_id}" >&2; exit 1;
      }
      removed=$((removed + removed_here))
    done
  done <<< "$grouped"

  processed=$((processed + batch_count))
  echo "processed=${processed}/${total} redis_removed=${removed} last_id=${last_id}" >&2
  (( batch_count < limit )) && break
done
echo "清理完成：检查=${processed} Redis 删除成员=${removed}"
