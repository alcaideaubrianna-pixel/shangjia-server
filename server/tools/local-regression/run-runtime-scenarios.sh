#!/usr/bin/env bash
# 在真实本地角色上执行发送场景；只访问固定Docker测试库和Hook，绝不连接Telegram。
set -euo pipefail
cd "$(dirname "$0")"
local_db_name="${YOUBAN_LOCAL_DB_NAME:-youban_hotgo}"
run_tag="rt$(date +%Y%m%d%H%M%S)-$$"
output="${YOUBAN_LOCAL_RUNTIME_OUTPUT:-/tmp/youban-runtime-$run_tag}"
mkdir -p "$output"
sql() { docker exec youban-postgres psql -U postgres -d "$local_db_name" -X -v ON_ERROR_STOP=1 -Atc "$1"; }
control() {
  if [[ "${YOUBAN_LOCAL_TG_BACKEND:-tgfake}" == "tg-hook" ]]; then
    local status
    status="$(curl --noproxy '*' -sS --fail --max-time 5 -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:19100/__test/scenario -H 'Content-Type: application/json' --data-binary "$1")"
    [[ "$status" == "204" ]] || { echo 'Hook控制接口无效' >&2; return 1; }
    return
  fi
  bash ./tgfake-control.sh "$1"
}
reset_hook() { control '{"default":{"action":"success","delayMs":2}}'; }
trap 'reset_hook >/dev/null 2>&1 || true' EXIT
wait_count() {
  local query="$1" expected="$2" timeout="$3" started="$SECONDS" value
  while (( SECONDS-started < timeout )); do
    value="$(sql "$query")"
    if (( value >= expected )); then return 0; fi
    sleep 2
  done
  echo "等待运行结果超时：expected=$expected actual=$value query=$query" >&2
  return 1
}
round_filter() { printf "account_id=999 AND operation_no LIKE 'profile_cycle:local:runtime:%s:%%'" "$1"; }
seed_round() {
  docker exec -i youban-postgres psql -U postgres -d "$local_db_name" -X -v ON_ERROR_STOP=1 -v "run_id=$1" -v "profiles=$2" -v "channels=$3" < scenarios/runtime_load.sql >> "$output/seed.log"
}
verify_round() {
  # 资料投影走后台队列，等待真实收口后再审计；超时仍判失败。
  wait_count "SELECT count(*) FROM hg_youban_publish_profile_state WHERE tenant_id=999 AND account_id=999 AND publish_operation_no LIKE 'profile_cycle:local:runtime:$1:%' AND COALESCE(publish_task_status,'')=''" "$2" 30
  bash verify-runtime-round.sh "$1" "$2" "$3" any > "$output/$1.json"
}

# 启动边界必须与线上角色一致，回调不能发给只有健康路由的push角色。
count="$(docker compose -f docker-compose.yml ps -q publish-worker | wc -l | tr -d ' ')"
[[ "$count" == 2 ]] || { echo '运行场景要求恰好两个publish-worker' >&2; exit 1; }
(( $(sql "SELECT count(*) FROM hg_content_profile WHERE source_type='local_regression'") >= 720 )) || { echo '缺少完整的1000份本地资料夹具' >&2; exit 1; }
curl --noproxy '*' -sS --fail --max-time 5 http://127.0.0.1:18080/readyz > "$output/web-ready.json"
for worker in $(docker compose -f docker-compose.yml ps -q publish-worker); do
  docker inspect "$worker" --format '{{range .Config.Env}}{{println .}}{{end}}' | rg -q '^YOUBAN_TELEGRAM_BOT_API_URL=http://tgfake:18790$'
done
reset_hook

echo "真实角色场景开始，报告目录：$output"
round="$run_tag-success"
seed_round "$round" 10 100
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE $(round_filter "$round") AND status='sent'" 1000 240
wait_count "SELECT count(*) FROM hg_youban_publish_profile_state WHERE tenant_id=999 AND publish_operation_no LIKE 'profile_cycle:local:runtime:$round:%' AND COALESCE(publish_task_status,'')=''" 10 30
verify_round "$round" 10 100
echo '通过：1000条真实发送、消息证据、频道顺序和资料投影'

