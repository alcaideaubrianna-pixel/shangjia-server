#!/usr/bin/env bash
set -euo pipefail

STARTED=$SECONDS
TAG="${1:-}"
if [[ ! "$TAG" =~ ^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$ ]]; then
  echo "用法：TENCENT_DEPLOY_TARGET=swarm $0 <image-tag> [角色 ...]" >&2
  exit 2
fi
shift

MANAGER_SSH_TARGET="${SWARM_MANAGER_SSH_TARGET:-ubuntu@43.160.212.235}"
MANAGER_SSH_KEY="${SWARM_MANAGER_SSH_KEY:-/Users/ley/.ssh/id_hk_rsa}"
IMAGE_REPOSITORY="${TENCENT_IMAGE_REPOSITORY:-sgccr.ccs.tencentyun.com/xiaohuiji-cc/xiaohuiji-package}"
WEB_NODE_URL="${SWARM_WEB_NODE_URL:-http://43.160.221.143}"
ROLES=(publish collector scheduler background media web)
if (( $# > 0 )); then ROLES=("$@"); fi

for role in "${ROLES[@]}"; do
  case "$role" in
    publish|collector|scheduler|background|media|web) ;;
    *) echo "未知 Swarm 角色：$role" >&2; exit 2 ;;
  esac
done

command -v ssh >/dev/null || { echo "需要 ssh" >&2; exit 1; }
IMAGE="$IMAGE_REPOSITORY:$TAG"
SSH=(ssh -i "$MANAGER_SSH_KEY" -o BatchMode=yes -o StrictHostKeyChecking=accept-new "$MANAGER_SSH_TARGET")

remote() { "${SSH[@]}" "$@"; }

wait_service() {
  local service="$1" deadline=$((SECONDS + ${SWARM_ROLLOUT_TIMEOUT_SECONDS:-300}))
  while (( SECONDS < deadline )); do
    if remote "sudo docker service ps --filter desired-state=running --format '{{.CurrentState}}' '$service'" |
      grep -Eq '^(Running|Ready)'; then
      return 0
    fi
    sleep 3
  done
  remote "sudo docker service ps --no-trunc '$service'"
  return 1
}

update_worker() {
  local role="$1" service="xiaohuiji-$1"
  echo "[$role] 开始部署"
  remote "sudo docker service update --with-registry-auth --image '$IMAGE' --env-add APP_REVISION='$TAG' --update-order stop-first --update-parallelism 1 '$service' >/dev/null"
  wait_service "$service"
  echo "[$role] 已就绪"
}

update_web_color() {
  local color="$1"
  local service="xiaohuiji-web-$color"
  echo "[web-$color] 开始部署"
  remote "sudo docker service update --with-registry-auth --image '$IMAGE' --env-add APP_REVISION='$TAG' --update-order stop-first --update-parallelism 1 '$service' >/dev/null"
  wait_service "$service"
  curl -fsS --max-time 8 "$WEB_NODE_URL/readyz" >/dev/null
  echo "[web-$color] readyz 通过"
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
for role in "${ROLES[@]}"; do [[ "$role" == web ]] || non_web+=("$role"); done
pids=()
for role in "${non_web[@]}"; do update_worker "$role" & pids+=("$!"); done
failed=0
for pid in "${pids[@]}"; do wait "$pid" || failed=1; done
(( failed == 0 )) || { echo "非 Web 角色部署失败" >&2; exit 1; }

if printf '%s\n' "${ROLES[@]}" | grep -qx web; then
  # 先更新非在线颜色，再切到它，更新另一色，最后切回，避免同一宿主端口被 start-first 占用。
  update_web_color blue
  switch_router blue
  update_web_color green
  switch_router green
fi

echo "Swarm 部署成功，镜像 ${IMAGE_REPOSITORY}:${TAG}，总耗时 $((SECONDS - STARTED)) 秒"
