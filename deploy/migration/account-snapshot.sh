#!/usr/bin/env bash
set -Eeuo pipefail

TENANT_ID="${MIGRATION_TENANT_ID:-}"
ACCOUNT_ID="${MIGRATION_ACCOUNT_ID:-}"
SSH_HOST="${MIGRATION_SSH_HOST:-43.129.181.9}"
SSH_PORT="${MIGRATION_SSH_PORT:-22}"
SSH_USER="${MIGRATION_SSH_USER:-ubuntu}"
SSH_KEY="${MIGRATION_SSH_KEY:-$HOME/.ssh/id_xiaohuiji}"
OUTPUT_DIR="${MIGRATION_OUTPUT_DIR:-$PWD/migration-artifacts/account-${ACCOUNT_ID:-unknown}}"
REMOTE_DIR="${MIGRATION_REMOTE_DIR:-/tmp/xiaohuiji-migration-account-${ACCOUNT_ID:-unknown}}"

log(){ printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
die(){ log "错误：$*"; exit 1; }
[ "$TENANT_ID" -gt 0 ] 2>/dev/null || die "MIGRATION_TENANT_ID 必须是正整数"
[ "$ACCOUNT_ID" -gt 0 ] 2>/dev/null || die "MIGRATION_ACCOUNT_ID 必须是正整数"
[ -r "$SSH_KEY" ] || die "SSH 私钥不可读：$SSH_KEY"
command -v ssh >/dev/null || die "本机缺少 ssh"
command -v scp >/dev/null || die "本机缺少 scp"
command -v sha256sum >/dev/null || die "本机缺少 sha256sum"
mkdir -p "$OUTPUT_DIR"
SSH=(ssh -i "$SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=30)
SCP=(scp -q -i "$SSH_KEY" -P "$SSH_PORT" -o BatchMode=yes)
run(){ "${SSH[@]}" "${SSH_USER}@${SSH_HOST}" "$@"; }

log "开始导出单账号数据 tenant_id=$TENANT_ID account_id=$ACCOUNT_ID"
result="$(${SSH[@]} "${SSH_USER}@${SSH_HOST}" 'bash -s' <<'REMOTE'
set -Eeuo pipefail
pgid="$(sudo docker ps -q --filter 'name=1Panel-postgresql' | head -1)"
[ -n "$pgid" ] || exit 1
env="$(sudo docker inspect "$pgid" --format '{{range .Config.Env}}{{println .}}{{end}}')"
u="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_USER=//p' | head -1)"
d="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_DB=//p' | head -1)"
p="$(printf '%s\n' "$env" | sed -n 's/^POSTGRES_PASSWORD=//p' | head -1)"
[ -n "$u" ] && [ -n "$p" ] || exit 1
if [ -z "$d" ]; then
  d="$(sudo docker exec -e PGPASSWORD="$p" "$pgid" psql -U "$u" -d postgres -Atqc "SELECT datname FROM pg_database WHERE datistemplate = false AND datallowconn = true AND datname <> 'postgres' ORDER BY pg_database_size(datname) DESC, datname LIMIT 1")"
fi
[ -n "$d" ] || exit 1
printf 'PGID=%s\nPGUSER=%s\nPGDB=%s\nPGPASS_B64=%s\n' "$pgid" "$u" "$d" "$(printf '%s' "$p" | base64 -w0)"
REMOTE
)" || die "无法发现源 PostgreSQL"
eval "$result"
PGPASS="$(printf '%s' "$PGPASS_B64" | base64 -d)"
run "sudo rm -rf '$REMOTE_DIR' && sudo mkdir -p '$REMOTE_DIR' && sudo chmod 700 '$REMOTE_DIR'"

run "sudo bash -s" <<REMOTE
set -Eeuo pipefail
PGID='$PGID'; PGUSER='$PGUSER'; PGDB='$PGDB'; PGPASS='$PGPASS'; ROOT='$REMOTE_DIR'; TENANT='$TENANT_ID'; ACCOUNT='$ACCOUNT_ID'
shopt -s nullglob
sudo docker exec -e PGPASSWORD="\$PGPASS" "\$PGID" psql -U "\$PGUSER" -d "\$PGDB" -v ON_ERROR_STOP=1 -At -c "SELECT table_name FROM information_schema.columns WHERE table_schema='public' AND column_name IN ('tenant_id','account_id') GROUP BY table_name ORDER BY table_name" > "\$ROOT/tables.txt"
sudo docker exec -e PGPASSWORD="\$PGPASS" "\$PGID" psql -U "\$PGUSER" -d "\$PGDB" -v ON_ERROR_STOP=1 -At -F $'\\t' -c "SELECT table_name, string_agg(quote_ident(column_name), ',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema='public' AND column_name IN ('tenant_id','account_id') GROUP BY table_name ORDER BY table_name" > "\$ROOT/table-columns.tsv"
while IFS= read -r table; do
  [ -n "\$table" ] || continue
  case "\$table" in
    hg_youban_publish_tg_account|hg_youban_publish_tg_session|hg_youban_publish_tg_login)
      echo "\$table|excluded_telegram_runtime_data" >> "\$ROOT/manifest.tsv"
      continue
      ;;
  esac
  columns="\$(sudo docker exec -e PGPASSWORD="\$PGPASS" "\$PGID" psql -U "\$PGUSER" -d "\$PGDB" -Atc "SELECT string_agg(quote_ident(column_name), ',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema='public' AND table_name='\$table'")"
  [ -n "\$columns" ] || continue
  has_account="\$(sudo docker exec -e PGPASSWORD="\$PGPASS" "\$PGID" psql -U "\$PGUSER" -d "\$PGDB" -Atc "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='\$table' AND column_name='account_id')")"
  if [ "\$has_account" = t ]; then
    where="account_id=\$ACCOUNT"
  else
    where="tenant_id=\$TENANT"
  fi
  output="\$ROOT/\$table.csv"
  echo "\$table|\$columns|\$where" >> "\$ROOT/manifest.tsv"
  sudo docker exec -e PGPASSWORD="\$PGPASS" "\$PGID" psql -U "\$PGUSER" -d "\$PGDB" -v ON_ERROR_STOP=1 -c "COPY (SELECT \$columns FROM public.\"\$table\" WHERE \$where) TO STDOUT WITH (FORMAT csv, HEADER true)" > "\$output"
