#!/usr/bin/env bash
set -Eeuo pipefail

MODE="${MIGRATION_MODE:-snapshot}"
REUSE_ARTIFACTS="${MIGRATION_REUSE_ARTIFACTS:-0}"
SSH_HOST="${MIGRATION_SSH_HOST:-43.129.181.9}"
SSH_PORT="${MIGRATION_SSH_PORT:-22}"
SSH_USER="${MIGRATION_SSH_USER:-ubuntu}"
SSH_KEY="${MIGRATION_SSH_KEY:-$HOME/.ssh/id_xiaohuiji}"
OUTPUT_DIR="${MIGRATION_OUTPUT_DIR:-$PWD/migration-artifacts}"
REMOTE_DIR="${MIGRATION_REMOTE_DIR:-/tmp/xiaohuiji-migration}"

TARGET_PGHOST="${TARGET_PGHOST:-}"
TARGET_PGPORT="${TARGET_PGPORT:-5432}"
TARGET_PGDATABASE="${TARGET_PGDATABASE:-}"
TARGET_PGUSER="${TARGET_PGUSER:-}"
TARGET_PGPASSWORD="${TARGET_PGPASSWORD:-}"
TARGET_REDIS_URL="${TARGET_REDIS_URL:-}"

log() { printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
die() { log "错误：$*"; exit 1; }

ssh_base=(ssh -i "$SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=30)
scp_base=(scp -q -i "$SSH_KEY" -P "$SSH_PORT" -o BatchMode=yes)
run_remote() { "${ssh_base[@]}" "${SSH_USER}@${SSH_HOST}" "$@"; }
remote_script() { "${ssh_base[@]}" "${SSH_USER}@${SSH_HOST}" 'bash -s'; }

check_requirements() {
  command -v ssh >/dev/null || die "本机缺少 ssh"
  command -v scp >/dev/null || die "本机缺少 scp"
  command -v sha256sum >/dev/null || die "本机缺少 sha256sum"
  command -v python3 >/dev/null || die "本机缺少 python3"
  [ -r "$SSH_KEY" ] || die "SSH 私钥不可读：$SSH_KEY"
  case "$MODE" in check|snapshot|restore-test|restore-full) ;; *) die "MIGRATION_MODE 必须是 check、snapshot、restore-test 或 restore-full" ;; esac
  case "$REUSE_ARTIFACTS" in 0|1) ;; *) die "MIGRATION_REUSE_ARTIFACTS 必须是 0 或 1" ;; esac
  mkdir -p "$OUTPUT_DIR"
}

discover_source() {
  log "[$1/8] 检查源服务器 SSH、Docker 和数据库容器"
  run_remote 'sudo -n docker ps >/dev/null' || die "源服务器无法通过 sudo 访问 Docker"
  local result
  result="$(remote_script <<'REMOTE'
set -Eeuo pipefail
postgres_id="$(sudo docker ps -q --filter 'name=1Panel-postgresql' | head -1)"
redis_id="$(sudo docker ps -q --filter 'name=1Panel-redis' | head -1)"
[ -n "$postgres_id" ] || exit 10
[ -n "$redis_id" ] || exit 11
postgres_env="$(sudo docker inspect "$postgres_id" --format '{{range .Config.Env}}{{println .}}{{end}}')"
postgres_user="$(printf '%s\n' "$postgres_env" | sed -n 's/^POSTGRES_USER=//p' | head -1)"
postgres_db="$(printf '%s\n' "$postgres_env" | sed -n 's/^POSTGRES_DB=//p' | head -1)"
postgres_password="$(printf '%s\n' "$postgres_env" | sed -n 's/^POSTGRES_PASSWORD=//p' | head -1)"
[ -n "$postgres_user" ] && [ -n "$postgres_password" ] || exit 12
if [ -z "$postgres_db" ]; then
  postgres_db="$(sudo docker exec -e PGPASSWORD="$postgres_password" "$postgres_id" psql -U "$postgres_user" -d postgres -Atqc "SELECT datname FROM pg_database WHERE datistemplate = false AND datallowconn = true AND datname <> 'postgres' ORDER BY pg_database_size(datname) DESC, datname LIMIT 1")"
fi
[ -n "$postgres_db" ] || exit 13
redis_password="$(printf '%s\n' "$(sudo docker inspect "$redis_id" --format '{{range .Config.Env}}{{println .}}{{end}}')" | sed -n 's/^REDIS_PASSWORD=//p' | head -1)"
if [ -z "$redis_password" ]; then
  redis_cmd="$(sudo docker inspect "$redis_id" --format '{{json .Config.Cmd}}')"
  redis_password="$(printf '%s' "$redis_cmd" | sed -n 's/.*--requirepass","\([^"]*\)".*/\1/p')"
