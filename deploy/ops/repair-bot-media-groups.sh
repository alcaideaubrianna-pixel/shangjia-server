#!/usr/bin/env bash
set -euo pipefail

MODE="dry-run"
SOURCE_ID="54"
CUTOFF="2026-08-12 19:04:37"
OUTPUT_DIR="${OUTPUT_DIR:-./bot-media-repair-$(date +%Y%m%d-%H%M%S)}"
PSQL_BIN="${PSQL_BIN:-psql}"

usage() {
  cat <<'USAGE'
Usage:
  DB_DSN='postgres://...' repair-bot-media-groups.sh [--dry-run|--apply] [--source-id ID] [--cutoff TIME] [--output-dir DIR]

The script repairs historical Bot media-group profiles that contain exactly one current media item while a complete account-collected event with the same Telegram grouped_id is available.

Safety:
  - Defaults to --dry-run.
  - Only uses source media rows whose cache_status is ready and storage_path is non-empty.
  - Skips profiles already containing more than one active media item.
  - Inserts only missing media matched by storage_path, md5, or file URL.
  - Does not create Telegram jobs or automatically republish profiles.
  - Writes candidate CSV, profile-count backup CSV, inserted-media CSV, and rollback SQL.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) MODE="dry-run"; shift ;;
    --apply) MODE="apply"; shift ;;
    --source-id) SOURCE_ID="${2:?missing source id}"; shift 2 ;;
    --cutoff) CUTOFF="${2:?missing cutoff}"; shift 2 ;;
    --output-dir) OUTPUT_DIR="${2:?missing output dir}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "${DB_DSN:-}" ]]; then
  echo "Missing DB_DSN" >&2
  exit 1
fi
if ! [[ "$SOURCE_ID" =~ ^[0-9]+$ ]]; then
  echo "source id must be numeric" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"
CANDIDATES_CSV="$OUTPUT_DIR/candidates.csv"
PROFILE_BACKUP_CSV="$OUTPUT_DIR/profile-counts-before.csv"
INSERTED_CSV="$OUTPUT_DIR/inserted-media.csv"
ROLLBACK_SQL="$OUTPUT_DIR/rollback.sql"

read -r -d '' CANDIDATE_SQL <<'SQL' || true
WITH bot_bad AS (
  SELECT e.id AS bot_event_id,e.tenant_id,e.account_id,e.source_grouped_id,d.profile_id,e.created_at
  FROM hg_youban_publish_collect_event e
  JOIN hg_youban_publish_collect_dispatch d ON d.event_id=e.id AND d.profile_id>0
  WHERE e.source_id=:'source_id'::bigint
    AND e.source_type='bot'
    AND e.media_count=1
    AND e.source_grouped_id<>''
    AND e.created_at<:'cutoff'::timestamp
), account_ranked AS (
  SELECT b.*,
         a.account_event_id,
         a.source_media_count,
         a.ready_media_count,
         ROW_NUMBER() OVER (PARTITION BY b.profile_id ORDER BY a.source_media_count DESC,a.account_event_id ASC) AS candidate_rank
  FROM bot_bad b
  JOIN LATERAL (
    SELECT e.id AS account_event_id,
           e.media_count AS source_media_count,
           COUNT(em.id) FILTER (WHERE em.cache_status='ready' AND em.storage_path<>'') AS ready_media_count
    FROM hg_youban_publish_collect_event e
    LEFT JOIN hg_youban_publish_collect_event_media em ON em.event_id=e.id
    WHERE e.source_type='account'
      AND e.source_grouped_id=b.source_grouped_id
      AND e.media_count>1
    GROUP BY e.id
    HAVING COUNT(em.id) FILTER (WHERE em.cache_status='ready' AND em.storage_path<>'')=e.media_count
    ORDER BY e.media_count DESC,e.id ASC
    LIMIT 1
  ) a ON true
), current_media AS (
  SELECT profile_id,COUNT(*) AS current_media_count
  FROM hg_youban_publish_media
  WHERE deleted_at IS NULL
  GROUP BY profile_id
)
SELECT r.profile_id,p.profile_no,r.bot_event_id,r.account_event_id,
       r.source_media_count,r.ready_media_count,COALESCE(cm.current_media_count,0) AS current_media_count,
       r.source_grouped_id,r.tenant_id,r.account_id,r.created_at
FROM account_ranked r
JOIN hg_content_profile p ON p.id=r.profile_id AND p.deleted_at IS NULL
LEFT JOIN current_media cm ON cm.profile_id=r.profile_id
WHERE r.candidate_rank=1
  AND COALESCE(cm.current_media_count,0)=1
  AND r.source_media_count>COALESCE(cm.current_media_count,0)
ORDER BY r.profile_id
SQL

