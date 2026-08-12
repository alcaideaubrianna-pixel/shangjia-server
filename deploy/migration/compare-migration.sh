#!/usr/bin/env bash
set -Eeuo pipefail

SSH_HOST="${MIGRATION_SSH_HOST:-43.129.181.9}"
SSH_PORT="${MIGRATION_SSH_PORT:-22}"
SSH_USER="${MIGRATION_SSH_USER:-ubuntu}"
SSH_KEY="${MIGRATION_SSH_KEY:-$HOME/.ssh/id_xiaohuiji}"
TARGET_PGHOST="${TARGET_PGHOST:-}"
TARGET_PGPORT="${TARGET_PGPORT:-5432}"
TARGET_PGDATABASE="${TARGET_PGDATABASE:-}"
TARGET_PGUSER="${TARGET_PGUSER:-}"
TARGET_PGPASSWORD="${TARGET_PGPASSWORD:-}"
TARGET_REDIS_URL="${TARGET_REDIS_URL:-}"
REMOTE_DIR="${MIGRATION_REMOTE_DIR:-/tmp/xiaohuiji-migration-compare}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log(){ printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
die(){ log "错误：$*" >&2; exit 1; }
[ -r "$SSH_KEY" ] || die "SSH 私钥不可读：$SSH_KEY"
[ -n "$TARGET_PGHOST" ] && [ -n "$TARGET_PGDATABASE" ] && [ -n "$TARGET_PGUSER" ] && [ -n "$TARGET_PGPASSWORD" ] || die '缺少目标 PostgreSQL 配置'
[ -n "$TARGET_REDIS_URL" ] || die '缺少 TARGET_REDIS_URL'
command -v ssh >/dev/null || die '本机缺少 ssh'
command -v psql >/dev/null || die '本机缺少 psql'
command -v python3 >/dev/null || die '本机缺少 python3'

SSH=(ssh -i "$SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=30)
run_remote(){ "${SSH[@]}" "${SSH_USER}@${SSH_HOST}" "$@"; }
remote_script(){ "${SSH[@]}" "${SSH_USER}@${SSH_HOST}" 'bash -s'; }
TMP_DIR="$(mktemp -d)"
tunnel_pid=''
cleanup(){
  [ -z "$tunnel_pid" ] || kill "$tunnel_pid" 2>/dev/null || true
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

log '发现源 PostgreSQL 和 Redis'
source_info="$(remote_script <<'REMOTE'
set -Eeuo pipefail
pgid="$(sudo docker ps -q --filter 'name=1Panel-postgresql' | head -1)"
rid="$(sudo docker ps -q --filter 'name=1Panel-redis' | head -1)"
[ -n "$pgid" ] && [ -n "$rid" ]
env="$(sudo docker inspect "$pgid" --format '{{range .Config.Env}}{{println .}}{{end}}')"
u="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_USER=//p' | head -1)"
d="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_DB=//p' | head -1)"
p="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_PASSWORD=//p' | head -1)"
[ -n "$u" ] && [ -n "$p" ]
if [ -z "$d" ]; then
  d="$(sudo docker exec -e PGPASSWORD="$p" "$pgid" psql -U "$u" -d postgres -Atqc "SELECT datname FROM pg_database WHERE datistemplate = false AND datallowconn = true AND datname <> 'postgres' ORDER BY pg_database_size(datname) DESC, datname LIMIT 1")"
fi
[ -n "$d" ]
redis_env="$(sudo docker inspect "$rid" --format '{{range .Config.Env}}{{println .}}{{end}}')"
rp="$(printf '%s\n' "$redis_env" | sed -n 's/^REDIS_PASSWORD=//p' | head -1)"
if [ -z "$rp" ]; then
  redis_cmd="$(sudo docker inspect "$rid" --format '{{json .Config.Cmd}}')"
  rp="$(printf '%s' "$redis_cmd" | sed -n 's/.*--requirepass","\([^"]*\)".*/\1/p')"
