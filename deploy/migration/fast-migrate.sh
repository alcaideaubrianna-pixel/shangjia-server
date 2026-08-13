#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
STATE_DIR="${MIGRATION_STATE_DIR:-$REPO_DIR/.migration-state/fast}"
STATE_FILE="$STATE_DIR/state.env"
LOG_DIR="$STATE_DIR/logs"
PGCOPYDB_DIR="$STATE_DIR/pgcopydb"
REDISSHAKE_DIR="$STATE_DIR/redisshake"

SSH_HOST="${MIGRATION_SSH_HOST:-43.129.181.9}"
SSH_PORT="${MIGRATION_SSH_PORT:-22}"
SSH_USER="${MIGRATION_SSH_USER:-ubuntu}"
SSH_KEY="${MIGRATION_SSH_KEY:-$HOME/.ssh/id_xiaohuiji}"
RAILWAY_BIN="${MIGRATION_RAILWAY_BIN:-$HOME/.railway/bin/railway}"
RAILWAY_ENVIRONMENT="${MIGRATION_RAILWAY_ENVIRONMENT:-production}"
RAILWAY_POSTGRES_SERVICE="${MIGRATION_RAILWAY_POSTGRES_SERVICE:-Postgres}"
RAILWAY_REDIS_SERVICE="${MIGRATION_RAILWAY_REDIS_SERVICE:-Redis}"
SOURCE_PG_PORT="${MIGRATION_SOURCE_PG_PORT:-15432}"
SOURCE_REDIS_PORT="${MIGRATION_SOURCE_REDIS_PORT:-16379}"
TARGET_PG_PORT="${MIGRATION_TARGET_PG_PORT:-25432}"
TARGET_REDIS_PORT="${MIGRATION_TARGET_REDIS_PORT:-26379}"
REMOTE_MODE="${MIGRATION_REMOTE_MODE:-0}"
RESUME_EXISTING="${MIGRATION_RESUME_EXISTING:-0}"
REMOTE_PG_HOST="${MIGRATION_REMOTE_PG_HOST:-10.5.4.5}"
REMOTE_REDIS_HOST="${MIGRATION_REMOTE_REDIS_HOST:-10.5.4.5}"
TARGET_PG_HOST="${MIGRATION_TARGET_PG_HOST:-}"
TARGET_REDIS_HOST="${MIGRATION_TARGET_REDIS_HOST:-}"
PGCOPYDB_IMAGE="${MIGRATION_PGCOPYDB_IMAGE:-dimitri/pgcopydb:latest}"
REDISSHAKE_IMAGE="${MIGRATION_REDISSHAKE_IMAGE:-ghcr.io/tair-opensource/redisshake:latest}"
POSTGRES_CLIENT_IMAGE="${MIGRATION_POSTGRES_CLIENT_IMAGE:-postgres:18-alpine}"
REDIS_CLIENT_IMAGE="${MIGRATION_REDIS_CLIENT_IMAGE:-redis:8.8.1}"
PGCOPYDB_CONTAINER="xiaohuiji-migration-pgcopydb"
REDISSHAKE_CONTAINER="xiaohuiji-migration-redisshake"
PGCOPYDB_SLOT="${MIGRATION_PGCOPYDB_SLOT:-xiaohuiji_migration}"
TABLE_JOBS="${MIGRATION_TABLE_JOBS:-8}"
INDEX_JOBS="${MIGRATION_INDEX_JOBS:-8}"
SPLIT_SIZE="${MIGRATION_SPLIT_SIZE:-1GB}"
SPLIT_PARTS="${MIGRATION_SPLIT_PARTS:-8}"
CUTOVER_TIMEOUT="${MIGRATION_CUTOVER_TIMEOUT_SECONDS:-3600}"
REDIS_SETTLE_SECONDS="${MIGRATION_REDIS_SETTLE_SECONDS:-20}"
REDIS_PAUSE_MS="${MIGRATION_REDIS_PAUSE_MILLISECONDS:-3600000}"
AUTO_START_RAILWAY="${MIGRATION_AUTO_START_RAILWAY:-1}"
TG_BOT_TOKEN="${MIGRATION_TG_BOT_TOKEN:-}"
TG_CHAT_ID="${MIGRATION_TG_CHAT_ID:-}"