export_candidate_csv() {
  "$PSQL_BIN" "$DB_DSN" -X -v ON_ERROR_STOP=1 -v source_id="$SOURCE_ID" -v cutoff="$CUTOFF" \
    -c "\\copy ($CANDIDATE_SQL) TO '$CANDIDATES_CSV' WITH (FORMAT csv, HEADER true)"
}

export_candidate_csv
candidate_count=$(($(wc -l < "$CANDIDATES_CSV") - 1))
echo "Candidates: $candidate_count"
echo "Candidate report: $CANDIDATES_CSV"
if (( candidate_count <= 0 )); then
  exit 0
fi

"$PSQL_BIN" "$DB_DSN" -X -v ON_ERROR_STOP=1 -v source_id="$SOURCE_ID" -v cutoff="$CUTOFF" \
  -c "\\copy (WITH candidates AS ($CANDIDATE_SQL) SELECT p.id,p.image_count,p.video_count,p.has_verification_video,p.updated_at FROM hg_content_profile p JOIN candidates c ON c.profile_id=p.id ORDER BY p.id) TO '$PROFILE_BACKUP_CSV' WITH (FORMAT csv, HEADER true)"

if [[ "$MODE" == "dry-run" ]]; then
  "$PSQL_BIN" "$DB_DSN" -X -P pager=off -v ON_ERROR_STOP=1 -v source_id="$SOURCE_ID" -v cutoff="$CUTOFF" \
    -c "WITH candidates AS ($CANDIDATE_SQL) SELECT profile_id,profile_no,current_media_count,source_media_count,account_event_id,source_grouped_id FROM candidates ORDER BY profile_id"
  echo "Dry-run completed. Re-run with --apply after reviewing the CSV files."
  exit 0
fi

repair_marker="bot-group-repair-$(date +%Y%m%d%H%M%S)"
"$PSQL_BIN" "$DB_DSN" -X -v ON_ERROR_STOP=1 -v source_id="$SOURCE_ID" -v cutoff="$CUTOFF" -v repair_marker="$repair_marker" <<'SQL'
BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='5min';

CREATE TEMP TABLE repair_candidates ON COMMIT DROP AS
WITH candidates AS (
  WITH bot_bad AS (
    SELECT e.id AS bot_event_id,e.tenant_id,e.account_id,e.source_grouped_id,d.profile_id,e.created_at
    FROM hg_youban_publish_collect_event e
    JOIN hg_youban_publish_collect_dispatch d ON d.event_id=e.id AND d.profile_id>0
    WHERE e.source_id=:'source_id'::bigint AND e.source_type='bot' AND e.media_count=1
      AND e.source_grouped_id<>'' AND e.created_at<:'cutoff'::timestamp
  ), account_ranked AS (
    SELECT b.*,a.account_event_id,a.source_media_count,
           ROW_NUMBER() OVER (PARTITION BY b.profile_id ORDER BY a.source_media_count DESC,a.account_event_id ASC) AS candidate_rank
    FROM bot_bad b
    JOIN LATERAL (
      SELECT e.id AS account_event_id,e.media_count AS source_media_count
      FROM hg_youban_publish_collect_event e
      WHERE e.source_type='account' AND e.source_grouped_id=b.source_grouped_id AND e.media_count>1
        AND (SELECT COUNT(*) FROM hg_youban_publish_collect_event_media em WHERE em.event_id=e.id AND em.cache_status='ready' AND em.storage_path<>'')=e.media_count
      ORDER BY e.media_count DESC,e.id ASC LIMIT 1
    ) a ON true
  ), current_media AS (
    SELECT profile_id,COUNT(*) AS current_media_count FROM hg_youban_publish_media WHERE deleted_at IS NULL GROUP BY profile_id
  )
  SELECT r.* FROM account_ranked r
  JOIN hg_content_profile p ON p.id=r.profile_id AND p.deleted_at IS NULL
  LEFT JOIN current_media cm ON cm.profile_id=r.profile_id
  WHERE r.candidate_rank=1 AND COALESCE(cm.current_media_count,0)=1 AND r.source_media_count>1
)
SELECT * FROM candidates;

LOCK TABLE hg_youban_publish_media IN ROW EXCLUSIVE MODE;

CREATE TEMP TABLE repair_source_media ON COMMIT DROP AS
SELECT c.profile_id,c.tenant_id,c.account_id,c.account_event_id,
       em.id AS event_media_id,
       CASE WHEN em.media_type='photo' THEN 'image' ELSE em.media_type END AS media_type,
       'display'::varchar AS purpose,
       ROW_NUMBER() OVER (PARTITION BY c.profile_id ORDER BY em.sort_index,em.id)::int AS sort_index,
       em.file_url,em.storage_path,em.poster_url,em.source_file_id,em.source_size,em.file_md5,em.file_phash
FROM repair_candidates c
JOIN hg_youban_publish_collect_event_media em ON em.event_id=c.account_event_id
WHERE em.cache_status='ready' AND em.storage_path<>'' AND em.media_type IN ('photo','image','video');