fi
rh="$(hostname -I | awk '{print $1}')"
rdb="$(sudo docker exec "$rid" redis-cli --no-auth-warning -a "$rp" INFO keyspace | sed -n 's/^db\([0-9][0-9]*\):keys=.*/\1/p' | head -1 | tr -d '\r')"
rdb="${rdb:-0}"
printf 'PGID=%s\nPGUSER=%s\nPGDB=%s\nPGPASS_B64=%s\nREDIS_HOST=%s\nREDIS_PASSWORD_B64=%s\nREDIS_DB=%s\n' "$pgid" "$u" "$d" "$(printf '%s' "$p" | base64 -w0)" "$rh" "$(printf '%s' "$rp" | base64 -w0)" "$rdb"
REMOTE
)" || die '源端容器发现失败'
eval "$source_info"
PGPASS="$(printf '%s' "$PGPASS_B64" | base64 -d)"
REDIS_PASSWORD="$(printf '%s' "$REDIS_PASSWORD_B64" | base64 -d)"

log '生成源 PostgreSQL 全表行数清单'
remote_script <<REMOTE > "$TMP_DIR/source-pg.tsv"
set -Eeuo pipefail
PGID='$PGID'; PGUSER='$PGUSER'; PGDB='$PGDB'; PGPASS='$PGPASS'
sudo docker exec -e PGPASSWORD="\$PGPASS" "\$PGID" psql -U "\$PGUSER" -d "\$PGDB" -Atqc "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name" | while IFS= read -r table; do
  [ -n "\$table" ] || continue
  count="\$(sudo docker exec -e PGPASSWORD="\$PGPASS" "\$PGID" psql -U "\$PGUSER" -d "\$PGDB" -Atqc "SELECT count(*) FROM public.\"\$table\"")"
  printf '%s\\t%s\\n' "\$table" "\$count"
done
REMOTE
sort "$TMP_DIR/source-pg.tsv" -o "$TMP_DIR/source-pg.tsv"

log '生成目标 PostgreSQL 全表行数清单'
export PGPASSWORD="$TARGET_PGPASSWORD"
psql -X -v ON_ERROR_STOP=1 -h "$TARGET_PGHOST" -p "$TARGET_PGPORT" -U "$TARGET_PGUSER" -d "$TARGET_PGDATABASE" -Atqc "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name" | while IFS= read -r table; do
  [ -n "$table" ] || continue
  count="$(psql -X -v ON_ERROR_STOP=1 -h "$TARGET_PGHOST" -p "$TARGET_PGPORT" -U "$TARGET_PGUSER" -d "$TARGET_PGDATABASE" -Atqc "SELECT count(*) FROM public.\"$table\"")"
  printf '%s\t%s\n' "$table" "$count"
done | sort > "$TMP_DIR/target-pg.tsv"

if ! diff -u "$TMP_DIR/source-pg.tsv" "$TMP_DIR/target-pg.tsv" > "$TMP_DIR/pg.diff"; then
  cat "$TMP_DIR/pg.diff"
  die 'PostgreSQL 全表行数不一致'
fi
log "PostgreSQL 一致性通过：$(wc -l < "$TMP_DIR/source-pg.tsv" | tr -d ' ') 张表行数一致"

log '建立源 Redis SSH 隧道并校验 Key、TTL 和内容摘要'
TUNNEL_PORT="${MIGRATION_COMPARE_TUNNEL_PORT:-16380}"
ssh -N -i "$SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ExitOnForwardFailure=yes -L "$TUNNEL_PORT:$REDIS_HOST:6379" "${SSH_USER}@${SSH_HOST}" &
tunnel_pid=$!
sleep 1
python3 "$SCRIPT_DIR/redis-compare.py" \
  --source-host 127.0.0.1 --source-port "$TUNNEL_PORT" --source-password "$REDIS_PASSWORD" --source-db "$REDIS_DB" \
  --target-url "$TARGET_REDIS_URL" --ttl-tolerance-ms "${MIGRATION_REDIS_TTL_TOLERANCE_MS:-5000}"
log '迁移后 PostgreSQL + Redis 一致性校验全部通过'