BUSINESS_SERVICES=(xiaohuiji-api xiaohuiji-worker xiaohuiji-account xiaohuiji-collector-worker xiaohuiji-media-worker xiaohuiji-publish-worker xiaohuiji-scheduler)
OPTIONAL_SERVICES=(xiaohuiji-otel-collector xiaohuiji-observe)
SSH=(ssh -i "$SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=30 -o ServerAliveCountMax=3)

log(){ printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
ok(){ printf '\033[32m[通过]\033[0m %s\n' "$*"; }
warn(){ printf '\033[33m[警告]\033[0m %s\n' "$*" >&2; }
tg_notify(){
  local message="$*"
  [[ -n "$TG_BOT_TOKEN" && -n "$TG_CHAT_ID" ]] || return 0
  curl -fsS --max-time 15 -X POST "https://api.telegram.org/bot${TG_BOT_TOKEN}/sendMessage" \
    --data-urlencode "chat_id=${TG_CHAT_ID}" \
    --data-urlencode "text=${message}" >/dev/null 2>&1 || warn "Telegram 通知发送失败"
}
die(){
  printf '\033[31m[失败]\033[0m %s\n' "$*" >&2
  tg_notify "迁移失败\n阶段：${MIGRATION_PHASE:-未知}\n原因：$*"
  exit 1
}

usage(){ cat <<'USAGE'
腾讯云 → Railway 一键快速迁移

  ./deploy/migration/fast-migrate.sh precheck
  ./deploy/migration/fast-migrate.sh prepare
  ./deploy/migration/fast-migrate.sh resume
  ./deploy/migration/fast-migrate.sh status
  ./deploy/migration/fast-migrate.sh watch
  ./deploy/migration/fast-migrate.sh cutover
  ./deploy/migration/fast-migrate.sh schedule 02:00
  ./deploy/migration/fast-migrate.sh run 02:00
  ./deploy/migration/fast-migrate.sh rollback
  ./deploy/migration/fast-migrate.sh cleanup

推荐迁移前一天执行 `run 02:00`：先预检查，检查通过后自动启动 PostgreSQL
并行全量复制 + CDC、Redis 全量 + 增量同步，再等待到凌晨 02:00 自动切换。
precheck 只读；任何校验失败都不会启动 Railway。
USAGE
}

ensure_dirs(){ mkdir -p "$STATE_DIR" "$LOG_DIR" "$PGCOPYDB_DIR" "$REDISSHAKE_DIR"; chmod 700 "$STATE_DIR" "$LOG_DIR" "$PGCOPYDB_DIR" "$REDISSHAKE_DIR"; }
load_state(){ if [[ -f "$STATE_FILE" ]]; then source "$STATE_FILE"; fi; return 0; }
save_state(){
  local key="$1" value="$2" tmp
  ensure_dirs; tmp="$(mktemp "$STATE_DIR/state.XXXX")"
  [[ ! -f "$STATE_FILE" ]] || grep -v "^${key}=" "$STATE_FILE" > "$tmp" || true
  printf '%s=%q\n' "$key" "$value" >> "$tmp"; mv "$tmp" "$STATE_FILE"; chmod 600 "$STATE_FILE"
}
set_phase(){
  local phase="$1" at
  at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  save_state MIGRATION_PHASE "$phase"
  save_state MIGRATION_PHASE_AT "$at"
  tg_notify "迁移阶段通知\n阶段：${phase}\n时间：${at}\n主机：$(hostname)"
}
require(){ command -v "$1" >/dev/null || die "缺少命令：$1"; }
railway(){ "$RAILWAY_BIN" "$@"; }
run_remote(){ "${SSH[@]}" "${SSH_USER}@${SSH_HOST}" "$@"; }
remote_script(){ "${SSH[@]}" "${SSH_USER}@${SSH_HOST}" 'bash -s'; }
pid_alive(){ [[ -n "${1:-}" ]] && kill -0 "$1" 2>/dev/null; }
stop_pid(){ local pid="${1:-}"; if pid_alive "$pid"; then kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi; }
kill_port(){
  local port="$1" pid
  while read -r pid; do [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true; done < <(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
}
port_free(){ ! lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }
clear_stale_port(){
  local port="$1" expected_pid="${2:-}" pid
  port_free "$port" && return 0
  pid="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null | head -1 || true)"
  if [[ -n "$pid" && -n "$expected_pid" && "$pid" == "$expected_pid" ]]; then stop_pid "$pid"; sleep 1; fi
  port_free "$port" || die "迁移端口 $port 被其他进程占用（PID ${pid:-未知}）"
}
wait_port(){ local port="$1" label="$2" end=$((SECONDS+30)); until nc -z 127.0.0.1 "$port" 2>/dev/null; do ((SECONDS<end)) || die "$label 隧道启动超时"; sleep 1; done; }
urlencode(){ python3 -c 'import sys,urllib.parse;print(urllib.parse.quote(sys.argv[1],safe=""))' "$1"; }
container_state(){ local state; state="$(docker inspect -f '{{.State.Status}}' "$1" 2>/dev/null || true)"; if [[ -n "$state" ]]; then printf '%s' "$state"; else printf 'missing'; fi; }

stop_tunnels(){
  load_state
  stop_pid "${SOURCE_TUNNEL_PID:-}"; stop_pid "${TARGET_PG_TUNNEL_PID:-}"; stop_pid "${TARGET_REDIS_TUNNEL_PID:-}"
  kill_port "$SOURCE_PG_PORT"; kill_port "$SOURCE_REDIS_PORT"; kill_port "$TARGET_PG_PORT"; kill_port "$TARGET_REDIS_PORT"
  save_state SOURCE_TUNNEL_PID ""; save_state TARGET_PG_TUNNEL_PID ""; save_state TARGET_REDIS_TUNNEL_PID ""
}

discover_source(){
  if [[ "$REMOTE_MODE" == 1 ]]; then
    local pgid rid pgenv u d p renv rp rdb
    pgid="$(sudo docker ps -q --filter name=1Panel-postgresql | head -1)"
    rid="$(sudo docker ps -q --filter name=1Panel-redis | head -1)"
    [[ -n "$pgid" && -n "$rid" ]] || die "服务器本地 PostgreSQL/Redis 容器不存在"
    pgenv="$(sudo docker inspect "$pgid" --format '{{range .Config.Env}}{{println .}}{{end}}')"
    u="$(sed -n 's/^POSTGRES_USER=//p' <<<"$pgenv" | head -1)"
    d="$(sed -n 's/^POSTGRES_DB=//p' <<<"$pgenv" | head -1)"
    p="$(sed -n 's/^POSTGRES_PASSWORD=//p' <<<"$pgenv" | head -1)"
    [[ -n "$d" ]] || d="$(sudo docker exec -e PGPASSWORD="$p" "$pgid" psql -U "$u" -d postgres -Atqc "SELECT datname FROM pg_database WHERE datistemplate=false AND datallowconn=true AND datname<>'postgres' ORDER BY pg_database_size(datname) DESC LIMIT 1")"
    renv="$(sudo docker inspect "$rid" --format '{{range .Config.Env}}{{println .}}{{end}}')"
    rp="$(sed -n 's/^REDIS_PASSWORD=//p' <<<"$renv" | head -1)"
    if [[ -z "$rp" ]]; then
      local cmd
      cmd="$(sudo docker inspect "$rid" --format '{{json .Config.Cmd}}')"
      rp="$(sed -n 's/.*--requirepass","\([^"]*\)".*/\1/p' <<<"$cmd")"
    fi
    rdb="$(sudo docker exec "$rid" redis-cli --no-auth-warning ${rp:+-a "$rp"} INFO keyspace | sed -n 's/^db\([0-9]*\):keys=.*/\1/p' | head -1 | tr -d '\r')"
    SOURCE_PG_CONTAINER="$pgid"; SOURCE_REDIS_CONTAINER="$rid"; SOURCE_PG_USER="$u"; SOURCE_PG_DATABASE="$d"; SOURCE_REDIS_DB="${rdb:-0}"
    SOURCE_PG_PASSWORD="$p"; SOURCE_REDIS_PASSWORD="$rp"
    save_state SOURCE_PG_CONTAINER "$SOURCE_PG_CONTAINER"; save_state SOURCE_REDIS_CONTAINER "$SOURCE_REDIS_CONTAINER"; save_state SOURCE_PG_USER "$SOURCE_PG_USER"; save_state SOURCE_PG_DATABASE "$SOURCE_PG_DATABASE"; save_state SOURCE_REDIS_DB "$SOURCE_REDIS_DB"
    return
  fi
  local result
  result="$(remote_script <<'REMOTE'
set -Eeuo pipefail
pgid="$(sudo docker ps -q --filter name=1Panel-postgresql | head -1)"; rid="$(sudo docker ps -q --filter name=1Panel-redis | head -1)"
[[ -n "$pgid" && -n "$rid" ]]
pgenv="$(sudo docker inspect "$pgid" --format '{{range .Config.Env}}{{println .}}{{end}}')"
u="$(sed -n 's/^POSTGRES_USER=//p' <<<"$pgenv" | head -1)"; d="$(sed -n 's/^POSTGRES_DB=//p' <<<"$pgenv" | head -1)"; p="$(sed -n 's/^POSTGRES_PASSWORD=//p' <<<"$pgenv" | head -1)"
[[ -n "$d" ]] || d="$(sudo docker exec -e PGPASSWORD="$p" "$pgid" psql -U "$u" -d postgres -Atqc "SELECT datname FROM pg_database WHERE datistemplate=false AND datallowconn=true AND datname<>'postgres' ORDER BY pg_database_size(datname) DESC LIMIT 1")"
renv="$(sudo docker inspect "$rid" --format '{{range .Config.Env}}{{println .}}{{end}}')"; rp="$(sed -n 's/^REDIS_PASSWORD=//p' <<<"$renv" | head -1)"
if [[ -z "$rp" ]]; then cmd="$(sudo docker inspect "$rid" --format '{{json .Config.Cmd}}')"; rp="$(sed -n 's/.*--requirepass","\([^"]*\)".*/\1/p' <<<"$cmd")"; fi
rdb="$(sudo docker exec "$rid" redis-cli --no-auth-warning ${rp:+-a "$rp"} INFO keyspace | sed -n 's/^db\([0-9]*\):keys=.*/\1/p' | head -1 | tr -d '\r')"; rdb="${rdb:-0}"
printf 'SOURCE_PG_CONTAINER=%q\nSOURCE_REDIS_CONTAINER=%q\nSOURCE_PG_USER=%q\nSOURCE_PG_DATABASE=%q\nSOURCE_PG_PASSWORD_B64=%q\nSOURCE_REDIS_PASSWORD_B64=%q\nSOURCE_REDIS_DB=%q\n' "$pgid" "$rid" "$u" "$d" "$(printf %s "$p"|base64 -w0)" "$(printf %s "$rp"|base64 -w0)" "$rdb"
REMOTE
)" || die "发现源数据库失败"
  eval "$result"
  SOURCE_PG_PASSWORD="$(printf %s "$SOURCE_PG_PASSWORD_B64"|base64 -d)"; SOURCE_REDIS_PASSWORD="$(printf %s "$SOURCE_REDIS_PASSWORD_B64"|base64 -d)"
  save_state SOURCE_PG_CONTAINER "$SOURCE_PG_CONTAINER"; save_state SOURCE_REDIS_CONTAINER "$SOURCE_REDIS_CONTAINER"; save_state SOURCE_PG_USER "$SOURCE_PG_USER"; save_state SOURCE_PG_DATABASE "$SOURCE_PG_DATABASE"; save_state SOURCE_REDIS_DB "$SOURCE_REDIS_DB"
}

