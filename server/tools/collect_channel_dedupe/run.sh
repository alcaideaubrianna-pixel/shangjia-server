#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
mode="preview"
if [[ "${1:-}" == "--apply" ]]; then
  mode="apply"
elif [[ "${1:-}" != "" && "${1:-}" != "--preview" ]]; then
  echo "用法: DATABASE_URL=... $0 [--preview|--apply]" >&2
  exit 2
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "缺少 DATABASE_URL；脚本不会读取或修改默认数据库。" >&2
  exit 2
fi

if [[ "$mode" == "preview" ]]; then
  exec psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$script_dir/preview_collect_channel_duplicates.sql"
fi

if [[ "${COLLECT_CHANNEL_DEDUPE_CONFIRM:-}" != "I_UNDERSTAND_COLLECT_CHANNEL_MERGE" ]]; then
  echo "拒绝执行迁移：请设置 COLLECT_CHANNEL_DEDUPE_CONFIRM=I_UNDERSTAND_COLLECT_CHANNEL_MERGE。" >&2
  exit 3
fi

echo "警告：即将执行受保护迁移；有引用的重复记录会被拒绝，失败会自动回滚。" >&2
exec psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$script_dir/migrate_collect_channel_duplicates.sql"
