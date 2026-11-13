#!/usr/bin/env bash
set -euo pipefail

# 本地连接线上 SAE 配置的调试入口。
# 默认只启动 publish-worker 诊断模式；写入线上前必须显式设置 ONLINE_DEBUG_WRITE=1。
# 禁止启动 scheduler，避免本地进程接管线上全局定时任务。

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVER_DIR="$ROOT_DIR"
APP_ID="${SAE_PUBLISH_APP_ID:-2fb5d3db-0975-4dea-b086-805d255131fa}"
REGION_ID="${SAE_REGION_ID:-ap-southeast-1}"
PROFILE="${ALIBABA_CLOUD_PROFILE:-xiaohuiji-sae}"
ROLE="${ONLINE_DEBUG_ROLE:-publish-worker}"
ALL_MODE="${ONLINE_DEBUG_ALL:-0}"
WRITE_ARG=0
WRITE_ALL=0
LOAD_MODE=0
REAL_TG=0
WITH_ACCOUNT=0
WEBHOOK_URL="${ONLINE_DEBUG_WEBHOOK_URL:-}"
# 默认使用 DEBUG 启动；调用方显式传入 false 时保留调用方配置。
: "${DEBUG:=true}"
: "${YOUBAN_DB_DEBUG:=true}"
for arg in "$@"; do
  case "$arg" in
    all) ALL_MODE=1 ;;
    --write|write) WRITE_ARG=1 ;;
    --write-all|write-all) WRITE_ARG=1; WRITE_ALL=1 ;;
    --load|load) LOAD_MODE=1; ALL_MODE=1; WRITE_ARG=1; WRITE_ALL=1 ;;
    --real-tg|real-tg) REAL_TG=1 ;;
    --with-account|with-account) WITH_ACCOUNT=1 ;;
    webhookurl=*) WEBHOOK_URL="${arg#webhookurl=}" ;;
  esac
done
SESSION="${ONLINE_DEBUG_SESSION:-$(date +%Y-%m-%d-%H-%M-%S)}"
LOG_DIR="${ONLINE_DEBUG_LOG_DIR:-/tmp/youban-online-debug/$SESSION}"
WRITE_MODE="${ONLINE_DEBUG_WRITE:-0}"
if [[ "$WRITE_ARG" == "1" ]]; then WRITE_MODE=1; fi
if [[ "$WRITE_ALL" == "1" ]]; then WRITE_MODE=1; fi
if [[ "${1:-}" == "all" && -z "${ONLINE_DEBUG_WRITE+x}" && "$WRITE_ARG" == "0" ]]; then WRITE_MODE=1; fi
BUILD_MODE="${ONLINE_DEBUG_BUILD:-1}"
APPLY_LOCAL_SCHEMA="${ONLINE_DEBUG_APPLY_SCHEMA:-1}"
if [[ -n "${ONLINE_DEBUG_ENV_FILE:-}" ]]; then
  ENV_FILE="$ONLINE_DEBUG_ENV_FILE"
else
  # macOS 要求 XXXXXX 位于模板末尾；mktemp -t 在 macOS/Linux 均可用。
  ENV_FILE="$(mktemp -t youban-online-debug)"
fi
BIN="${ONLINE_DEBUG_SERVER_BIN:-$ROOT_DIR/bin/hotgo}"
FRONTEND_DIR="${ONLINE_DEBUG_FRONTEND_DIR:-$ROOT_DIR/../web}"
FRONTEND_PORT="${ONLINE_DEBUG_FRONTEND_PORT:-8001}"
FRONTEND_PID=""
TG_FAKE_PORT="${ONLINE_DEBUG_TG_FAKE_PORT:-19100}"
ACTIVE_LOCK_DIR="${ONLINE_DEBUG_ACTIVE_LOCK_DIR:-/tmp/youban-online-debug/active.lock}"

cleanup() {
  if [[ -n "$FRONTEND_PID" ]]; then kill "$FRONTEND_PID" 2>/dev/null || true; fi
  if [[ "${KEEP_ONLINE_DEBUG_ENV:-0}" != "1" ]]; then rm -f "$ENV_FILE"; fi
  if [[ "${ONLINE_DEBUG_ALLOW_MULTIPLE:-0}" != "1" ]]; then rmdir "$ACTIVE_LOCK_DIR" 2>/dev/null || true; fi
}
trap cleanup EXIT INT TERM

