#!/usr/bin/env bash
# 重建隔离回归库后启动采集、媒体、处理、调度和发布全角色。
# 仅允许本地专用数据库，避免历史任务和 Redis 状态污染完整链路结果。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"
db="${YOUBAN_LOCAL_DB_NAME:-youban_tgfake_test}"
case "$db" in
  railway|youban_hotgo|youban_production|postgres)
    echo "拒绝使用共享数据库执行本地完整回归: $db" >&2
    exit 1
    ;;
esac

YOUBAN_LOCAL_DB_NAME="$db" ./tools/local-regression/reset-tgfake-db.sh
YOUBAN_LOCAL_DB_NAME="$db" YOUBAN_LOCAL_RESET_DB=0 ./tools/local-regression/run-all-roles.sh
