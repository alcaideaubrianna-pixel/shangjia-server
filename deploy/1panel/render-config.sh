#!/usr/bin/env bash
set -euo pipefail

APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$APP_DIR"

if [[ ! -f .env ]]; then
  echo "missing .env, copy .env.example first" >&2
  exit 2
fi

set -a
# shellcheck disable=SC1091
source .env
set +a

: "${YOUBAN_SERVER_IMAGE:?YOUBAN_SERVER_IMAGE is required}"

CONFIG_FILE="${YOUBAN_CONFIG_FILE:-./server.config.yaml}"
CONFIG_FILE="${CONFIG_FILE#./}"

if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "initializing $CONFIG_FILE from image $YOUBAN_SERVER_IMAGE"
  docker run --rm "$YOUBAN_SERVER_IMAGE" cat /app/manifest/config/config.yaml > "$CONFIG_FILE"
fi

python3 - "$CONFIG_FILE" <<'PY'
import os
import re
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as f:
    text = f.read()


def env(name, default=""):
    return os.environ.get(name, default)


def yaml_quote(value):
    value = str(value)
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def yaml_bool(value):
    return "true" if str(value).lower() in ("1", "true", "yes", "on") else "false"


def top_block(name):
    pattern = rf"(?ms)^{re.escape(name)}:\n.*?(?=^\S|\Z)"
    match = re.search(pattern, text)
    if not match:
        raise SystemExit(f"missing top-level config section: {name}")
    return match


def replace_in_top(name, field, value):
    global text
    match = top_block(name)
    block = match.group(0)
    field_pattern = rf"(?m)^(\s+{re.escape(field)}:\s*).*$"
    if not re.search(field_pattern, block):
        raise SystemExit(f"missing config field: {name}.{field}")
    next_block = re.sub(field_pattern, rf"\g<1>{value}", block, count=1)
    text = text[:match.start()] + next_block + text[match.end():]


def replace_in_nested(top, nested, field, value):
    global text
    match = top_block(top)
    block = match.group(0)
    nested_pattern = rf"(?ms)^  {re.escape(nested)}:\n.*?(?=^  \S|^\S|\Z)"
    nested_match = re.search(nested_pattern, block)
    if not nested_match:
        raise SystemExit(f"missing config section: {top}.{nested}")
    nested_block = nested_match.group(0)
    field_pattern = rf"(?m)^(\s+{re.escape(field)}:\s*).*$"
    if not re.search(field_pattern, nested_block):
        raise SystemExit(f"missing config field: {top}.{nested}.{field}")
    next_nested = re.sub(field_pattern, rf"\g<1>{value}", nested_block, count=1)
    next_block = block[:nested_match.start()] + next_nested + block[nested_match.end():]
    text = text[:match.start()] + next_block + text[match.end():]


def replace_in_double_nested(top, first, second, field, value):
    global text
    match = top_block(top)
    block = match.group(0)
    first_pattern = rf"(?ms)^  {re.escape(first)}:\n.*?(?=^  \S|^\S|\Z)"
    first_match = re.search(first_pattern, block)
    if not first_match:
        raise SystemExit(f"missing config section: {top}.{first}")
    first_block = first_match.group(0)
    second_pattern = rf"(?ms)^    {re.escape(second)}:\n.*?(?=^    \S|^  \S|^\S|\Z)"
    second_match = re.search(second_pattern, first_block)
    if not second_match:
        raise SystemExit(f"missing config section: {top}.{first}.{second}")
    second_block = second_match.group(0)
    field_pattern = rf"(?m)^(\s+{re.escape(field)}:\s*).*$"
    if not re.search(field_pattern, second_block):
        raise SystemExit(f"missing config field: {top}.{first}.{second}.{field}")
    next_second = re.sub(field_pattern, rf"\g<1>{value}", second_block, count=1)
    next_first = first_block[:second_match.start()] + next_second + first_block[second_match.end():]
    next_block = block[:first_match.start()] + next_first + block[first_match.end():]
    text = text[:match.start()] + next_block + text[match.end():]


