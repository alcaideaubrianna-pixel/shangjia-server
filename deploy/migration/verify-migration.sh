#!/usr/bin/env bash
set -Eeuo pipefail

PGHOST="${TARGET_PGHOST:-}"
PGPORT="${TARGET_PGPORT:-5432}"
PGDATABASE="${TARGET_PGDATABASE:-}"
PGUSER="${TARGET_PGUSER:-}"
PGPASSWORD_VALUE="${TARGET_PGPASSWORD:-}"
REDIS_URL="${TARGET_REDIS_URL:-}"
SKIP_TG_SESSION="${MIGRATION_TEST_SKIP_TG_SESSION:-1}"

die() { printf '验证失败：%s\n' "$*" >&2; exit 1; }
log() { printf '[验证] %s\n' "$*"; }

[ -n "$PGHOST" ] && [ -n "$PGDATABASE" ] && [ -n "$PGUSER" ] && [ -n "$PGPASSWORD_VALUE" ] || die "缺少 TARGET_PGHOST、TARGET_PGDATABASE、TARGET_PGUSER、TARGET_PGPASSWORD"
[ -n "$REDIS_URL" ] || die "缺少 TARGET_REDIS_URL"
command -v psql >/dev/null || die "本机缺少 psql"
command -v python3 >/dev/null || die "本机缺少 python3"

export PGPASSWORD="$PGPASSWORD_VALUE"
log "检查 PostgreSQL 连通性"
psql -X -v ON_ERROR_STOP=1 -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -Atc 'SELECT 1' | grep -qx 1 || die "PostgreSQL 不可用"

log "检查核心业务表和数量"
for item in \
  'tenant:hg_youban_publish_tenant' \
  'account:hg_youban_publish_account' \
  'profile:hg_youban_publish_profile' \
  'media:hg_youban_publish_media' \
  'collect_source:hg_youban_publish_collect_source' \
  'collect_event:hg_youban_publish_collect_event' \
  'tg_account:hg_youban_publish_tg_account' \
  'order:hg_admin_order'; do
  label="${item%%:*}"; table="${item#*:}"
  result=$(psql -X -v ON_ERROR_STOP=1 -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -Atc "SELECT CASE WHEN to_regclass('public.\"$table\"') IS NULL THEN 'SKIP' ELSE COUNT(*)::text END FROM public.\"$table\"" 2>/dev/null || true)
  if [ "$result" = SKIP ] || [ -z "$result" ]; then log "$label：表不存在，跳过"; else printf '%s=%s\n' "$label" "$result"; fi
done

if [ "$SKIP_TG_SESSION" = 1 ]; then
  log "按测试模式跳过 Telegram session 校验"
else
  log "检查 Telegram session 数量"
  psql -X -v ON_ERROR_STOP=1 -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -c 'SELECT COUNT(*) AS tg_session_rows FROM hg_youban_publish_tg_session;'
fi

log "检查 Redis 连通性和 Key 数量"
python3 - "$REDIS_URL" <<'PY'
import socket
import ssl
import sys
from urllib.parse import urlparse

url = urlparse(sys.argv[1])
host = url.hostname
port = url.port or 6379
password = url.password
sock = socket.create_connection((host, port), timeout=20)
if url.scheme == "rediss":
    sock = ssl.create_default_context().wrap_socket(sock, server_hostname=host)
file = sock.makefile("rb")

def send(*args):
    body = [f"*{len(args)}\r\n".encode()]
    for value in args:
        value = str(value).encode()
        body.append(f"${len(value)}\r\n".encode() + value + b"\r\n")
    sock.sendall(b"".join(body))
    prefix = file.read(1)
    line = file.readline().rstrip(b"\r\n")
    if prefix == b"+":
        return line.decode()
    if prefix == b":":
        return int(line)
    if prefix == b"-":
        raise RuntimeError(line.decode())
    raise RuntimeError(f"unexpected redis response: {prefix!r}")

if password:
    send("AUTH", password)
if url.path.strip("/"):
    send("SELECT", int(url.path.strip("/")))
if send("PING") != "PONG":
    raise RuntimeError("Redis PING failed")
print(f"redis_keys={send('DBSIZE')}")
sock.close()
PY

log "基础迁移验证完成；测试模式不会启动 Account，不会触发 TG session 登录"