fi
redis_host="$(hostname -I | awk '{print $1}')"
redis_db="$(sudo docker exec "$redis_id" redis-cli --no-auth-warning -a "$redis_password" INFO keyspace | sed -n 's/^db\([0-9][0-9]*\):keys=.*/\1/p' | head -1 | tr -d '\r')"
redis_db="${redis_db:-0}"
printf 'POSTGRES_CONTAINER=%s\n' "$postgres_id"
printf 'POSTGRES_USER=%s\n' "$postgres_user"
printf 'POSTGRES_DB=%s\n' "$postgres_db"
printf 'POSTGRES_PASSWORD_B64=%s\n' "$(printf '%s' "$postgres_password" | base64 -w0)"
printf 'REDIS_CONTAINER=%s\n' "$redis_id"
printf 'REDIS_PASSWORD_B64=%s\n' "$(printf '%s' "$redis_password" | base64 -w0)"
printf 'REDIS_HOST=%s\n' "$redis_host"
printf 'REDIS_DB=%s\n' "$redis_db"
REMOTE
  )" || die "自动发现源容器失败"
  eval "$result"
  POSTGRES_PASSWORD="$(printf '%s' "$POSTGRES_PASSWORD_B64" | base64 -d)"
  REDIS_PASSWORD="$(printf '%s' "${REDIS_PASSWORD_B64:-}" | base64 -d 2>/dev/null || true)"
  export POSTGRES_USER POSTGRES_DB POSTGRES_PASSWORD REDIS_PASSWORD
  log "源 PostgreSQL：$POSTGRES_CONTAINER / $POSTGRES_DB"
  log "源 Redis：$REDIS_CONTAINER"
  log "源 Redis DB：$REDIS_DB"
}

check_source() {
  log "[2/2] 执行只读迁移能力检查，不生成备份、不写入目标"
  local result
  result="$(remote_script <<'REMOTE'
set -Eeuo pipefail
pgid="$(sudo docker ps -q --filter 'name=1Panel-postgresql' | head -1)"
rid="$(sudo docker ps -q --filter 'name=1Panel-redis' | head -1)"
env="$(sudo docker inspect "$pgid" --format '{{range .Config.Env}}{{println .}}{{end}}')"
u="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_USER=//p' | head -1)"
d="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_DB=//p' | head -1)"
p="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_PASSWORD=//p' | head -1)"
rp="$(printf '%s\n' "$(sudo docker inspect "$rid" --format '{{json .Config.Cmd}}')" | sed -n 's/.*--requirepass","\([^"]*\)".*/\1/p')"
sudo docker exec -e PGPASSWORD="$p" "$pgid" psql -U "$u" -d "$d" -v ON_ERROR_STOP=1 -Atqc 'SELECT 1' | grep -qx 1
sudo docker exec -e PGPASSWORD="$p" "$pgid" pg_dump -U "$u" -d "$d" --schema-only --no-owner --no-acl --file=/dev/null
sudo docker exec "$rid" redis-cli --no-auth-warning -a "$rp" PING | grep -qx PONG
printf 'postgres=ok\npostgres_schema_dump=ok\nredis=ok\nredis_password=present\n'
REMOTE
  )" || die "源端只读检查失败"
  printf '%s\n' "$result"
  log "只读迁移能力检查通过"
}