done < "\$ROOT/tables.txt"
printf 'tenant_id=%s\\naccount_id=%s\\ninclude_tg_account_and_session=0\\n' "\$TENANT" "\$ACCOUNT" > "\$ROOT/README.txt"
csv_files=("\$ROOT"/*.csv)
tar_files=(tables.txt table-columns.tsv manifest.tsv README.txt)
for csv_file in "\${csv_files[@]}"; do
  tar_files+=("\$(basename "\$csv_file")")
done
tar -C "\$ROOT" -czf "\$ROOT/account-data.tar.gz" "\${tar_files[@]}"
chown '$SSH_USER':'$SSH_USER' "\$ROOT/account-data.tar.gz"
chmod 600 "\$ROOT/account-data.tar.gz"
REMOTE

"${SCP[@]}" "${SSH_USER}@${SSH_HOST}:$REMOTE_DIR/account-data.tar.gz" "$OUTPUT_DIR/account-data.tar.gz"
sha256sum "$OUTPUT_DIR/account-data.tar.gz" | tee "$OUTPUT_DIR/account-data.sha256"
cat > "$OUTPUT_DIR/manifest.txt" <<EOF_MANIFEST
created_at=$(date -Iseconds)
source_host=$SSH_HOST
scope=single-account-direct-tenant-account-columns
tenant_id=$TENANT_ID
account_id=$ACCOUNT_ID
telegram_session=included_0
redis=not_included_use_full_redis_snapshot
warning=only_tables_with_tenant_id_or_account_id_are_exported; review manifest.tsv before import
EOF_MANIFEST
run "sudo rm -rf '$REMOTE_DIR'" || true
log "单账号数据导出完成：$OUTPUT_DIR/account-data.tar.gz"
log "导入前必须查看 tables.txt/manifest.tsv，确认不会覆盖其他租户"
