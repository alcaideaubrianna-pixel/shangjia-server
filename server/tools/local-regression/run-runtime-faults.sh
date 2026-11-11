#!/usr/bin/env bash
# 仅重启固定的独立本地测试Redis，不清空任何队列；验证PG恢复意图及消费者自动重连。
set -euo pipefail
cd "$(dirname "$0")"
local_db_name="${YOUBAN_LOCAL_DB_NAME:-youban_hotgo}"
tag="fault$(date +%Y%m%d%H%M%S)-$$"
output="/tmp/youban-runtime-$tag"
mkdir -p "$output"
cleanup() {
  bash ./tgfake-control.sh >/dev/null 2>&1 || true
  if [[ -n "${crashed_worker:-}" && -n "${standby_worker:-}" ]]; then docker start "$crashed_worker" "$standby_worker" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT
sql() { docker exec youban-postgres psql -U postgres -d "$local_db_name" -X -v ON_ERROR_STOP=1 -Atc "$1"; }
wait_count() {
  local query="$1" expected="$2" deadline="$3" started="$SECONDS" value
  while (( SECONDS-started < deadline )); do
    value="$(sql "$query")"
    if (( value == expected )); then return 0; fi
    sleep 1
  done
  echo "故障恢复等待超时 expected=$expected actual=$value query=$query" >&2
  return 1
}
seed() { docker exec -i youban-postgres psql -U postgres -d "$local_db_name" -X -v ON_ERROR_STOP=1 -v "run_id=$1" -v "profiles=$2" -v "channels=$3" "${@:4}" < scenarios/runtime_load.sql >> "$output/seed.log"; }
verify() {
  wait_count "SELECT count(*) FROM hg_youban_publish_profile_state WHERE tenant_id=999 AND account_id=999 AND publish_operation_no LIKE 'profile_cycle:local:runtime:$1:%' AND COALESCE(publish_task_status,'')=''" "$2" 30
  bash verify-runtime-round.sh "$1" "$2" "$3" any > "$output/$1.json"
}
[[ "$(docker compose -f docker-compose.yml ps -q publish-worker | wc -l | tr -d ' ')" == 2 ]] || { echo '需要两个真实push worker' >&2; exit 1; }
for worker in $(docker compose -f docker-compose.yml ps -q publish-worker); do
  docker inspect "$worker" --format '{{range .Config.Env}}{{println .}}{{end}}' | rg -q '^YOUBAN_REDIS_ADDRESS=host.docker.internal:16382$'
  docker inspect "$worker" --format '{{range .Config.Env}}{{println .}}{{end}}' | rg -q '^YOUBAN_TELEGRAM_BOT_API_URL=http://tgfake:18790$'
done
[[ "$(docker inspect youban-runtime-clean-redis --format '{{(index (index .NetworkSettings.Ports "6379/tcp") 0).HostPort}}')" == 16382 ]] || { echo '目标不是独立测试Redis' >&2; exit 1; }
for worker in $(docker compose -f docker-compose.yml ps -q publish-worker); do docker inspect "$worker" --format '{{.Id}} {{.State.StartedAt}}'; done > "$output/workers-before.txt"
echo "真实故障场景开始：$output"
round="$tag-lost-ready"
seed "$round" 3 3 -v simulate_ready_loss=1
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE tenant_id=999 AND account_id=999 AND operation_no LIKE 'profile_cycle:local:runtime:$round:%' AND status='sent'" 9 60
verify "$round" 3 3
echo '通过：Redis就绪通知缺失，PG持久化意图恢复，9/9发送完成'
round="$tag-restart"
seed "$round" 10 20
wait_count "SELECT CASE WHEN count(*)>0 THEN 1 ELSE 0 END FROM hg_youban_publish_tg_job WHERE account_id=999 AND operation_no LIKE 'profile_cycle:local:runtime:$round:%' AND status='sent'" 1 30
docker restart --timeout 1 youban-runtime-clean-redis > "$output/redis-restart.log"
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE account_id=999 AND operation_no LIKE 'profile_cycle:local:runtime:$round:%' AND status='sent'" 200 150
verify "$round" 10 20
echo '通过：仅重启Redis，原worker自动恢复，200/200发送完成'
round="$tag-after-restart"
seed "$round" 3 20
# 新任务必须在陈旧任务恢复阈值30秒之前完成，防止恢复扫描掩盖消费者已死亡。
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE account_id=999 AND operation_no LIKE 'profile_cycle:local:runtime:$round:%' AND status='sent'" 60 25
verify "$round" 3 20
for worker in $(docker compose -f docker-compose.yml ps -q publish-worker); do docker inspect "$worker" --format '{{.Id}} {{.State.StartedAt}}'; done > "$output/workers-after.txt"
diff -u "$output/workers-before.txt" "$output/workers-after.txt"
echo "通过：Redis恢复后新任务即时消费，worker未重启；报告：$output"

# 强杀真实发送进程。先让另一个节点待机，确保已发出的请求确实属于被杀节点。
crashed_worker="$(docker compose -f docker-compose.yml ps -q publish-worker | head -1)"
standby_worker="$(docker compose -f docker-compose.yml ps -q publish-worker | tail -1)"
docker stop --timeout 5 "$standby_worker" > "$output/standby-stop.log"
chat="$(sql "SELECT target_chat_id FROM hg_youban_publish_channel WHERE tenant_id=999 AND channel_title LIKE 'local-hook-channel-%' AND status=1 ORDER BY id LIMIT 1")"
[[ "$chat" =~ ^-?[0-9]+$ ]] || { echo '虚拟频道标识无效' >&2; exit 1; }
  bash ./tgfake-control.sh "{\"default\":{\"action\":\"success\",\"delayMs\":2},\"rules\":[{\"method\":\"sendMediaGroup\",\"chatId\":\"$chat\",\"action\":\"success\",\"delayMs\":10000}]}"
round="$tag-killed-worker"
seed "$round" 1 1
wait_count "SELECT count(DISTINCT j.id) FROM hg_youban_publish_tg_job j JOIN hg_youban_publish_tg_attempt a ON a.job_id=j.id WHERE j.account_id=999 AND j.operation_no LIKE 'profile_cycle:local:runtime:$round:%' AND j.status='sending' AND j.send_phase='display_sending' AND a.status='sending'" 1 30
sleep 1
docker kill --signal KILL "$crashed_worker" > "$output/worker-kill.log"
docker start "$crashed_worker" "$standby_worker" > "$output/worker-restart.log"
  bash ./tgfake-control.sh
free_round="$tag-healthy-channel"
seed "$free_round" 1 1 -v profile_offset=1 -v channel_offset=1
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE account_id=999 AND operation_no LIKE 'profile_cycle:local:runtime:$free_round:%' AND status='sent'" 1 15
verify "$free_round" 1 1
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE account_id=999 AND operation_no LIKE 'profile_cycle:local:runtime:$round:%' AND status='sent'" 1 100
verify "$round" 1 1
wait_count "SELECT count(*) FROM hg_youban_publish_tg_attempt a JOIN hg_youban_publish_tg_job j ON j.id=a.job_id WHERE j.account_id=999 AND j.operation_no LIKE 'profile_cycle:local:runtime:$round:%'" 1 1
echo "通过：强杀发送节点后Webhook确认续跑，未重复发送，其他频道继续；报告：$output"