create_manifest() {
  cat >"$OUTPUT_DIR/manifest.txt" <<EOF_MANIFEST
created_at=$(date -Iseconds)
mode=$MODE
source_host=$SSH_HOST
source_postgres_container=$POSTGRES_CONTAINER
source_redis_container=$REDIS_CONTAINER
telegram_session_policy=$([ "$MODE" = restore-test ] && echo excluded || echo included)
redis_policy=key_copy_with_ttl
cos_policy=do_not_copy_reuse_existing_cos
EOF_MANIFEST
}

dump_postgres() {
  log "[$1/8] 导出 PostgreSQL（全库 custom dump）"
  local remote_dump="$REMOTE_DIR/youban-postgres.dump"
  run_remote "sudo mkdir -p '$REMOTE_DIR' && sudo chmod 700 '$REMOTE_DIR'"
  run_remote "sudo docker exec -e PGPASSWORD='$POSTGRES_PASSWORD' '$POSTGRES_CONTAINER' pg_dump -U '$POSTGRES_USER' -d '$POSTGRES_DB' --format=custom --no-owner --no-acl --verbose -f /tmp/youban-postgres.dump"
  run_remote "sudo docker cp '$POSTGRES_CONTAINER:/tmp/youban-postgres.dump' '$remote_dump' && sudo chown '$SSH_USER':'$SSH_USER' '$remote_dump' && chmod 600 '$remote_dump'"
  run_remote "sudo sha256sum '$remote_dump'" | tee "$OUTPUT_DIR/postgres.remote.sha256"
  "${scp_base[@]}" "${SSH_USER}@${SSH_HOST}:$remote_dump" "$OUTPUT_DIR/youban-postgres.dump"
  sha256sum "$OUTPUT_DIR/youban-postgres.dump" | tee "$OUTPUT_DIR/postgres.sha256"
  log "PostgreSQL 完成：$(du -h "$OUTPUT_DIR/youban-postgres.dump" | awk '{print $1}')"
}

snapshot_redis() {
  log "[$1/8] 导出 Redis RDB（保留 Asynq、租约、锁和去重数据）"
  local remote_rdb="$REMOTE_DIR/youban-redis.rdb"
  [ -n "$REDIS_PASSWORD" ] || die "未发现 Redis 密码，拒绝生成未认证快照"
  run_remote "sudo mkdir -p '$REMOTE_DIR' && sudo docker exec '$REDIS_CONTAINER' redis-cli --no-auth-warning -a '$REDIS_PASSWORD' --rdb /tmp/youban-redis.rdb"
  run_remote "sudo docker cp '$REDIS_CONTAINER:/tmp/youban-redis.rdb' '$remote_rdb' && sudo chown '$SSH_USER':'$SSH_USER' '$remote_rdb' && chmod 600 '$remote_rdb'"
  run_remote "sudo sha256sum '$remote_rdb'" | tee "$OUTPUT_DIR/redis.remote.sha256"
  "${scp_base[@]}" "${SSH_USER}@${SSH_HOST}:$remote_rdb" "$OUTPUT_DIR/youban-redis.rdb"
  sha256sum "$OUTPUT_DIR/youban-redis.rdb" | tee "$OUTPUT_DIR/redis.sha256"
  log "Redis RDB 完成：$(du -h "$OUTPUT_DIR/youban-redis.rdb" | awk '{print $1}')"
}