load_target(){
  if [[ "$REMOTE_MODE" == 1 ]]; then
    TARGET_PG_USER="${MIGRATION_TARGET_PG_USER:?缺少 MIGRATION_TARGET_PG_USER}"
    TARGET_PG_PASSWORD="${MIGRATION_TARGET_PG_PASSWORD:?缺少 MIGRATION_TARGET_PG_PASSWORD}"
    TARGET_PG_DATABASE="${MIGRATION_TARGET_PG_DATABASE:?缺少 MIGRATION_TARGET_PG_DATABASE}"
    TARGET_REDIS_USER="${MIGRATION_TARGET_REDIS_USER:-default}"
    TARGET_REDIS_PASSWORD="${MIGRATION_TARGET_REDIS_PASSWORD:?缺少 MIGRATION_TARGET_REDIS_PASSWORD}"
    TARGET_PG_HOST="${MIGRATION_TARGET_PG_HOST:?缺少 MIGRATION_TARGET_PG_HOST}"
    TARGET_REDIS_HOST="${MIGRATION_TARGET_REDIS_HOST:?缺少 MIGRATION_TARGET_REDIS_HOST}"
    return
  fi
  local pg redis
  pg="$(railway variable list --service "$RAILWAY_POSTGRES_SERVICE" --environment "$RAILWAY_ENVIRONMENT" --json)" || die "读取 Railway PostgreSQL 变量失败"
  redis="$(railway variable list --service "$RAILWAY_REDIS_SERVICE" --environment "$RAILWAY_ENVIRONMENT" --json)" || die "读取 Railway Redis 变量失败"
  TARGET_PG_USER="$(jq -r '.PGUSER//.POSTGRES_USER//empty' <<<"$pg")"; TARGET_PG_PASSWORD="$(jq -r '.PGPASSWORD//.POSTGRES_PASSWORD//empty' <<<"$pg")"; TARGET_PG_DATABASE="$(jq -r '.PGDATABASE//.POSTGRES_DB//empty' <<<"$pg")"
  TARGET_REDIS_USER="$(jq -r '.REDISUSER//"default"' <<<"$redis")"; TARGET_REDIS_PASSWORD="$(jq -r '.REDISPASSWORD//.REDIS_PASSWORD//empty' <<<"$redis")"
  [[ -n "$TARGET_PG_USER" && -n "$TARGET_PG_PASSWORD" && -n "$TARGET_PG_DATABASE" && -n "$TARGET_REDIS_PASSWORD" ]] || die "Railway 数据库变量不完整"
}

start_tunnels(){
  ensure_dirs; load_state; discover_source; load_target
  [[ "$REMOTE_MODE" == 1 ]] && return 0
  if ! pid_alive "${SOURCE_TUNNEL_PID:-}"; then
    clear_stale_port "$SOURCE_PG_PORT" "${SOURCE_TUNNEL_PID:-}"; clear_stale_port "$SOURCE_REDIS_PORT" "${SOURCE_TUNNEL_PID:-}"
    nohup "${SSH[@]}" -N -o ExitOnForwardFailure=yes -L "$SOURCE_PG_PORT:10.5.4.5:5432" -L "$SOURCE_REDIS_PORT:10.5.4.5:6379" "${SSH_USER}@${SSH_HOST}" >"$LOG_DIR/source-tunnel.log" 2>&1 </dev/null & SOURCE_TUNNEL_PID=$!; disown "$SOURCE_TUNNEL_PID" 2>/dev/null || true; save_state SOURCE_TUNNEL_PID "$SOURCE_TUNNEL_PID"
  fi
  if ! pid_alive "${TARGET_PG_TUNNEL_PID:-}"; then
    clear_stale_port "$TARGET_PG_PORT" "${TARGET_PG_TUNNEL_PID:-}"
    nohup "$RAILWAY_BIN" connect "$RAILWAY_POSTGRES_SERVICE" --environment "$RAILWAY_ENVIRONMENT" --tunnel-only --port "$TARGET_PG_PORT" >"$LOG_DIR/target-pg-tunnel.log" 2>&1 </dev/null & TARGET_PG_TUNNEL_PID=$!; disown "$TARGET_PG_TUNNEL_PID" 2>/dev/null || true; save_state TARGET_PG_TUNNEL_PID "$TARGET_PG_TUNNEL_PID"
  fi
  if ! pid_alive "${TARGET_REDIS_TUNNEL_PID:-}"; then
    clear_stale_port "$TARGET_REDIS_PORT" "${TARGET_REDIS_TUNNEL_PID:-}"
    nohup "$RAILWAY_BIN" connect "$RAILWAY_REDIS_SERVICE" --environment "$RAILWAY_ENVIRONMENT" --tunnel-only --port "$TARGET_REDIS_PORT" >"$LOG_DIR/target-redis-tunnel.log" 2>&1 </dev/null & TARGET_REDIS_TUNNEL_PID=$!; disown "$TARGET_REDIS_TUNNEL_PID" 2>/dev/null || true; save_state TARGET_REDIS_TUNNEL_PID "$TARGET_REDIS_TUNNEL_PID"
  fi
  wait_port "$SOURCE_PG_PORT" "源 PostgreSQL"; wait_port "$SOURCE_REDIS_PORT" "源 Redis"; wait_port "$TARGET_PG_PORT" "目标 PostgreSQL"; wait_port "$TARGET_REDIS_PORT" "目标 Redis"
}

