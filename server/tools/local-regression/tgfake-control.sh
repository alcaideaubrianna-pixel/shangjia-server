#!/usr/bin/env bash
# 用 tgfake 的 /sim/fault 替代旧 tg-hook 的场景控制接口。
set -euo pipefail

base="${YOUBAN_LOCAL_TGFAKE_URL:-http://127.0.0.1:18790}"
payload="${1:-}"
if [[ -z "$payload" ]]; then
  payload='{"default":{"action":"success","delayMs":2}}'
fi

python3 - "$base" "$payload" <<'PY'
import json, sys, urllib.request

base, raw = sys.argv[1:]
if raw.startswith("@"):
    with open(raw[1:], encoding="utf-8") as handle:
        value = json.load(handle)
else:
    value = json.loads(raw)

def post(path, body):
    request = urllib.request.Request(
        base + path,
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=5) as response:
        if response.status != 200:
            raise RuntimeError(f"tgfake {path} status={response.status}")

# 每次切换场景先清掉旧规则，避免不同测试之间相互污染。
request = urllib.request.Request(base + "/sim/fault", method="DELETE")
with urllib.request.urlopen(request, timeout=5):
    pass

default = value.get("default", {})
rules = value.get("rules", [])
if not rules:
    raise SystemExit(0)
for rule in rules:
    action = rule.get("action", default.get("action", "success"))
    method = rule.get("method", "*")
    fault = {"method": method, "times": 0}
    if rule.get("chatId"):
        fault["chat_id"] = int(rule["chatId"])
    delay = int(rule.get("delayMs", default.get("delayMs", 0)) or 0)
    if delay:
        fault["delay_ms"] = delay
    if action in ("timeout", "accepted_timeout"):
        fault["disconnect"] = True
        fault["after_apply"] = action == "accepted_timeout"
        fault["times"] = 1
    elif action == "error":
        fault["code"] = int(rule.get("code", 500))
        fault["times"] = int(rule.get("times", 0))
    elif action not in ("success", ""):
        raise SystemExit(f"unsupported tgfake action: {action}")
    post("/sim/fault", fault)
PY
