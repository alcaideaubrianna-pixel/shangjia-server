#!/usr/bin/env bash
set -euo pipefail

# Backfill the denormalized publish record read model in small, resumable
# transactions. The cursor is the source success_record.id, so rerunning the
# script is safe and does not duplicate rows.

DB_NAME="${YOUBAN_DB_NAME:-youban_hotgo}"
DB_HOST="${YOUBAN_DB_HOST:-127.0.0.1}"
DB_PORT="${YOUBAN_DB_PORT:-5432}"
DB_USER="${YOUBAN_DB_USER:-postgres}"
export PGPASSWORD="${YOUBAN_DB_PASSWORD:-postgres}"
BATCH_SIZE="${YOUBAN_RECORD_LOG_BATCH_SIZE:-500}"
START_ID="${YOUBAN_RECORD_LOG_START_ID:-0}"
MAX_BATCHES="${YOUBAN_RECORD_LOG_MAX_BATCHES:-0}"

PSQL=(psql -X -v ON_ERROR_STOP=1 -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME")

last_id="$START_ID"
batch_no=0

echo "publish record log backfill: db=$DB_NAME host=$DB_HOST batch=$BATCH_SIZE start=$START_ID"

while :; do
  if [[ "$MAX_BATCHES" != "0" && "$batch_no" -ge "$MAX_BATCHES" ]]; then
    break
  fi

  result="$("${PSQL[@]}" -At -F $'\t' -c "
WITH batch AS MATERIALIZED (
  SELECT r.*
  FROM hg_youban_publish_success_record r
  WHERE r.id > $last_id
  ORDER BY r.id
  LIMIT $BATCH_SIZE
), snapshot AS (
  SELECT b.job_id,b.task_id,b.tenant_id,b.account_id,b.profile_id,b.channel_id,b.bot_id,
         b.operation_no,b.target_chat_id,b.action,b.status,b.message,b.created_at,
         COALESCE(p.title,'') AS title,
         COALESCE(NULLIF(a.nickname,''),NULLIF(a.username,''),'') AS account_name,
         COALESCE(NULLIF(owner.username,''),NULLIF(t.name,''),'') AS tenant_name,
         COALESCE(c.channel_title,'') AS channel_title,
         COALESCE(c.channel_username,'') AS channel_username,
         COALESCE(bot.bot_name,'') AS bot_name,
         COALESCE(bot.bot_username,'') AS bot_username,
         COALESCE(j.retry_count,0) AS retry_count,
         b.id AS source_id
  FROM batch b
  LEFT JOIN hg_youban_publish_tg_job j ON j.id=b.job_id
  LEFT JOIN hg_content_profile p ON p.id=b.profile_id
  LEFT JOIN hg_youban_publish_account a ON a.id=b.account_id AND a.deleted_at IS NULL
  LEFT JOIN hg_youban_publish_tenant t ON t.id=b.tenant_id AND t.deleted_at IS NULL
  LEFT JOIN hg_youban_publish_account owner ON owner.tenant_id=b.tenant_id
       AND owner.account_type='admin' AND owner.deleted_at IS NULL
  LEFT JOIN hg_youban_publish_channel c ON c.id=COALESCE(NULLIF(b.channel_id,0),j.channel_id)
  LEFT JOIN hg_youban_publish_bot bot ON bot.id=b.bot_id AND bot.deleted_at IS NULL
), inserted AS (
  INSERT INTO hg_youban_publish_record_log
    (job_id,task_id,tenant_id,account_id,profile_id,channel_id,bot_id,operation_no,
     target_chat_id,action,title,account_name,tenant_name,channel_title,
     channel_username,bot_name,bot_username,status,message,retry_count,created_at,updated_at)
  SELECT job_id,task_id,tenant_id,account_id,profile_id,channel_id,bot_id,operation_no,
         target_chat_id,action,title,account_name,tenant_name,channel_title,
         channel_username,bot_name,bot_username,status,LEFT(COALESCE(message,''),255),
         retry_count,created_at,created_at
  FROM snapshot
  ON CONFLICT (job_id) DO NOTHING
  RETURNING job_id
)
SELECT COALESCE((SELECT count(*) FROM inserted),0),
       COALESCE((SELECT max(source_id) FROM snapshot),$last_id),
       COALESCE((SELECT count(*) FROM batch),0);
" )"

  IFS=$'\t' read -r inserted next_id selected <<<"$result"
  if [[ "${selected:-0}" == "0" ]]; then
    break
  fi

  last_id="$next_id"
  batch_no=$((batch_no + 1))
  echo "batch=$batch_no selected=$selected inserted=$inserted last_id=$last_id"
done

echo "publish record log backfill complete: batches=$batch_no last_id=$last_id"