db_link = env("YOUBAN_DATABASE_LINK")
if not db_link:
    db_driver = env("YOUBAN_DB_DRIVER", "pgsql")
    db_user = env("YOUBAN_DB_USER", "youban")
    db_password = env("YOUBAN_DB_PASSWORD", "")
    db_host = env("YOUBAN_DB_HOST", "127.0.0.1")
    db_port = env("YOUBAN_DB_PORT", "5432")
    db_name = env("YOUBAN_DB_NAME", "youban")
    if db_driver == "mysql":
        db_link = f"mysql:{db_user}:{db_password}@tcp({db_host}:{db_port})/{db_name}?loc=Local&parseTime=true&charset=utf8mb4"
    else:
        db_link = f"pgsql:{db_user}:{db_password}@tcp({db_host}:{db_port})/{db_name}"

replace_in_top("system", "appName", yaml_quote(env("YOUBAN_APP_NAME", "youban")))
replace_in_top("system", "debug", yaml_bool(env("YOUBAN_DEBUG", "false")))
replace_in_top("system", "mode", yaml_quote(env("YOUBAN_MODE", "product")))

replace_in_top("server", "address", yaml_quote(":" + env("YOUBAN_SERVER_PORT", "8000").lstrip(":")))

runtime_roles = [
    role.strip()
    for role in re.split(r"[,;|]", env("YOUBAN_RUNTIME_ROLES", "all"))
    if role.strip()
]
runtime_roles_yaml = "[" + ", ".join(yaml_quote(role) for role in runtime_roles) + "]"
replace_in_top("runtime", "roles", runtime_roles_yaml)

replace_in_top("cache", "adapter", yaml_quote(env("YOUBAN_CACHE_ADAPTER", "redis")))
replace_in_top("token", "secretKey", yaml_quote(env("YOUBAN_TOKEN_SECRET", "change-me")))
replace_in_top("queue", "driver", yaml_quote(env("YOUBAN_QUEUE_DRIVER", "redis")))
replace_in_top("queue", "groupName", yaml_quote(env("YOUBAN_APP_NAME", "youban")))

replace_in_nested("redis", "default", "address", yaml_quote(f"{env('YOUBAN_REDIS_HOST', '127.0.0.1')}:{env('YOUBAN_REDIS_PORT', '6379')}"))
replace_in_nested("redis", "default", "db", yaml_quote(env("YOUBAN_REDIS_DB", "3")))
replace_in_nested("redis", "default", "pass", yaml_quote(env("YOUBAN_REDIS_PASSWORD", "")))

replace_in_nested("database", "default", "link", yaml_quote(db_link))
replace_in_nested("database", "default", "debug", yaml_bool(env("YOUBAN_DB_DEBUG", "false")))
replace_in_nested("database", "default", "Prefix", yaml_quote(env("YOUBAN_DB_PREFIX", "hg_")))

replace_in_top("content", "cdnBaseUrl", yaml_quote(env("YOUBAN_CONTENT_CDN_BASE_URL", "")))
replace_in_nested("youbanChat", "pocketPing", "baseUrl", yaml_quote(env("YOUBAN_CHAT_POCKETPING_BASE_URL", "")))
replace_in_nested("youbanChat", "pocketPing", "apiKey", yaml_quote(env("YOUBAN_CHAT_POCKETPING_API_KEY", "")))
replace_in_nested("youbanChat", "telegram", "chatId", yaml_quote(env("YOUBAN_CHAT_TELEGRAM_CHAT_ID", "")))
replace_in_nested("youbanChat", "telegram", "webhookBaseUrl", yaml_quote(env("YOUBAN_CHAT_TELEGRAM_WEBHOOK_BASE_URL", "")))

with open(path, "w", encoding="utf-8") as f:
    f.write(text)
PY

echo "rendered $CONFIG_FILE"