source_psql(){
  if [[ "$REMOTE_MODE" == 1 ]]; then
    docker run --rm -i --network host -e PGPASSWORD="${ACTIVE_SOURCE_PG_PASSWORD:-$SOURCE_PG_PASSWORD}" "$POSTGRES_CLIENT_IMAGE" psql -X -v ON_ERROR_STOP=1 -h "$REMOTE_PG_HOST" -p 5432 -U "${ACTIVE_SOURCE_PG_USER:-$SOURCE_PG_USER}" -d "$SOURCE_PG_DATABASE" "$@"
  else
    docker run --rm -i --add-host host.docker.internal:host-gateway -e PGPASSWORD="${ACTIVE_SOURCE_PG_PASSWORD:-$SOURCE_PG_PASSWORD}" "$POSTGRES_CLIENT_IMAGE" psql -X -v ON_ERROR_STOP=1 -h host.docker.internal -p "$SOURCE_PG_PORT" -U "${ACTIVE_SOURCE_PG_USER:-$SOURCE_PG_USER}" -d "$SOURCE_PG_DATABASE" "$@"
  fi
}
target_psql(){
  if [[ "$REMOTE_MODE" == 1 ]]; then
    docker run --rm -i --network host -e PGPASSWORD="$TARGET_PG_PASSWORD" "$POSTGRES_CLIENT_IMAGE" psql -X -v ON_ERROR_STOP=1 -h "$TARGET_PG_HOST" -p "$TARGET_PG_PORT" -U "$TARGET_PG_USER" -d "$TARGET_PG_DATABASE" "$@"
  else
    docker run --rm -i --add-host host.docker.internal:host-gateway -e PGPASSWORD="$TARGET_PG_PASSWORD" "$POSTGRES_CLIENT_IMAGE" psql -X -v ON_ERROR_STOP=1 -h host.docker.internal -p "$TARGET_PG_PORT" -U "$TARGET_PG_USER" -d "$TARGET_PG_DATABASE" "$@"
  fi
}
source_redis(){
  if [[ "$REMOTE_MODE" == 1 ]]; then
    docker run --rm -i --network host "$REDIS_CLIENT_IMAGE" redis-cli -h "$REMOTE_REDIS_HOST" -p 6379 --no-auth-warning ${SOURCE_REDIS_PASSWORD:+-a "$SOURCE_REDIS_PASSWORD"} "$@"
  else
    docker run --rm -i --add-host host.docker.internal:host-gateway "$REDIS_CLIENT_IMAGE" redis-cli -h host.docker.internal -p "$SOURCE_REDIS_PORT" --no-auth-warning ${SOURCE_REDIS_PASSWORD:+-a "$SOURCE_REDIS_PASSWORD"} "$@"
  fi
}
target_redis(){
  if [[ "$REMOTE_MODE" == 1 ]]; then
    docker run --rm -i --network host "$REDIS_CLIENT_IMAGE" redis-cli -h "$TARGET_REDIS_HOST" -p "$TARGET_REDIS_PORT" --user "$TARGET_REDIS_USER" --no-auth-warning -a "$TARGET_REDIS_PASSWORD" "$@"
  else
    docker run --rm -i --add-host host.docker.internal:host-gateway "$REDIS_CLIENT_IMAGE" redis-cli -h host.docker.internal -p "$TARGET_REDIS_PORT" --user "$TARGET_REDIS_USER" --no-auth-warning -a "$TARGET_REDIS_PASSWORD" "$@"
  fi
}
railway_json(){ railway status --environment "$RAILWAY_ENVIRONMENT" --json; }
service_exists(){ jq -e --arg n "$2" '.services.edges[].node.name|select(.==$n)' <<<"$1" >/dev/null; }
active_count(){ jq -r --arg n "$2" '[.environments.edges[].node.serviceInstances.edges[].node|select(.serviceName==$n)|.activeDeployments[]?]|length' <<<"$1"; }

check_services_stopped(){
  [[ "$REMOTE_MODE" == 1 ]] && { ok "服务器直连模式，跳过本机 Railway 服务检查"; return; }
  local json service count failed=0; json="$(railway_json)"
  for service in "${BUSINESS_SERVICES[@]}"; do service_exists "$json" "$service" || die "Railway 缺少服务：$service"; count="$(active_count "$json" "$service")"; if [[ "$count" == 0 ]]; then ok "${service} 已停止"; else warn "${service} 有 ${count} 个活动部署"; failed=1; fi; done
  ((failed==0)) || die "Railway 业务服务未全部停止"
}

check_target_empty(){
  local tables keys other_keys
  tables="$(target_psql -Atqc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")"
  keys="$(target_redis -n "$SOURCE_REDIS_DB" DBSIZE | tr -d '\r')"
  other_keys="$(target_redis INFO keyspace | tr -d '\r' | awk -v target="db${SOURCE_REDIS_DB}:" -F'[=,]' '/^db[0-9]+:keys=/{if (index($0,target) != 1) sum += $2} END{print sum+0}')"
  if [[ "$RESUME_EXISTING" == 1 ]]; then
    ok "恢复模式：保留目标 PostgreSQL/Redis 已同步数据"
  else
    [[ "$tables" == 0 ]] || die "目标 PostgreSQL 有 ${tables} 张表，不是空库"
    [[ "$keys" == 0 ]] || die "目标 Redis DB${SOURCE_REDIS_DB} 有 ${keys} 个 Key，不是空库"
    ok "目标 PostgreSQL 和 Redis DB${SOURCE_REDIS_DB} 为空"
  fi
  [[ "$other_keys" == 0 ]] || warn "目标 Redis 其他 DB 存在 ${other_keys} 个 Key，本次不会删除或迁移"
}

