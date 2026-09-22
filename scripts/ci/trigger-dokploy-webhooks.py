#!/usr/bin/env python3
import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from html import escape
from pathlib import Path


DEFAULT_TIMEOUT_SECONDS = 30
DEFAULT_RETRIES = 3
# Wait through a start-first rollout and image extraction on slower nodes.
DEFAULT_HEALTH_RETRIES = 60
DEFAULT_REVISION_CONFIRMATIONS = 5
DEFAULT_RUNTIME_RETRIES = 60


def load_targets(path):
    with Path(path).open(encoding="utf-8") as handle:
        document = json.load(handle)

    image = str(document.get("image", "")).strip()
    if not image:
        raise ValueError("image is required")
    targets = document.get("targets")
    if not isinstance(targets, list):
        raise ValueError("targets must be a list")

    enabled = []
    names = set()
    orders = set()
    for index, target in enumerate(targets):
        if not isinstance(target, dict):
            raise ValueError(f"targets[{index}] must be an object")
        name = str(target.get("name", "")).strip()
        if not name:
            raise ValueError(f"targets[{index}].name is required")
        if name in names:
            raise ValueError(f"duplicate target name: {name}")
        names.add(name)

        order = target.get("order")
        if not isinstance(order, int):
            raise ValueError(f"target {name} order must be an integer")
        if order in orders:
            raise ValueError(f"duplicate target order: {order}")
        orders.add(order)

        if target.get("enabled") is not True:
            continue
        for field in ("applicationId", "appName", "serverId"):
            if not str(target.get(field, "")).strip():
                raise ValueError(f"enabled target {name} requires {field}")
        webhook = str(target.get("webhook", "")).strip()
        parsed = urllib.parse.urlparse(webhook)
        if parsed.scheme != "https" or not parsed.netloc:
            raise ValueError(f"enabled target {name} requires an HTTPS webhook")
        wait_seconds = target.get("waitSeconds", 10)
        if not isinstance(wait_seconds, int) or wait_seconds < 0:
            raise ValueError(f"target {name} waitSeconds must be a non-negative integer")
        health_url = str(target.get("healthUrl", "")).strip()
        if health_url:
            parsed_health = urllib.parse.urlparse(health_url)
            if parsed_health.scheme != "https" or not parsed_health.netloc:
                raise ValueError(f"target {name} requires an HTTPS healthUrl")
        enabled.append({
            "name": name,
            "webhook": webhook,
            "order": order,
            "wait_seconds": wait_seconds,
            "health_url": health_url,
            "verify_revision": target.get("verifyRevision") is True,
            "application_id": str(target["applicationId"]).strip(),
            "app_name": str(target["appName"]).strip(),
            "server_id": str(target["serverId"]).strip(),
            "image": image,
        })

    return sorted(enabled, key=lambda item: item["order"])


def post(url, payload, timeout=DEFAULT_TIMEOUT_SECONDS):
    request = urllib.request.Request(
        url,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json", "User-Agent": "youban-deploy-ci/1.0"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        response.read()
        return response.status


def dokploy_request(path, payload=None, query=None, timeout=DEFAULT_TIMEOUT_SECONDS):
    base_url = os.environ.get("DOKPLOY_URL", "").strip().rstrip("/")
    api_key = os.environ.get("DOKPLOY_API_KEY", "").strip()
    if not base_url or not api_key:
        raise RuntimeError("DOKPLOY_URL and DOKPLOY_API_KEY are required")
    url = f"{base_url}/api/{path}"
    if query:
        url = f"{url}?{urllib.parse.urlencode(query)}"
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "User-Agent": "youban-deploy-ci/1.0",
            "x-api-key": api_key,
        },
        method="GET" if payload is None else "POST",
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        body = response.read()
        if not 200 <= response.status < 300:
            raise RuntimeError(f"Dokploy API {path} returned HTTP {response.status}")
    try:
        result = json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise RuntimeError(f"Dokploy API {path} returned invalid JSON: {error}") from error
    if isinstance(result, dict) and result.get("success") is False:
        raise RuntimeError(f"Dokploy API {path} failed: {result.get('message', 'unknown error')}")
    return result.get("data", result) if isinstance(result, dict) else result


def pin_target_image(target, version):
    image = f"{target['image']}:{version}"
    dokploy_request("application.update", {
        "applicationId": target["application_id"],
        "dockerImage": image,
    })
    application = dokploy_request(
        "application.one", query={"applicationId": target["application_id"]},
    )
    if application.get("dockerImage") != image:
        raise RuntimeError(
            f"Dokploy image is {application.get('dockerImage')!r}, expected {image!r}"
        )
    return image


def running_container_images(target):
    containers = dokploy_request("docker.getServiceContainersByAppName", query={
        "appName": target["app_name"],
        "serverId": target["server_id"],
    })
    images = []
    for container in containers or []:
        if container.get("state") != "running":
            continue
        config = dokploy_request("docker.getConfig", query={
            "containerId": container.get("containerId", ""),
            "serverId": target["server_id"],
        })
        image = (((config or {}).get("Spec") or {}).get("ContainerSpec") or {}).get("Image")
        images.append(str(image or ""))
    return images


def wait_for_running_image(target, expected_image, retries=DEFAULT_RUNTIME_RETRIES, sleep=time.sleep):
    last_images = []
    for attempt in range(1, retries + 1):
        try:
            last_images = running_container_images(target)
            if last_images and all(image == expected_image for image in last_images):
                print(
                    f"runtime check {target['name']}: attempt {attempt}/{retries}, "
                    f"running={len(last_images)} image={expected_image}",
                    flush=True,
                )
                return
        except (urllib.error.URLError, TimeoutError, RuntimeError) as error:
            last_images = [f"error: {error}"]
        print(
            f"runtime check {target['name']}: attempt {attempt}/{retries} "
            f"images={last_images!r}, expected={expected_image!r}",
            file=sys.stderr,
            flush=True,
        )
        if attempt < retries:
            sleep(5)
    raise RuntimeError(f"running container image mismatch: {last_images!r}")


