#!/usr/bin/env bash
set -euo pipefail

# 本地完整回归入口：依赖 run-all-roles.sh 的同一套本地 PG/Redis/Telegram Mock。
# 该脚本只执行数据库/队列/发送策略测试，不会访问真实 Telegram。
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"

docker exec youban-postgres pg_isready -U postgres >/dev/null
docker exec r1rpc-redis redis-cli ping | grep -q PONG

YOUBAN_TELEGRAM_RUNTIME_API_URL="http://127.0.0.1:19100" \
YOUBAN_TELEGRAM_BOT_API_URL="http://127.0.0.1:19100" \
go test ./addons/youban_publish/logic/sys \
  -run 'Test(.*Cycle.*|.*ProfileCycle.*|.*Channel.*|.*Outbox.*|.*Priority.*|.*Dispatch.*|.*Queue.*|.*Telegram.*Delivery.*)' \
  -count=1 -timeout=10m -v

# 资料级循环高负载回放库：10,000 资料、40,000 频道任务，独立于业务库。
if docker exec youban-postgres psql -U postgres -d cycle_replay -Atc 'select 1' >/dev/null 2>&1; then
  docker exec -i youban-postgres psql -U postgres -d cycle_replay < "$ROOT_DIR/addons/youban_publish/devtools/profile_cycle_replay.sql" >/tmp/youban-cycle-replay.log
  docker exec youban-postgres psql -U postgres -d cycle_replay -Atc \
    "select count(*) from hg_youban_publish_profile_cycle_run; select count(*) from hg_youban_publish_profile_cycle_run_channel;" \
    | tee /tmp/youban-cycle-replay-counts.log
fi

echo "local regression passed"