precheck(){
  log "开始只读预检查"; ensure_dirs; trap stop_tunnels EXIT
  for c in ssh docker jq python3 nc lsof openssl; do require "$c"; done
  if [[ "$REMOTE_MODE" == 1 ]]; then
    docker info >/dev/null 2>&1 || die "服务器 Docker 未启动"
  else
    [[ -x "$RAILWAY_BIN" && -r "$SSH_KEY" ]] || die "Railway CLI 或 SSH 私钥不可用"
    docker info >/dev/null 2>&1 || die "Docker Desktop 未启动"
    railway_json >/dev/null
    run_remote 'sudo -n docker ps >/dev/null' || die "源服务器不可访问"
  fi
  docker image inspect "$POSTGRES_CLIENT_IMAGE" >/dev/null 2>&1 || docker pull "$POSTGRES_CLIENT_IMAGE" >/dev/null
  docker image inspect "$REDIS_CLIENT_IMAGE" >/dev/null 2>&1 || docker pull "$REDIS_CLIENT_IMAGE" >/dev/null
  stop_tunnels; start_tunnels; check_services_stopped; check_target_empty
  local size tables keys wal nopk
  size="$(source_psql -Atqc 'SELECT pg_size_pretty(pg_database_size(current_database()))')"; tables="$(source_psql -Atqc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")"; keys="$(source_redis -n "$SOURCE_REDIS_DB" DBSIZE|tr -d '\r')"; wal="$(source_psql -Atqc 'SHOW wal_level')"; nopk="$(source_psql -Atqc "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN('r','p') AND NOT EXISTS(SELECT 1 FROM pg_index i WHERE i.indrelid=c.oid AND i.indisprimary)")"
  ok "源 PostgreSQL：${size}，${tables} 张表"; ok "源 Redis DB${SOURCE_REDIS_DB}：${keys} 个 Key"; [[ "$wal" == logical ]] && ok "wal_level=logical" || warn "wal_level=${wal}，prepare 会自动修改并重启 PostgreSQL 一次"; [[ "$nopk" == 0 ]] || warn "${nopk} 张表无主键，prepare 会设置 REPLICA IDENTITY FULL"
  save_state SOURCE_DATABASE_SIZE "$size"; save_state SOURCE_TABLE_COUNT "$tables"; save_state SOURCE_REDIS_KEY_COUNT "$keys"; save_state PRECHECK_AT "$(date -u '+%FT%TZ')"; set_phase prechecked; log "预检查通过"; trap - EXIT; stop_tunnels
}

configure_source(){
  if [[ "$(source_psql -Atqc 'SHOW wal_level')" != logical ]]; then
    source_psql -c "ALTER SYSTEM SET wal_level='logical'" -c "ALTER SYSTEM SET max_replication_slots='16'" -c "ALTER SYSTEM SET max_wal_senders='16'" -c "ALTER SYSTEM SET max_logical_replication_workers='8'" -c "ALTER SYSTEM SET max_sync_workers_per_subscription='4'" >/dev/null
    if [[ "$REMOTE_MODE" == 1 ]]; then
      sudo docker restart "$SOURCE_PG_CONTAINER" >/dev/null
    else
      run_remote "sudo docker restart '$SOURCE_PG_CONTAINER' >/dev/null"
    fi
    sleep 5; local end=$((SECONDS+120)); until source_psql -Atqc 'SELECT 1' >/dev/null 2>&1; do ((SECONDS<end)) || die "PostgreSQL 重启失败"; sleep 2; done
  fi
  [[ "$(source_psql -Atqc 'SHOW wal_level')" == logical ]] || die "wal_level 配置失败"
  local sql; sql="$(source_psql -Atqc "SELECT format('ALTER TABLE %I.%I REPLICA IDENTITY FULL;',n.nspname,c.relname) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN('r','p') AND NOT EXISTS(SELECT 1 FROM pg_index i WHERE i.indrelid=c.oid AND i.indisprimary)")"; [[ -z "$sql" ]] || source_psql <<<"$sql" >/dev/null
  MIGRATOR_USER="xiaohuiji_migrator"; MIGRATOR_PASSWORD="$(openssl rand -hex 24)"; local escaped="${MIGRATOR_PASSWORD//\'/\'\'}"
  source_psql -c "DO \$\$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='$MIGRATOR_USER') THEN CREATE ROLE $MIGRATOR_USER LOGIN REPLICATION SUPERUSER PASSWORD '$escaped'; ELSE ALTER ROLE $MIGRATOR_USER LOGIN REPLICATION SUPERUSER PASSWORD '$escaped'; END IF; END \$\$" -c "GRANT CONNECT ON DATABASE \"$SOURCE_PG_DATABASE\" TO $MIGRATOR_USER" -c "GRANT USAGE ON SCHEMA public TO $MIGRATOR_USER" -c "GRANT SELECT ON ALL TABLES IN SCHEMA public TO $MIGRATOR_USER" -c "GRANT SELECT,USAGE ON ALL SEQUENCES IN SCHEMA public TO $MIGRATOR_USER" >/dev/null
  save_state MIGRATOR_USER "$MIGRATOR_USER"; save_state MIGRATOR_PASSWORD "$MIGRATOR_PASSWORD"; ok "逻辑复制和迁移账号已就绪"
}

write_redis_config(){ cat >"$REDISSHAKE_DIR/shake.toml" <<EOF2
[sync_reader]
cluster=false
address="${REMOTE_MODE:+$REMOTE_REDIS_HOST:6379}"
username=""
password="${SOURCE_REDIS_PASSWORD//\"/\\\"}"
tls=false
sync_rdb=true
sync_aof=true
prefer_replica=false
try_diskless=false
[redis_writer]
cluster=false
address="${REMOTE_MODE:+$TARGET_REDIS_HOST:$TARGET_REDIS_PORT}"
username="${TARGET_REDIS_USER//\"/\\\"}"
password="${TARGET_REDIS_PASSWORD//\"/\\\"}"
tls=false
off_reply=false
[filter]
allow_db=[$SOURCE_REDIS_DB]
block_db=[]
allow_keys=[]
allow_key_prefix=[]
allow_key_suffix=[]
allow_key_regex=[]
block_keys=[]
block_key_prefix=[]
block_key_suffix=[]
block_key_regex=[]
allow_command=[]
block_command=[]
allow_command_group=[]
block_command_group=[]
function=""
[advanced]
dir="/data"
ncpu=0
pprof_port=0
status_port=0
log_file="/data/shake.log"
log_level="info"
log_interval=5
log_rotation=true
log_max_size=128
log_max_age=3
log_max_backups=3
log_compress=true
rdb_restore_command_behavior="rewrite"
pipeline_count_limit=1024
target_redis_max_qps=300000
target_redis_client_max_querybuf_len=1073741824
target_redis_proto_max_bulk_len=512000000
empty_db_before_sync=false
EOF2
  if [[ "$REMOTE_MODE" != 1 ]]; then
    sed -i "s#address=\"\"#address=\"host.docker.internal:$SOURCE_REDIS_PORT\"#; 0,/address=\"host.docker.internal:$SOURCE_REDIS_PORT\"/! s#address=\"\"#address=\"host.docker.internal:$TARGET_REDIS_PORT\"#" "$REDISSHAKE_DIR/shake.toml"
  fi
chmod 600 "$REDISSHAKE_DIR/shake.toml"; }

