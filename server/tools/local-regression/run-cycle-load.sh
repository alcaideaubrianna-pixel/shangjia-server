#!/usr/bin/env bash
# 固定本地环境：1000份完整资料，真实首次发布和真实scheduler循环，不直接修改任务成功状态。
set -euo pipefail
cd "$(dirname "$0")/../.."
[[ "$(docker compose -f tools/local-regression/docker-compose.yml ps -q publish-worker | wc -l | tr -d ' ')" == 2 ]] || { echo '需要两个本地publish-worker' >&2; exit 1; }
curl --noproxy '*' --silent --fail --max-time 5 http://127.0.0.1:18080/readyz
YOUBAN_LOCAL_ACCOUNT_RUNTIME=1 YOUBAN_LOCAL_CYCLE_LOAD=1 go test ./addons/youban_publish/logic/sys \
  -run '^TestLocalCycleLoadRuntime$' -count=1 -v -timeout=15m
