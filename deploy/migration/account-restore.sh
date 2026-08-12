#!/usr/bin/env bash
set -Eeuo pipefail

INPUT_DIR="${MIGRATION_ACCOUNT_INPUT_DIR:-}"
PGHOST="${TARGET_PGHOST:-}"
PGPORT="${TARGET_PGPORT:-5432}"
PGDATABASE="${TARGET_PGDATABASE:-}"
PGUSER="${TARGET_PGUSER:-}"
PGPASSWORD_VALUE="${TARGET_PGPASSWORD:-}"
TARGET_TENANT_ID="${MIGRATION_TARGET_TENANT_ID:-}"
TARGET_ACCOUNT_ID="${MIGRATION_TARGET_ACCOUNT_ID:-}"
DRY_RUN="${MIGRATION_DRY_RUN:-1}"
CONFIRM="${MIGRATION_ACCOUNT_RESTORE_CONFIRM:-}"

log(){ printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
die(){ log "错误：$*" >&2; exit 1; }
[ -n "$INPUT_DIR" ] || die '请设置 MIGRATION_ACCOUNT_INPUT_DIR'
[ -d "$INPUT_DIR" ] || die "找不到导出目录：$INPUT_DIR"
[ -n "$PGHOST" ] && [ -n "$PGDATABASE" ] && [ -n "$PGUSER" ] && [ -n "$PGPASSWORD_VALUE" ] || die '缺少 TARGET_PGHOST、TARGET_PGDATABASE、TARGET_PGUSER、TARGET_PGPASSWORD'
[ "$TARGET_TENANT_ID" -gt 0 ] 2>/dev/null || die 'MIGRATION_TARGET_TENANT_ID 必须是正整数'
[ "$TARGET_ACCOUNT_ID" -gt 0 ] 2>/dev/null || die 'MIGRATION_TARGET_ACCOUNT_ID 必须是正整数'
case "$DRY_RUN" in 0|1) ;; *) die 'MIGRATION_DRY_RUN 必须是 0 或 1' ;; esac
command -v psql >/dev/null || die '本机缺少 psql'
[ -r "$INPUT_DIR/manifest.tsv" ] || die '导出包缺少 manifest.tsv'
[ -r "$INPUT_DIR/tables.txt" ] || die '导出包缺少 tables.txt'

export PGPASSWORD="$PGPASSWORD_VALUE"
psql_cmd=(psql -X -v ON_ERROR_STOP=1 -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE")

log '检查目标 PostgreSQL 连通性'
"${psql_cmd[@]}" -Atc 'SELECT 1' | grep -qx 1 || die '目标 PostgreSQL 不可用'

# 账号数据导出保持源端 ID；目标租户必须已存在，目标账号必须不存在。
if [ "$DRY_RUN" = 1 ]; then
  log 'dry-run：仅检查文件、目标连通性和目标租户/账号占用情况，不写入数据库'
else
  [ "$CONFIRM" = 'I_UNDERSTAND_ACCOUNT_DATA_WILL_BE_INSERTED' ] || die '真实导入请设置 MIGRATION_ACCOUNT_RESTORE_CONFIRM=I_UNDERSTAND_ACCOUNT_DATA_WILL_BE_INSERTED'
fi

tenant_exists=$("${psql_cmd[@]}" -Atc "SELECT CASE WHEN to_regclass('public.hg_youban_publish_tenant') IS NULL THEN 0 WHEN EXISTS (SELECT 1 FROM hg_youban_publish_tenant WHERE id=$TARGET_TENANT_ID) THEN 1 ELSE 0 END")
account_exists=$("${psql_cmd[@]}" -Atc "SELECT CASE WHEN to_regclass('public.hg_youban_publish_account') IS NULL THEN 0 WHEN EXISTS (SELECT 1 FROM hg_youban_publish_account WHERE id=$TARGET_ACCOUNT_ID) THEN 1 ELSE 0 END")
[ "$tenant_exists" = 1 ] || die "目标 tenant_id=$TARGET_TENANT_ID 不存在，请先创建或迁移目标租户"
[ "$account_exists" = 0 ] || die "目标 account_id=$TARGET_ACCOUNT_ID 已存在，拒绝导入以避免覆盖数据"

mapfile -t tables < "$INPUT_DIR/tables.txt"
[ "${#tables[@]}" -gt 0 ] || die 'tables.txt 为空'
for table in "${tables[@]}"; do
  [ -n "$table" ] || continue
  case "$table" in
    hg_youban_publish_tg_account|hg_youban_publish_tg_session|hg_youban_publish_tg_login) die "导出包包含被禁止导入的 Telegram 运行时表：$table" ;;
  esac
  [ -r "$INPUT_DIR/$table.csv" ] || die "缺少数据文件：$table.csv"
  columns=$(awk -F'|' -v name="$table" '$1 == name {print $2; exit}' "$INPUT_DIR/manifest.tsv")
  [ -n "$columns" ] || die "manifest.tsv 缺少字段定义：$table"
  if [ "$DRY_RUN" = 1 ]; then
    rows=$(tail -n +2 "$INPUT_DIR/$table.csv" | wc -l | tr -d ' ')
    log "检查 $table：$rows 行"
  fi
done

[ "$DRY_RUN" = 1 ] && { log 'dry-run 完成，未写入任何数据'; exit 0; }

log "开始导入 ${#tables[@]} 张表（单事务）"
# 导入前先建临时表，再用 INSERT 保持列白名单；任何一张表失败都会回滚整个账号导入。
{
  printf 'BEGIN;\nSET CONSTRAINTS ALL DEFERRED;\n'
  for table in "${tables[@]}"; do
    [ -n "$table" ] || continue
    columns=$(awk -F'|' -v name="$table" '$1 == name {print $2; exit}' "$INPUT_DIR/manifest.tsv")
    tmp="migration_import_${table}"
    printf 'CREATE TEMP TABLE "%s" (LIKE public."%s" INCLUDING DEFAULTS) ON COMMIT DROP;\n' "$tmp" "$table"
    printf '\\copy "%s" (%s) FROM %s WITH (FORMAT csv, HEADER true);\n' "$tmp" "$columns" "'${INPUT_DIR}/${table}.csv'"
    printf 'INSERT INTO public."%s" (%s) SELECT %s FROM "%s";\n' "$table" "$columns" "$columns" "$tmp"
  done
  printf 'COMMIT;\n'
} | "${psql_cmd[@]}"
log '单账号导入完成；Telegram session、Redis、Asynq 运行态均未导入'