start_sync_tools(){
  load_state; load_target
  [[ -n "${MIGRATOR_USER:-}" && -n "${MIGRATOR_PASSWORD:-}" ]] || die "缺少迁移账号状态，请重新执行 prepare"
  local src dst
  if [[ "$REMOTE_MODE" == 1 ]]; then
    src="postgresql://$(urlencode "$MIGRATOR_USER"):$(urlencode "$MIGRATOR_PASSWORD")@${REMOTE_PG_HOST}:5432/$(urlencode "$SOURCE_PG_DATABASE")?sslmode=disable"
    dst="postgresql://$(urlencode "$TARGET_PG_USER"):$(urlencode "$TARGET_PG_PASSWORD")@${TARGET_PG_HOST}:$TARGET_PG_PORT/$(urlencode "$TARGET_PG_DATABASE")?sslmode=disable"
  else
    src="postgresql://$(urlencode "$MIGRATOR_USER"):$(urlencode "$MIGRATOR_PASSWORD")@host.docker.internal:$SOURCE_PG_PORT/$(urlencode "$SOURCE_PG_DATABASE")?sslmode=disable"
    dst="postgresql://$(urlencode "$TARGET_PG_USER"):$(urlencode "$TARGET_PG_PASSWORD")@host.docker.internal:$TARGET_PG_PORT/$(urlencode "$TARGET_PG_DATABASE")?sslmode=disable"
  fi
  docker rm -f "$PGCOPYDB_CONTAINER" >/dev/null 2>&1 || true
  mkdir -p "$PGCOPYDB_DIR" "$STATE_DIR/pgcopydb"
  chmod 777 "$PGCOPYDB_DIR"
  docker run -d --name "$PGCOPYDB_CONTAINER" --restart unless-stopped --network host -v "$STATE_DIR:/state" -e PGCOPYDB_SOURCE_PGURI="$src" -e PGCOPYDB_TARGET_PGURI="$dst" "$PGCOPYDB_IMAGE" pgcopydb clone --follow --dir /state/pgcopydb --slot-name "$PGCOPYDB_SLOT" --create-slot --table-jobs "$TABLE_JOBS" --index-jobs "$INDEX_JOBS" --large-objects-jobs 2 --split-tables-larger-than "$SPLIT_SIZE" --split-max-parts "$SPLIT_PARTS" --use-copy-binary --no-owner --no-acl --no-tablespaces --fail-fast --restart >/dev/null
  if [[ "$(container_state "$REDISSHAKE_CONTAINER")" != running ]]; then
    write_redis_config
    chmod 777 "$REDISSHAKE_DIR"
    docker rm -f "$REDISSHAKE_CONTAINER" >/dev/null 2>&1 || true
    docker run -d --name "$REDISSHAKE_CONTAINER" --restart unless-stopped --entrypoint /app/redis-shake -v "$REDISSHAKE_DIR:/data" -v "$REDISSHAKE_DIR/shake.toml:/app/shake.toml:ro" "$REDISSHAKE_IMAGE" /app/shake.toml >/dev/null
  else
    log "RedisShake 已在运行，保留当前同步进度"
  fi
  sleep 5
  [[ "$(container_state "$PGCOPYDB_CONTAINER")" == running ]] || { docker logs --tail 100 "$PGCOPYDB_CONTAINER" >&2; die "pgcopydb 启动失败"; }
  [[ "$(container_state "$REDISSHAKE_CONTAINER")" == running ]] || { docker logs --tail 100 "$REDISSHAKE_CONTAINER" >&2; die "RedisShake 启动失败"; }
}

prepare(){
  precheck; log "开始在线预同步"; start_tunnels; configure_source
  docker image inspect "$PGCOPYDB_IMAGE" >/dev/null 2>&1 || docker pull "$PGCOPYDB_IMAGE" >/dev/null
  docker image inspect "$REDISSHAKE_IMAGE" >/dev/null 2>&1 || docker pull "$REDISSHAKE_IMAGE" >/dev/null
  start_sync_tools; set_phase syncing; status; log "预同步已启动，脚本会自动防休眠并监控隧道"
}

resume_sync(){
  load_state
  [[ "${MIGRATION_PHASE:-}" == syncing ]] || die "只有 syncing 阶段可以恢复同步"
  start_tunnels; start_sync_tools; ok "PostgreSQL/Redis 同步已从原状态恢复"
}

human_bytes(){ python3 - "$1" <<'PY'
import sys
n=float(sys.argv[1] or 0)
for u in ('B','KB','MB','GB','TB'):
 if n<1024: print(f'{n:.1f}{u}'); break
 n/=1024
PY
}
status(){
  ensure_dirs; load_state; printf '\n========== 迁移状态 ==========\n阶段：%s（%s）\n' "${MIGRATION_PHASE:-未开始}" "${MIGRATION_PHASE_AT:-无}"; printf '源库：%s / %s 张表 / Redis %s Key\n' "${SOURCE_DATABASE_SIZE:-未知}" "${SOURCE_TABLE_COUNT:-未知}" "${SOURCE_REDIS_KEY_COUNT:-未知}"; printf 'pgcopydb：%s\nRedisShake：%s\n' "$(container_state "$PGCOPYDB_CONTAINER")" "$(container_state "$REDISSHAKE_CONTAINER")"
  if [[ "$(container_state "$PGCOPYDB_CONTAINER")" != missing ]]; then
    docker exec "$PGCOPYDB_CONTAINER" pgcopydb list progress --dir /state/pgcopydb --json --summary 2>/dev/null | jq . || docker logs --tail 20 "$PGCOPYDB_CONTAINER" 2>&1 || true
  else
    printf 'PostgreSQL 同步：尚未启动\n'
  fi
  if [[ -n "${MIGRATOR_USER:-}" ]]; then start_tunnels >/dev/null; local lag; lag="$(source_psql -Atqc "SELECT coalesce(pg_wal_lsn_diff(pg_current_wal_flush_lsn(),confirmed_flush_lsn),0)::bigint FROM pg_replication_slots WHERE slot_name='$PGCOPYDB_SLOT'" 2>/dev/null || true)"; [[ -z "$lag" ]] || printf 'PostgreSQL CDC 待追平：%s\n' "$(human_bytes "$lag")"; printf '目标 PostgreSQL：%s / %s 张表\n' "$(target_psql -Atqc 'SELECT pg_size_pretty(pg_database_size(current_database()))' 2>/dev/null||echo '?')" "$(target_psql -Atqc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'" 2>/dev/null||echo '?')"; printf 'Redis Key：源 DB%s=%s，目标 DB%s=%s\n' "$SOURCE_REDIS_DB" "$(source_redis -n "$SOURCE_REDIS_DB" DBSIZE 2>/dev/null|tr -d '\r'||echo '?')" "$SOURCE_REDIS_DB" "$(target_redis -n "$SOURCE_REDIS_DB" DBSIZE 2>/dev/null|tr -d '\r'||echo '?')"; fi
  if [[ "$(container_state "$REDISSHAKE_CONTAINER")" == missing ]]; then
    printf '\nRedis 同步：尚未启动\n'
  else
    printf '\n--- RedisShake 最近日志 ---\n'
    docker logs --tail 12 "$REDISSHAKE_CONTAINER" 2>&1 || true
  fi
  printf '日志：%s\n==============================\n\n' "$LOG_DIR"
}

