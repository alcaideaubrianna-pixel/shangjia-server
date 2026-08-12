#!/usr/bin/env bash
set -Eeuo pipefail
SSH_HOST="${MIGRATION_SSH_HOST:-43.129.181.9}"
SSH_PORT="${MIGRATION_SSH_PORT:-22}"
SSH_USER="${MIGRATION_SSH_USER:-ubuntu}"
SSH_KEY="${MIGRATION_SSH_KEY:-$HOME/.ssh/id_xiaohuiji}"
OUTPUT_DIR="${MIGRATION_OUTPUT_DIR:-$PWD/migration-artifacts/system}"
REMOTE_DIR="${MIGRATION_REMOTE_DIR:-/tmp/xiaohuiji-migration-system}"
log(){ printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
die(){ log "错误：$*"; exit 1; }
[ -r "$SSH_KEY" ] || die "SSH 私钥不可读：$SSH_KEY"
command -v ssh >/dev/null || die "本机缺少 ssh"
command -v scp >/dev/null || die "本机缺少 scp"
command -v sha256sum >/dev/null || die "本机缺少 sha256sum"
mkdir -p "$OUTPUT_DIR"
SSH=(ssh -i "$SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=30)
SCP=(scp -q -i "$SSH_KEY" -P "$SSH_PORT" -o BatchMode=yes)
run(){ "${SSH[@]}" "${SSH_USER}@${SSH_HOST}" "$@"; }
result="$(${SSH[@]} "${SSH_USER}@${SSH_HOST}" 'bash -s' <<'REMOTE'
set -Eeuo pipefail
id="$(sudo docker ps -q --filter 'name=1Panel-postgresql' | head -1)"
[ -n "$id" ] || exit 1
env="$(sudo docker inspect "$id" --format '{{range .Config.Env}}{{println .}}{{end}}')"
u="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_USER=//p' | head -1)"
d="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_DB=//p' | head -1)"
p="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_PASSWORD=//p' | head -1)"
[ -n "$u" ] && [ -n "$p" ] || exit 1
if [ -z "$d" ]; then
  d="$(sudo docker exec -e PGPASSWORD="$p" "$id" psql -U "$u" -d postgres -Atqc "SELECT datname FROM pg_database WHERE datistemplate = false AND datallowconn = true AND datname <> 'postgres' ORDER BY pg_database_size(datname) DESC, datname LIMIT 1")"
fi
[ -n "$d" ] || exit 1
printf 'ID=%s\nPGUSER=%s\nPGDB=%s\nPASS_B64=%s\n' "$id" "$u" "$d" "$(printf '%s' "$p" | base64 -w0)"
REMOTE
)" || die "无法发现源 PostgreSQL"
eval "$result"
PGPASS="$(printf '%s' "$PASS_B64" | base64 -d)"
run "sudo mkdir -p '$REMOTE_DIR' && sudo chmod 700 '$REMOTE_DIR'"
run "sudo bash -s" <<REMOTE
set -Eeuo pipefail
ID='$ID'; PGUSER='$PGUSER'; PGDB='$PGDB'; PGPASS='$PGPASS'; ROOT='$REMOTE_DIR'
tables=(hg_sys_config hg_sys_addons_config hg_admin_role hg_admin_role_casbin hg_admin_role_menu hg_admin_menu hg_sys_dict_type hg_sys_dict_data)
args=()
existing=()
for table in "\${tables[@]}"; do
  found="\$(sudo docker exec -e PGPASSWORD="\$PGPASS" "\$ID" psql -U "\$PGUSER" -d "\$PGDB" -Atqc "SELECT to_regclass('public.\"\$table\"') IS NOT NULL")"
  if [ "\$found" = t ]; then
    args+=(--table="\$table")
    existing+=("\$table")
  fi
done
[ "\${#existing[@]}" -gt 0 ] || { echo '没有发现可导出的系统配置表' >&2; exit 1; }
sudo docker exec -e PGPASSWORD="\$PGPASS" "\$ID" pg_dump -U "\$PGUSER" -d "\$PGDB" --data-only --no-owner --no-acl --column-inserts "\${args[@]}" -f /tmp/system-config.sql
sudo docker cp "\$ID:/tmp/system-config.sql" "\$ROOT/system-config.sql"
printf '%s\\n' "\${existing[@]}" | sudo tee "\$ROOT/tables.txt" >/dev/null
sudo chown '$SSH_USER':'$SSH_USER' "\$ROOT/system-config.sql" "\$ROOT/tables.txt"
sudo chmod 600 "\$ROOT/system-config.sql" "\$ROOT/tables.txt"
REMOTE
"${SCP[@]}" "${SSH_USER}@${SSH_HOST}:$REMOTE_DIR/system-config.sql" "$OUTPUT_DIR/system-config.sql"
"${SCP[@]}" "${SSH_USER}@${SSH_HOST}:$REMOTE_DIR/tables.txt" "$OUTPUT_DIR/tables.txt"
sha256sum "$OUTPUT_DIR/system-config.sql" | tee "$OUTPUT_DIR/system-config.sha256"
cat > "$OUTPUT_DIR/manifest.txt" <<EOF_MANIFEST
created_at=$(date -Iseconds)
source_host=$SSH_HOST
scope=system-config
telegram_session=included_in_no_tables
business_tenant_data=excluded
cos=not_copied
tables_file=tables.txt
EOF_MANIFEST
run "sudo rm -f '$REMOTE_DIR/system-config.sql'" || true
log "系统配置快照完成：$OUTPUT_DIR/system-config.sql"
