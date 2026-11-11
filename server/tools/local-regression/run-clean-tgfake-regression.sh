#!/usr/bin/env bash
# 重建独立 tgfake 回归环境后运行真实角色测试，避免历史任务污染结果。
set -euo pipefail
cd "$(dirname "$0")"

db="${YOUBAN_LOCAL_DB_NAME:-youban_tgfake_test}"
compose=(docker compose -f docker-compose.yml)

"${compose[@]}" stop publish-worker background-worker scheduler web media-worker >/dev/null 2>&1 || true
YOUBAN_LOCAL_DB_NAME="$db" ./reset-tgfake-db.sh
curl --noproxy '*' -fsS -X POST http://127.0.0.1:18790/sim/reset >/dev/null
YOUBAN_LOCAL_DB_NAME="$db" ./prepare-tgfake-media.sh

YOUBAN_LOCAL_DB_NAME="$db" "${compose[@]}" up -d \
  --scale publish-worker=2 --scale background-worker=2 \
  publish-worker background-worker scheduler web media-worker
sleep "${YOUBAN_LOCAL_READY_WAIT:-5}"

YOUBAN_LOCAL_DB_NAME="$db" \
YOUBAN_LOCAL_POLL_INTERVAL="${YOUBAN_LOCAL_POLL_INTERVAL:-0.2}" \
YOUBAN_SKIP_AMBIGUOUS="${YOUBAN_SKIP_AMBIGUOUS:-1}" \
  bash ./run-runtime-scenarios.sh
