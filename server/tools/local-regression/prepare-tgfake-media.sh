#!/usr/bin/env bash
# tgfake 每次 /sim/reset 会清空文件；重置后重新上传夹具并回写当前 file_id。
set -euo pipefail

base="${YOUBAN_LOCAL_TGFAKE_URL:-http://127.0.0.1:18790}"
db="${YOUBAN_LOCAL_DB_NAME:-youban_tgfake_test}"
image_path="${YOUBAN_LOCAL_MEDIA_IMAGE:-tools/local-regression/testdata/image.jpg}"
pg_password="${YOUBAN_DB_PASSWORD:-postgres}"

export PGPASSWORD="$pg_password"
image_data="$(base64 < "$image_path" | tr -d '\n')"
curl --noproxy '*' -fsS -X POST "$base/sim/media" \
  -H 'content-type: application/json' \
  -d "{\"type\":\"photo\",\"data\":\"$image_data\",\"caption\":\"tgfake fixture\"}" >/dev/null

file_id="$(curl --noproxy '*' -fsS "$base/sim/chat/4242" | python3 -c '
import json,sys
messages=json.load(sys.stdin)["messages"]
photos=[m["photo"]["file_id"] for m in messages if "photo" in m]
if not photos:
    raise SystemExit("tgfake 未返回图片 file_id")
print(photos[-1])
')"

psql -h "${YOUBAN_DB_HOST:-127.0.0.1}" -U "${YOUBAN_DB_USER:-postgres}" -d "$db" \
  -v ON_ERROR_STOP=1 -c "UPDATE hg_youban_publish_media SET tg_file_id='$file_id',tg_cache_status='ready' WHERE media_type IN ('image','photo')" >/dev/null
echo "tgfake 媒体已准备：$file_id -> $db"
