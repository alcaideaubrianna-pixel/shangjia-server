#!/usr/bin/env bash
set -euo pipefail

batch_size=200
sleep_seconds=1
last_id=0
state_file="./note-search-backfill.state"
start_id_set=0
psql_args=()

usage() {
  cat <<'HELP'
Usage: note_search_backfill_slow.sh [options]
  --batch-size N       Rows per batch (default: 200)
  --sleep-seconds N    Delay between batches (default: 1)
  --start-id N         Initial note-index id (overrides state file)
  --state-file PATH    Cursor file (default: ./note-search-backfill.state)
  --db NAME            PostgreSQL database name
  --help               Show this help
Connection uses DATABASE_URL or libpq environment variables.
HELP
}

while (($# > 0)); do
  case "$1" in
    --batch-size) batch_size="$2"; shift 2 ;;
    --sleep-seconds) sleep_seconds="$2"; shift 2 ;;
    --start-id) last_id="$2"; start_id_set=1; shift 2 ;;
    --state-file) state_file="$2"; shift 2 ;;
    --db) psql_args+=("--dbname" "$2"); shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$batch_size" in ''|*[!0-9]*) echo "--batch-size must be an integer" >&2; exit 2 ;; esac
case "$last_id" in ''|*[!0-9]*) echo "--start-id must be an integer" >&2; exit 2 ;; esac
case "$sleep_seconds" in ''|*[!0-9.]*) echo "--sleep-seconds must be numeric" >&2; exit 2 ;; esac
((batch_size > 0)) || { echo "--batch-size must be greater than zero" >&2; exit 2; }

if ((start_id_set == 0)) && [[ -f "$state_file" ]]; then
  saved_id=$(tr -d '[:space:]' <"$state_file")
  case "$saved_id" in ''|*[!0-9]*) echo "invalid cursor in $state_file" >&2; exit 2 ;; esac
  last_id="$saved_id"
fi

mkdir -p "$(dirname "$state_file")"
trap 'echo "stopped; resume with --state-file '"$state_file"'" >&2' INT TERM

echo "starting from id=$last_id batch_size=$batch_size sleep=${sleep_seconds}s"

while :; do
  result=$(psql "${psql_args[@]}" -X -q -v ON_ERROR_STOP=1 -At <<SQL
WITH batch AS (
  SELECT id, tenant_id, account_id, profile_id, is_collected, status,
         profile_no, title, summary, plain_text, deleted_at
  FROM hg_youban_publish_note_index
  WHERE id > $last_id
  ORDER BY id
  LIMIT $batch_size
), upserted AS (
  INSERT INTO hg_youban_publish_note_search
    (tenant_id, account_id, profile_id, is_collected, status, search_text, deleted_at, updated_at)
  SELECT tenant_id, account_id, profile_id, is_collected, status,
         concat_ws(' ', profile_no, title, summary, plain_text), deleted_at, now()
  FROM batch
  ON CONFLICT (tenant_id, account_id, profile_id) DO UPDATE SET
    is_collected = EXCLUDED.is_collected,
    status = EXCLUDED.status,
    search_text = EXCLUDED.search_text,
    deleted_at = EXCLUDED.deleted_at,
    updated_at = EXCLUDED.updated_at
  RETURNING 1
)
SELECT COALESCE((SELECT max(id) FROM batch), 0) || '|' || (SELECT count(*) FROM upserted);
SQL
)

  [[ "$result" == "0|0" ]] && break
  IFS='|' read -r cursor written <<<"$result"
  [[ "$cursor" =~ ^[0-9]+$ && "$cursor" -gt "$last_id" ]] || { echo "cursor did not advance: $result" >&2; exit 1; }
  printf '%s\n' "$cursor" >"$state_file"
  echo "backfilled rows=$written through id=$cursor"
  last_id="$cursor"
  sleep "$sleep_seconds"
done

psql "${psql_args[@]}" -X -q -v ON_ERROR_STOP=1 -c "ANALYZE hg_youban_publish_note_search;" >/dev/null
echo "backfill complete at id=$last_id"