watch_progress(){
  local interval="${1:-10}"
  [[ "$interval" =~ ^[0-9]+$ && "$interval" -ge 2 ]] || die "刷新秒数必须是不小于 2 的整数"
  while :; do
    clear 2>/dev/null || printf '\033[2J\033[H'
    status
    printf '每 %s 秒刷新，按 Ctrl+C 退出监控（不会停止迁移）\n' "$interval"
    sleep "$interval"
  done
}

assert_syncing(){ load_state; [[ "${MIGRATION_PHASE:-}" == syncing ]] || die "请先执行 prepare"; [[ "$(container_state "$PGCOPYDB_CONTAINER")" == running ]] || die "pgcopydb 未运行"; [[ "$(container_state "$REDISSHAKE_CONTAINER")" == running ]] || die "RedisShake 未运行"; }

fence_source(){
  source_psql -c "ALTER ROLE \"$SOURCE_PG_USER\" NOLOGIN" -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=current_database() AND usename='$SOURCE_PG_USER' AND pid<>pg_backend_pid()" >/dev/null; save_state SOURCE_PG_FENCED 1; ACTIVE_SOURCE_PG_USER="$MIGRATOR_USER"; ACTIVE_SOURCE_PG_PASSWORD="$MIGRATOR_PASSWORD"; export ACTIVE_SOURCE_PG_USER ACTIVE_SOURCE_PG_PASSWORD
  source_redis CLIENT PAUSE "$REDIS_PAUSE_MS" WRITE >/dev/null; save_state SOURCE_REDIS_PAUSED 1; ok "源 PostgreSQL/Redis 已停止业务写入"
}

wait_pg(){
  docker exec "$PGCOPYDB_CONTAINER" pgcopydb stream sentinel set endpos --current >/dev/null; local end=$((SECONDS+CUTOVER_TIMEOUT)) state lag
  while :; do state="$(container_state "$PGCOPYDB_CONTAINER")"; [[ "$state" == exited ]] && break; [[ "$state" == running ]] || die "pgcopydb 状态异常：$state"; ((SECONDS<end)) || die "CDC 追平超时"; lag="$(source_psql -Atqc "SELECT coalesce(pg_wal_lsn_diff(pg_current_wal_flush_lsn(),confirmed_flush_lsn),0)::bigint FROM pg_replication_slots WHERE slot_name='$PGCOPYDB_SLOT'" 2>/dev/null||echo '?')"; log "等待 PostgreSQL CDC，lag=${lag} bytes"; sleep 5; done
  [[ "$(docker inspect -f '{{.State.ExitCode}}' "$PGCOPYDB_CONTAINER")" == 0 ]] || { docker logs --tail 100 "$PGCOPYDB_CONTAINER" >&2; die "pgcopydb 失败"; }; ok "PostgreSQL CDC 已追平"
}

sync_sequences(){ local file="$STATE_DIR/sequences.sql"; source_psql -Atqc "SELECT format('SELECT pg_catalog.setval(%L,%s,true);',schemaname||'.'||sequencename,last_value) FROM pg_sequences WHERE schemaname='public' AND last_value IS NOT NULL" >"$file"; [[ ! -s "$file" ]] || target_psql -f "$file" >/dev/null; ok "Sequence 已同步"; }

target_redis_url(){ printf 'redis://%s:%s@127.0.0.1:%s/%s' "$(urlencode "$TARGET_REDIS_USER")" "$(urlencode "$TARGET_REDIS_PASSWORD")" "$TARGET_REDIS_PORT" "$SOURCE_REDIS_DB"; }
verify(){
  local ss ts sk tk source_counts target_counts
  ss="$(source_psql -Atqc 'SELECT count(*) FROM public.hg_youban_publish_tg_session' 2>/dev/null||echo 0)"; ts="$(target_psql -Atqc 'SELECT count(*) FROM public.hg_youban_publish_tg_session' 2>/dev/null||echo -1)"; [[ "$ss" == "$ts" ]] || die "TG session 不一致：${ss}/${ts}"
  sk="$(source_redis -n "$SOURCE_REDIS_DB" DBSIZE|tr -d '\r')"; tk="$(target_redis -n "$SOURCE_REDIS_DB" DBSIZE|tr -d '\r')"; [[ "$sk" == "$tk" ]] || die "Redis Key 不一致：${sk}/${tk}"
  source_counts="$STATE_DIR/source-counts.tsv"; target_counts="$STATE_DIR/target-counts.tsv"
  source_psql -Atqc "SELECT format('SELECT %L,count(*) FROM %I.%I;',table_name,table_schema,table_name) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name" | source_psql -At >"$source_counts"
  target_psql -Atqc "SELECT format('SELECT %L,count(*) FROM %I.%I;',table_name,table_schema,table_name) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name" | target_psql -At >"$target_counts"
  sort -o "$source_counts" "$source_counts"; sort -o "$target_counts" "$target_counts"
  diff -u "$source_counts" "$target_counts" >"$STATE_DIR/postgres-counts.diff" || { cat "$STATE_DIR/postgres-counts.diff"; die "PostgreSQL 全表行数不一致"; }
  python3 "$SCRIPT_DIR/redis-compare.py" --source-host 127.0.0.1 --source-port "$SOURCE_REDIS_PORT" --source-password "$SOURCE_REDIS_PASSWORD" --source-db "$SOURCE_REDIS_DB" --target-url "$(target_redis_url)" --ttl-tolerance-ms "${MIGRATION_REDIS_TTL_TOLERANCE_MS:-10000}"
  ok "PostgreSQL、Redis、TG session 全部一致"
}

