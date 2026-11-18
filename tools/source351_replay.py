#!/usr/bin/env python3
"""Bounded manifest-driven source replay. No DB, Redis, or Telegram access."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

ACTION = "replay_bot_source_updates"
EXCLUDED = {"-1003438550143", "-1003763589169", "-1003763752459",
            "-1004429787816", "-1004449446193"}
SCOPE = {"sourceId": 351, "tenantId": 78, "accountId": 615}


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def positive(value):
    return type(value) is int and 0 < value < 2**63


def records(path, chats):
    """Validate the entire export before permitting the first network request."""
    previous = 0
    with path.open(encoding="utf-8") as stream:
        header = json.loads(next(stream))
        require(all(header.get(k) == v for k, v in SCOPE.items()),
                "manifest header scope must be source351/tenant78/account615")
        for line in stream:
            require(bool(line.strip()), "blank manifest row")
            row = json.loads(line)
            event_id = row.get("id", row.get("collectorEventId"))
            require(positive(event_id) and event_id > previous,
                    "raw IDs must be unique positive integers in strictly ascending order")
            require(positive(row.get("messageId")), "invalid messageId")
            chat = row.get("chatId")
            require(isinstance(chat, str) and chat in chats and chat not in EXCLUDED,
                    "manifest contains an excluded or non-explicit chat")
            require(all(type(row.get(k)) is int and row[k] == v for k, v in SCOPE.items()),
                    "every prepared row must explicitly match source351/tenant78/account615")
            if "ruleId" in row:
                require(row["ruleId"] == chats[chat], "manifest rule mismatch")
            if "originSourceId" in row:
                require(positive(row["originSourceId"]), "invalid origin source")
            previous = event_id
            yield dict(row, id=event_id)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class API:
    def __init__(self, base, timeout):
        parsed = urllib.parse.urlsplit(base)
        require(parsed.scheme == "https" and parsed.hostname and not parsed.username
                and not parsed.password and not parsed.query and not parsed.fragment,
                "base URL must be HTTPS without credentials/query/fragment")
        self.base = base.rstrip("/") + "/collector/source/replay/"
        self.token = os.environ.get("YOUBAN_AI_OPS_TOKEN", "").strip()
        require(bool(self.token), "YOUBAN_AI_OPS_TOKEN is required")
        self.timeout = timeout
        self.opener = urllib.request.build_opener(NoRedirect())

    def call(self, operation, body=None):
        url = self.base + operation
        if operation == "status":
            url += "?" + urllib.parse.urlencode(body)
            body = None
        request = urllib.request.Request(
            url, data=json.dumps(body).encode() if body is not None else None,
            headers={"X-AI-Ops-Token": self.token, "Content-Type": "application/json"})
        try:
            with self.opener.open(request, timeout=self.timeout) as response:
                raw = response.read(4 * 1024 * 1024 + 1)
                require(len(raw) <= 4 * 1024 * 1024, "API response too large")
                envelope = json.loads(raw)
            require(type(envelope.get("code")) is int and envelope["code"] == 0
                    and isinstance(envelope.get("data"), dict), "API rejected request")
            return envelope["data"]
        except (urllib.error.URLError, OSError, ValueError):
            # Never print server bodies, URLs, headers, or exception text containing secrets.
            raise RuntimeError("HTTP/JSON failure; resume with the same checkpoint") from None


def check_preview(plan, rows, chats):
    ids = {row["id"] for row in rows}
    require(plan.get("action") == ACTION and plan.get("candidates") == len(rows),
            "preview action/count mismatch")
    returned = plan.get("collectorEventIds")
    require(isinstance(returned, list) and len(returned) == len(ids)
            and set(returned) == ids, "preview raw ID mismatch; refusing apply")
    preview = plan.get("replayPreview")
    require(isinstance(preview, list) and len(preview) == len(rows), "missing replayPreview")
    by_id = {item.get("collectorEventId"): item for item in preview}
    require(set(by_id) == ids, "preview item ID mismatch")
    for row in rows:
        item = by_id[row["id"]]
        require(item.get("chatId") == row["chatId"]
                and item.get("messageId") == row["messageId"]
                and item.get("ruleSourceId") == 351
                and item.get("ruleId") == chats[row["chatId"]],
                "preview channel/message/source/rule mismatch")
        if "originSourceId" in row:
            require(item.get("originSourceId") == row["originSourceId"],
                    "preview origin source mismatch")
    require(all(isinstance(plan.get(k), str) and plan[k]
                for k in ("planId", "confirmToken", "expiresAt")), "incomplete preview plan")


def save(path, state):
    descriptor, name = tempfile.mkstemp(prefix=path.name + ".", dir=path.parent)
    try:
        with os.fdopen(descriptor, "w") as stream:
            json.dump(state, stream, sort_keys=True)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(name, path)
        directory = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def reconcile(api, state, checkpoint, wait_seconds=0, poll_interval=2):
    current = state["current"]
    deadline = time.monotonic() + wait_seconds
    while True:
        status = api.call("status", {"planId": current["planId"]})
        require(status.get("planId") == current["planId"] and status.get("action") == ACTION
                and status.get("candidates") == len(current["rows"]), "status scope mismatch")
        if status.get("status") != "running":
            break
        remaining = deadline - time.monotonic()
        require(remaining > 0,
                "plan still running; wait expired. Checkpoint preserved; resume the same plan")
        time.sleep(min(poll_interval, remaining))
    phase = status.get("status")
    if phase == "pending":
        raise RuntimeError("plan pending (including expired): manual reconciliation required; no replay issued")
    require(phase == "completed",
            "plan running/failed/unknown: stop for reconciliation; no new replay issued")
    failed = status.get("failed") or []
    require(isinstance(failed, list) and all(positive(x) for x in failed)
            and len(set(failed)) == len(failed)
            and set(failed) <= {row["id"] for row in current["rows"]}, "invalid failed ID set")
    require(type(status.get("applied")) is int
            and 0 <= status["applied"] <= len(current["rows"]) - len(failed),
            "invalid applied count")
    state["history"].append({"planId": current["planId"], "status": phase,
                             "ids": [r["id"] for r in current["rows"]],
                             "failed": failed, "queued": status["applied"]})
    if failed:
        current["rows"] = [r for r in current["rows"] if r["id"] in set(failed)]
        current.pop("planId")
        current["retryRequired"] = True
    else:
        state["cursor"] = current["endCursor"]
        state["current"] = None
    save(checkpoint, state)
    print(json.dumps({"planId": status["planId"], "planStatus": phase,
                      "queued": status["applied"], "failed": len(failed),
                      "cursor": state["cursor"], "businessComplete": False}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--manifest-sha256", required=True, help="independently recorded export digest")
    parser.add_argument("--checkpoint", type=Path)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--dry-run", "--dryrun", action="store_true", help="offline; default")
    mode.add_argument("--preview", action="store_true", help="create plans only; no apply/checkpoint")
    mode.add_argument("--apply", action="store_true")
    parser.add_argument("--chat", action="append", default=[], metavar="CHAT_ID:RULE_ID",
                        help="extra explicit approved chat and expected rule")
    parser.add_argument("--batch-size", type=int, default=200)
    parser.add_argument("--max-batches", type=int, default=1)
    parser.add_argument("--retry-failed", action="store_true",
                        help="retry only explicit failed IDs of a completed plan")
    parser.add_argument("--timeout", type=float, default=45)
    parser.add_argument("--wait-seconds", type=int, default=0,
                        help="poll the same running plan for 0..600 seconds; default 0")
    parser.add_argument("--poll-interval", type=int, default=2,
                        help="running plan status polling interval, 1..30 seconds")
    parser.add_argument("--base-url", default=os.environ.get(
        "YOUBAN_AI_OPS_BASE_URL", "https://api.xiaohuiji.cc/api/youban_publish/ai-ops"))
    args = parser.parse_args()
    require(1 <= args.batch_size <= 200 and 1 <= args.max_batches <= 1000
            and 0 < args.timeout <= 120 and 0 <= args.wait_seconds <= 600
            and 1 <= args.poll_interval <= 30, "invalid batch/timeout/poll bounds")
    chats = {"-1004411799859": 338}
    for entry in args.chat:
        chat, rule = entry.rsplit(":", 1) if ":" in entry else (entry, "338")
        require(chat.startswith("-100") and chat[4:].isdigit() and chat not in EXCLUDED
                and positive(int(rule)), "invalid/excluded explicit chat")
        require(chat not in chats or chats[chat] == int(rule), "conflicting rule")
        chats[chat] = int(rule)
    # A stable open file snapshot prevents a replaced export being used after digest validation.
    with tempfile.TemporaryDirectory(prefix="source351-manifest-") as temporary:
        snapshot = Path(temporary) / "manifest.jsonl"
        digest = hashlib.sha256()
        with args.manifest.open("rb") as source, snapshot.open("wb") as target:
            while chunk := source.read(1024 * 1024):
                digest.update(chunk)
                target.write(chunk)
        require(digest.hexdigest() == args.manifest_sha256.lower(), "manifest SHA256 mismatch")
        count = sum(1 for _ in records(snapshot, chats))
        binding = {"version": 1, "sha256": digest.hexdigest(), "scope": SCOPE,
                   "chats": chats, "baseURL": args.base_url.rstrip("/")}
        if not args.apply and not args.preview:
            print(json.dumps({"mode": "dry-run", "records": count, "batchSize": args.batch_size,
                              "maxBatches": args.max_batches, "networkRequests": 0}))
            return
        require(not args.apply or args.checkpoint is not None, "--apply requires --checkpoint")
        api = API(args.base_url, args.timeout)
        if args.preview:
            batch = []
            batches = 0
            for row in records(snapshot, chats):
                batch.append(row)
                if len(batch) == args.batch_size:
                    plan = api.call("plan", {"sourceId": 351, "collectorEventIds": [r["id"] for r in batch], "limit": len(batch)})
                    check_preview(plan, batch, chats)
                    print(json.dumps({"mode": "preview", "planId": plan["planId"], "candidates": len(batch)}))
                    batches += 1
                    batch = []
                    if batches >= args.max_batches:
                        break
            if batch and batches < args.max_batches:
                plan = api.call("plan", {"sourceId": 351, "collectorEventIds": [r["id"] for r in batch], "limit": len(batch)})
                check_preview(plan, batch, chats)
                print(json.dumps({"mode": "preview", "planId": plan["planId"], "candidates": len(batch)}))
            return
        checkpoint = args.checkpoint.resolve()
        require(checkpoint != args.manifest.resolve(), "checkpoint cannot overwrite manifest")
        require(checkpoint.parent.is_dir(), "checkpoint parent must exist")
        lock_fd = os.open(str(checkpoint) + ".lock", os.O_CREAT | os.O_RDWR, 0o600)
        with os.fdopen(lock_fd, "w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            state = json.loads(checkpoint.read_text()) if checkpoint.exists() else {
                "binding": binding, "cursor": 0, "current": None, "history": []}
            require(state["binding"] == binding, "checkpoint manifest/scope/endpoint mismatch")
            save(checkpoint, state)
            if state["current"] and "planId" in state["current"]:
                reconcile(api, state, checkpoint, args.wait_seconds, args.poll_interval)
            iterator = records(snapshot, chats)
            for _ in range(args.max_batches):
                current = state["current"]
                if current is None:
                    batch = []
                    for row in iterator:
                        if row["id"] > state["cursor"]:
                            batch.append(row)
                            if len(batch) >= args.batch_size:
                                break
                    if not batch:
                        print(json.dumps({"manifestExhausted": True, "businessComplete": False,
                                          "cursor": state["cursor"]}))
                        return
                    current = {"rows": batch, "endCursor": batch[-1]["id"]}
                    state["current"] = current
                    save(checkpoint, state)
                require(not current.get("retryRequired") or args.retry_failed,
                        "explicit failed IDs retained; use --retry-failed after review")
                batch = current["rows"]
                require(len(batch) <= args.batch_size, "resume batch exceeds requested size")
                plan = api.call("plan", {"sourceId": 351,
                                         "collectorEventIds": [r["id"] for r in batch], "limit": len(batch)})
                check_preview(plan, batch, chats)
                current["planId"] = plan["planId"]
                save(checkpoint, state)  # Must be durable BEFORE sending apply.
                api.call("apply", {"planId": plan["planId"], "confirmToken": plan["confirmToken"]})
                reconcile(api, state, checkpoint, args.wait_seconds, args.poll_interval)
                require(state["current"] is None,
                        "batch has explicit failed IDs; stopped before next batch")


if __name__ == "__main__":
    try:
        main()
    except RuntimeError as error:
        print("Stopped: " + str(error), file=sys.stderr)
        sys.exit(1)
    except (OSError, ValueError, KeyError, TypeError, StopIteration):
        # Do not expose remote messages or malformed manifest content.
        print("Stopped: validation, API or checkpoint condition. Preserve checkpoint; "
              "inspect plan status before retry. No automatic apply retry.", file=sys.stderr)
        sys.exit(1)