if [[ "$WRITE_MODE" != "0" && "$WRITE_MODE" != "1" ]]; then echo "ONLINE_DEBUG_WRITE 必须为 0 或 1" >&2; exit 2; fi
command -v jq >/dev/null || { echo "缺少 jq" >&2; exit 1; }
if [[ "$ALL_MODE" == "1" && "${ONLINE_DEBUG_ALLOW_MULTIPLE:-0}" != "1" ]]; then
  if ! mkdir "$ACTIVE_LOCK_DIR" 2>/dev/null; then
    echo "已有本地线上调试会话正在运行；为避免多个 Worker 同时消费同一 PG/Redis，请先停止旧会话，或设置 ONLINE_DEBUG_ALLOW_MULTIPLE=1。" >&2
    exit 1
  fi
fi
if [[ "$BUILD_MODE" == "1" ]]; then
  echo "构建本地二进制：$BIN"
  (cd "$ROOT_DIR" && go build -o "$BIN" ./main.go)
fi
if [[ ! -x "$BIN" ]]; then echo "本地二进制不存在：${BIN}；构建失败" >&2; exit 1; fi
umask 077
if [[ "$LOAD_MODE" == "1" ]]; then
  echo "LOAD模式：使用本地PG/Redis/TG Fake，不读取线上SAE配置。"
  cat > "$ENV_FILE" <<'ENV_LOCAL'
export YOUBAN_DB_DRIVER=pgsql
export YOUBAN_DB_HOST=127.0.0.1
export YOUBAN_DB_PORT=5432
export YOUBAN_DB_USER=postgres
export YOUBAN_DB_PASSWORD=postgres
export YOUBAN_DB_NAME=youban_tgfake_test
export YOUBAN_DATABASE_LINK='pgsql:postgres:postgres@tcp(127.0.0.1:5432)/youban_tgfake_test'
export GF_REDIS_DEFAULT_ADDRESS=127.0.0.1:6379
export GF_REDIS_DEFAULT_DB=15
export YOUBAN_REDIS_ADDRESS=127.0.0.1:6379
export YOUBAN_REDIS_DB=15
export REDIS_ADDRESS=127.0.0.1:6379
export REDIS_DB=15
export YOUBAN_TELEGRAM_RUNTIME_API_URL=http://127.0.0.1:19100
export YOUBAN_TELEGRAM_BOT_API_URL=http://127.0.0.1:${TG_FAKE_PORT}
# 本机无法访问 SAE 同地域 OSS 内网 Endpoint；本地角色统一使用公网 Endpoint。
export YOUBAN_STORAGE_OSS_ENDPOINT=https://oss-ap-southeast-1.aliyuncs.com
export YOUBAN_LOCAL_REGRESSION=1
ENV_LOCAL
  if [[ "$REAL_TG" == "1" ]]; then
    command -v aliyun >/dev/null || { echo "--real-tg 需要 aliyun CLI" >&2; exit 1; }
    echo "警告：LOAD模式使用本地PG/Redis，但 Telegram 将调用线上真实接口。"
    real_cfg="$(ALIBABA_CLOUD_REGION_ID="$REGION_ID" aliyun sae DescribeApplicationConfig --region "$REGION_ID" --RegionId "$REGION_ID" --AppId "$APP_ID" --profile "$PROFILE")"
    printf '%s' "$real_cfg" | jq -r '.Data.Envs[]? | select(.name != null and (.name|test("TELEGRAM|TG_|BOT_TOKEN|BOT_API";"i"))) | "export " + .name + "=" + (.value|@sh)' >> "$ENV_FILE"
    cat >> "$ENV_FILE" <<'ENV_REAL_TG'
export YOUBAN_TELEGRAM_RUNTIME_API_URL=https://api.telegram.org
export YOUBAN_TELEGRAM_BOT_API_URL=https://api.telegram.org
export YOUBAN_TELEGRAM_LOCAL_BOT_ENABLED=1
ENV_REAL_TG
  fi
