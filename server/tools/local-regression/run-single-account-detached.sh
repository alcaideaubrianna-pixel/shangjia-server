#!/usr/bin/env bash
set -euo pipefail

# 无交互单账号回归：进程、日志、PID 和最终 SQL 报告全部落盘。
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"
PID_DIR="${YOUBAN_LOCAL_PID_DIR:-/tmp/youban-local-regression}"
mkdir -p "$PID_DIR"

kill_old() {
  if [[ -f "$PID_DIR/worker.pid" ]]; then kill "$(cat "$PID_DIR/worker.pid")" 2>/dev/null || true; fi
  if [[ -f "$PID_DIR/hook.pid" ]]; then kill "$(cat "$PID_DIR/hook.pid")" 2>/dev/null || true; fi
}
kill_old

go build -o "$PID_DIR/youban-server" ./main.go
go build -o "$PID_DIR/tg-hook" ./tools/tg_hook_mock

nohup "$PID_DIR/tg-hook" --addr 127.0.0.1:19100 --record "$PID_DIR/hook.json" \
  --webhook-url "${YOUBAN_LOCAL_TG_WEBHOOK_URL:-http://127.0.0.1:18004/api/youban_tg_bot_gateway/telegram/webhook}" \
  >"$PID_DIR/hook.log" 2>&1 & echo $! >"$PID_DIR/hook.pid"

YOUBAN_LOCAL_REGRESSION=1 YOUBAN_TELEGRAM_LOCAL_BOT_ENABLED=1 \
YOUBAN_PUBLISH_PUSH_ENGINE="${YOUBAN_PUBLISH_PUSH_ENGINE:-}" \
YOUBAN_RUNTIME_ROLES=publish-worker YOUBAN_DB_DRIVER=pgsql \
YOUBAN_DB_HOST=127.0.0.1 YOUBAN_DB_PORT=5432 YOUBAN_DB_USER=postgres \
YOUBAN_DB_PASSWORD=postgres YOUBAN_DB_NAME=youban_hotgo \
YOUBAN_REDIS_ADDRESS=127.0.0.1:6379 YOUBAN_REDIS_DB=15 \
YOUBAN_TELEGRAM_RUNTIME_API_URL=http://127.0.0.1:19100 \
YOUBAN_TELEGRAM_BOT_API_URL=http://127.0.0.1:19100 \
  nohup "$PID_DIR/youban-server" publish-worker >"$PID_DIR/worker.log" 2>&1 & echo $! >"$PID_DIR/worker.pid"

echo "started pid_dir=$PID_DIR"
