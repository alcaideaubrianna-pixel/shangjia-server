#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/../.."
RUN_DIR="${COLLECT_REPAIR_RUN_DIR:-/tmp/collect-repair-run}"
mkdir -p "$RUN_DIR"
chmod 700 "$RUN_DIR"
if pgrep -f '^/tmp/youban-collect-repair .*apply' >/dev/null; then
  echo '已有恢复进程运行，请先检查，避免重复启动。' >&2
  exit 1
fi
export COLLECT_REPAIR_SSH_PROXY_COMMAND="${COLLECT_REPAIR_SSH_PROXY_COMMAND-nc -x 127.0.0.1:6153 %h %p}"
export COLLECT_REPAIR_BOT_API_FORWARD="${COLLECT_REPAIR_BOT_API_FORWARD-127.0.0.1:15436:10.3.4.11:18081}"
export COLLECT_REPAIR_BOT_API_URL="${COLLECT_REPAIR_BOT_API_URL-http://127.0.0.1:15436}"
export COLLECT_REPAIR_BOT_FILE_FORWARD="${COLLECT_REPAIR_BOT_FILE_FORWARD-127.0.0.1:15437:10.3.4.11:18083}"
export COLLECT_REPAIR_BOT_FILE_URL="${COLLECT_REPAIR_BOT_FILE_URL-http://127.0.0.1:15437}"
export YOUBAN_TELEGRAM_PROXY_URL="${YOUBAN_TELEGRAM_PROXY_URL-}"
# Go HTTP 客户端只识别大写代理变量；内网转发保持直连。
export HTTP_PROXY="${HTTP_PROXY-${http_proxy-}}"
export HTTPS_PROXY="${HTTPS_PROXY-${https_proxy-}}"
export NO_PROXY="${NO_PROXY:-localhost,127.0.0.1}"
go build -o /tmp/youban-collect-repair ./tools/collect_repair
test -f /tmp/youban-collect-repair-env.json
test -f /tmp/run-youban-collect-repair.py
python3 -u /tmp/run-youban-collect-repair.py -apply -all-suspended -limit 100000 -concurrency "${COLLECT_REPAIR_CONCURRENCY:-8}" -report "$RUN_DIR/report.json" -ssh-host "${COLLECT_REPAIR_SSH_HOST:-ubuntu@43.159.54.73}" -ssh-identity "${COLLECT_REPAIR_SSH_IDENTITY:-/Users/ley/.ssh/id_hk_rsa}" -db-port 15433 -redis-port 16380 "$@" 2>&1 | tee "$RUN_DIR/progress.log"