# 让回调晚于HTTP失败20秒；期间本频道第二条不能开始，其他频道继续。
control '{"default":{"action":"success","delayMs":2},"rules":[{"method":"sendMediaGroup","chatId":"-100999000012","action":"accepted_timeout","webhookDelayMs":20000}]}'
round="$run_tag-ambiguous"
seed_round "$round" 2 100
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE $(round_filter "$round") AND target_chat_id='-100999000012' AND status IN ('unknown','confirming')" 1 150
sql "SELECT json_build_object('at',clock_timestamp(),'attempts',(SELECT count(*) FROM hg_youban_publish_tg_attempt a JOIN hg_youban_publish_tg_job j ON j.id=a.job_id WHERE j.account_id=999 AND j.operation_no LIKE 'profile_cycle:local:runtime:$round:%' AND j.target_chat_id='-100999000012'),'other_sent',(SELECT count(*) FROM hg_youban_publish_tg_job WHERE $(round_filter "$round") AND target_chat_id<>'-100999000012' AND status='sent'))" > "$output/unknown-isolation.json"
attempts="$(sql "SELECT count(*) FROM hg_youban_publish_tg_attempt a JOIN hg_youban_publish_tg_job j ON j.id=a.job_id WHERE j.account_id=999 AND j.operation_no LIKE 'profile_cycle:local:runtime:$round:%' AND j.target_chat_id='-100999000012'")"
[[ "$attempts" == 1 ]] || { echo '未知发送期间本频道后续任务越过了屏障' >&2; exit 1; }
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE $(round_filter "$round") AND target_chat_id<>'-100999000012' AND status='sent'" 198 30
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE $(round_filter "$round") AND status='sent'" 200 120
verify_round "$round" 2 100
echo '通过：响应丢失、延迟Webhook、未知发送屏障及跨频道隔离'

control @scenarios/slow_channel_isolation.json
round="$run_tag-slow"
seed_round "$round" 3 100
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE $(round_filter "$round") AND target_chat_id='-100999000013' AND status='sent'" 3 45
sql "SELECT json_build_object('at',clock_timestamp(),'pending_cleanup',count(*)) FROM hg_youban_publish_tg_outbox o JOIN hg_youban_publish_tg_job j ON j.id=o.job_id WHERE o.event_type LIKE 'telegram_job.cleanup.%' AND o.status<>'delivered' AND j.tenant_id=999 AND j.target_chat_id='-100999000013'" > "$output/slow-delete-isolation.json"
python3 -c 'import json,sys;r=json.load(open(sys.argv[1]));assert r["pending_cleanup"]>0,r' "$output/slow-delete-isolation.json"
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE $(round_filter "$round") AND status='sent'" 300 120
verify_round "$round" 3 100
echo '通过：慢TG发送与慢删除互不阻塞其他频道'

reset_hook
mix="$run_tag-mix"
  docker exec -i youban-postgres psql -U postgres -d "$local_db_name" -X -v ON_ERROR_STOP=1 -v "run_id=$mix" -v phase=history < scenarios/mixed_load.sql >> "$output/seed.log"
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE account_id=999 AND operation_no LIKE '%:local:mix:$mix:%' AND status='sent'" 1 30
docker exec -i youban-postgres psql -U postgres -d "$local_db_name" -X -v ON_ERROR_STOP=1 -v "run_id=$mix" -v phase=live < scenarios/mixed_load.sql >> "$output/seed.log"
wait_count "SELECT count(*) FROM hg_youban_publish_tg_job WHERE account_id=999 AND operation_no LIKE '%:local:mix:$mix:%' AND status='sent'" 720 360
sql "WITH jobs AS (SELECT *,send_deadline_at-interval '21 minutes' started_at FROM hg_youban_publish_tg_job WHERE account_id=999 AND operation_no LIKE '%:local:mix:$mix:%'), ordered AS (SELECT sent_at,lag(sent_at) OVER(PARTITION BY target_chat_id,priority ORDER BY created_at,id) previous_sent FROM jobs) SELECT json_build_object('total',(SELECT count(*) FROM jobs),'sent',(SELECT count(*) FROM jobs WHERE status='sent'),'order_violations',(SELECT count(*) FROM ordered WHERE sent_at<previous_sent),'priority_violations',(SELECT count(*) FROM jobs low WHERE low.priority>1 AND EXISTS(SELECT 1 FROM jobs manual WHERE manual.priority=1 AND manual.target_chat_id=low.target_chat_id AND manual.created_at+interval '100 milliseconds'<low.started_at AND manual.sent_at>low.started_at)),'manual_p50_seconds',(SELECT percentile_cont(.5) WITHIN GROUP(ORDER BY extract(epoch FROM sent_at-created_at)) FROM jobs WHERE priority=1),'manual_p95_seconds',(SELECT percentile_cont(.95) WITHIN GROUP(ORDER BY extract(epoch FROM sent_at-created_at)) FROM jobs WHERE priority=1))" > "$output/mixed-load.json"
python3 -c 'import json,sys;r=json.load(open(sys.argv[1]));assert r["total"]==r["sent"]==720 and r["order_violations"]==r["priority_violations"]==0,r' "$output/mixed-load.json"
echo '通过：500历史积压下插入100实时采集、100循环和20手工任务'
echo "真实运行场景完成：$output"
