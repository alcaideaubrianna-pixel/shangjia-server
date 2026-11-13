#!/usr/bin/env bash
set -euo pipefail

# 本地 SAE 等价回归入口：所有角色共用本地 PostgreSQL、Redis 和 Telegram Hook Mock。
# 默认不启动 account（该角色需要 MTProto 登录态）；传 --with-account 才启动。
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"

HOOK_ADDR="${YOUBAN_LOCAL_TG_HOOK_ADDR:-127.0.0.1:19100}"
# 默认使用专用回归库，避免误连开发库或残留历史任务污染真实链路验证。
PG_DB="${YOUBAN_LOCAL_DB_NAME:-youban_tgfake_test}"
PG_LINK="pgsql:postgres:postgres@tcp(127.0.0.1:5432)/${PG_DB}"
REDIS_ADDR="${YOUBAN_LOCAL_REDIS_ADDR:-127.0.0.1:6379}"
SCENARIO="${YOUBAN_LOCAL_TG_SCENARIO:-$ROOT_DIR/tools/local-regression/scenarios/display_verify_success.json}"
HOOK_RECORD="${YOUBAN_LOCAL_TG_HOOK_RECORD:-}"
HOOK_EXTERNAL="${YOUBAN_LOCAL_TG_HOOK_EXTERNAL:-0}"
HOOK_WEBHOOK_URL="${YOUBAN_LOCAL_TG_HOOK_WEBHOOK_URL:-}"
HOOK_WEBHOOK_KEY="${YOUBAN_LOCAL_TG_HOOK_WEBHOOK_KEY:-}"
WITH_ACCOUNT=0
if [[ "${1:-}" == "--with-account" ]]; then WITH_ACCOUNT=1; fi

command -v docker >/dev/null
docker exec youban-postgres pg_isready -U postgres >/dev/null
docker exec r1rpc-redis redis-cli ping | grep -q PONG

# 需要显式开启才重建本地数据库；禁止使用生产库名称，避免测试脚本误删真实数据。
if [[ "${YOUBAN_LOCAL_RESET_DB:-0}" == "1" ]]; then
  case "$PG_DB" in
    railway|youban_hotgo|youban_production|postgres)
      echo "拒绝重建疑似共享数据库: $PG_DB" >&2
      exit 1
      ;;
  esac
  YOUBAN_LOCAL_DB_NAME="$PG_DB" ./tools/local-regression/reset-tgfake-db.sh
fi

# 本地测试库可选应用插件升级 DDL，确保实际运行角色拥有生产同版本表结构。
if [[ "${YOUBAN_LOCAL_APPLY_SCHEMA:-1}" == "1" ]]; then
  psql_cmd=(docker exec -i youban-postgres psql -U postgres -d "$PG_DB")
  "${psql_cmd[@]}" < "$ROOT_DIR/addons/youban_publish/resource/sql/upgrade.pgsql.sql" >/tmp/youban-local-schema-upgrade.log 2>&1
  "${psql_cmd[@]}" < "$ROOT_DIR/addons/youban_publish/resource/sql/upgrade_safe.pgsql.sql" >>/tmp/youban-local-schema-upgrade.log 2>&1
  "${psql_cmd[@]}" < "$ROOT_DIR/addons/youban_publish/resource/sql/upgrade_online.pgsql.sql" >>/tmp/youban-local-schema-upgrade.log 2>&1
  "${psql_cmd[@]}" < "$ROOT_DIR/addons/youban_publish/resource/sql/channel_permission_circuit.pgsql.sql" >>/tmp/youban-local-schema-upgrade.log 2>&1
  "${psql_cmd[@]}" < "$ROOT_DIR/addons/youban_publish/resource/sql/telegram_send_watchdog.pgsql.sql" >>/tmp/youban-local-schema-upgrade.log 2>&1
  "${psql_cmd[@]}" < "$ROOT_DIR/addons/youban_publish/resource/sql/channel_dispatch_engine.pgsql.sql" >>/tmp/youban-local-schema-upgrade.log 2>&1
  "${psql_cmd[@]}" < "$ROOT_DIR/addons/youban_publish/resource/sql/collect_process_history.pgsql.sql" >>/tmp/youban-local-schema-upgrade.log 2>&1
  echo 'ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS bot_type varchar(32) NOT NULL DEFAULT '\''normal'\'';' | "${psql_cmd[@]}" >>/tmp/youban-local-schema-upgrade.log 2>&1
fi

PIDS=()
if [[ "$HOOK_EXTERNAL" != "1" ]]; then
  hook_args=(--addr "$HOOK_ADDR" --scenario "$SCENARIO")
  if [[ -n "$HOOK_RECORD" ]]; then hook_args+=(--record "$HOOK_RECORD"); fi
  go run ./tools/tg_hook_mock "${hook_args[@]}" >/tmp/youban-tg-hook.log 2>&1 &
  HOOK_PID=$!
  PIDS=($HOOK_PID)
fi
cleanup() { kill "${PIDS[@]}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
sleep 1

export YOUBAN_DB_DRIVER=pgsql YOUBAN_DB_HOST=127.0.0.1 YOUBAN_DB_PORT=5432
export YOUBAN_DB_USER=postgres YOUBAN_DB_PASSWORD=postgres YOUBAN_DB_NAME="$PG_DB"
# GoFrame 的默认数据库配置读取完整 link；只设置 YOUBAN_DB_NAME 会让部分角色
# 回退到 youban_hotgo，导致采集、媒体和发布角色不在同一测试库中。
export YOUBAN_DATABASE_LINK="pgsql:postgres:postgres@tcp(127.0.0.1:5432)/$PG_DB"
export YOUBAN_REDIS_ADDRESS="$REDIS_ADDR" YOUBAN_REDIS_DB="${YOUBAN_LOCAL_REDIS_DB:-15}"
export YOUBAN_TELEGRAM_RUNTIME_API_URL="http://$HOOK_ADDR"
export YOUBAN_TELEGRAM_BOT_API_URL="http://$HOOK_ADDR"
export YOUBAN_TELEGRAM_LOCAL_BOT_ENABLED=1
export YOUBAN_LOCAL_REGRESSION=1

roles=(scheduler collector-worker collect-process-worker media-worker publish-worker background-worker)
if (( WITH_ACCOUNT )); then roles+=(account); fi
LOCAL_SERVER_BIN="${YOUBAN_LOCAL_SERVER_BIN:-/tmp/youban-local-server}"
go build -o "$LOCAL_SERVER_BIN" ./main.go
base_port="${YOUBAN_LOCAL_HTTP_PORT_BASE:-18000}"
index=0
for role in "${roles[@]}"; do
  role_port=$((base_port + index))
  GF_SERVER_ADDRESS=":${role_port}" YOUBAN_SERVER_ADDRESS=":${role_port}" YOUBAN_RUNTIME_ROLES="$role" YOUBAN_LOCAL_REGRESSION=1 \
    "$LOCAL_SERVER_BIN" "$role" >/tmp/youban-local-${role}.log 2>&1 &
  PIDS+=("$!")
  index=$((index + 1))
done

echo "local roles started: ${roles[*]}"
echo "database: $PG_LINK"
echo "telegram hook: http://$HOOK_ADDR"
echo "press Ctrl-C to stop"
wait