configure_db3(){ local json service; json="$(railway_json)"; for service in "${BUSINESS_SERVICES[@]}" "${OPTIONAL_SERVICES[@]}"; do service_exists "$json" "$service" || continue; railway variable set "YOUBAN_REDIS_DB=$SOURCE_REDIS_DB" --service "$service" --environment "$RAILWAY_ENVIRONMENT" --skip-deploys --json >/dev/null; done; }
wait_healthy(){ local service="$1" end=$((SECONDS+600)) json active state; while :; do json="$(railway_json)"; active="$(active_count "$json" "$service")"; state="$(jq -r --arg n "$service" '[.environments.edges[].node.serviceInstances.edges[].node|select(.serviceName==$n)|.latestDeployment.status][0]//"UNKNOWN"' <<<"$json")"; [[ "$active" != 0 && "$state" == SUCCESS ]] && { ok "${service} 已健康"; return; }; [[ "$state" != FAILED && "$state" != CRASHED ]] || die "$service 启动失败：$state"; ((SECONDS<end)) || die "$service 启动超时"; log "等待 ${service}：$state"; sleep 10; done; }
start_services(){ [[ "$AUTO_START_RAILWAY" == 1 ]] || { warn "已跳过启动 Railway"; return; }; configure_db3; local s; for s in xiaohuiji-account xiaohuiji-collector-worker xiaohuiji-media-worker xiaohuiji-publish-worker xiaohuiji-scheduler xiaohuiji-api; do railway redeploy --service "$s" --environment "$RAILWAY_ENVIRONMENT" --yes --json >/dev/null; wait_healthy "$s"; done; }

cutover(){ assert_syncing; start_tunnels; check_services_stopped; set_phase cutting_over; fence_source; wait_pg; sync_sequences; sleep "$REDIS_SETTLE_SECONDS"; docker stop --time 30 "$REDISSHAKE_CONTAINER" >/dev/null; verify; set_phase verified; start_services; [[ -z "${MIGRATION_CUTOVER_HOOK:-}" ]] || bash -lc "$MIGRATION_CUTOVER_HOOK"; set_phase completed; log "迁移完成，请验证登录、TG、采集、推送、COS 和支付"; }

start_keepawake(){
  if command -v caffeinate >/dev/null 2>&1 && ! pid_alive "${KEEP_AWAKE_PID:-}"; then
    caffeinate -dimsu -w $$ >/dev/null 2>&1 & KEEP_AWAKE_PID=$!; save_state KEEP_AWAKE_PID "$KEEP_AWAKE_PID"
    ok "已启用 macOS 防休眠"
  fi
}

ensure_sync_health(){
  local unhealthy=0
  nc -z 127.0.0.1 "$SOURCE_PG_PORT" 2>/dev/null || unhealthy=1
  nc -z 127.0.0.1 "$SOURCE_REDIS_PORT" 2>/dev/null || unhealthy=1
  nc -z 127.0.0.1 "$TARGET_PG_PORT" 2>/dev/null || unhealthy=1
  nc -z 127.0.0.1 "$TARGET_REDIS_PORT" 2>/dev/null || unhealthy=1
  [[ "$(container_state "$PGCOPYDB_CONTAINER")" == running ]] || unhealthy=1
  [[ "$(container_state "$REDISSHAKE_CONTAINER")" == running ]] || unhealthy=1
  if ((unhealthy)); then
    warn "检测到同步隧道或容器异常，正在自动恢复"
    stop_tunnels; start_tunnels; start_sync_tools
  fi
}

target_epoch(){ python3 - "$1" <<'PY'
import datetime,sys
h,m=map(int,sys.argv[1].split(':')); now=datetime.datetime.now(); t=now.replace(hour=h,minute=m,second=0,microsecond=0)
if t<=now:t+=datetime.timedelta(days=1)
print(int(t.timestamp()))
PY
}
schedule(){ local t="${1:-02:00}"; [[ "$t" =~ ^([01][0-9]|2[0-3]):[0-5][0-9]$ ]] || die "时间必须是 HH:MM"; assert_syncing; start_keepawake; local epoch now left recoveries=0; epoch="$(target_epoch "$t")"; save_state SCHEDULED_TIME "$t"; while :; do now="$(date +%s)"; left=$((epoch-now)); ((left<=0)) && break; if ! ensure_sync_health; then recoveries=$((recoveries+1)); ((recoveries<=3)) || die "同步连续恢复失败 3 次，请人工检查"; else recoveries=0; fi; ((left%300<30 || left<=60)) && log "切换倒计时：${left} 秒"; sleep $((left>30?30:left)); done; cutover; }
run_all(){ load_state; [[ "${MIGRATION_PHASE:-}" == syncing ]] || prepare; schedule "${1:-02:00}"; }

rollback(){ load_state; start_tunnels; ACTIVE_SOURCE_PG_USER="${MIGRATOR_USER:-$SOURCE_PG_USER}"; ACTIVE_SOURCE_PG_PASSWORD="${MIGRATOR_PASSWORD:-$SOURCE_PG_PASSWORD}"; export ACTIVE_SOURCE_PG_USER ACTIVE_SOURCE_PG_PASSWORD; local json s; json="$(railway_json)"; for s in "${BUSINESS_SERVICES[@]}"; do service_exists "$json" "$s" && railway down --service "$s" --environment "$RAILWAY_ENVIRONMENT" --yes >/dev/null 2>&1 || true; done; source_psql -c "ALTER ROLE \"$SOURCE_PG_USER\" LOGIN" >/dev/null; source_redis CLIENT UNPAUSE >/dev/null 2>&1 || true; set_phase rolled_back; log "源数据库已解除限制，请启动原腾讯云业务"; }
cleanup(){ load_state; docker rm -f "$PGCOPYDB_CONTAINER" "$REDISSHAKE_CONTAINER" >/dev/null 2>&1 || true; stop_tunnels; set_phase cleaned; log "迁移容器和隧道已清理，状态日志保留：$STATE_DIR"; }

main(){ local cmd="${1:-help}"; shift || true; case "$cmd" in precheck|check) precheck "$@";; prepare) prepare "$@";; resume) resume_sync "$@";; status) status "$@";; watch) watch_progress "$@";; cutover) cutover "$@";; schedule) schedule "$@";; run) run_all "$@";; rollback) rollback "$@";; cleanup) cleanup "$@";; help|-h|--help) usage;; *) usage; die "未知命令：$cmd";; esac; }
main "$@"
