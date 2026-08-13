#!/usr/bin/env bash
set -euo pipefail

container_name="${COS_CHECK_DB_CONTAINER:-youban-hotgo-postgres}"
public_base="${COS_CHECK_PUBLIC_BASE:-https://img.yuebanby.com}"
limit="${COS_CHECK_LIMIT:-10}"

rows="$(docker exec "$container_name" sh -lc \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -F "|" -Atc "SELECT id,path,mime_type,size FROM hg_sys_attachment WHERE drive='"'"'cos'"'"' ORDER BY id DESC LIMIT '"$limit"';"')"

if [[ -z "$rows" ]]; then
  echo "没有找到 COS 附件"
  exit 0
fi

while IFS='|' read -r id path expected_type expected_size; do
  url="${public_base%/}/${path#/}"
  temp_file="$(mktemp)"
  trap 'rm -f "$temp_file"' EXIT
  metrics="$(curl --fail --silent --show-error --location --max-time 30 \
    --output "$temp_file" --write-out '%{http_code}|%{content_type}|%{size_download}' "$url")"
  IFS='|' read -r status actual_type actual_size <<<"$metrics"
  file_type="$(file --brief "$temp_file")"
  printf 'id=%s status=%s type=%s expected_type=%s size=%s expected_size=%s file=%s url=%s\n' \
    "$id" "$status" "$actual_type" "$expected_type" "$actual_size" "$expected_size" "$file_type" "$url"
  rm -f "$temp_file"
  trap - EXIT
done <<<"$rows"
