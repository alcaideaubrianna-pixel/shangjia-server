#!/usr/bin/env python3
"""Offline conversion of collect_backlog_export.py output to a replay manifest."""
import argparse
from contextlib import closing
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import sys
import tempfile

from source351_replay import EXCLUDED, SCOPE, positive, require


def canonical(chat):
    require(isinstance(chat, str), "chatId must be a string")
    digits = chat[4:] if chat.startswith("-100") else chat
    require(digits.isdigit() and int(digits) > 0, "invalid channel chatId")
    return "-100" + digits


def prepare(manifest, summary_path, output):
    require(output.resolve() not in {manifest.resolve(), summary_path.resolve()},
            "output cannot overwrite export inputs")
    summary_bytes = summary_path.read_bytes()
    summary = json.loads(summary_bytes)
    require(all(type(summary.get(k)) is int and summary[k] == v for k, v in SCOPE.items()),
            "summary source/tenant/account mismatch")
    require(bool(summary.get("finishedAt")), "export unfinished: summary.finishedAt is required")
    channels = {canonical(row["chatId"]) for row in summary["channels"]}
    excluded = EXCLUDED | {canonical(chat) for chat in summary.get("excludedPublishChats", [])}
    require(type(summary.get("rawMessages")) is int and summary["rawMessages"] >= 0,
            "invalid summary rawMessages")
    count = 0
    source_hash = hashlib.sha256()
    with tempfile.TemporaryDirectory(prefix="source351-sort-") as temporary:
        with closing(sqlite3.connect(str(Path(temporary) / "rows.sqlite"))) as db:
            db.execute("CREATE TABLE rows (id INTEGER PRIMARY KEY, payload TEXT NOT NULL)")
            with manifest.open("rb") as stream:
                for line in stream:
                    source_hash.update(line)
                    row = json.loads(line)
                    require(all(type(row.get(k)) is int and row[k] == SCOPE[k]
                                for k in ("tenantId", "sourceId")),
                            "every export row must explicitly match tenant78/source351")
                    require("accountId" not in row or
                            type(row["accountId"]) is int and row["accountId"] == 615,
                            "row account mismatch")
                    require(positive(row.get("id")) and positive(row.get("messageId")), "invalid raw/message ID")
                    chat = canonical(row.get("chatId"))
                    require(chat in channels and chat not in excluded,
                            "row outside summary channels or in publish exclusions")
                    if "ruleId" in row:
                        require(type(row["ruleId"]) is int and row["ruleId"] == 338, "unexpected rule")
                    normalized = {**SCOPE, "id": row["id"], "messageId": row["messageId"],
                                  "chatId": chat, "ruleId": 338}
                    payload = json.dumps(normalized, sort_keys=True, separators=(",", ":"))
                    existing = db.execute("SELECT payload FROM rows WHERE id=?", (row["id"],)).fetchone()
                    require(existing is None or existing[0] == payload, "conflicting duplicate raw ID")
                    db.execute("INSERT OR IGNORE INTO rows VALUES (?,?)", (row["id"], payload))
                    count += 1
            require(count == summary["rawMessages"], "export row count differs from summary")
            require(summary_path.read_bytes() == summary_bytes, "summary changed during preparation")
            unique = db.execute("SELECT count(*) FROM rows").fetchone()[0]
            used_chats = sorted({json.loads(payload)["chatId"] for (payload,) in
                                 db.execute("SELECT payload FROM rows")})
            header = {**SCOPE, "version": 1, "exportedAt": summary["finishedAt"],
                      "inputSha256": source_hash.hexdigest(),
                      "summarySha256": hashlib.sha256(summary_bytes).hexdigest(),
                      "inputRows": count, "uniqueRows": unique, "chats": used_chats,
                      "scope": summary.get("scope", ""),
                      "authorizationEvidence": "summary from controlled export; not independent authorization"}
            # O_EXCL prevents accidental overwrite of a prepared manifest/checkpoint binding.
            descriptor, temporary_output = tempfile.mkstemp(prefix=".prepared-", dir=output.parent)
            digest = hashlib.sha256()
            try:
                with os.fdopen(descriptor, "wb") as stream:
                    for payload in [json.dumps(header, sort_keys=True)]:
                        data = (payload + "\n").encode()
                        stream.write(data)
                        digest.update(data)
                    for (payload,) in db.execute("SELECT payload FROM rows ORDER BY id"):
                        data = (payload + "\n").encode()
                        stream.write(data)
                        digest.update(data)
                    stream.flush()
                    os.fsync(stream.fileno())
                os.link(temporary_output, output)
            finally:
                os.unlink(temporary_output)
    return {"inputRows": count, "uniqueRows": unique, "duplicatesRemoved": count - unique,
            "sha256": digest.hexdigest(), "chatArguments": ["--chat=" + chat + ":338" for chat in used_chats],
            "networkRequests": 0, "businessComplete": False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--summary", type=Path, help="default: sibling summary.json")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(prepare(args.manifest, args.summary or args.manifest.with_name("summary.json"), args.output)))


if __name__ == "__main__":
    try:
        main()
    except RuntimeError as error:
        print("Stopped: " + str(error), file=sys.stderr)
        sys.exit(1)
    except (OSError, ValueError, KeyError, TypeError):
        print("Stopped: invalid export/summary or output already exists; no network requests", file=sys.stderr)
        sys.exit(1)
