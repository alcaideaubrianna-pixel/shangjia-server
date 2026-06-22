#!/usr/bin/env python3
import json
import os
import subprocess
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


HOST = os.environ.get("DEPLOY_WEBHOOK_HOST", "0.0.0.0")
PORT = int(os.environ.get("DEPLOY_WEBHOOK_PORT", "9088"))
TOKEN = os.environ.get("DEPLOY_WEBHOOK_TOKEN", "")
SCRIPT = os.environ.get("DEPLOY_SCRIPT", "/opt/youban/deploy-webhook.sh")
IMAGE_PREFIX = os.environ.get("IMAGE_PREFIX", "ghcr.io/mjiadfwaff-bot/youban-server:")


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
        if not image.startswith(IMAGE_PREFIX):
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
            self._send(500, {"error": "deploy failed", "output": exc.stdout[-4000:], "stderr": exc.stderr[-4000:]})
            return
        except subprocess.TimeoutExpired:
            self._send(504, {"error": "deploy timeout"})
            return

        self._send(200, {"ok": True, "image": image, "output": result.stdout[-4000:]})


if __name__ == "__main__":
    server = ThreadingHTTPServer((HOST, PORT), Handler)
    print(f"deploy webhook listening on {HOST}:{PORT}", flush=True)
    server.serve_forever()
