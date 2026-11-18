#!/usr/bin/env python3
"""Export scoped collection facts and inject JSONL into the local Go matcher."""
import argparse
from collections import defaultdict
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
SQL = """
WITH page AS MATERIALIZED (
    SELECT id, tenant_id, account_id, tg_account_id, source_id, source_chat_id,
           source_message_id, source_grouped_id, raw_text, received_at, created_at,
           material_role, material_parent_event_id, media_count
    FROM hg_youban_publish_collect_event
    WHERE tenant_id = :'tenant'::bigint AND account_id = :'account'::bigint
      AND source_id = :'source'::bigint AND source_chat_id = :'chat'
      AND source_message_id BETWEEN :'first'::bigint AND :'last'::bigint
    ORDER BY source_message_id, id LIMIT (:'limit'::int + 1)
)
SELECT row_to_json(fixture)
FROM (
    SELECT e.tenant_id, e.account_id, e.tg_account_id, e.source_id, e.source_chat_id,
           e.id AS event_id, e.source_message_id AS message_id,
           e.source_grouped_id AS grouped_id, COALESCE(e.raw_text, '') AS raw_text,
           to_char(COALESCE(e.received_at, e.created_at), 'YYYY-MM-DD"T"HH24:MI:SS.US') || '+08:00' AS received_at,
           COALESCE(e.material_role, '') AS material_role,
           e.material_parent_event_id AS parent_event_id, e.media_count,
           ARRAY(SELECT m.media_type FROM hg_youban_publish_collect_event_media m
                 WHERE m.event_id = e.id AND m.tenant_id = e.tenant_id
                   AND m.source_id = e.source_id ORDER BY m.id) AS media_types
    FROM page e ORDER BY e.source_message_id, e.id
) fixture;
"""


def export(args):
    if (args.tenant <= 0 or args.source <= 0 or args.account < 0
            or args.first <= 0 or not 0 <= args.last - args.first < 50000
            or not 1 <= args.limit <= 10000
            or re.fullmatch(r"-?[0-9]{1,19}", args.chat) is None):
        raise SystemExit("Invalid collection scope, interval, or limit.")
    values = {k: getattr(args, k) for k in ("tenant", "account", "source", "chat", "first", "last", "limit")}
    cmd = ["psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1"]
    for key, value in values.items():
        cmd.extend(["-v", f"{key}={value}"])
    env = dict(os.environ, PGSERVICE=args.service, PGAPPNAME="collect_cursor_replay_readonly",
               PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=5000 -c lock_timeout=500")
    with tempfile.TemporaryFile(mode="w+t", encoding="utf-8") as snapshot:
        subprocess.run(cmd, env=env, input=SQL, stdout=snapshot, text=True, check=True, timeout=20)
        snapshot.seek(0)
        count = sum(1 for _ in snapshot)
        if count == 0 or count > args.limit:
            raise SystemExit(f"Export returned {count} rows. Choose a smaller nonempty message range.")
        snapshot.seek(0)
        with open(args.output, "x", encoding="utf-8", opener=private_file) as output:
            for line in snapshot:
                output.write(line)
    print(f"Exported {count} rows to {args.output}")


def private_file(path, flags):
    return os.open(path, flags, 0o600)


def import_archive(args):
    with args.events.open(encoding="utf-8") as records:
        events = [json.loads(line) for _, line in zip(range(10001), records)]
    if not 1 <= len(events) <= 10000:
        raise SystemExit("Archive must contain 1–10,000 event rows.")
    ids = {event["id"] for event in events}
    if len(ids) != len(events):
        raise SystemExit("Archive contains duplicate event IDs.")
    media = defaultdict(list)
    with args.media.open(encoding="utf-8") as records:
        for line in records:
            item = json.loads(line)
            if item["eventId"] in ids:
                media[item["eventId"]].append(item["mediaType"])
    events.sort(key=lambda row: tuple(row[field] for field in (
        "tenantId", "accountId", "tgAccountId", "sourceId", "sourceChatId", "sourceMessageId", "id")))
    local_zone = timezone(timedelta(hours=8))
    with open(args.output, "x", encoding="utf-8", opener=private_file) as output:
        for event in events:
            received = datetime.fromisoformat(event.get("receivedAt") or event["createdAt"])
            if received.tzinfo is None:
                received = received.replace(tzinfo=local_zone)
            row = {
                "tenant_id": event["tenantId"], "account_id": event["accountId"],
                "tg_account_id": event["tgAccountId"], "source_id": event["sourceId"],
                "source_chat_id": event["sourceChatId"], "event_id": event["id"],
                "message_id": event["sourceMessageId"], "grouped_id": event["sourceGroupedId"],
                "raw_text": event.get("rawText") or "", "received_at": received.isoformat(),
                "media_types": media[event["id"]], "media_count": event["mediaCount"],
                "material_role": event.get("materialRole") or "",
                "parent_event_id": event.get("materialParentEventId") or 0,
            }
            output.write(json.dumps(row, ensure_ascii=False) + "\n")
    print(f"Imported {len(events)} rows into {args.output}")


def run(args):
    if not 1 <= args.batch_size <= 1000:
        raise SystemExit("Batch size must be 1–1000.")
    env = dict(os.environ, YOUBAN_COLLECT_REPLAY_STDIN="1", YOUBAN_COLLECT_REPLAY_BOUNDED="1",
               YOUBAN_COLLECT_REPLAY_CURSOR_BATCH_SIZE=str(args.batch_size),
               YOUBAN_COLLECT_REPLAY_REPORT_ONLY="1" if args.report_only else "0")
    env.pop("YOUBAN_COLLECT_REPLAY_FILE", None)
    env.setdefault("GOMAXPROCS", "4")
    binary = args.binary.resolve(strict=True)
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise SystemExit("Replay binary must be an executable Go test binary.")
    with args.input.open("rb") as snapshot:
        subprocess.run([str(binary), "-test.v", "-test.run", "^TestCollectMaterialReplayProductionMessages$"],
                       cwd=ROOT / "server", env=env, stdin=snapshot, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(required=True)
    capture = commands.add_parser("export", help="Export a single explicit scope through a read-only libpq service.")
    capture.add_argument("--service", required=True)
    for key in ("tenant", "account", "source", "first", "last"):
        capture.add_argument("--" + key, required=True, type=int)
    capture.add_argument("--chat", required=True)
    capture.add_argument("--limit", type=int, default=1000)
    capture.add_argument("--output", required=True, type=Path)
    capture.set_defaults(handler=export)
    archive = commands.add_parser("import", help="Convert existing event and media JSONL exports.")
    archive.add_argument("--events", required=True, type=Path)
    archive.add_argument("--media", required=True, type=Path)
    archive.add_argument("--output", required=True, type=Path)
    archive.set_defaults(handler=import_archive)
    replay = commands.add_parser("run", help="Run the local matcher on a JSONL snapshot.")
    replay.add_argument("--binary", required=True, type=Path, help="Existing Go test binary containing the cursor replay test.")
    replay.add_argument("--input", required=True, type=Path)
    replay.add_argument("--batch-size", type=int, default=100)
    replay.add_argument("--report-only", action="store_true",
                        help="Report context audit differences while keeping cursor equality checks strict.")
    replay.set_defaults(handler=run)
    args = parser.parse_args()
    args.handler(args)


if __name__ == "__main__":
    main()