CREATE TEMP TABLE inserted_media(id bigint,profile_id bigint,event_media_id bigint) ON COMMIT DROP;

WITH inserted AS (
  INSERT INTO hg_youban_publish_media(
    tenant_id,merchant_id,account_id,profile_id,attachment_id,media_type,purpose,name,file_url,poster_url,
    poster_storage_path,tg_file_id,tg_thumb_file_id,storage_path,mime_type,md5,perceptual_hash,size,sort_index,status,
    created_by,updated_by,deleted_by,created_at,updated_at,tg_cache_asset_hash,tg_cache_status,
    original_attachment_id,original_file_url,original_storage_path,edited_attachment_id,edited_file_url,
    edited_storage_path,edit_config_json,edit_status,must_send,processing_status,processing_error
  )
  SELECT s.tenant_id,s.tenant_id,s.account_id,s.profile_id,0,s.media_type,s.purpose,
         :'repair_marker' || '-' || s.account_event_id || '-' || s.event_media_id,
         s.file_url,s.poster_url,'',s.source_file_id,'',s.storage_path,'',s.file_md5,s.file_phash,
         COALESCE(s.source_size,0),s.sort_index,1,s.account_id,s.account_id,0,NOW(),NOW(),
         COALESCE(NULLIF(s.storage_path,''),s.file_url),'valid',0,'','',0,'','','','raw',0,'ready',''
  FROM repair_source_media s
  WHERE NOT EXISTS (
    SELECT 1 FROM hg_youban_publish_media m
    WHERE m.profile_id=s.profile_id AND m.deleted_at IS NULL AND (
      (s.storage_path<>'' AND m.storage_path=s.storage_path) OR
      (s.file_md5<>'' AND m.md5=s.file_md5) OR
      (s.file_url<>'' AND m.file_url=s.file_url)
    )
  )
  RETURNING id,profile_id,name
)
INSERT INTO inserted_media(id,profile_id,event_media_id)
SELECT id,profile_id,split_part(name,'-',6)::bigint FROM inserted;

UPDATE hg_content_profile p
SET image_count=x.image_count,
    video_count=x.video_count,
    has_verification_video=x.has_verification_video,
    updated_at=NOW()
FROM (
  SELECT c.profile_id,
         COUNT(*) FILTER (WHERE m.media_type='image' AND m.deleted_at IS NULL)::int AS image_count,
         COUNT(*) FILTER (WHERE m.media_type='video' AND m.deleted_at IS NULL)::int AS video_count,
         CASE WHEN COUNT(*) FILTER (WHERE m.media_type='video' AND m.purpose='verify' AND m.deleted_at IS NULL)>0 THEN 1 ELSE 0 END AS has_verification_video
  FROM repair_candidates c
  JOIN hg_youban_publish_media m ON m.profile_id=c.profile_id
  GROUP BY c.profile_id
) x
WHERE p.id=x.profile_id;

COMMIT;
SQL

"$PSQL_BIN" "$DB_DSN" -X -v ON_ERROR_STOP=1 -v repair_marker="$repair_marker" \
  -c "\\copy (SELECT id,profile_id,media_type,purpose,name,storage_path,file_url,created_at FROM hg_youban_publish_media WHERE name LIKE :'repair_marker' || '%' ORDER BY profile_id,sort_index,id) TO '$INSERTED_CSV' WITH (FORMAT csv, HEADER true)"

{
  echo 'BEGIN;'
  echo "UPDATE hg_youban_publish_media SET deleted_at=NOW(),updated_at=NOW() WHERE name LIKE '${repair_marker}%';"
  tail -n +2 "$PROFILE_BACKUP_CSV" | while IFS=, read -r id image_count video_count has_verification_video updated_at; do
    printf "UPDATE hg_content_profile SET image_count=%s,video_count=%s,has_verification_video=%s,updated_at='%s' WHERE id=%s;\n" \
      "$image_count" "$video_count" "$has_verification_video" "$updated_at" "$id"
  done
  echo 'COMMIT;'
} > "$ROLLBACK_SQL"

"$PSQL_BIN" "$DB_DSN" -X -P pager=off -v ON_ERROR_STOP=1 -v repair_marker="$repair_marker" \
  -c "SELECT p.id,p.profile_no,p.image_count,p.video_count,p.has_verification_video,COUNT(m.id) FILTER (WHERE m.deleted_at IS NULL) AS active_media FROM hg_content_profile p JOIN hg_youban_publish_media m ON m.profile_id=p.id WHERE EXISTS (SELECT 1 FROM hg_youban_publish_media r WHERE r.profile_id=p.id AND r.name LIKE :'repair_marker' || '%') GROUP BY p.id ORDER BY p.id"

echo "Repair completed with marker: $repair_marker"
echo "Inserted media report: $INSERTED_CSV"
echo "Rollback SQL: $ROLLBACK_SQL"
