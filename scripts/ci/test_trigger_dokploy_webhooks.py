import importlib.util
import json
import tempfile
import unittest
import urllib.error
from pathlib import Path
from unittest import mock


SCRIPT = Path(__file__).with_name("trigger-dokploy-webhooks.py")
PRODUCTION_CONFIG = SCRIPT.parents[2] / "deploy" / "dokploy-targets.json"
SPEC = importlib.util.spec_from_file_location("trigger_dokploy_webhooks", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class TriggerDokployWebhooksTest(unittest.TestCase):
    def test_production_config_contains_all_deployment_targets(self):
        targets = MODULE.load_targets(PRODUCTION_CONFIG)

        self.assertEqual(
            [
                "xiaohuiji-api",
                "xiaohuiji-account",
                "xiaohuiji-scheduler",
                "xiaohuiji-worker-app2",
                "xiaohuiji-worker-app3",
                "xiaohuiji-media-worker-app3",
                "xiaohuiji-collector-worker-app3",
                "xiaohuiji-publish-worker-app1",
                "xiaohuiji-publish-worker-app3",
            ],
            [target["name"] for target in targets],
        )

    def write_config(self, targets):
        handle = tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", delete=False)
        json.dump({"targets": targets}, handle)
        handle.close()
        self.addCleanup(Path(handle.name).unlink)
        return handle.name

    def test_loads_only_enabled_targets_in_order(self):
        path = self.write_config([
            {"name": "second", "webhook": "https://example.com/2", "enabled": True, "order": 20},
            {"name": "disabled", "webhook": "", "enabled": False, "order": 30},
            {"name": "first", "webhook": "https://example.com/1", "enabled": True, "order": 10},
        ])

        targets = MODULE.load_targets(path)

        self.assertEqual(["first", "second"], [target["name"] for target in targets])

    def test_enabled_target_requires_https_webhook(self):
        path = self.write_config([
            {"name": "api", "webhook": "http://example.com/deploy", "enabled": True, "order": 10},
        ])

        with self.assertRaisesRegex(ValueError, "requires an HTTPS webhook"):
            MODULE.load_targets(path)

    def test_trigger_retries_then_succeeds(self):
        target = {"name": "worker", "webhook": "https://example.com/deploy", "order": 10}
        with mock.patch.object(MODULE, "post", side_effect=[TimeoutError("timeout"), 200]) as post:
            MODULE.trigger(target, "sha-1234567", sleep=lambda _: None)

        self.assertEqual(2, post.call_count)

    def test_wait_until_healthy_waits_for_success(self):
        target = {
            "name": "api",
            "health_url": "https://example.com/readyz",
            "wait_seconds": 0,
        }
        failed = urllib.error.URLError("not ready")
        with mock.patch.object(MODULE, "read_health", side_effect=[failed, {"status": "ready"}]) as open_url:
            MODULE.wait_until_healthy(target, retries=2, sleep=lambda _: None)

        self.assertEqual(2, open_url.call_count)

    def test_wait_until_healthy_requires_consecutive_target_revision(self):
        target = {
            "name": "api",
            "health_url": "https://example.com/readyz",
            "wait_seconds": 0,
            "verify_revision": True,
        }
        responses = [
            {"revision": "old"},
            {"revision": "1234567890"},
            {"revision": "1234567890"},
        ]
        with mock.patch.object(MODULE, "read_health", side_effect=responses) as read_health:
            MODULE.wait_until_healthy(
                target, revision="sha-1234567", retries=3, confirmations=2, sleep=lambda _: None,
            )

        self.assertEqual(3, read_health.call_count)

    def test_revision_matches_short_or_prefixed_revision(self):
        self.assertTrue(MODULE.revision_matches("1234567890abcdef", "sha-1234567"))
        self.assertFalse(MODULE.revision_matches("7654321", "sha-1234567"))

    def test_main_skips_when_every_target_is_disabled(self):
        path = self.write_config([
            {"name": "api", "webhook": "", "enabled": False, "order": 10},
        ])

        self.assertEqual(0, MODULE.main(["--config", path, "--version", "sha-1234567"]))


if __name__ == "__main__":
    unittest.main()
