#!/usr/bin/env python3
"""Read-only audit for collected intro-fee suffix normalization.

Usage:
  YOUBAN_PG_DSN='postgresql://...' python3 tools/collect_suffix_audit.py

The script scans every bot-collected dispatch, joins its rule suffix, and
reports profiles whose stored text still contains a fee line that does not
end with the configured suffix. It never updates production data.
"""

import json
import os
import re
import sys
from collections import Counter

try:
    import psycopg
except ImportError as exc:
    raise SystemExit("install psycopg or run with the project's Python environment") from exc

FEE_RE = re.compile(r"(?:介绍|推荐|牵线|居间|对接)费", re.UNICODE)
NUMBER_RE = re.compile(r"[¥￥]?\s*[0-9０-９][0-9０-９,，.]*", re.UNICODE)
INVISIBLE_RE = re.compile(r"[\u200b-\u200f\u2060\ufeff\ufe00-\ufe0f]")


def audit_line(line: str, suffix: str) -> str:
    clean = INVISIBLE_RE.sub("", line).strip()
    if not FEE_RE.search(clean):
        return "no_fee"
    if not NUMBER_RE.search(clean):
        return "no_amount"
    if suffix and clean.endswith(suffix):
        return "ok"
    if any(mark in clean for mark in ("国内", "国外", "村内", "村外", "一人", "双人", "/", "、")):
        return "multi_price_or_variant"
    return "suffix_mismatch"


def main() -> int:
    dsn = os.environ.get("YOUBAN_PG_DSN")
    if not dsn:
        raise SystemExit("YOUBAN_PG_DSN is required")
    sql = """
    SELECT d.profile_id, p.profile_no, p.plain_text, i.value AS suffix
    FROM hg_youban_publish_collect_dispatch d
    JOIN hg_youban_publish_collect_rule_item i
      ON i.rule_id=d.rule_id AND i.item_type='intro_fee_suffix'
    JOIN hg_content_profile p ON p.id=d.profile_id
    WHERE d.source_type_snapshot='bot'
      AND NULLIF(BTRIM(i.value),'') IS NOT NULL
      AND p.deleted_at IS NULL
    ORDER BY d.profile_id
    """
    counters = Counter()
    affected = {}
    with psycopg.connect(dsn) as conn, conn.cursor() as cur:
        cur.execute(sql)
        for profile_id, profile_no, text, suffix in cur:
            states = {audit_line(line, suffix) for line in (text or "").splitlines()}
            state = "ok"
            for candidate in ("suffix_mismatch", "multi_price_or_variant", "no_amount", "no_fee"):
                if candidate in states:
                    state = candidate
                    break
            counters[state] += 1
            if state != "ok":
                affected[str(profile_id)] = {"profileNo": profile_no, "suffix": suffix, "state": state}
    print(json.dumps({"summary": counters, "affected": affected}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