else
  command -v aliyun >/dev/null || { echo "缺少 aliyun CLI" >&2; exit 1; }
  cfg="$(ALIBABA_CLOUD_REGION_ID="$REGION_ID" aliyun sae DescribeApplicationConfig --region "$REGION_ID" --RegionId "$REGION_ID" --AppId "$APP_ID" --profile "$PROFILE")"
  printf '%s' "$cfg" | jq -e '.Data.Envs' >/dev/null
  printf '%s' "$cfg" | jq -r '.Data.Envs[]? | select(.name != null and .value != null) | "export " + .name + "=" + (.value|@sh)' > "$ENV_FILE"
fi
cat >> "$ENV_FILE" <<ENV
export DEBUG=$(printf '%q' "$DEBUG")
export YOUBAN_DB_DEBUG=$(printf '%q' "$YOUBAN_DB_DEBUG")
${WEBHOOK_URL:+export YOUBAN_DEBUG_WEBHOOK_URL=$(printf '%q' "$WEBHOOK_URL")}
export YOUBAN_LOCAL_APPLY_SCHEMA=0
export YOUBAN_PUBLISH_RUNTIME_ROLE=$(printf '%q' "$ROLE")
export YOUBAN_PUBLISH_PUSH_ENGINE=unified_stream
export YOUBAN_PUBLISH_SCHEDULER_ENABLED=false
export YOUBAN_PUBLISH_RECOVERY_ENABLED=false
ENV
if [[ "$WRITE_MODE" == "0" ]]; then
  cat >> "$ENV_FILE" <<'ENV'
export YOUBAN_ONLINE_DEBUG_READ_ONLY=true
export YOUBAN_PUBLISH_QUEUE_CONCURRENCY=1
export YOUBAN_PUBLISH_BULK_CONCURRENCY=1
export YOUBAN_PUBLISH_HISTORY_CONCURRENCY=0
export YOUBAN_PUBLISH_COLLECT_REALTIME_CONCURRENCY=0
export YOUBAN_PUBLISH_MEDIA_ACCOUNT_CONCURRENCY=0
export YOUBAN_PUBLISH_MEDIA_SOURCE_CONCURRENCY=0
ENV
else
  if [[ "$ALL_MODE" == "1" || "$ROLE" != "web" ]]; then
    if [[ -z "${ONLINE_DEBUG_TENANT_ID:-}" && "$ALL_MODE" == "1" && "$WRITE_ALL" != "1" ]]; then
      read -r -p "请输入线上测试租户ID（直接回车进入只读模式）：" ONLINE_DEBUG_TENANT_ID
    fi
    if [[ -z "${ONLINE_DEBUG_TENANT_ID:-}" && "$WRITE_ALL" != "1" ]]; then
      echo "未填写测试租户ID，自动切换为只读调试模式。"
      WRITE_MODE=0
    else
      ONLINE_DEBUG_RUN_ID="${ONLINE_DEBUG_RUN_ID:-online-debug-$(date +%Y%m%d%H%M%S)}"
    fi
  fi
  cat >> "$ENV_FILE" <<ENV
