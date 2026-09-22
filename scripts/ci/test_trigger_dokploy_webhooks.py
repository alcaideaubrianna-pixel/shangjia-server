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
                "xiaohuiji-publish-worker-app2",
                "xiaohuiji-worker-app4",
                "xiaohuiji-media-worker-app3",
                "xiaohuiji-collector-worker-app3",
                "xiaohuiji-publish-worker-app3",
            ],
            [target["name"] for target in targets],
        )

    def write_config(self, targets):
        handle = tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", delete=False)
        json.dump({"image": "registry.example.com/server", "targets": targets}, handle)
        handle.close()
        self.addCleanup(Path(handle.name).unlink)
        return handle.name

    def test_loads_only_enabled_targets_in_order(self):
        path = self.write_config([
            {"name": "second", "webhook": "https://example.com/2", "enabled": True, "order": 20,
             "applicationId": "app-2", "appName": "second-app", "serverId": "server-1"},
            {"name": "disabled", "webhook": "", "enabled": False, "order": 30},
            {"name": "first", "webhook": "https://example.com/1", "enabled": True, "order": 10,
             "applicationId": "app-1", "appName": "first-app", "serverId": "server-1"},
        ])

        targets = MODULE.load_targets(path)

        self.assertEqual(["first", "second"], [target["name"] for target in targets])

    def test_enabled_target_requires_https_webhook(self):
        path = self.write_config([
            {"name": "api", "webhook": "http://example.com/deploy", "enabled": True, "order": 10,
             "applicationId": "app-1", "appName": "api-app", "serverId": "server-1"},
        ])

        with self.assertRaisesRegex(ValueError, "requires an HTTPS webhook"):
            MODULE.load_targets(path)

    def test_trigger_retries_then_succeeds(self):
        target = {"name": "worker", "webhook": "https://example.com/deploy", "order": 10}
        with mock.patch.object(MODULE, "post", side_effect=[TimeoutError("timeout"), 200]) as post:
            MODULE.trigger(target, "sha-1234567", sleep=lambda _: None)

        self.assertEqual(2, post.call_count)

    def test_pin_target_image_updates_and_confirms_immutable_image(self):
        target = {"application_id": "app-1", "image": "registry.example.com/server"}
        with mock.patch.object(MODULE, "dokploy_request", side_effect=[{}, {
            "dockerImage": "registry.example.com/server:sha-1234567",
        }]) as request:
            image = MODULE.pin_target_image(target, "sha-1234567")

        self.assertEqual("registry.example.com/server:sha-1234567", image)
        self.assertEqual("application.update", request.call_args_list[0].args[0])

    def test_runtime_check_requires_all_running_containers_on_expected_image(self):
        target = {"name": "worker"}
        with mock.patch.object(MODULE, "running_container_images", side_effect=[
            ["registry/server:old", "registry/server:sha-1234567"],
            ["registry/server:sha-1234567"],
        ]) as images:
            MODULE.wait_for_running_image(
                target, "registry/server:sha-1234567", retries=2, sleep=lambda _: None,
            )

        self.assertEqual(2, images.call_count)

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

    def test_wait_until_healthy_keeps_confirmation_across_transport_error(self):
        target = {
            "name": "api",
            "health_url": "https://example.com/readyz",
            "wait_seconds": 0,
            "verify_revision": True,
        }
        responses = [
            {"revision": "1234567890"},
            TimeoutError("transient timeout"),
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

    def test_main_continues_after_target_failure_and_returns_failure(self):
        targets = [
            {"name": "first", "application_id": "app-1"},
            {"name": "second", "application_id": "app-2"},
        ]
        with mock.patch.object(MODULE, "load_targets", return_value=targets), \
                mock.patch.object(MODULE, "pin_target_image", side_effect=[
                    RuntimeError("update failed"), "registry/server:sha-1234567",
                ]), \
                mock.patch.object(MODULE, "trigger") as trigger, \
                mock.patch.object(MODULE, "wait_until_healthy"), \
                mock.patch.object(MODULE, "wait_for_running_image"), \
                mock.patch.object(MODULE, "send_telegram"):
            result = MODULE.main([
                "--config", "unused.json", "--version", "sha-1234567",
                "--revision", "1234567",
            ])

        self.assertEqual(1, result)
        trigger.assert_called_once_with(targets[1], "sha-1234567")


if __name__ == "__main__":
    unittest.main()
