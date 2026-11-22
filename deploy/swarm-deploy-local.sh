#!/usr/bin/env bash
set -euo pipefail
export LANG=C
export LC_ALL=C

STARTED=$SECONDS
CHILD_PIDS=()
INTERRUPTED=0

timestamp() {
  date '+%Y-%m-%d %H:%M:%S'
}

logf() {
  printf '[%s] ' "$(timestamp)"
  printf "$@"
}

kill_tree() {
  local pid="$1" child
  while read -r child; do
    [[ -n "$child" ]] || continue
    kill_tree "$child"
  done < <(pgrep -P "$pid" 2>/dev/null || true)
  kill -TERM "$pid" 2>/dev/null || true
}

cleanup_children() {
  local pid
  for pid in "${CHILD_PIDS[@]}"; do
    kill_tree "$pid"
  done
  CHILD_PIDS=()
}

handle_signal() {
  local code="$1"
  INTERRUPTED=1
  trap - INT TERM
  echo "发布已中断，正在清理后台部署进程" >&2
  cleanup_children
  wait 2>/dev/null || true
  exit "$code"
}

trap 'handle_signal 130' INT
trap 'handle_signal 143' TERM
trap 'cleanup_children' EXIT

TAG="${1:-}"
if [[ "$TAG" == "-h" || "$TAG" == "--help" || -z "$TAG" ]]; then
  cat <<'HELP'
用法：
  deploy/tencent-deploy-all.sh <image-tag> [-f|--force] [角色 ...]

默认角色：publish collector scheduler background media web
account 可作为显式角色部署，始终保持单副本并使用 stop-first，避免两个账号运行时同时在线
非 Web 角色并发部署。Web 按 blue → 切流 → green → 切回执行。
发布前通过 tccli TCR API 校验标签并锁定镜像 digest。
HELP
  [[ "$TAG" != "" ]] && exit 0
  exit 2
fi
if [[ ! "$TAG" =~ ^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$ ]]; then
  echo "用法：TENCENT_DEPLOY_TARGET=swarm $0 <image-tag> [-f|--force] [角色 ...]" >&2
  exit 2
fi
shift

FORCE=false
ROLE_ARGS=()
for arg in "$@"; do
  if [[ "$arg" == "-f" || "$arg" == "--force" ]]; then
    FORCE=true
  else
    ROLE_ARGS+=("$arg")
  fi
done