restore_postgres() {
  [ -n "$TARGET_PGHOST" ] && [ -n "$TARGET_PGDATABASE" ] && [ -n "$TARGET_PGUSER" ] && [ -n "$TARGET_PGPASSWORD" ] || die "恢复 PostgreSQL 需要 TARGET_PGHOST、TARGET_PGDATABASE、TARGET_PGUSER、TARGET_PGPASSWORD"
  [ -f "$OUTPUT_DIR/youban-postgres.dump" ] || die "找不到 PostgreSQL 备份"
  log "[$1/8] 校验并恢复 PostgreSQL：$MODE"
  sha256sum -c "$OUTPUT_DIR/postgres.sha256"
  local exclude=()
  if [ "$MODE" = restore-test ]; then
    exclude+=(--exclude-table-data=hg_youban_publish_tg_session)
    log "测试模式：保留 TG session 表结构，但不导入 session_data"
  else
    log "正式模式：导入 TG session，迁移后不需要重新登录"
  fi
  PGPASSWORD="$TARGET_PGPASSWORD" pg_restore \
    --verbose --no-owner --no-acl --exit-on-error \
    -h "$TARGET_PGHOST" -p "$TARGET_PGPORT" -U "$TARGET_PGUSER" -d "$TARGET_PGDATABASE" \
    "${exclude[@]}" "$OUTPUT_DIR/youban-postgres.dump"
}

restore_redis() {
  [ -n "$TARGET_REDIS_URL" ] || die "恢复 Redis 需要 TARGET_REDIS_URL"
  [ -f "$OUTPUT_DIR/youban-redis.rdb" ] || die "找不到 Redis RDB 备份"
  sha256sum -c "$OUTPUT_DIR/redis.sha256"
  log "[$1/8] Redis RDB 已校验；通过停机后的源 Redis 复制全部 Key 和 TTL"
  log "RDB 用于留档和回滚，Redis 协议复制保证 Railway 能直接恢复 Key/TTL"
  log "目标 Redis 将被覆盖同名 Key，不会执行 FLUSHDB"
  python3 "$SCRIPT_DIR/redis-copy.py" \
    --source-host "$REDIS_TUNNEL_HOST" --source-port "$REDIS_TUNNEL_PORT" --source-password "$REDIS_PASSWORD" --source-db "$REDIS_DB" \
    --target-url "$TARGET_REDIS_URL" --progress-every 500
}

cleanup_remote() {
  log "[$1/8] 清理源服务器临时文件"
  run_remote "sudo rm -f '$REMOTE_DIR/youban-postgres.dump' '$REMOTE_DIR/youban-redis.rdb'" || true
}

main() {
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  export SCRIPT_DIR
  check_requirements
  log "迁移模式：$MODE"
  log "说明：脚本不会调用 Telegram API，不会注销或刷新任何 TG session"
  discover_source 1
  if [ "$MODE" = check ]; then
    check_source
    return
  fi
  create_manifest
  if [ "$REUSE_ARTIFACTS" = 1 ]; then
    log "复用已有迁移产物：$OUTPUT_DIR"
    [ -f "$OUTPUT_DIR/youban-postgres.dump" ] || die "缺少 youban-postgres.dump"
    [ -f "$OUTPUT_DIR/youban-redis.rdb" ] || die "缺少 youban-redis.rdb"
  else
    dump_postgres 2
    snapshot_redis 3
    cleanup_remote 4
  fi
  if [ "$MODE" = snapshot ]; then
    log "[8/8] 快照完成：$OUTPUT_DIR"
    return
  fi
  [ "${MIGRATION_CONFIRM:-}" = "I_UNDERSTAND_TARGET_WILL_BE_WRITTEN" ] || die "恢复会写入目标 PostgreSQL/Redis，请设置 MIGRATION_CONFIRM=I_UNDERSTAND_TARGET_WILL_BE_WRITTEN"
  restore_postgres 5
  log "[6/8] PostgreSQL 恢复完成"
  log "建立源 Redis SSH 隧道，供 Redis 协议复制使用"
  REDIS_TUNNEL_PORT="${REDIS_TUNNEL_PORT:-16379}"
  REDIS_TUNNEL_HOST=127.0.0.1
  ssh -N -i "$SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ExitOnForwardFailure=yes \
    -L "$REDIS_TUNNEL_PORT:$REDIS_HOST:6379" "${SSH_USER}@${SSH_HOST}" &
  tunnel_pid=$!
  trap 'kill "$tunnel_pid" 2>/dev/null || true' EXIT
  restore_redis 7
  log "[8/8] 全量迁移完成，请执行迁移后 case 验证"
}

main "$@"
