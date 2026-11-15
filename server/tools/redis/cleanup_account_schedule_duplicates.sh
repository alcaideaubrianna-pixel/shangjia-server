#!/usr/bin/env bash
set -euo pipefail

# 清理账户调度队列中的重复任务。
# 使用方式：
#   REDISCLI_AUTH='...' ./cleanup_account_schedule_duplicates.sh
#
# 默认只处理 DB3 和目标队列。Asynq 的 msg 是 protobuf，不能直接按 JSON
# 解析；脚本使用任务 hash 中的 unique_key 去重，每个账户唯一键保留一条任务。

REDIS_HOST="${REDIS_HOST:-172.19.57.135}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_DB="${REDIS_DB:-3}"
QUEUE="${QUEUE:-youban_publish_account_schedule}"
BATCH_SIZE="${BATCH_SIZE:-5000}"
MAX_ITEMS="${MAX_ITEMS:-0}"

redis() {
  redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" --user "${REDIS_USER:-default}" --no-auth-warning "$@"
}

if [[ -z "${REDISCLI_AUTH:-}" ]]; then
  echo "缺少 REDISCLI_AUTH，拒绝执行清理" >&2
  exit 2
fi

pending_key="asynq:{${QUEUE}}:pending"
tmp_key="youban:cleanup:${QUEUE}:tmp:$$"
seen_key="youban:cleanup:${QUEUE}:seen:$$"

cleanup=''
read -r -d '' cleanup <<'LUA' || true
local pending = KEYS[1]
local tmp = KEYS[2]
local seen = KEYS[3]
local task_prefix = ARGV[1]
local limit = tonumber(ARGV[2])
local moved = 0
local kept = 0
local removed = 0
local malformed = 0

while moved < limit do
  local task_id = redis.call("LMOVE", pending, tmp, "RIGHT", "LEFT")
  if not task_id then break end
  moved = moved + 1
  local task_key = task_prefix .. task_id
  local scope = redis.call("HGET", task_key, "unique_key")
  if not scope then
    malformed = malformed + 1
    redis.call("RPUSH", pending, task_id)
  elseif redis.call("SADD", seen, scope) == 1 then
    kept = kept + 1
    redis.call("RPUSH", pending, task_id)
  else
    removed = removed + 1
    redis.call("DEL", task_key)
  end
  redis.call("LPOP", tmp)
end

return {moved, kept, removed, malformed, redis.call("LLEN", pending)}
LUA

initial="$(redis LLEN "$pending_key")"
echo "开始清理 queue=${QUEUE} db=${REDIS_DB} pending=${initial} batch=${BATCH_SIZE}"

if [[ "$initial" == "0" ]]; then
  echo "队列为空"
  exit 0
fi

remaining="$initial"
if (( MAX_ITEMS > 0 && remaining > MAX_ITEMS )); then
  remaining="$MAX_ITEMS"
fi
total_removed=0
while (( remaining > 0 )); do
  step="$BATCH_SIZE"
  (( step > remaining )) && step="$remaining"
  result="$(redis --eval <(printf '%s' "$cleanup") "$pending_key" "$tmp_key" "$seen_key" , "asynq:{${QUEUE}}:t:" "$step")"
  mapfile -t values <<<"$result"
  moved="${values[0]:-0}"
  kept="${values[1]:-0}"
  removed="${values[2]:-0}"
  malformed="${values[3]:-0}"
  current="${values[4]:-0}"
  total_removed=$((total_removed + removed))
  remaining=$((remaining - moved))
  echo "moved=${moved} kept=${kept} removed=${removed} malformed=${malformed} pending=${current} remaining=${remaining}"
  (( moved == 0 )) && break
done

redis DEL "$tmp_key" >/dev/null
redis EXPIRE "$seen_key" 3600 >/dev/null
echo "清理完成：removed=${total_removed} pending=$(redis LLEN "$pending_key")"
