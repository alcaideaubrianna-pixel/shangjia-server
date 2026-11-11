#!/usr/bin/env bash
# 运行真实业务入口和scheduler，而非只运行模拟单测；只使用固定本地PG/Redis/Hook。
set -euo pipefail
cd "$(dirname "$0")/../.."
ready=0
deadline=$((SECONDS+30))
while (( SECONDS<deadline )); do
  ready=1
  for role in scheduler background-worker web; do
    [[ -n "$(docker compose -f tools/local-regression/docker-compose.yml ps -q "$role")" ]] || ready=0
  done
  [[ "$(docker compose -f tools/local-regression/docker-compose.yml ps -q publish-worker | wc -l | tr -d ' ')" == 2 ]] || ready=0
  if (( ready==1 )) && curl --noproxy '*' --silent --fail --max-time 2 http://127.0.0.1:18080/readyz; then break; fi
  ready=0
  sleep 1
done
(( ready==1 )) || { echo '本地角色30秒内未就绪；需要scheduler、web、background-worker和两个push worker' >&2; exit 1; }
YOUBAN_LOCAL_ACCOUNT_RUNTIME=1 go test ./addons/youban_publish/logic/sys \
  -run '^TestLocal(Account(Business|Cycle|Interval|ChannelFailure)|Cycle(Coverage|Protection|Commit))Runtime$' -count=1 -v -timeout=15m
