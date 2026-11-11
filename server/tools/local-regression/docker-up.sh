#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
# 默认使用独立测试Redis，避免将实际运行测试连到宿主的其他业务队列。
if [[ "${YOUBAN_LOCAL_REDIS_ADDRESS:-host.docker.internal:16382}" == host.docker.internal:16382 ]]; then
  if docker inspect youban-runtime-clean-redis >/dev/null 2>&1; then
    docker start youban-runtime-clean-redis >/dev/null
  else
    docker run -d --name youban-runtime-clean-redis --restart unless-stopped \
      -p 127.0.0.1:16382:6379 -v youban-local-runtime-redis:/data redis:7 redis-server --appendonly yes >/dev/null
  fi
fi
if docker exec youban-postgres pg_isready -U postgres >/dev/null 2>&1; then
  # HotGo 在启动时固定使用 Asia/Shanghai，测试库必须使用同一时区，
  # 否则 timestamp without time zone 会让 watchdog 误判任务超时。
  docker exec youban-postgres psql -U postgres -d postgres -c \
    "ALTER DATABASE youban_hotgo SET timezone TO 'Asia/Shanghai';" >/dev/null
  # 仅固定本地测试库：显式升级计划字段，生产仍由运维单独执行迁移。
  docker exec -i youban-postgres psql -U postgres -d youban_hotgo -v ON_ERROR_STOP=1 \
    < ../../addons/youban_publish/resource/sql/cycle_plan_runtime.pgsql.sql >/dev/null
fi
replicas="${YOUBAN_LOCAL_PUBLISH_REPLICAS:-2}"
background_replicas="${YOUBAN_LOCAL_BACKGROUND_REPLICAS:-1}"
if ! [[ "$replicas" =~ ^[1-9][0-9]*$ ]]; then
  echo 'YOUBAN_LOCAL_PUBLISH_REPLICAS 必须为正整数' >&2
  exit 1
fi
if ! [[ "$background_replicas" =~ ^[1-9][0-9]*$ ]]; then
  echo 'YOUBAN_LOCAL_BACKGROUND_REPLICAS 必须为正整数' >&2
  exit 1
fi
if [[ -n "${YOUBAN_LOCAL_BUILD_PROXY:-}" ]]; then
  # 构建容器使用宿主代理地址；不依赖 Bash 4 的空数组展开行为。
  docker compose -f docker-compose.yml build --build-arg "HTTP_PROXY=$YOUBAN_LOCAL_BUILD_PROXY" --build-arg "HTTPS_PROXY=$YOUBAN_LOCAL_BUILD_PROXY"
else
  docker compose -f docker-compose.yml build
fi
docker compose -f docker-compose.yml up -d --no-build --scale "publish-worker=$replicas" --scale "background-worker=$background_replicas"
docker compose -f docker-compose.yml ps