def trigger(target, version, retries=DEFAULT_RETRIES, sleep=time.sleep):
    payload = {"version": version, "target": target["name"]}
    last_error = None
    for attempt in range(1, retries + 1):
        try:
            status = post(target["webhook"], payload)
            if 200 <= status < 300:
                return
            last_error = RuntimeError(f"unexpected HTTP status {status}")
        except (urllib.error.URLError, TimeoutError, RuntimeError) as error:
            last_error = error
        if attempt < retries:
            sleep(2 ** (attempt - 1))
    raise RuntimeError(f"webhook failed after {retries} attempts: {last_error}")


def read_health(url):
    request = urllib.request.Request(
        url,
        headers={"User-Agent": "youban-deploy-ci/1.0", "Cache-Control": "no-cache"},
    )
    try:
        response_context = urllib.request.urlopen(request, timeout=10)
    except urllib.error.HTTPError as error:
        # Dokploy/API may report 409 while old and new replicas overlap.
        # Return the body so the caller can keep polling for the expected revision.
        if error.code != 409:
            raise
        response_context = error
    with response_context as response:
        body = response.read()
        if response.status == 409:
            try:
                return json.loads(body.decode("utf-8"))
            except (UnicodeDecodeError, json.JSONDecodeError):
                raise RuntimeError("health endpoint is still converging (HTTP 409)")
        if not 200 <= response.status < 300:
            raise RuntimeError(f"unexpected HTTP status {response.status}")
        try:
            return json.loads(body.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise RuntimeError(f"invalid health response: {error}") from error


def revision_matches(actual, expected):
    actual = str(actual or "").strip()
    expected = str(expected or "").strip().removeprefix("sha-")
    return bool(actual and expected and actual.startswith(expected))


def wait_until_healthy(target, revision="", retries=DEFAULT_HEALTH_RETRIES,
                       confirmations=DEFAULT_REVISION_CONFIRMATIONS, sleep=time.sleep):
    sleep(target["wait_seconds"])
    health_url = target["health_url"]
    if not health_url:
        return
    last_error = None
    confirmed = 0
    for attempt in range(1, retries + 1):
        try:
            query = urllib.parse.urlencode({"revision": revision}) if target.get("verify_revision") else ""
            separator = "&" if "?" in health_url else "?"
            health = read_health(f"{health_url}{separator}{query}" if query else health_url)
            if target.get("verify_revision") and not revision_matches(health.get("revision"), revision):
                confirmed = 0
                raise RuntimeError(f"revision is {health.get('revision')!r}, expected {revision!r}")
            confirmed += 1
            print(
                f"health check {target['name']}: attempt {attempt}/{retries}, "
                f"confirmation {confirmed}/{confirmations if target.get('verify_revision') else 1}",
                flush=True,
            )
            if confirmed >= (confirmations if target.get("verify_revision") else 1):
                return
        except (urllib.error.URLError, TimeoutError, RuntimeError) as error:
            last_error = error
            print(
                f"health check {target['name']}: attempt {attempt}/{retries} failed: {error}",
                file=sys.stderr,
                flush=True,
            )
        if attempt < retries:
            sleep(5)
    raise RuntimeError(f"health check failed after {retries} attempts: {last_error}")


def send_telegram(text):
    token = os.environ.get("TELEGRAM_BOT_TOKEN", "").strip()
    chat_id = os.environ.get("TELEGRAM_CHAT_ID", "").strip()
    if not token or not chat_id:
        return
    data = urllib.parse.urlencode({"chat_id": chat_id, "parse_mode": "HTML", "text": text}).encode()
    request = urllib.request.Request(
        f"https://api.telegram.org/bot{token}/sendMessage",
        data=data,
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=15) as response:
            response.read()
    except Exception as error:
        print(f"telegram notification failed: {error}", file=sys.stderr)


def main(argv=None):
    parser = argparse.ArgumentParser(description="Trigger enabled Dokploy webhooks in order")
    parser.add_argument("--config", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--revision", default="")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    try:
        targets = load_targets(args.config)
    except (OSError, json.JSONDecodeError, ValueError) as error:
        print(f"invalid Dokploy target config: {error}", file=sys.stderr)
        return 2

    if not targets:
        print("no Dokploy targets enabled; deployment skipped")
        return 0

    failures = []
    for target in targets:
        name = target["name"]
        if args.dry_run:
            print(f"would trigger {name} ({args.version})")
            continue
        print(f"triggering {name} ({args.version})", flush=True)
        try:
            expected_image = pin_target_image(target, args.version)
            trigger(target, args.version)
            wait_until_healthy(target, args.revision or args.version)
            wait_for_running_image(target, expected_image)
        except (urllib.error.URLError, TimeoutError, RuntimeError) as error:
            failures.append((name, str(error)))
            send_telegram(
                f"❌ <b>{escape(name)} 部署触发失败</b>\n"
                f"版本：<code>{escape(args.version)}</code>\n"
                f"错误：<code>{escape(str(error))}</code>"
            )
            print(f"failed to trigger {name}: {error}", file=sys.stderr)
            continue
        send_telegram(
            f"🚀 <b>{escape(name)} 已触发部署</b>\n"
            f"版本：<code>{escape(args.version)}</code>"
        )
        print(f"triggered {name}", flush=True)
    if failures:
        summary = "; ".join(f"{name}: {error}" for name, error in failures)
        print(f"deployment failures: {summary}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
