#!/usr/bin/env python3
import json
import os
import subprocess
import urllib.parse
import urllib.request
from html import escape
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


HOST = os.environ.get("DEPLOY_WEBHOOK_HOST", "0.0.0.0")
PORT = int(os.environ.get("DEPLOY_WEBHOOK_PORT", "9088"))
TOKEN = os.environ.get("DEPLOY_WEBHOOK_TOKEN", "")
SCRIPT = os.environ.get("DEPLOY_SCRIPT", "/opt/youban/deploy-webhook.sh")
IMAGE_PREFIX = os.environ.get("IMAGE_PREFIX", "ghcr.io/mjiadfwaff-bot/youban-server:")
IMAGE_PREFIXES = [item.strip() for item in os.environ.get("IMAGE_PREFIXES", IMAGE_PREFIX).split(",") if item.strip()]
TELEGRAM_BOT_TOKEN = os.environ.get("TELEGRAM_BOT_TOKEN", "")
TELEGRAM_CHAT_ID = os.environ.get("TELEGRAM_CHAT_ID", "")


def send_telegram(text):
    if not TELEGRAM_BOT_TOKEN or not TELEGRAM_CHAT_ID:
        return
    data = urllib.parse.urlencode({
        "chat_id": TELEGRAM_CHAT_ID,
        "parse_mode": "HTML",
        "text": text,
    }).encode()
    req = urllib.request.Request(
        f"https://api.telegram.org/bot{TELEGRAM_BOT_TOKEN}/sendMessage",
        data=data,
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            resp.read()
    except Exception as exc:
        print(f"telegram notify failed: {exc}", flush=True)


def image_service_name(image):
    name = image.rsplit("/", 1)[-1].split(":", 1)[0]
    if name == "youban-server":
        return "youban-server"
    if name == "youban-h5":
        return "youban-h5"
    return name or "youban"


def image_version(image):
    if ":" not in image:
        return image
    return image.rsplit(":", 1)[-1]


def html_code(value):
    return escape(str(value), quote=False)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        print("%s - %s" % (self.address_string(), fmt % args), flush=True)

    def _send(self, code, payload):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/health":
            self._send(200, {"ok": True})
            return
        self._send(404, {"error": "not found"})

    def do_POST(self):
        if self.path != "/deploy":
            self._send(404, {"error": "not found"})
            return
        if not TOKEN:
            self._send(500, {"error": "token is not configured"})
            return
        if self.headers.get("Authorization", "") != "Bearer " + TOKEN:
            self._send(401, {"error": "unauthorized"})
            return

        length = int(self.headers.get("Content-Length", "0"))
        try:
            payload = json.loads(self.rfile.read(length) or b"{}")
        except json.JSONDecodeError:
            self._send(400, {"error": "invalid json"})
            return

        image = str(payload.get("image", "")).strip()
        if not any(image.startswith(prefix) for prefix in IMAGE_PREFIXES):
            self._send(400, {"error": "image is not allowed"})
            return

        try:
            result = subprocess.run(
                [SCRIPT, image],
                check=True,
                text=True,
                capture_output=True,
                timeout=600,
            )
        except subprocess.CalledProcessError as exc:
            send_telegram(
                "❌ <b>{service} 部署失败</b>\n"
                "版本：{version}\n"
                "镜像：<code>{image}</code>\n"
                "错误：<code>{error}</code>".format(
                    service=image_service_name(image),
                    version=image_version(image),
                    image=html_code(image),
                    error=html_code((exc.stderr or exc.stdout or "deploy failed")[-800:]),
                )
            )
            self._send(500, {"error": "deploy failed", "output": exc.stdout[-4000:], "stderr": exc.stderr[-4000:]})
            return
        except subprocess.TimeoutExpired:
            send_telegram(
                "❌ <b>{service} 部署超时</b>\n"
                "版本：{version}\n"
                "镜像：<code>{image}</code>".format(
                    service=image_service_name(image),
                    version=image_version(image),
                    image=html_code(image),
                )
            )
            self._send(504, {"error": "deploy timeout"})
            return

        send_telegram(
            "✅ <b>{service} 部署成功</b>\n"
            "版本：{version}\n"
            "镜像：<code>{image}</code>".format(
                service=image_service_name(image),
                version=image_version(image),
                image=html_code(image),
            )
        )
        self._send(200, {"ok": True, "image": image, "output": result.stdout[-4000:]})


if __name__ == "__main__":
    server = ThreadingHTTPServer((HOST, PORT), Handler)
    print(f"deploy webhook listening on {HOST}:{PORT}", flush=True)
    server.serve_forever()
