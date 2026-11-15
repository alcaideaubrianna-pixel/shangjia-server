#!/usr/bin/env bash
set -euo pipefail

# Rebuild missing LSH rows from the existing perceptual_hash column.
# No Telegram/OSS download is performed. The script is resumable by media id.

DB_NAME="${YOUBAN_DB_NAME:-youban_hotgo}"
DB_HOST="${YOUBAN_DB_HOST:-127.0.0.1}"
DB_PORT="${YOUBAN_DB_PORT:-5432}"
DB_USER="${YOUBAN_DB_USER:-postgres}"
export PGPASSWORD="${YOUBAN_DB_PASSWORD:-postgres}"
BATCH_SIZE="${YOUBAN_LSH_BATCH_SIZE:-1000}"
START_ID="${YOUBAN_LSH_START_ID:-0}"
MAX_BATCHES="${YOUBAN_LSH_MAX_BATCHES:-0}"

PSQL=(psql -X -v ON_ERROR_STOP=1 -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME")
last_id="$START_ID"
batch_no=0

echo "phash lsh backfill: db=$DB_NAME batch=$BATCH_SIZE start=$START_ID"
while :; do
  if [[ "$MAX_BATCHES" != "0" && "$batch_no" -ge "$MAX_BATCHES" ]]; then break; fi
  result="$("${PSQL[@]}" -At -F $'\t' -c "
WITH batch AS MATERIALIZED (
  SELECT id,tenant_id,account_id,profile_id,media_type,lower(trim(perceptual_hash)) AS hash_value
  FROM hg_youban_publish_media
  WHERE id > $last_id AND deleted_at IS NULL
    AND perceptual_hash IS NOT NULL
    AND length(trim(perceptual_hash)) >= 16
    AND trim(perceptual_hash) ~ '^[0-9a-fA-F]{16,64}$'
  ORDER BY id
  LIMIT $BATCH_SIZE
), expanded AS (
  SELECT b.*, p.pos,
         ('x' || substr(b.hash_value, (p.pos-1)*4+1, 4))::bit(16)::int AS bucket_value
  FROM batch b
  CROSS JOIN (VALUES (1),(2),(3),(4)) AS p(pos)
), inserted AS (
  INSERT INTO hg_youban_publish_media_phash_lsh
    (tenant_id,account_id,profile_id,media_id,media_type,hash_value,bucket_pos,bucket_value,created_at,updated_at)
  SELECT tenant_id,account_id,profile_id,id,media_type,hash_value,pos,bucket_value,NOW(),NOW()
  FROM expanded
  ON CONFLICT (media_id,bucket_pos) DO UPDATE SET
    tenant_id=EXCLUDED.tenant_id, account_id=EXCLUDED.account_id,
    profile_id=EXCLUDED.profile_id, media_type=EXCLUDED.media_type,
    hash_value=EXCLUDED.hash_value, bucket_value=EXCLUDED.bucket_value,
    updated_at=EXCLUDED.updated_at
  RETURNING media_id
)
SELECT COALESCE((SELECT count(DISTINCT media_id) FROM inserted),0),
       COALESCE((SELECT max(id) FROM batch),$last_id),
       COALESCE((SELECT count(*) FROM batch),0);
" )"
  IFS=$'\t' read -r written next_id selected <<<"$result"
  if [[ "${selected:-0}" == "0" ]]; then break; fi
  last_id="$next_id"
  batch_no=$((batch_no + 1))
  echo "batch=$batch_no media=$selected written=$written last_id=$last_id"
done
echo "phash lsh backfill complete: batches=$batch_no last_id=$last_id"