MANAGER_SSH_TARGET="${SWARM_MANAGER_SSH_TARGET:-ubuntu@43.160.212.235}"
MANAGER_SSH_KEY="${SWARM_MANAGER_SSH_KEY:-/Users/ley/.ssh/id_hk_rsa}"
IMAGE_REPOSITORY="${TENCENT_IMAGE_REPOSITORY:-sgccr.ccs.tencentyun.com/xiaohuiji-cc/xiaohuiji-package}"
WEB_NODE_URL="${SWARM_WEB_NODE_URL:-http://43.160.221.143}"
ROLES=(publish collector scheduler background media web)
if (( ${#ROLE_ARGS[@]} > 0 )); then ROLES=("${ROLE_ARGS[@]}"); fi

for role in "${ROLES[@]}"; do
  case "$role" in
    publish|collector|scheduler|background|media|web|account) ;;
    *) echo "未知 Swarm 角色：$role" >&2; exit 2 ;;
  esac
done

command -v ssh >/dev/null || { echo "需要 ssh" >&2; exit 1; }
command -v tccli >/dev/null || { echo "需要 tccli，用于发布前校验 TCR 标签" >&2; exit 1; }
command -v jq >/dev/null || { echo "需要 jq，用于解析 TCR 镜像摘要" >&2; exit 1; }
IMAGE="$IMAGE_REPOSITORY:$TAG"
TENCENT_REGION="${TENCENT_REGION:-ap-singapore}"
IMAGE_DIGEST=$(tccli tcr DescribeImagePersonal --region "$TENCENT_REGION" \
  --RepoName "${IMAGE_REPOSITORY#*/}" --Tag "$TAG" |
  jq -er --arg tag "$TAG" '
    [(.Response // .).Data.TagInfo[]? | select(.TagName == $tag) | .TagId
     | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))]
    | if length == 1 then .[0] else error("TCR 标签不存在或没有唯一镜像摘要") end')
IMAGE="$IMAGE_REPOSITORY@$IMAGE_DIGEST"
logf '[preflight] TCR 标签 %s 已确认，摘要 %s\n' "$TAG" "$IMAGE_DIGEST"
SSH=(ssh -i "$MANAGER_SSH_KEY" -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o SetEnv=LANG=C -o SetEnv=LC_ALL=C "$MANAGER_SSH_TARGET")

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  C_RESET=$'\033[0m'
  C_BLUE=$'\033[36m'
  C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'
  C_RED=$'\033[31m'
else
  C_RESET='' C_BLUE='' C_GREEN='' C_YELLOW='' C_RED=''
fi

remote() { "${SSH[@]}" "$@"; }

logf '[preflight] Manager 预拉取目标镜像\n'
remote "LANG=C LC_ALL=C sudo docker pull '$IMAGE' >/dev/null"
logf '[preflight] Manager 镜像摘要已确认\n'

service_has_digest() {
  local service="$1" image
  image=$(remote "sudo docker service inspect '$service' --format '{{.Spec.TaskTemplate.ContainerSpec.Image}}'" 2>/dev/null || true)
  [[ "$image" == *"@$IMAGE_DIGEST" ]]
}

wait_service() {
  local service="$1" desired="$2" deadline=$((SECONDS + ${SWARM_ROLLOUT_TIMEOUT_SECONDS:-300})) state base last_state='' running matching progress_state
  while (( SECONDS < deadline )); do
    state=$(remote "sudo docker service ls --filter name='$service' --format '{{.Replicas}}'" | head -1 || true)
    running=$(remote "sudo docker service ps --filter desired-state=running --format '{{.CurrentState}}|{{.Image}}' '$service'" | awk -F'|' '$1 ~ /^(Running|Ready)/ {count++} END {print count+0}')
    matching=$(remote "sudo docker service ps --filter desired-state=running --format '{{.CurrentState}}|{{.Image}}' '$service'" | awk -F'|' -v digest="$IMAGE_DIGEST" '$1 ~ /^(Running|Ready)/ && index($2, digest) {count++} END {print count+0}')
    base="${state%% *}"
    progress_state="$state|$running|$matching"
    if [[ "$progress_state" != "$last_state" ]]; then
      printf '[%s] %s[%s] 状态 %s，目标 %s/%s，运行任务 %s/%s，目标摘要任务 %s/%s%s\n' "$(timestamp)" "$C_BLUE" "$service" "${state:-unknown}" "$desired" "$desired" "$running" "$desired" "$matching" "$desired" "$C_RESET"
      last_state="$progress_state"
    fi
    if [[ "$base" == "$desired/$desired" && "$running" == "$desired" && "$matching" == "$desired" ]]; then
      return 0
    fi
    sleep 3
  done
  remote "sudo docker service ps --no-trunc '$service'"
  return 1
}

update_worker() {
  local role="$1" service="xiaohuiji-$1"
  local desired role_started=$SECONDS
  if [[ "$FORCE" != true ]] && service_has_digest "$service"; then
    logf '%s[%s] 已是版本 %s，跳过部署%s\n' "$C_GREEN" "$role" "$TAG" "$C_RESET"
    return 0
  fi
  logf '%s[%s] 开始部署%s\n' "$C_YELLOW" "$role" "$C_RESET"
  desired=$(remote "sudo docker service inspect '$service' --format '{{if .Spec.Mode.Replicated}}{{.Spec.Mode.Replicated.Replicas}}{{else}}1{{end}}'")
  remote "LANG=C LC_ALL=C sudo docker service update --detach=true --with-registry-auth --image '$IMAGE' --env-add APP_REVISION='$TAG' --update-order stop-first --update-parallelism 1 '$service' >/dev/null"
  logf '%s[%s] 已提交更新，等待 %s/%s 就绪%s\n' "$C_BLUE" "$role" "$desired" "$desired" "$C_RESET"
  wait_service "$service" "$desired"
  logf '%s[%s] 部署成功，耗时 %s 秒%s\n' "$C_GREEN" "$role" "$((SECONDS - role_started))" "$C_RESET"
}

update_web_color() {
  local color="$1"
  local service="xiaohuiji-web-$color" role_started=$SECONDS
  if [[ "$FORCE" != true ]] && service_has_digest "$service"; then
    logf '%s[web-%s] 已是版本 %s，跳过部署%s\n' "$C_GREEN" "$color" "$TAG" "$C_RESET"
    return 0
  fi
  logf '%s[web-%s] 开始部署%s\n' "$C_YELLOW" "$color" "$C_RESET"
  remote "LANG=C LC_ALL=C sudo docker service update --detach=true --with-registry-auth --image '$IMAGE' --env-add APP_REVISION='$TAG' --update-order stop-first --update-parallelism 1 '$service' >/dev/null"
  logf '%s[web-%s] 已提交更新，等待 1/1 就绪%s\n' "$C_BLUE" "$color" "$C_RESET"
  wait_service "$service" 1
  curl -fsS --max-time 8 "$WEB_NODE_URL/readyz" >/dev/null
  logf '%s[web-%s] readyz 通过，耗时 %s 秒%s\n' "$C_GREEN" "$color" "$((SECONDS - role_started))" "$C_RESET"
}

switch_router() {
  local color="$1" current_config next_config
  if [[ "$color" == blue ]]; then
    current_config=web-caddyfile-green
    next_config=web-caddyfile-blue
  else
    current_config=web-caddyfile-blue
    next_config=web-caddyfile-green
  fi
  remote "sudo docker service update --config-rm '$current_config' --config-add source='$next_config',target=/etc/caddy/Caddyfile xiaohuiji-web-router >/dev/null"
  for _ in $(seq 1 20); do
    if curl -fsS --max-time 5 "$WEB_NODE_URL/readyz" >/dev/null; then return 0; fi
    sleep 1
  done
  return 1
}

non_web=()
for role in "${ROLES[@]}"; do
  if [[ "$role" != web ]]; then
    non_web+=("$role")
  fi
done
pids=()
for role in "${non_web[@]}"; do
  update_worker "$role" &
  pids+=("$!")
  CHILD_PIDS+=("$!")
done
failed=0
for pid in "${pids[@]}"; do
  if ! wait "$pid"; then
    failed=1
  fi
done
CHILD_PIDS=()
(( failed == 0 )) || { echo "非 Web 角色部署失败" >&2; exit 1; }

if printf '%s\n' "${ROLES[@]}" | grep -qx web; then
  web_blue_current=false
  web_green_current=false
  if [[ "$FORCE" != true ]]; then
    service_has_digest xiaohuiji-web-blue && web_blue_current=true
    service_has_digest xiaohuiji-web-green && web_green_current=true
  fi
  if [[ "$FORCE" == true || "$web_blue_current" != true || "$web_green_current" != true ]]; then
    # 先更新非在线颜色，再切到它，更新另一色，最后切回，避免同一宿主端口被 start-first 占用。
    update_web_color blue
    switch_router blue
    update_web_color green
    switch_router green
  else
    logf '%s[web] blue/green 都已是版本 %s，跳过部署%s\n' "$C_GREEN" "$TAG" "$C_RESET"
  fi
fi

logf '%sSwarm 部署全部成功，镜像 %s，总耗时 %s 秒%s\n' "$C_GREEN" "${IMAGE_REPOSITORY}:${TAG}" "$((SECONDS - STARTED))" "$C_RESET"
