"""Offline state-machine tests; never use production credentials or networking."""
import contextlib
import hashlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import source351_replay as replay


class FakeAPI:
    def __init__(self):
        self.plans = {}
        self.calls = []
        self.timeout_apply = False
        self.failed = []
        self.phase = "completed"
        self.bad_preview = False
        self.status_phases = []

    def call(self, operation, body):
        self.calls.append((operation, body))
        if operation == "plan":
            ids = body["collectorEventIds"]
            plan_id = "p" + str(len(self.plans) + 1)
            self.plans[plan_id] = ids
            return {"planId": plan_id, "confirmToken": "SECRET_CONFIRM", "expiresAt": "2030-01-01T00:00:00Z",
                    "action": replay.ACTION, "candidates": len(ids), "collectorEventIds": ids,
                    "replayPreview": [{"collectorEventId": i, "messageId": i + 100,
                                       "chatId": "-1004411799859", "ruleSourceId": 351,
                                       "ruleId": 999 if self.bad_preview else 338} for i in ids]}
        if operation == "apply":
            if self.timeout_apply:
                raise RuntimeError("simulated transport loss")
            return {"status": "completed"}
        ids = self.plans[body["planId"]]
        failed = [i for i in self.failed if i in ids]
        phase = self.status_phases.pop(0) if self.status_phases else self.phase
        return {"planId": body["planId"], "action": replay.ACTION, "status": phase,
                "candidates": len(ids), "applied": len(ids) - len(failed), "failed": failed}


class ReplayTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.manifest = self.root / "export.jsonl"
        self.checkpoint = self.root / "checkpoint.json"
        self.rows = [{**replay.SCOPE, "id": i, "messageId": i + 100, "chatId": "-1004411799859"}
                     for i in range(1, 6)]
        self.api = FakeAPI()
        self.write_manifest()

    def write_manifest(self):
        self.manifest.write_text("\n".join(json.dumps(x) for x in [replay.SCOPE] + self.rows) + "\n")
        self.digest = hashlib.sha256(self.manifest.read_bytes()).hexdigest()

    def run_script(self, *arguments):
        argv = ["replay", str(self.manifest), "--manifest-sha256", self.digest,
                "--checkpoint", str(self.checkpoint), "--batch-size", "2", *arguments]
        output = io.StringIO()
        with patch("sys.argv", argv), patch.object(replay, "API", return_value=self.api), contextlib.redirect_stdout(output):
            replay.main()
        self.assertNotIn("SECRET_CONFIRM", output.getvalue())
        if self.checkpoint.exists():
            self.assertNotIn("SECRET_CONFIRM", self.checkpoint.read_text())
        return output.getvalue()

    def state(self):
        return json.loads(self.checkpoint.read_text())

    def test_dry_run_no_network_or_checkpoint(self):
        self.run_script()
        self.assertFalse(self.api.calls)
        self.assertFalse(self.checkpoint.exists())

    def test_explicit_chat_defaults_to_verified_rule338(self):
        self.run_script("--chat=-1004411799859")
        self.assertFalse(self.api.calls)

    def test_bounded_cursor_resume_and_exhaustion(self):
        self.run_script("--apply")
        self.assertEqual(self.state()["cursor"], 2)
        self.run_script("--apply", "--max-batches", "3")
        self.assertEqual(self.state()["cursor"], 5)
        plans = [b["collectorEventIds"] for op, b in self.api.calls if op == "plan"]
        self.assertEqual(plans, [[1, 2], [3, 4], [5]])
        self.assertIn('"manifestExhausted": true', self.run_script("--apply"))

    def test_unknown_apply_reconciles_without_resubmitting(self):
        self.api.timeout_apply = True
        with self.assertRaises(RuntimeError):
            self.run_script("--apply")
        self.assertEqual(self.state()["cursor"], 0)
        self.assertEqual(self.state()["current"]["planId"], "p1")
        self.api.timeout_apply = False
        self.run_script("--apply")
        self.assertEqual(sum(op == "apply" for op, _ in self.api.calls), 2)
        self.assertEqual([b["planId"] for op, b in self.api.calls if op == "apply"], ["p1", "p2"])
        self.assertEqual(self.state()["cursor"], 4)

    def test_running_failed_pending_never_replay_unknown(self):
        self.api.timeout_apply = True
        with self.assertRaises(RuntimeError):
            self.run_script("--apply")
        for phase in ("running", "failed", "pending", "mystery"):
            self.api.phase = phase
            with self.assertRaises(RuntimeError):
                self.run_script("--apply", "--retry-failed")
        self.assertEqual(sum(op == "apply" for op, _ in self.api.calls), 1)
        self.assertEqual(sum(op == "plan" for op, _ in self.api.calls), 1)

    def test_only_explicit_failed_ids_retried(self):
        self.api.failed = [2]
        with self.assertRaises(RuntimeError):
            self.run_script("--apply")
        self.assertEqual(self.state()["cursor"], 0)
        self.assertEqual([r["id"] for r in self.state()["current"]["rows"]], [2])
        with self.assertRaises(RuntimeError):
            self.run_script("--apply")
        self.api.failed = []
        self.run_script("--apply", "--retry-failed")
        self.assertEqual(self.state()["cursor"], 2)
        self.assertEqual([b["collectorEventIds"] for op, b in self.api.calls if op == "plan"], [[1, 2], [2]])

    def test_invalid_late_row_rejected_before_network(self):
        self.rows[-1]["chatId"] = next(iter(replay.EXCLUDED))
        self.write_manifest()
        with self.assertRaises(RuntimeError):
            self.run_script("--apply")
        self.assertFalse(self.api.calls)

    def test_preview_rule_mismatch_never_applies(self):
        self.api.bad_preview = True
        with self.assertRaises(RuntimeError):
            self.run_script("--apply")
        self.assertFalse(any(op == "apply" for op, _ in self.api.calls))

    def test_manifest_binding_change_rejected(self):
        self.run_script("--apply")
        self.rows.append({**replay.SCOPE, "id": 6, "messageId": 106, "chatId": "-1004411799859"})
        self.write_manifest()
        before = len(self.api.calls)
        with self.assertRaises(RuntimeError):
            self.run_script("--apply")
        self.assertEqual(len(self.api.calls), before)

    def test_running_then_completed_serial_batches(self):
        self.api.status_phases = ["running", "completed", "running", "completed"]
        with patch.object(replay.time, "sleep"):
            self.run_script("--apply", "--wait-seconds", "330", "--max-batches", "2")
        self.assertEqual(self.state()["cursor"], 4)
        self.assertEqual([op for op, _ in self.api.calls],
                         ["plan", "apply", "status", "status", "plan", "apply", "status", "status"])
        statuses = [b["planId"] for op, b in self.api.calls if op == "status"]
        self.assertEqual(statuses, ["p1", "p1", "p2", "p2"])

    def test_running_timeout_preserves_checkpoint_and_never_reapplies(self):
        self.api.phase = "running"
        with patch.object(replay.time, "monotonic", side_effect=[0, 0, 2]), patch.object(replay.time, "sleep"):
            with self.assertRaisesRegex(RuntimeError, "wait expired"):
                self.run_script("--apply", "--wait-seconds", "2", "--max-batches", "14")
        before = self.checkpoint.read_bytes()
        self.assertEqual(self.state()["cursor"], 0)
        self.assertEqual(self.state()["current"]["planId"], "p1")
        with patch.object(replay.time, "monotonic", side_effect=[0, 0, 2]), patch.object(replay.time, "sleep"):
            with self.assertRaisesRegex(RuntimeError, "wait expired"):
                self.run_script("--apply", "--wait-seconds", "2", "--max-batches", "14")
        self.assertEqual(self.checkpoint.read_bytes(), before)
        self.assertEqual(sum(op == "plan" for op, _ in self.api.calls), 1)
        self.assertEqual(sum(op == "apply" for op, _ in self.api.calls), 1)
        self.assertTrue(all(b["planId"] == "p1" for op, b in self.api.calls if op == "status"))

    def test_startup_wait_resumes_original_plan_before_next_batch(self):
        self.api.timeout_apply = True
        with self.assertRaises(RuntimeError):
            self.run_script("--apply")
        self.api.timeout_apply = False
        self.api.status_phases = ["running", "completed", "completed"]
        with patch.object(replay.time, "sleep"):
            self.run_script("--apply", "--wait-seconds", "330")
        self.assertEqual(self.state()["cursor"], 4)
        self.assertEqual([b["planId"] for op, b in self.api.calls if op == "apply"], ["p1", "p2"])

    def test_poll_validates_scope_on_every_response(self):
        original = self.api.call
        polls = 0
        def call(operation, body):
            nonlocal polls
            result = original(operation, body)
            if operation == "status":
                polls += 1
                result["status"] = "running"
                if polls == 2:
                    result["planId"] = "wrong-plan"
            return result
        with patch.object(self.api, "call", side_effect=call), patch.object(replay.time, "sleep"):
            with self.assertRaisesRegex(RuntimeError, "scope mismatch"):
                self.run_script("--apply", "--wait-seconds", "330")
        self.assertEqual(sum(op == "apply" for op, _ in self.api.calls), 1)
        self.assertEqual(self.state()["cursor"], 0)


if __name__ == "__main__":
    unittest.main()
