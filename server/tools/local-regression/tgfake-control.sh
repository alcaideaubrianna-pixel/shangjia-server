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
        # contains 对 multipart 方法同样生效；比 chat_id 字段更贴近实际 Bot API 编码形式。
        fault["contains"] = str(rule["chatId"])
    delay = int(rule.get("delayMs", default.get("delayMs", 0)) or 0)
    if delay:
        fault["delay_ms"] = delay
    if action == "timeout":
        fault["disconnect"] = True
        fault["times"] = 1
    elif action == "accepted_timeout":
        # 让客户端真实超时，Telegram 请求仍在 tgfake 内完成；业务应进入 unknown 对账状态。
        fault.pop("delay_ms", None)
        # 发送客户端的请求预算约 30 秒，使用 60 秒确保进入 unknown 对账分支。
        fault["delay_ms"] = max(delay, 60000)
        fault["times"] = 1
    elif action == "error":
        fault["code"] = int(rule.get("code", 500))
        fault["times"] = int(rule.get("times", 0))
    elif action not in ("success", ""):
        raise SystemExit(f"unsupported tgfake action: {action}")
    post("/sim/fault", fault)
PY
