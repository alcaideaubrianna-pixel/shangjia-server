#!/usr/bin/env python3
"""Verify fee-rule replay output without changing production data.

Input JSONL rows must contain rawText and outputText. The verifier treats the
matched fee line plus its immediate neighbours as the only mutable window.
"""
import json
import re
import sys
import unicodedata

FEE = re.compile(r"(?:介绍|推荐|牵线|居间|对接|感谢)费|大米", re.I)
CRITICAL_VALUE = re.compile(
    r"(净身高|身高|height|年龄|年齡|age|体重|體重|weight)\s*[:：=]?\s*"
    r"([0-9０-９]{1,3}(?:[.．][0-9０-９]+)?\s*(?:岁|cm|厘米|kg|公斤|斤)?\+?)",
    re.I,
)

def lines(value):
    return value.replace("\r\n", "\n").split("\n")

def normalize_keyword_text(value):
    value = unicodedata.normalize("NFKC", value)
    return "".join(
        char for char in value
        if unicodedata.category(char) != "Cf"
        and not (0xFE00 <= ord(char) <= 0xFE0F)
        and not (0xE0100 <= ord(char) <= 0xE01EF)
    )

def fee_indexes(rows):
    return [i for i, row in enumerate(rows) if FEE.search(normalize_keyword_text(row))]

def normalized_outside(raw, out):
    ri, oi = lines(raw), lines(out)
    mutable = set()
    for index in fee_indexes(ri):
        mutable.update(range(max(0, index - 1), min(len(ri), index + 2)))

    protected_runs = []
    index = 0
    while index < len(ri):
        if index in mutable:
            index += 1
            continue
        start = index
        while index < len(ri) and index not in mutable:
            index += 1
        protected_runs.append((start, ri[start:index]))

    if not protected_runs:
        return True

    def find_run(run, start_at):
        width = len(run)
        for start in range(start_at, len(oi) - width + 1):
            if oi[start:start + width] == run:
                yield start, start + width

    output_cursor = 0
    for run_index, (source_start, run) in enumerate(protected_runs):
        is_first = run_index == 0
        is_last = run_index == len(protected_runs) - 1
        has_mutable_prefix = any(i in mutable for i in range(source_start))
        has_mutable_suffix = any(i in mutable for i in range(source_start + len(run), len(ri)))
        if is_first and not has_mutable_prefix:
            candidates = ((0, len(run)),) if oi[:len(run)] == run else ()
        else:
            candidates = find_run(run, output_cursor)

        match = None
        for start, end in candidates:
            if is_last and not has_mutable_suffix and end != len(oi):
                continue
            match = (start, end)
            break
        if match is None:
            return False
        output_cursor = match[1]

    return True

def critical_fields_preserved(raw, out):
    def fields(value):
        normalized = normalize_keyword_text(value)
        normalized = unicodedata.normalize("NFKC", normalized)
        return [(label.casefold(), re.sub(r"\s+", "", number).casefold())
                for label, number in CRITICAL_VALUE.findall(normalized)]

    before = fields(raw)
    after = fields(out)
    return before == after

def main(path):
    counts = {"total": 0, "pass": 0, "outside_fee_changed": 0,
              "critical_field_lost": 0, "old_suffix_kept": 0, "invalid": 0}
    failures = []
    with open(path, encoding="utf-8") as handle:
        for raw_line in handle:
            if not raw_line.strip():
                continue
            counts["total"] += 1
            try:
                row = json.loads(raw_line)
                source, output = row["rawText"], row["outputText"]
            except (ValueError, KeyError):
                counts["invalid"] += 1
                continue
            reasons = []
            if not normalized_outside(source, output):
                counts["outside_fee_changed"] += 1; reasons.append("outside_fee_changed")
            if not critical_fields_preserved(source, output):
                counts["critical_field_lost"] += 1; reasons.append("critical_field_lost")
            if reasons:
                failures.append({"eventId": row.get("eventId"), "reasons": reasons})
            else:
                counts["pass"] += 1
    print(json.dumps({"counts": counts, "failures": failures[:1000]}, ensure_ascii=False, indent=2))

if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: verify_fee_replay.py replay.jsonl")
    main(sys.argv[1])
