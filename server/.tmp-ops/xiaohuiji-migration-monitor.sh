#!/usr/bin/env bash
set -Eeuo pipefail

source /etc/xiaohuiji-migration-monitor.env
LOG=/tmp/pgcopydb-xiaohuiji.log
STATE=/tmp/xiaohuiji-migration-monitor.state
PROGRESS_STATE=/tmp/xiaohuiji-migration-progress.state
THRESHOLD_STATE=/tmp/xiaohuiji-migration-threshold.state
ENV_FILE=/tmp/xiaohuiji-pg.env

notify() {
  curl -fsS --max-time 15 -X POST "https://api.telegram.org/bot${MIGRATION_TG_BOT_TOKEN}/sendMessage" \
    --data-urlencode "chat_id=${MIGRATION_TG_CHAT_ID}" \
    --data-urlencode "text=$1" >/dev/null 2>&1 || true
}

phase() {
  if grep -q 'STEP 4: starting' "$LOG" 2>/dev/null; then
    if grep -q 'STEP 5\|STEP 6: starting' "$LOG" 2>/dev/null; then
      printf '索引与约束'
    else
      printf '数据复制'
    fi
  elif grep -q 'Fetched information for' "$LOG" 2>/dev/null; then
    printf '准备复制'
  else
    printf '初始化'
  fi
}

format_seconds() {
  local seconds="${1:-0}"
  ((seconds < 0)) && seconds=0
  printf '%02dh %02dm' $((seconds / 3600)) $(((seconds % 3600) / 60))
}

finish_time() {
  local seconds="${1:-0}"
  ((seconds > 0)) || { printf '暂无法估算'; return; }
  date -d "+${seconds} seconds" '+%Y-%m-%d %H:%M:%S %Z' 2>/dev/null || date -u -r "$(( $(date +%s) + seconds ))" '+%Y-%m-%d %H:%M:%S UTC'
}

progress_message() {
  local src dst percent previous_size previous_time now elapsed rate remaining threshold
  src="$(sed -n 's/^PGCOPYDB_SOURCE_PGURI=//p' "$ENV_FILE" | sed 's#10.5.4.5:5432#127.0.0.1:15432#')"
  dst="$(sed -n 's/^PGCOPYDB_TARGET_PGURI=//p' "$ENV_FILE")"
  src_size="$(psql "$src" -Atqc 'SELECT pg_database_size(current_database())' 2>/dev/null || echo 0)"
  dst_size="$(psql "$dst" -Atqc 'SELECT pg_database_size(current_database())' 2>/dev/null || echo 0)"
  [[ "$src_size" =~ ^[0-9]+$ && "$dst_size" =~ ^[0-9]+$ && "$src_size" -gt 0 ]] || return 0
  percent=$((dst_size * 100 / src_size))
  ((percent > 99)) && percent=99
  now="$(date +%s)"
  previous_size=0
  previous_time="$now"
  [[ -f "$PROGRESS_STATE" ]] && read -r previous_size previous_time < "$PROGRESS_STATE" || true
  printf '%s %s\n' "$dst_size" "$now" > "$PROGRESS_STATE"
  elapsed=$((now - previous_time))
  rate=0
  ((elapsed > 0 && dst_size > previous_size)) && rate=$(((dst_size - previous_size) / elapsed))
  remaining=0
  ((rate > 0)) && remaining=$(((src_size - dst_size) / rate))
  threshold=$((percent / 10 * 10))
  [[ "$threshold" -lt 10 ]] && return 0
  [[ "${last_threshold:-0}" -ge "$threshold" ]] && return 0
  last_threshold="$threshold"
  printf '%s\n' "$last_threshold" > "$THRESHOLD_STATE"
  notify "迁移进度通知\n当前进度：${percent}%\n剩余进度：$((100 - percent))%\n预计剩余：$(format_seconds "$remaining")\n预计结束：$(finish_time "$remaining")\n当前阶段：$(phase)\n目标库：$((dst_size / 1024 / 1024))MB / $((src_size / 1024 / 1024))MB"
}

last=''
[[ -f "$STATE" ]] && last="$(cat "$STATE")"
last_threshold=0
[[ -f "$THRESHOLD_STATE" ]] && last_threshold="$(cat "$THRESHOLD_STATE" 2>/dev/null || echo 0)"
while :; do
  current="$(phase)"
  if [[ "$current" != "$last" ]]; then
    notify "迁移阶段通知\n阶段：${current}\n时间：$(date '+%Y-%m-%d %H:%M:%S %Z')\n并行 PostgreSQL 迁移正在运行。"
    printf '%s' "$current" > "$STATE"
    last="$current"
  fi
  progress_message
  if ! pgrep -f '^pgcopydb clone ' >/dev/null 2>&1; then
    if grep -q 'ERROR\|FATAL' "$LOG" 2>/dev/null; then
      notify "迁移失败\n时间：$(date '+%Y-%m-%d %H:%M:%S %Z')\n请检查中转机日志：${LOG}"
    else
      notify "迁移进程已结束\n时间：$(date '+%Y-%m-%d %H:%M:%S %Z')\n请执行最终一致性校验。"
    fi
    exit 0
  fi
  sleep 30
done
