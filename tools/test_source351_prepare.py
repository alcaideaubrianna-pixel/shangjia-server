"""Local preparation tests; no database, cloud or HTTP access."""
import json
from pathlib import Path
import tempfile
import unittest

import source351_prepare as prepare
import source351_replay as replay


class PrepareTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.manifest = self.root / "manifest.jsonl"
        self.summary_path = self.root / "summary.json"
        self.output = self.root / "prepared.jsonl"
        self.rows = [{"id": i, "tenantId": 78, "sourceId": 351,
                      "chatId": "-1004411799859", "messageId": 100 + i} for i in (3, 1, 2, 1)]
        self.summary = {**replay.SCOPE, "finishedAt": "2026-11-18T12:00:00+0800",
                        "channels": [{"chatId": "-1004411799859"}], "rawMessages": 4}

    def run_prepare(self):
        self.manifest.write_text("".join(json.dumps(row) + "\n" for row in self.rows))
        self.summary_path.write_text(json.dumps(self.summary))
        return prepare.prepare(self.manifest, self.summary_path, self.output)

    def test_sort_deduplicate_and_strict_replay_compatibility(self):
        result = self.run_prepare()
        self.assertEqual(result["duplicatesRemoved"], 1)
        rows = list(replay.records(self.output, {"-1004411799859": 338}))
        self.assertEqual([r["id"] for r in rows], [1, 2, 3])
        self.assertTrue(all(r["accountId"] == 615 for r in rows))
        self.assertEqual(result["networkRequests"], 0)
        self.assertEqual(self.output.stat().st_mode & 0o777, 0o600)

    def test_missing_and_cross_tenant_scope_rejected(self):
        for key, value in (("tenantId", None), ("sourceId", None),
                           ("tenantId", 79), ("sourceId", 352), ("accountId", 616)):
            original = self.rows[0].copy()
            if value is None:
                self.rows[0].pop(key)
            else:
                self.rows[0][key] = value
            with self.assertRaises(RuntimeError):
                self.run_prepare()
            self.assertFalse(self.output.exists())
            self.rows[0] = original

    def test_conflicting_duplicates_rejected(self):
        self.rows[-1]["messageId"] = 999
        with self.assertRaises(RuntimeError):
            self.run_prepare()
        self.assertFalse(self.output.exists())

    def test_unfinished_export_and_count_mismatch_rejected(self):
        self.summary.pop("finishedAt")
        with self.assertRaises(RuntimeError):
            self.run_prepare()
        self.summary["finishedAt"] = "now"
        self.summary["rawMessages"] = 999
        with self.assertRaises(RuntimeError):
            self.run_prepare()

    def test_excluded_and_unlisted_chats_rejected(self):
        for chat in (next(iter(replay.EXCLUDED)), "-1009999999999"):
            self.rows[0]["chatId"] = chat
            with self.assertRaises(RuntimeError):
                self.run_prepare()

    def test_numeric_channel_alias_normalized(self):
        self.rows[0]["chatId"] = "4411799859"
        self.run_prepare()
        self.assertEqual(len(list(replay.records(self.output, {"-1004411799859": 338}))), 3)

    def test_replay_rejects_missing_scope(self):
        self.run_prepare()
        lines = self.output.read_text().splitlines()
        row = json.loads(lines[1])
        row.pop("tenantId")
        lines[1] = json.dumps(row)
        self.output.write_text("\n".join(lines) + "\n")
        with self.assertRaises(RuntimeError):
            list(replay.records(self.output, {"-1004411799859": 338}))

    def test_summary_scope_and_overwrite_rejected(self):
        self.summary["accountId"] = 616
        with self.assertRaises(RuntimeError):
            self.run_prepare()
        self.summary["accountId"] = 615
        self.run_prepare()
        original = self.output.read_bytes()
        with self.assertRaises(FileExistsError):
            self.run_prepare()
        self.assertEqual(self.output.read_bytes(), original)


if __name__ == "__main__":
    unittest.main()
