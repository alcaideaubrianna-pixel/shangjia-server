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

[ -n "${CACHE_ADAPTER:-}" ] && replace_range '^cache:' '^token:' 'adapter: "[^"]*"' "adapter: \"${CACHE_ADAPTER}\""
[ -n "${TOKEN_SECRET_KEY:-}" ] && replace_range '^token:' '^queue:' 'secretKey: "[^"]*"' "secretKey: \"${TOKEN_SECRET_KEY}\""
[ -n "${QUEUE_DRIVER:-}" ] && replace_range '^queue:' '^redis:' 'driver: "[^"]*"' "driver: \"${QUEUE_DRIVER}\""

[ -n "${REDIS_ADDRESS:-}" ] && replace_range '^redis:' '^database:' 'address: "[^"]*"' "address: \"${REDIS_ADDRESS}\""
[ -n "${REDIS_DB:-}" ] && replace_range '^redis:' '^database:' 'db: "[^"]*"' "db: \"${REDIS_DB}\""
[ -n "${REDIS_PASS:-}" ] && replace_range '^redis:' '^database:' 'pass: "[^"]*"' "pass: \"${REDIS_PASS}\""

[ -n "${DATABASE_DEFAULT_LINK:-}" ] && replace_range '^database:' '^jaeger:' 'link: "pgsql:[^"]*"' "link: \"${DATABASE_DEFAULT_LINK}\""
[ -n "${DATABASE_DEBUG:-}" ] && replace_range_raw '^database:' '^jaeger:' 'debug: \(true\|false\)' "debug: ${DATABASE_DEBUG}"
[ -n "${DATABASE_PREFIX:-}" ] && replace_range '^database:' '^jaeger:' 'Prefix: "[^"]*"' "Prefix: \"${DATABASE_PREFIX}\""

exec ./hotgo
