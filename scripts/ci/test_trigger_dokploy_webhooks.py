import importlib.util
import json
import tempfile
import unittest
import urllib.error
from pathlib import Path
from unittest import mock


SCRIPT = Path(__file__).with_name("trigger-dokploy-webhooks.py")
SPEC = importlib.util.spec_from_file_location("trigger_dokploy_webhooks", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class TriggerDokployWebhooksTest(unittest.TestCase):
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
        response = mock.MagicMock()
        response.__enter__.return_value.status = 200
        with mock.patch.object(MODULE.urllib.request, "urlopen", side_effect=[failed, response]) as open_url:
            MODULE.wait_until_healthy(target, retries=2, sleep=lambda _: None)

        self.assertEqual(2, open_url.call_count)

    def test_main_skips_when_every_target_is_disabled(self):
        path = self.write_config([
            {"name": "api", "webhook": "", "enabled": False, "order": 10},
        ])

        self.assertEqual(0, MODULE.main(["--config", path, "--version", "sha-1234567"]))


if __name__ == "__main__":
    unittest.main()
