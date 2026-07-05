#!/bin/sh
set -e

cd "${WORKDIR:-/app}"

CONFIG="manifest/config/config.yaml"

escape_sed() {
  printf '%s' "$1" | sed 's/[|&\]/\\&/g'
}

replace_range() {
  start="$1"
  end="$2"
  pattern="$3"
  replacement="$4"
  value="$(escape_sed "$replacement")"
  sed -i "/${start}/,/${end}/ s|${pattern}|${value}|" "$CONFIG"
}

replace_range_raw() {
  start="$1"
  end="$2"
  pattern="$3"
  replacement="$4"
  sed -i "/${start}/,/${end}/ s|${pattern}|${replacement}|" "$CONFIG"
}

[ -n "${PORT:-}" ] && replace_range '^server:' '^tcp:' 'address: ":[0-9]*"' "address: \":${PORT}\""
[ -n "${APP_MODE:-}" ] && replace_range '^system:' '^license:' 'mode: "[^"]*"' "mode: \"${APP_MODE}\""
[ -n "${APP_DEBUG:-}" ] && replace_range_raw '^system:' '^license:' 'debug: \(true\|false\)' "debug: ${APP_DEBUG}"

[ -n "${LICENSE_EDITION:-}" ] && replace_range '^license:' '^defaultLogger:' 'edition: "[^"]*"' "edition: \"${LICENSE_EDITION}\""
[ -n "${LICENSE_SERVER:-}" ] && replace_range '^license:' '^defaultLogger:' 'server: "[^"]*"' "server: \"${LICENSE_SERVER}\""
[ -n "${LICENSE_KEY:-}" ] && replace_range '^license:' '^defaultLogger:' 'key: "[^"]*"' "key: \"${LICENSE_KEY}\""
[ -n "${LICENSE_SECRET:-}" ] && replace_range '^license:' '^defaultLogger:' 'secret: "[^"]*"' "secret: \"${LICENSE_SECRET}\""
[ -n "${LICENSE_DOMAIN:-}" ] && replace_range '^license:' '^defaultLogger:' 'domain: "[^"]*"' "domain: \"${LICENSE_DOMAIN}\""

[ -n "${CACHE_ADAPTER:-}" ] && replace_range '^cache:' '^token:' 'adapter: "[^"]*"' "adapter: \"${CACHE_ADAPTER}\""
[ -n "${TOKEN_SECRET_KEY:-}" ] && replace_range '^token:' '^queue:' 'secretKey: "[^"]*"' "secretKey: \"${TOKEN_SECRET_KEY}\""
[ -n "${QUEUE_DRIVER:-}" ] && replace_range '^queue:' '^redis:' 'driver: "[^"]*"' "driver: \"${QUEUE_DRIVER}\""

[ -n "${REDIS_ADDRESS:-}" ] && replace_range '^redis:' '^database:' 'address: "[^"]*"' "address: \"${REDIS_ADDRESS}\""
[ -n "${REDIS_DB:-}" ] && replace_range '^redis:' '^database:' 'db: "[^"]*"' "db: \"${REDIS_DB}\""
[ -n "${REDIS_PASS:-}" ] && replace_range '^redis:' '^database:' 'pass: "[^"]*"' "pass: \"${REDIS_PASS}\""

[ -n "${DATABASE_DEFAULT_LINK:-}" ] && replace_range '^database:' '^jaeger:' 'link: "pgsql:[^"]*"' "link: \"${DATABASE_DEFAULT_LINK}\""
[ -n "${DATABASE_DEBUG:-}" ] && replace_range_raw '^database:' '^jaeger:' 'debug: \(true\|false\)' "debug: ${DATABASE_DEBUG}"
[ -n "${DATABASE_PREFIX:-}" ] && replace_range '^database:' '^jaeger:' 'Prefix: "[^"]*"' "Prefix: \"${DATABASE_PREFIX}\""

[ -n "${CONTENT_CDN_BASE_URL:-}" ] && replace_range '^content:' '^youbanChat:' 'cdnBaseUrl: "[^"]*"' "cdnBaseUrl: \"${CONTENT_CDN_BASE_URL}\""
[ -n "${YOUBAN_CHAT_POCKETPING_BASE_URL:-}" ] && replace_range '^youbanChat:' '^# 生成代码' 'baseUrl: "[^"]*"' "baseUrl: \"${YOUBAN_CHAT_POCKETPING_BASE_URL}\""
[ -n "${YOUBAN_CHAT_POCKETPING_API_KEY:-}" ] && replace_range '^youbanChat:' '^# 生成代码' 'apiKey: "[^"]*"' "apiKey: \"${YOUBAN_CHAT_POCKETPING_API_KEY}\""
[ -n "${YOUBAN_CHAT_TELEGRAM_CHAT_ID:-}" ] && replace_range '^youbanChat:' '^# 生成代码' 'chatId: "[^"]*"' "chatId: \"${YOUBAN_CHAT_TELEGRAM_CHAT_ID}\""
[ -n "${YOUBAN_CHAT_TELEGRAM_WEBHOOK_BASE_URL:-}" ] && replace_range '^youbanChat:' '^# 生成代码' 'webhookBaseUrl: "[^"]*"' "webhookBaseUrl: \"${YOUBAN_CHAT_TELEGRAM_WEBHOOK_BASE_URL}\""

exec ./hotgo