export YOUBAN_ONLINE_DEBUG_READ_ONLY=false
${ONLINE_DEBUG_TENANT_ID:+export YOUBAN_ONLINE_DEBUG_TENANT_ID=$(printf '%q' "$ONLINE_DEBUG_TENANT_ID")}
${ONLINE_DEBUG_RUN_ID:+export YOUBAN_ONLINE_DEBUG_RUN_ID=$(printf '%q' "$ONLINE_DEBUG_RUN_ID")}
export YOUBAN_PUBLISH_QUEUE_CONCURRENCY=${ONLINE_DEBUG_QUEUE_CONCURRENCY:-8}
export YOUBAN_PUBLISH_BULK_CONCURRENCY=${ONLINE_DEBUG_BULK_CONCURRENCY:-8}
export YOUBAN_PUBLISH_HISTORY_CONCURRENCY=0
export YOUBAN_PUBLISH_COLLECT_REALTIME_CONCURRENCY=${ONLINE_DEBUG_COLLECT_CONCURRENCY:-1}
export YOUBAN_PUBLISH_MEDIA_ACCOUNT_CONCURRENCY=${ONLINE_DEBUG_MEDIA_CONCURRENCY:-4}
export YOUBAN_PUBLISH_MEDIA_SOURCE_CONCURRENCY=${ONLINE_DEBUG_MEDIA_SOURCE_CONCURRENCY:-4}
ENV
fi
# shellcheck disable=SC1090
source "$ENV_FILE"
if [[ "$LOAD_MODE" == "1" && "$APPLY_LOCAL_SCHEMA" == "1" ]]; then
  command -v psql >/dev/null || { echo "--load 默认迁移需要 psql" >&2; exit 1; }
  echo "执行本地上架系统完整迁移：addons/youban_publish/resource/sql/upgrade.pgsql.sql"
  PGPASSWORD="${YOUBAN_DB_PASSWORD:-}" psql \
    -h "${YOUBAN_DB_HOST:-127.0.0.1}" \
    -p "${YOUBAN_DB_PORT:-5432}" \
    -U "${YOUBAN_DB_USER:-postgres}" \
    -d "${YOUBAN_DB_NAME:-youban_tgfake_test}" \
    -v ON_ERROR_STOP=1 \
    -f "$ROOT_DIR/addons/youban_publish/resource/sql/upgrade.pgsql.sql"
  echo "执行本地 Telegram Collector 初始化：addons/telegram_collector/resource/sql/install.pgsql.sql"
  PGPASSWORD="${YOUBAN_DB_PASSWORD:-}" psql \
    -h "${YOUBAN_DB_HOST:-127.0.0.1}" \
    -p "${YOUBAN_DB_PORT:-5432}" \
    -U "${YOUBAN_DB_USER:-postgres}" \
    -d "${YOUBAN_DB_NAME:-youban_tgfake_test}" \
    -v ON_ERROR_STOP=1 \
    -f "$ROOT_DIR/addons/telegram_collector/resource/sql/install.pgsql.sql"
  PGPASSWORD="${YOUBAN_DB_PASSWORD:-}" psql \
    -h "${YOUBAN_DB_HOST:-127.0.0.1}" \
    -p "${YOUBAN_DB_PORT:-5432}" \
    -U "${YOUBAN_DB_USER:-postgres}" \
    -d "${YOUBAN_DB_NAME:-youban_tgfake_test}" \
    -v ON_ERROR_STOP=1 \
    -f "$ROOT_DIR/addons/telegram_collector/resource/sql/ingest_spool.pgsql.sql"
fi
echo "线上调试配置已加载：region=$REGION_ID app=$APP_ID role=$ROLE all=${ALL_MODE} write=${WRITE_MODE} writeAll=${WRITE_ALL}"
echo "session=$SESSION"
echo "角色日志目录：$LOG_DIR"
echo "Web入口日志：$LOG_DIR/web.log"

