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
for arg in "$@"; do
  case "$arg" in
    all) ALL_MODE=1 ;;
    --write|write) WRITE_ARG=1 ;;
    --write-all|write-all) WRITE_ARG=1; WRITE_ALL=1 ;;
  esac
done
SESSION="${ONLINE_DEBUG_SESSION:-$(date +%Y-%m-%d-%H-%M-%S)}"
LOG_DIR="${ONLINE_DEBUG_LOG_DIR:-/tmp/youban-online-debug/$SESSION}"
WRITE_MODE="${ONLINE_DEBUG_WRITE:-0}"
if [[ "$WRITE_ARG" == "1" ]]; then WRITE_MODE=1; fi
if [[ "$WRITE_ALL" == "1" ]]; then WRITE_MODE=1; fi
if [[ "${1:-}" == "all" && -z "${ONLINE_DEBUG_WRITE+x}" && "$WRITE_ARG" == "0" ]]; then WRITE_MODE=1; fi
BUILD_MODE="${ONLINE_DEBUG_BUILD:-1}"
ENV_FILE="${ONLINE_DEBUG_ENV_FILE:-$(mktemp /tmp/youban-online-debug.XXXXXX.env)}"
BIN="${ONLINE_DEBUG_SERVER_BIN:-$ROOT_DIR/bin/hotgo}"

cleanup() {
  if [[ "${KEEP_ONLINE_DEBUG_ENV:-0}" != "1" ]]; then rm -f "$ENV_FILE"; fi
}
trap cleanup EXIT INT TERM

if [[ "$WRITE_MODE" != "0" && "$WRITE_MODE" != "1" ]]; then echo "ONLINE_DEBUG_WRITE 必须为 0 或 1" >&2; exit 2; fi
command -v aliyun >/dev/null || { echo "缺少 aliyun CLI" >&2; exit 1; }
command -v jq >/dev/null || { echo "缺少 jq" >&2; exit 1; }
if [[ "$BUILD_MODE" == "1" ]]; then
  echo "构建本地二进制：$BIN"
  (cd "$ROOT_DIR" && go build -o "$BIN" ./main.go)
fi
if [[ ! -x "$BIN" ]]; then echo "本地二进制不存在：${BIN}；构建失败" >&2; exit 1; fi
umask 077
cfg="$(ALIBABA_CLOUD_REGION_ID="$REGION_ID" aliyun sae DescribeApplicationConfig --region "$REGION_ID" --RegionId "$REGION_ID" --AppId "$APP_ID" --profile "$PROFILE")"
printf '%s' "$cfg" | jq -e '.Data.Envs' >/dev/null
printf '%s' "$cfg" | jq -r '.Data.Envs[]? | select(.name != null and .value != null) | "export " + .name + "=" + (.value|@sh)' > "$ENV_FILE"
cat >> "$ENV_FILE" <<ENV
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
export YOUBAN_PUBLISH_QUEUE_CONCURRENCY=${ONLINE_DEBUG_QUEUE_CONCURRENCY:-1}
export YOUBAN_PUBLISH_BULK_CONCURRENCY=${ONLINE_DEBUG_BULK_CONCURRENCY:-1}
export YOUBAN_PUBLISH_HISTORY_CONCURRENCY=0
export YOUBAN_PUBLISH_COLLECT_REALTIME_CONCURRENCY=${ONLINE_DEBUG_COLLECT_CONCURRENCY:-1}
export YOUBAN_PUBLISH_MEDIA_ACCOUNT_CONCURRENCY=${ONLINE_DEBUG_MEDIA_CONCURRENCY:-1}
export YOUBAN_PUBLISH_MEDIA_SOURCE_CONCURRENCY=1
ENV
fi
# shellcheck disable=SC1090
source "$ENV_FILE"
echo "线上调试配置已加载：region=$REGION_ID app=$APP_ID role=$ROLE all=$ALL_MODE write=$WRITE_MODE writeAll=$WRITE_ALL"
echo "session=$SESSION"
echo "角色日志目录：$LOG_DIR"
echo "Web入口日志：$LOG_DIR/web.log"

if [[ "$ALL_MODE" == "1" ]]; then
  # account 是 Telegram 协议号长连接，必须继续使用 SAE 单实例，不在本地重复启动。
  roles=(web collector-worker background-worker media-worker publish-worker scheduler)
  base_port="${ONLINE_DEBUG_HTTP_PORT_BASE:-18000}"
  mkdir -p "$LOG_DIR"
  printf 'session=%s\nregion=%s\napp=%s\nwrite=%s\n' "$SESSION" "$REGION_ID" "$APP_ID" "$WRITE_MODE" > "$LOG_DIR/session.env"
  pids=()
  stop_roles() {
    for pid in "${pids[@]:-}"; do kill "$pid" 2>/dev/null || true; done
    wait 2>/dev/null || true
  }
  trap 'stop_roles; cleanup' EXIT INT TERM
  index=0
  for role in "${roles[@]}"; do
    role_port=$((base_port + index))
    log_file="$LOG_DIR/${role}.log"
    echo "启动本地线上调试角色：$role port=$role_port pid=starting 日志=$log_file"
    (source "$ENV_FILE"; export YOUBAN_PUBLISH_RUNTIME_ROLE="$role" GF_SERVER_ADDRESS=":$role_port" YOUBAN_SERVER_ADDRESS=":$role_port"; exec "$BIN" "$role") >"$log_file" 2>&1 &
    pids+=("$!")
    echo "角色已启动：$role pid=${pids[-1]} 日志=$log_file"
    if [[ "$role" == "web" ]]; then echo "Web入口地址：http://127.0.0.1:$role_port"; fi
    index=$((index + 1))
  done
  echo "已启动 ${#roles[@]} 个角色；account 未启动；按 Ctrl-C 统一停止。"
  wait
else
  echo "单角色模式：$ROLE；account 未启动。"
  exec "$BIN" "$ROLE"
fi