if [[ "$ALL_MODE" == "1" ]]; then
  # 先创建会话日志目录，再启动 TG Fake 和各角色，避免重定向目标不存在。
  mkdir -p "$LOG_DIR"
  # account 是 Telegram 协议号长连接，必须继续使用 SAE 单实例，不在本地重复启动。
  roles=(web collector-worker background-worker media-worker publish-worker scheduler)
  if [[ "$WITH_ACCOUNT" == "1" ]]; then roles+=(account); fi
  base_port="${ONLINE_DEBUG_HTTP_PORT_BASE:-18000}"
  # 每次启动使用独立端口组，避免旧会话残留导致新角色打印 PID 后立即退出。
  if command -v lsof >/dev/null; then
    while :; do
      occupied=0
      for offset in 0 1 2 3 4 5 6; do
        if lsof -nP -iTCP:$((base_port + offset)) -sTCP:LISTEN >/dev/null 2>&1; then
          occupied=1
          break
        fi
      done
      [[ "$occupied" == "0" ]] && break
      base_port=$((base_port + 10))
    done
  fi
  if [[ "$LOAD_MODE" == "1" && "$REAL_TG" != "1" ]]; then
    if command -v lsof >/dev/null; then
      while lsof -nP -iTCP:"$TG_FAKE_PORT" -sTCP:LISTEN >/dev/null 2>&1; do
        TG_FAKE_PORT=$((TG_FAKE_PORT + 1))
      done
    fi
    sed -i.bak "s#127.0.0.1:19100#127.0.0.1:${TG_FAKE_PORT}#g" "$ENV_FILE"
    echo "启动本地 TG Fake Hook：127.0.0.1:${TG_FAKE_PORT}"
    (cd "$ROOT_DIR" && go run ./tools/tg_hook_mock --addr "127.0.0.1:${TG_FAKE_PORT}") >"$LOG_DIR/tg-fake.log" 2>&1 &
    pids=("$!")
    sleep 1
  fi
  printf 'session=%s\nregion=%s\napp=%s\nwrite=%s\n' "$SESSION" "$REGION_ID" "$APP_ID" "$WRITE_MODE" > "$LOG_DIR/session.env"
  pids=()
  stop_roles() {
    for pid in "${pids[@]:-}"; do kill "$pid" 2>/dev/null || true; done
    if [[ -n "$FRONTEND_PID" ]]; then kill "$FRONTEND_PID" 2>/dev/null || true; fi
    wait 2>/dev/null || true
  }
  trap 'stop_roles; cleanup' EXIT INT TERM
  if [[ "${ONLINE_DEBUG_FRONTEND:-1}" == "1" ]]; then
    [[ -f "$FRONTEND_DIR/package.json" ]] || {
      echo "前端目录不存在或缺少 package.json：$FRONTEND_DIR" >&2
      exit 1
    }
    if command -v lsof >/dev/null; then
      while lsof -nP -iTCP:"$FRONTEND_PORT" -sTCP:LISTEN >/dev/null 2>&1; do
        FRONTEND_PORT=$((FRONTEND_PORT + 1))
      done
    fi
    frontend_log="$LOG_DIR/frontend.log"
    echo "启动后台前端：pnpm dev port=$FRONTEND_PORT 日志=$frontend_log"
    (
      export DEBUG=true
      export VITE_PORT="$FRONTEND_PORT"
      export VITE_PROXY="[[\"/admin\",\"http://127.0.0.1:$base_port/admin\"],[\"/api\",\"http://127.0.0.1:$base_port/api\"]]"
      cd "$FRONTEND_DIR"
      exec pnpm run dev -- --port "$FRONTEND_PORT"
    ) >"$frontend_log" 2>&1 &
    FRONTEND_PID="$!"
    echo "后台前端已启动：http://127.0.0.1:$FRONTEND_PORT pid=$FRONTEND_PID"
  fi
  index=0
  for role in "${roles[@]}"; do
    role_port=$((base_port + index))
    log_file="$LOG_DIR/${role}.log"
    echo "启动本地线上调试角色：$role port=$role_port pid=starting 日志=$log_file"
    (source "$ENV_FILE";
      if [[ "$REAL_TG" == "1" ]]; then
        export YOUBAN_TELEGRAM_RUNTIME_API_URL="https://api.telegram.org"
        export YOUBAN_TELEGRAM_BOT_API_URL="https://api.telegram.org"
        export YOUBAN_TELEGRAM_LOCAL_BOT_ENABLED=1
      fi
      export YOUBAN_PUBLISH_RUNTIME_ROLE="$role" GF_SERVER_ADDRESS=":$role_port" YOUBAN_SERVER_ADDRESS=":$role_port";
      exec "$BIN" "$role") >"$log_file" 2>&1 &
    pids+=("$!")
    role_pid="$!"
    echo "角色已启动：$role pid=$role_pid 日志=$log_file"
    if [[ "$role" == "web" ]]; then echo "Web入口地址：http://127.0.0.1:$role_port"; echo "管理后台地址：http://127.0.0.1:$role_port${YOUBAN_ADMIN_ENTRY_PATH:-/admin}"; echo "后台API地址：http://127.0.0.1:$role_port/admin"; fi
    index=$((index + 1))
  done
  echo "已启动 ${#roles[@]} 个角色；account=${WITH_ACCOUNT}；按 Ctrl-C 统一停止。"
  wait
else
  echo "单角色模式：${ROLE}；account=$WITH_ACCOUNT。"
  exec "$BIN" "$ROLE"
fi
