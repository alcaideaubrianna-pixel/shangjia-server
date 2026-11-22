#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'HELP'
腾讯云 TKE 红蓝部署工具

用法：
  deploy/tencent-deploy-all.sh <image-tag> [角色 ...]
  deploy/tencent-deploy-all.sh --env-file <env.json> [角色 ...]
  TENCENT_DEPLOY_TARGET=swarm deploy/tencent-deploy-all.sh <image-tag> [角色 ...]
  deploy/tencent-deploy-all.sh -h|--help

默认角色：web collector background media publish scheduler account
指定角色或使用默认角色时，所选角色并发发布，独占保护按角色执行。
镜像模式校验 TCR 标签与摘要，同步发布 ENV、启动参数和副本数（web 2，其余 1）。
ENV 模式合并 JSON 字符串键值对象，保留镜像、启动参数和副本数。
Web 候选先通过逐 Pod 探测，再切换 Service。后台角色先停旧再启新。
account 和 scheduler 单实例交接。失败时恢复该角色的旧版本。

环境变量（可选）：
  TENCENT_REGION_ID         默认 ap-singapore
  TENCENT_CLUSTER_ID        默认 cls-7wx2ohw2
  TENCENT_NAMESPACE         默认 xiaohuiji
  TENCENT_CLI_PROFILE       tccli 配置名
  TENCENT_KUBECONFIG        现有 kubeconfig；省略时由 tccli 获取临时文件
  TENCENT_KUBE_CONTEXT      kubectl context；默认使用上述文件的当前 context
  TENCENT_KUBE_EXTRANET    获取公网 kubeconfig，默认 true
  TENCENT_DEPLOY_PROXY     本地 HTTP 代理，默认 http://127.0.0.1:6152
  TENCENT_ROLLOUT_TIMEOUT  每角色就绪等待上限，默认 5m（例如 90s、2m30s）
  TENCENT_WEB_PARALLEL_VERIFIED=true  已验证 Web 可双套并行时启用红蓝；默认使用零额外副本的滚动发布
  TENCENT_WEB_READY_URL   切流后的公网健康探测，默认 https://api.xiaohuiji.cc/readyz
  TENCENT_WEB_SMOKE_SCRIPT  可执行的业务探测脚本，接收 preview <pod> 或 live <url>
  TENCENT_WEB_OBSERVE_SECONDS  切流后的持续探测时间，默认 30 秒
  TENCENT_IMAGE_URL        完整外部镜像地址，跳过 TCR 校验
  TENCENT_IMAGE_REPOSITORY TCR 个人版镜像仓库，默认项目的新加坡仓库
  TENCENT_MEDIA_FALLBACK_CDN_URL                 媒体回源 CDN
  TENCENT_PUBLISH_QUEUE_CONCURRENCY              推送并发，默认 3
  TENCENT_PUBLISH_BULK_CONCURRENCY               批量推送并发，默认 4
  TENCENT_PUBLISH_COLLECT_PROCESS_OUTBOX_MODE    采集处理队列模式，默认 redis_first
  TENCENT_TELEGRAM_COLLECTOR_INGEST_SPOOL_MODE  接收 Spool 模式，默认 redis_first
  TENCENT_HISTORY_PENDING_EVENT_LIMIT            历史待处理事件上限，默认 240
  TENCENT_HISTORY_CONCURRENCY                    历史并发，默认 4
  TENCENT_MEDIA_ACCOUNT_CONCURRENCY              媒体账户并发，默认 8
  TENCENT_MEDIA_SOURCE_CONCURRENCY               媒体来源并发，默认 4
  TENCENT_FACEPP_CONCURRENCY                     Face++ 并发，默认 1
  TENCENT_ALIYUN_MATTING_CONCURRENCY              阿里云抠图并发，默认 5
  TENCENT_TENCENT_MATTING_CONCURRENCY             腾讯云抠图并发，默认 4
  TENCENT_FAPIHUB_CONCURRENCY                    FAPIHUB 并发，默认 2
  TENCENT_FAPIHUB_RATE_LIMIT_PER_MINUTE           FAPIHUB 每分钟限额，默认 150
  TENCENT_DB_HOST_OVERRIDE                      覆盖数据库地址
  TENCENT_REDIS_ADDRESS_OVERRIDE                覆盖 Redis 地址
  TENCENT_ANTISCAN_FALLBACK_PROVIDER              防扫图备用服务商
  TENCENT_PUBLISH_MEDIA_PERMANENT_FAILURE_ENABLED 永久媒体失败处理，默认 true
HELP
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

DEPLOY_TARGET="${TENCENT_DEPLOY_TARGET:-swarm}"
if [[ "$DEPLOY_TARGET" == "swarm" ]]; then
  exec "$(dirname "$0")/swarm-deploy-local.sh" "$@"
fi

STARTED=$SECONDS
TAG="${1:-}"
ENV_JSON=""
if [[ "$TAG" == "--env-file" ]]; then
  if [[ $# -lt 2 || ! -r "$2" ]]; then
    echo "请提供可读的 ENV JSON 文件" >&2
    exit 2
  fi
  command -v jq >/dev/null || { echo "需要 jq" >&2; exit 1; }
  ENV_JSON=$(jq -ce -s '
    if length == 1 and (.[0] | type == "object" and all(to_entries[]; (.key | length) > 0 and (.value | type == "string")))
    then .[0] else error("ENV JSON 必须是字符串键值对象") end
  ' "$2")
  if ! jq -e 'all(to_entries[]; (.key != "YOUBAN_AUTO_INIT_DATABASE" and .key != "GF_AUTO_INIT_DATABASE") or .value == "false")' <<<"$ENV_JSON" >/dev/null; then
    echo "红蓝部署不允许 Pod 自动初始化数据库" >&2
    exit 2
  fi
  shift
elif [[ ! "$TAG" =~ ^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$ ]]; then
  usage >&2
  exit 2
fi
shift

command_args_for_role() {
  case "$1" in
    web) printf '%s' '["web"]' ;;
    background) printf '%s' '["background-worker"]' ;;
    collector) printf '%s' '["collector-worker"]' ;;
    media) printf '%s' '["media-worker"]' ;;
    publish) printf '%s' '["publish-worker"]' ;;
    scheduler) printf '%s' '["scheduler"]' ;;
    account) printf '%s' '["account"]' ;;
    *) return 1 ;;
  esac
}

ROLES=(web collector background media publish scheduler account)
if (( $# > 0 )); then
  ROLES=("$@")
fi
seen=" "
for role in "${ROLES[@]}"; do
  command_args_for_role "$role" >/dev/null || { echo "未知角色：$role" >&2; exit 2; }
  [[ "$seen" != *" $role "* ]] || { echo "角色重复：$role" >&2; exit 2; }
  seen+="$role "
done

for tool in tccli kubectl jq curl dig; do
  command -v "$tool" >/dev/null || { echo "需要 $tool" >&2; exit 1; }
done

REGION_ID="${TENCENT_REGION_ID:-ap-singapore}"
CLUSTER_ID="${TENCENT_CLUSTER_ID:-cls-7wx2ohw2}"
NAMESPACE="${TENCENT_NAMESPACE:-xiaohuiji}"
IMAGE_REPOSITORY="${TENCENT_IMAGE_REPOSITORY:-sgccr.ccs.tencentyun.com/xiaohuiji-cc/xiaohuiji-package}"
ROLLOUT_TIMEOUT="${TENCENT_ROLLOUT_TIMEOUT:-5m}"
DEPLOY_PROXY="${TENCENT_DEPLOY_PROXY:-http://127.0.0.1:6152}"
ROLLOUT_SECONDS=0
duration="$ROLLOUT_TIMEOUT"
while [[ "$duration" =~ ^([0-9]+)([hms])(.*)$ ]]; do
  amount="${BASH_REMATCH[1]}"
  unit="${BASH_REMATCH[2]}"
  duration="${BASH_REMATCH[3]}"
  case "$unit" in h) factor=3600 ;; m) factor=60 ;; s) factor=1 ;; esac
  ROLLOUT_SECONDS=$((ROLLOUT_SECONDS + 10#$amount * factor))
done
if [[ -n "$duration" ]] || (( ROLLOUT_SECONDS <= 0 )); then
  echo "TENCENT_ROLLOUT_TIMEOUT 必须为正数时长，例如 90s 或 2m30s" >&2
  exit 2
fi
MEDIA_FALLBACK_CDN_URL="${TENCENT_MEDIA_FALLBACK_CDN_URL:-https://awdaffwaffawf-1480411854.cos.ap-hongkong.myqcloud.com}"
PUBLISH_QUEUE_CONCURRENCY="${TENCENT_PUBLISH_QUEUE_CONCURRENCY:-3}"
PUBLISH_BULK_CONCURRENCY="${TENCENT_PUBLISH_BULK_CONCURRENCY:-4}"
COLLECT_PROCESS_OUTBOX_MODE="${TENCENT_PUBLISH_COLLECT_PROCESS_OUTBOX_MODE:-redis_first}"
COLLECTOR_INGEST_SPOOL_MODE="${TENCENT_TELEGRAM_COLLECTOR_INGEST_SPOOL_MODE:-redis_first}"
HISTORY_PENDING_EVENT_LIMIT="${TENCENT_HISTORY_PENDING_EVENT_LIMIT:-240}"
HISTORY_CONCURRENCY="${TENCENT_HISTORY_CONCURRENCY:-4}"
FACEPP_CONCURRENCY="${TENCENT_FACEPP_CONCURRENCY:-1}"
ALIYUN_MATTING_CONCURRENCY="${TENCENT_ALIYUN_MATTING_CONCURRENCY:-5}"
TENCENT_MATTING_CONCURRENCY="${TENCENT_TENCENT_MATTING_CONCURRENCY:-4}"
FAPIHUB_CONCURRENCY="${TENCENT_FAPIHUB_CONCURRENCY:-2}"
FAPIHUB_RATE_LIMIT_PER_MINUTE="${TENCENT_FAPIHUB_RATE_LIMIT_PER_MINUTE:-150}"
MEDIA_ACCOUNT_CONCURRENCY="${TENCENT_MEDIA_ACCOUNT_CONCURRENCY:-8}"
MEDIA_SOURCE_CONCURRENCY="${TENCENT_MEDIA_SOURCE_CONCURRENCY:-4}"
DB_HOST_OVERRIDE="${TENCENT_DB_HOST_OVERRIDE:-}"
REDIS_ADDRESS_OVERRIDE="${TENCENT_REDIS_ADDRESS_OVERRIDE:-}"
ANTISCAN_FALLBACK_PROVIDER="${TENCENT_ANTISCAN_FALLBACK_PROVIDER:-}"
MEDIA_PERMANENT_FAILURE_ENABLED="${TENCENT_PUBLISH_MEDIA_PERMANENT_FAILURE_ENABLED:-true}"

tencent_cli() {
  local service="$1" action="$2"
  shift 2
  local options=(--region "$REGION_ID")
  if [[ -n "${TENCENT_CLI_PROFILE:-}" ]]; then
    options+=(--profile "$TENCENT_CLI_PROFILE")
  fi
  NO_PROXY= no_proxy= tccli "$service" "$action" "${options[@]}" --https-proxy "$DEPLOY_PROXY" "$@"
}

IMAGE_URL="${TENCENT_IMAGE_URL:-}"
IMAGE_DIGEST=""
if [[ -z "$ENV_JSON" && -z "$IMAGE_URL" ]]; then
  IMAGE_URL="$IMAGE_REPOSITORY:$TAG"
  IMAGE_DIGEST=$(tencent_cli tcr DescribeImagePersonal --RepoName "${IMAGE_REPOSITORY#*/}" --Tag "$TAG" |
    jq -er --arg tag "$TAG" '
      [(.Response // .).Data.TagInfo[]? | select(.TagName == $tag) | .TagId
       | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))]
      | if length == 1 then .[0] else error("未找到唯一的 TCR 标签及摘要") end')
fi

KUBE_TMP=$(mktemp -d "${TMPDIR:-/tmp}/youban-tke.XXXXXXXX")
trap 'rm -f -- "$KUBE_TMP/config" "$KUBE_TMP/ca" "$KUBE_TMP/cert" "$KUBE_TMP/key"; rmdir "$KUBE_TMP"' EXIT
KUBE_CONFIG="${TENCENT_KUBECONFIG:-}"
if [[ -z "$KUBE_CONFIG" ]]; then
  KUBE_CONFIG="$KUBE_TMP/config"
  tencent_cli tke DescribeClusterKubeconfig --ClusterId "$CLUSTER_ID" --IsExtranet "${TENCENT_KUBE_EXTRANET:-true}" |
    jq -er '(.Response // .).Kubeconfig | select(type == "string" and length > 0)' >"$KUBE_CONFIG"
fi
KUBE=(kubectl --kubeconfig "$KUBE_CONFIG")
if [[ -n "${TENCENT_KUBE_CONTEXT:-}" ]]; then
  KUBE+=(--context "$TENCENT_KUBE_CONTEXT")
fi
KUBE_VIEW=$("${KUBE[@]}" config view --raw --flatten --minify -o json)
KUBE_SERVER=$(jq -er '.clusters[0].cluster.server' <<<"$KUBE_VIEW")
if [[ ! "$KUBE_SERVER" =~ ^https://([a-zA-Z0-9.-]+)(:([0-9]+))?/?$ ]]; then
  echo "kubeconfig 需要 HTTPS 集群地址" >&2
  exit 1
fi
KUBE_HOST="${BASH_REMATCH[1]}"
KUBE_PORT="${BASH_REMATCH[3]:-443}"
KUBE_IP="$KUBE_HOST"
if [[ ! "$KUBE_HOST" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  KUBE_IP=$(dig +short +time=3 +tries=1 "$KUBE_HOST" A |
    awk '/^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/ && !found {print; found=1}')
fi
if [[ -z "$KUBE_IP" ]]; then
  echo "无法解析 TKE API 地址 $KUBE_HOST" >&2
  exit 1
fi

# Decode credentials inside the private temporary directory.
jq -er '.clusters[0].cluster["certificate-authority-data"] | select(length > 0) | @base64d' <<<"$KUBE_VIEW" >"$KUBE_TMP/ca"
jq -er '.users[0].user["client-certificate-data"] | select(length > 0) | @base64d' <<<"$KUBE_VIEW" >"$KUBE_TMP/cert"
jq -er '.users[0].user["client-key-data"] | select(length > 0) | @base64d' <<<"$KUBE_VIEW" >"$KUBE_TMP/key"
unset KUBE_VIEW

kube_request() {
  local path="$1" timeout="$2"
  shift 2
  curl --noproxy '' --proxy "$DEPLOY_PROXY" \
    --connect-to "$KUBE_HOST:$KUBE_PORT:$KUBE_IP:$KUBE_PORT" \
    --cacert "$KUBE_TMP/ca" --cert "$KUBE_TMP/cert" --key "$KUBE_TMP/key" \
    --connect-timeout 8 --max-time "$timeout" -fsS "$@" \
    "${KUBE_SERVER%/}$path"
}

wait_for_rollout() {
  local role="$1" nonce="$2" deadline=$((SECONDS + ROLLOUT_SECONDS))
  local current state remaining
  while (( (remaining = deadline - SECONDS) > 0 )); do
    if (( remaining > 15 )); then remaining=15; fi
    if current=$(kube_request "$DEPLOYMENTS/$role" "$remaining"); then
      state=$(jq -r --arg nonce "$nonce" '
        if $nonce != "" and .spec.template.metadata.annotations["youban.dev/deploy-id"] != $nonce then "replaced"
        elif .metadata.generation > (.status.observedGeneration // 0) then "pending"
        elif any(.status.conditions[]?; .type == "Progressing" and .reason == "ProgressDeadlineExceeded") then "failed"
        elif ((.status.updatedReplicas // 0) == (.spec.replicas // 1))
         and ((.status.replicas // 0) == (.status.updatedReplicas // 0))
         and ((.status.availableReplicas // 0) == (.spec.replicas // 1)) then "ready"
        else "pending" end
      ' <<<"$current")
      case "$state" in
        ready) return 0 ;;
        replaced) echo "[$role] 发布已被另一项更新取代" >&2; return 1 ;;
        failed) echo "[$role] Deployment 超过进度期限" >&2; return 1 ;;
      esac
    fi
    if (( SECONDS < deadline )); then sleep 1; fi
  done
  echo "[$role] 等待发布就绪超时（${ROLLOUT_TIMEOUT}）" >&2
  return 1
}

image_env() {
  jq -c --arg fallback "$MEDIA_FALLBACK_CDN_URL" --arg permanent "$MEDIA_PERMANENT_FAILURE_ENABLED" --arg queue "$PUBLISH_QUEUE_CONCURRENCY" --arg bulk "$PUBLISH_BULK_CONCURRENCY" --arg image "$IMAGE_URL" --arg digest "$IMAGE_DIGEST" '
    def upsert($name;$value): if any(.[]; .name == $name) then map(if .name == $name then {"name":$name,"value":$value} else . end) else . + [{"name":$name,"value":$value}] end;
    upsert("YOUBAN_PUBLISH_MEDIA_CACHE_FALLBACK_CDN_BASE_URL";$fallback)
    | upsert("YOUBAN_PUBLISH_MEDIA_PERMANENT_FAILURE_ENABLED";$permanent)
    | upsert("APP_IMAGE_REFERENCE";$image)
    | upsert("APP_IMAGE_DIGEST";$digest)
    | if $DB_HOST_OVERRIDE != "" then upsert("YOUBAN_DB_HOST";$DB_HOST_OVERRIDE) else . end
    | if $REDIS_ADDRESS_OVERRIDE != "" then upsert("YOUBAN_REDIS_ADDRESS";$REDIS_ADDRESS_OVERRIDE) else . end
    | if ($ENV_ROLE == "publish" or $ENV_ROLE == "media") then upsert("YOUBAN_PUBLISH_MEDIA_ACCOUNT_CONCURRENCY";$MEDIA_ACCOUNT_CONCURRENCY) | upsert("YOUBAN_PUBLISH_MEDIA_SOURCE_CONCURRENCY";$MEDIA_SOURCE_CONCURRENCY) else . end
    | if ($ENV_ROLE == "publish" or $ENV_ROLE == "background") then upsert("YOUBAN_PUBLISH_COLLECT_PROCESS_OUTBOX_MODE";$collect_outbox_mode) else . end
    | if $ENV_ROLE == "collector" then upsert("YOUBAN_TELEGRAM_COLLECTOR_INGEST_SPOOL_MODE";$collector_spool_mode) else . end
    | if ($ENV_ROLE == "publish" or $ENV_ROLE == "account") then upsert("YOUBAN_PUBLISH_HISTORY_PENDING_EVENT_LIMIT";$history_limit) | upsert("YOUBAN_PUBLISH_HISTORY_CONCURRENCY";$history_concurrency) else . end
    | if $ENV_ROLE == "publish" then upsert("YOUBAN_PUBLISH_QUEUE_CONCURRENCY";$queue) | upsert("YOUBAN_PUBLISH_BULK_CONCURRENCY";$bulk) | upsert("YOUBAN_FACEPP_CONCURRENCY";$FACEPP_CONCURRENCY) | upsert("YOUBAN_ALIYUN_MATTING_CONCURRENCY";$ALIYUN_MATTING_CONCURRENCY) | upsert("YOUBAN_TENCENT_MATTING_CONCURRENCY";$TENCENT_MATTING_CONCURRENCY) | upsert("YOUBAN_FAPIHUB_CONCURRENCY";$FAPIHUB_CONCURRENCY) | upsert("YOUBAN_FAPIHUB_RATE_LIMIT_PER_MINUTE";$FAPIHUB_RATE_LIMIT_PER_MINUTE) | upsert("YOUBAN_ANTISCAN_FALLBACK_PROVIDER";$ANTISCAN_FALLBACK_PROVIDER) else . end
  ' \
    --arg ENV_ROLE "$1" \
    --arg DB_HOST_OVERRIDE "$DB_HOST_OVERRIDE" \
    --arg REDIS_ADDRESS_OVERRIDE "$REDIS_ADDRESS_OVERRIDE" \
    --arg MEDIA_ACCOUNT_CONCURRENCY "$MEDIA_ACCOUNT_CONCURRENCY" \
    --arg MEDIA_SOURCE_CONCURRENCY "$MEDIA_SOURCE_CONCURRENCY" \
    --arg FACEPP_CONCURRENCY "$FACEPP_CONCURRENCY" \
    --arg ALIYUN_MATTING_CONCURRENCY "$ALIYUN_MATTING_CONCURRENCY" \
    --arg TENCENT_MATTING_CONCURRENCY "$TENCENT_MATTING_CONCURRENCY" \
    --arg FAPIHUB_CONCURRENCY "$FAPIHUB_CONCURRENCY" \
    --arg FAPIHUB_RATE_LIMIT_PER_MINUTE "$FAPIHUB_RATE_LIMIT_PER_MINUTE" \
    --arg ANTISCAN_FALLBACK_PROVIDER "$ANTISCAN_FALLBACK_PROVIDER" \
    --arg history_limit "$HISTORY_PENDING_EVENT_LIMIT" \
    --arg history_concurrency "$HISTORY_CONCURRENCY" \
    --arg collect_outbox_mode "$COLLECT_PROCESS_OUTBOX_MODE" \
    --arg collector_spool_mode "$COLLECTOR_INGEST_SPOOL_MODE"
}

CORE="/api/v1/namespaces/$NAMESPACE"
DEPLOYMENTS="/apis/apps/v1/namespaces/$NAMESPACE/deployments"
WEB_SERVICE="$CORE/services/web"
PUBLIC_READY_URL="${TENCENT_WEB_READY_URL:-https://api.xiaohuiji.cc/readyz}"
OBSERVE_SECONDS="${TENCENT_WEB_OBSERVE_SECONDS:-30}"
SMOKE_SCRIPT="${TENCENT_WEB_SMOKE_SCRIPT:-}"
LOCK_NAME=youban-production-release
WEB_PARALLEL_VERIFIED="${TENCENT_WEB_PARALLEL_VERIFIED:-false}"
ROLLING_WEB_STRATEGY='{"type":"RollingUpdate","rollingUpdate":{"maxSurge":0,"maxUnavailable":1}}'

if [[ "$seen" == *" web "* ]]; then
  [[ "$OBSERVE_SECONDS" =~ ^[1-9][0-9]{0,3}$ && "$PUBLIC_READY_URL" == https://* ]] || {
    echo "需要正数观察秒数和 HTTPS 入口探针" >&2
    exit 2
  }
  [[ -z "$SMOKE_SCRIPT" || -f "$SMOKE_SCRIPT" && -x "$SMOKE_SCRIPT" ]] || {
    echo "Web 业务探测脚本必须可执行" >&2
    exit 2
  }
fi

json_request() {
  local path="$1" method="$2" format="${3:-json}"
  kube_request "$path" 15 -X "$method" -H "Content-Type: application/$format" --data-binary @-
}

get_deployment() {
  kube_request "$DEPLOYMENTS/$1" 15
}

pod_list() {
  local selector="$1" encoded
  encoded=$(jq -er '
    if ((.matchExpressions // []) | length) != 0 or ((.matchLabels // {}) | length) == 0
    then error("需要显式的 matchLabels Deployment selector")
    else .matchLabels | to_entries | map(.key + "=" + .value) | join(",") | @uri end
  ' <<<"$selector") || return 1
  kube_request "$CORE/pods?labelSelector=$encoded" 15
}

wait_pods_gone() {
  local selector="$1" deadline=$((SECONDS + ROLLOUT_SECONDS)) pods
  while (( SECONDS < deadline )); do
    if pods=$(pod_list "$selector") && jq -e '.items | length == 0' <<<"$pods" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo "[$role] 等待旧 Pod 完全退出超时" >&2
  return 1
}

scale_deployment() {
  local name="$1" expected="$2" desired="$3" uid="$4" nonce="${5:-}" current
  local attempt
  for attempt in 1 2 3; do
    current=$(get_deployment "$name") || return 1
    jq -e --arg uid "$uid" --arg nonce "$nonce" '
      .metadata.uid == $uid
      and ($nonce == "" or .spec.template.metadata.annotations["youban.dev/deploy-id"] == $nonce)
    ' <<<"$current" >/dev/null || return 1
    if jq -e --argjson desired "$desired" '.spec.replicas == $desired' <<<"$current" >/dev/null; then
      return 0
    fi
    jq -e --argjson expected "$expected" '.spec.replicas == $expected' <<<"$current" >/dev/null || return 1
    if jq -c --argjson replicas "$desired" '
      [{op:"test", path:"/metadata/resourceVersion", value:.metadata.resourceVersion},
       {op:"replace", path:"/spec/replicas", value:$replicas}]
    ' <<<"$current" | json_request "$DEPLOYMENTS/$name" PATCH json-patch+json >/dev/null; then
      return 0
    fi
    sleep 1
  done
  return 1
}

switch_web() {
  local previous="$1" next="$2"
  jq -cn --arg uid "$service_uid" --argjson previous "$previous" --argjson next "$next" '
    [{op:"test",path:"/metadata/uid",value:$uid},
     {op:"test",path:"/spec/selector",value:$previous},
     {op:"replace",path:"/spec/selector",value:$next}]
  ' | json_request "$WEB_SERVICE" PATCH json-patch+json >/dev/null
}

ready_endpoints() {
  local slices
  slices=$(kube_request "/apis/discovery.k8s.io/v1/namespaces/$NAMESPACE/endpointslices?labelSelector=kubernetes.io%2Fservice-name%3Dweb" 15) ||
    return 1
  jq -c '[.items[].endpoints[]? | select(.conditions.ready == true) | .targetRef.uid] | unique' <<<"$slices"
}

probe_public() {
  local status
  status=$(curl --connect-timeout 4 --max-time 8 -fsS -o /dev/null -w '%{http_code}' "$PUBLIC_READY_URL") ||
    return 1
  [[ "$status" == 200 ]]
}

require_ready_node() {
  local node_name="$1" node
  if [[ -z "$node_name" ]]; then return 0; fi
  node=$(kube_request "/api/v1/nodes/$node_name" 15) || return 1
  jq -e 'any(.status.conditions[]?; .type == "Ready" and .status == "True")' <<<"$node" >/dev/null
}

prepare_role() {
  local role="$1" active_matches name color labels old_pods

  if [[ "$role" == web ]]; then
    web=$(kube_request "$WEB_SERVICE" 15)
    service_uid=$(jq -er '.metadata.uid' <<<"$web")
    old_service_selector=$(jq -ce '.spec.selector | select(type == "object" and length > 0)' <<<"$web")
    active_matches=$(jq -c --argjson selector "$old_service_selector" '
      [.items[] | select(.metadata.name == "web" or .metadata.name == "web-red" or .metadata.name == "web-blue")
       | select((.spec.replicas // 0) > 0)
       | .spec.template.metadata.labels as $labels
       | select($selector | to_entries | all(.[]; $labels[.key] == .value))]
    ' <<<"$DEPLOYMENT_LIST")
  else
    active_matches=$(jq -c --arg role "$role" '
      [.items[] | select(.metadata.name as $name | [$role, $role+"-red", $role+"-blue"] | index($name))
       | select((.spec.replicas // 0) > 0)]
    ' <<<"$DEPLOYMENT_LIST")
  fi
  if ! source=$(jq -ce 'if length == 1 then .[0] else empty end' <<<"$active_matches" 2>/dev/null); then
    if [[ "$DEPLOY_TARGET" == "tke" ]]; then
      echo "[$role] TKE 中没有唯一在线 Deployment。该角色若已迁移到 Swarm，请使用 TENCENT_DEPLOY_TARGET=swarm；否则先核对 TKE 副本和颜色" >&2
    else
      echo "[$role] 请先核对在线角色与副本" >&2
    fi
    return 1
  fi
  source_name=$(jq -er '.metadata.name' <<<"$source")
  source_uid=$(jq -er '.metadata.uid' <<<"$source")
  source_replicas=$(jq -er '.spec.replicas' <<<"$source")
  source_selector=$(jq -ce '.spec.selector' <<<"$source")
  if ! jq -e '
    .spec.paused != true and .metadata.generation <= (.status.observedGeneration // 0)
    and .spec.replicas == .status.availableReplicas
    and .spec.replicas == .status.updatedReplicas
    and .spec.replicas == .status.replicas
  ' <<<"$source" >/dev/null; then
    echo "[$role] 在线 Deployment 尚未稳定" >&2
    return 1
  fi
  if [[ "$role" == account || "$role" == scheduler ]]; then
    if (( source_replicas != 1 )); then
      echo "[$role] 需要恰好一个在线副本" >&2
      return 1
    fi
    old_pods=$(pod_list "$source_selector")
    jq -e '.items | length == 1' <<<"$old_pods" >/dev/null || {
      echo "[$role] 在线 Pod 数量异常，包含可能尚未退出的实例" >&2
      return 1
    }
    source_node=$(jq -er '.items[0].spec.nodeName | select(length > 0)' <<<"$old_pods")
    if ! require_ready_node "$source_node"; then
      echo "[$role] 无法确认旧 Pod 所在节点正常，停止交接" >&2
      return 1
    fi
  fi

  if ! jq -e --arg role "$role" '
    [ .items[] | .spec.scaleTargetRef | .name as $name
      | select(.kind == "Deployment" and ([$role, $role+"-red", $role+"-blue"] | index($name))) ] | length == 0
  ' <<<"$HPA_LIST" >/dev/null; then
    echo "[$role] 检测到 HPA，先确认发布期间的扩缩容归属" >&2
    return 1
  fi

  if [[ "$role" == web && "$WEB_PARALLEL_VERIFIED" != true ]]; then
    target_name="$source_name"
    target_selector="$source_selector"
    return 0
  fi

  color=blue
  [[ "$source_name" != "$role-blue" ]] || color=red
  target_name="$role-$color"
  target=$(jq -c --arg name "$target_name" '[.items[] | select(.metadata.name == $name)][0] // null' <<<"$DEPLOYMENT_LIST")
  target_selector=$(jq -cn --arg app "xiaohuiji-$role" --arg color "$color" '
    {"matchLabels":{"app.kubernetes.io/name":$app,"rollout-color":$color}}')
  labels=$(jq -c --argjson selector "$target_selector" '.spec.template.metadata.labels + $selector.matchLabels' <<<"$source")
  if ! jq -e --argjson labels "$labels" '
    (.matchExpressions // [] | length == 0)
    and (.matchLabels | to_entries | any(.[]; $labels[.key] != .value))
  ' <<<"$source_selector" >/dev/null; then
    echo "[$role] 候选标签可能匹配旧 Deployment selector" >&2
    return 1
  fi
  if [[ "$role" == web ]] && ! jq -e --argjson labels "$labels" '
    to_entries | any(.[]; $labels[.key] != .value)
  ' <<<"$old_service_selector" >/dev/null; then
    echo "[web] 候选标签会被线上 Service 选中" >&2
    return 1
  fi
  if ! jq -e --argjson selector "$target_selector" --arg role "$role" '
    . == null or (.spec.selector == $selector
      and ($role == "web" or .spec.replicas == 0))
  ' <<<"$target" >/dev/null; then
    echo "[$role] 候选 Deployment 已存在但标签或副本数不符合红蓝交接要求" >&2
    return 1
  fi
  # A scaled-down previous color can still have a terminating Pod.
  if [[ "$role" != web ]]; then
    for name in "$role" "$role-red" "$role-blue"; do
      [[ "$name" != "$source_name" ]] || continue
      local standby
      standby=$(jq -c --arg name "$name" '[.items[] | select(.metadata.name == $name)][0].spec.selector // empty' <<<"$DEPLOYMENT_LIST")
      if [[ -n "$standby" ]]; then wait_pods_gone "$standby" || return 1; fi
    done
    if [[ "$target" == null ]]; then wait_pods_gone "$target_selector" || return 1; fi
  fi
}

release_template() {
  local current_env merged_env image index args
  index=$(jq -er --arg role "$role" '.spec.template.spec.containers | map(.name) | index($role) // error("缺少角色容器")' <<<"$source") || return 1
  current_env=$(jq -c --argjson index "$index" '.spec.template.spec.containers[$index].env // []' <<<"$source") || return 1
  args=$(command_args_for_role "$role") || return 1
  image="$IMAGE_URL"
  if [[ -n "$IMAGE_DIGEST" ]]; then image="$IMAGE_REPOSITORY@$IMAGE_DIGEST"; fi
  if [[ -z "$ENV_JSON" ]]; then
    merged_env=$(image_env "$role" <<<"$current_env") || return 1
  else
    merged_env=$(jq -ce --slurpfile overrides <(printf '%s\n' "$ENV_JSON") '
      $overrides[0] as $values
      | map(.name) as $names
      | map(.name as $key | if $values | has($key) then {name:$key,value:$values[$key]} else . end)
        + ($values | to_entries | map(select(.key as $key | $names | index($key) | not) | {name:.key,value}))
    ' <<<"$current_env") || return 1
  fi

  jq -c --arg role "$role" --arg nonce "$nonce" --arg tag "$TAG" \
    --arg image "$image" --argjson args "$args" --argjson index "$index" \
    --slurpfile envs <(printf '%s\n' "$merged_env") '
    .spec.template
    | .metadata.annotations = ((.metadata.annotations // {}) + {"youban.dev/deploy-id":$nonce})
    | .spec.containers[$index] |= (
        .env=($envs[0] | map(select(.name != "YOUBAN_AUTO_INIT_DATABASE" and .name != "GF_AUTO_INIT_DATABASE"))
          + [{name:"YOUBAN_AUTO_INIT_DATABASE",value:"false"},{name:"GF_AUTO_INIT_DATABASE",value:"false"}])
        | if $tag == "--env-file" then . else .image=$image | if $role == "web" then . else .args=$args end end
        | if $role == "web" and ((.readinessProbe.httpGet.path // "") | startswith("/readyz?revision="))
          then .readinessProbe.httpGet.path="/readyz" else . end
      )
  ' <<<"$source"
}

stage_candidate() {
  local desired="$1" method=POST path="$DEPLOYMENTS" template staged
  if [[ "$target" != null ]]; then
    method=PUT
    path="$DEPLOYMENTS/$target_name"
  fi
  template=$(release_template) || return 1
  staged=$(jq -c --arg name "$target_name" --argjson selector "$target_selector" \
    --argjson replicas "$desired" --slurpfile template <(printf '%s\n' "$template") \
    --slurpfile target <(printf '%s\n' "$target") '
    {
      apiVersion:"apps/v1", kind:"Deployment",
      metadata: ({name:$name,namespace:.metadata.namespace,labels:((.metadata.labels // {}) + $selector.matchLabels)}
        + if $target[0] == null then {} else ($target[0].metadata | {resourceVersion}) end),
      spec:(.spec | .replicas=$replicas | .selector=$selector
        | .template = $template[0] | .template.metadata.labels += $selector.matchLabels)
    }
  ' <<<"$source") || return 1
  printf '%s\n' "$staged" | json_request "$path?dryRun=All" "$method" >/dev/null || return 1
  printf '%s\n' "$staged" | json_request "$path" "$method"
}

probe_candidate() {
  local pods pod address
  pods=$(pod_list "$target_selector") || return 1
  candidate_uids=$(jq -ce --argjson replicas "$replicas" '
    [.items[] | select(.metadata.deletionTimestamp == null)
     | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.uid]
    | if length == $replicas then sort else error("候选就绪 Pod 数不足") end
  ' <<<"$pods") || return 1
  while IFS= read -r pod; do
    address="$CORE/pods/$pod:8000/proxy/readyz"
    kube_request "$address" 10 >/dev/null || return 1
    if [[ -n "$SMOKE_SCRIPT" ]]; then "$SMOKE_SCRIPT" preview "$pod" || return 1; fi
  done < <(jq -r '.items[] | select(.metadata.deletionTimestamp == null) | .metadata.name' <<<"$pods")
}

wait_endpoints() {
  local expected="$1" deadline=$((SECONDS + ROLLOUT_SECONDS)) current
  while (( SECONDS < deadline )); do
    if current=$(ready_endpoints) && [[ "$current" == "$expected" ]]; then return 0; fi
    sleep 1
  done
  echo "[web] Service 就绪端点未与目标 Pod 一致" >&2
  return 1
}

check_web_baseline() {
  local baseline source_pods expected
  baseline=$(ready_endpoints) || return 1
  baseline_uids="$baseline"
  jq -e --argjson replicas "$source_replicas" 'length == $replicas and all(.[]; type == "string")' <<<"$baseline" >/dev/null ||
    return 1
  source_pods=$(pod_list "$source_selector") || return 1
  expected=$(jq -c '[.items[] | select(.metadata.deletionTimestamp == null) | .metadata.uid] | sort' <<<"$source_pods") ||
    return 1
  if [[ "$baseline" != "$expected" ]]; then
    echo "[web] 线上 Service 端点与当前部署副本不一致" >&2
    return 1
  fi
  probe_public
}

observe_web() {
  local current deadline=$((SECONDS + OBSERVE_SECONDS))
  while :; do
    current=$(ready_endpoints) || return 1
    [[ "$current" == "$candidate_uids" ]] || return 1
    probe_public || return 1
    if [[ -n "$SMOKE_SCRIPT" ]]; then "$SMOKE_SCRIPT" live "$PUBLIC_READY_URL" || return 1; fi
    if (( SECONDS >= deadline )); then break; fi
    sleep 2
  done
}

promote_web() {
  local candidate current
  check_web_baseline || return 1
  candidate=$(stage_candidate "$replicas") || return 1
  target_uid=$(jq -er '.metadata.uid' <<<"$candidate") || return 1
  wait_for_rollout "$target_name" "$nonce" || return 1
  probe_candidate || return 1
  # Check that the old endpoints stayed healthy during candidate startup.
  current=$(ready_endpoints) || return 1
  [[ "$current" == "$baseline_uids" ]] || return 1
  next_service_selector=$(jq -c '.matchLabels' <<<"$target_selector") || return 1
  web_switch_attempted=true
  switch_web "$old_service_selector" "$next_service_selector" || return 1
  wait_endpoints "$candidate_uids" || return 1
  observe_web || return 1
  echo "[web] 已切换 $source_name → ${target_name}，保留旧版本及其就绪副本"
  echo "[web] 回滚 selector：${old_service_selector}；Service UID：$service_uid"
  # After the first two color releases, retire any remaining legacy warm replica set.
  if [[ "$source_name" != web && "$target_name" != web ]]; then
    local legacy legacy_uid legacy_replicas
    legacy=$(jq -c '[.items[] | select(.metadata.name == "web")][0] // null' <<<"$DEPLOYMENT_LIST")
    legacy_replicas=$(jq -r '.spec.replicas // 0' <<<"$legacy")
    if (( legacy_replicas > 0 )); then
      legacy_uid=$(jq -er '.metadata.uid' <<<"$legacy")
      if ! scale_deployment web "$legacy_replicas" 0 "$legacy_uid"; then
        echo "[web] 历史 Deployment/web 缩容失败，请核对实际流量及副本" >&2
      fi
    fi
  fi
}

patch_web_rollout() {
  local current="$1" strategy="$2" template="${3:-}" dry_run="${4:-}"
  jq -c --arg uid "$source_uid" --argjson strategy "$strategy" \
    --slurpfile template <(printf '%s\n' "$template") '
    [{op:"test",path:"/metadata/uid",value:$uid},
     {op:"test",path:"/metadata/resourceVersion",value:.metadata.resourceVersion},
     {op:"add",path:"/spec/strategy",value:$strategy}]
    + (if ($template | length) == 0 then []
       else [{op:"replace",path:"/spec/template",value:$template[0]}] end)
  ' <<<"$current" | json_request "$DEPLOYMENTS/$source_name$dry_run" PATCH json-patch+json >/dev/null
}

roll_web() {
  if (( source_replicas < 2 )); then
    echo "[web] 零额外副本滚动发布至少需要 2 个在线副本" >&2
    return 1
  fi
  replicas=$source_replicas
  check_web_baseline || return 1
  local template previous_strategy current
  template=$(release_template) || return 1
  previous_strategy=$(jq -ce '.spec.strategy' <<<"$source") || return 1
  patch_web_rollout "$source" "$ROLLING_WEB_STRATEGY" "$template" '?dryRun=All' || return 1
  web_rollout_attempted=true
  echo "[web] 未验证双套并行，使用 maxSurge=0、maxUnavailable=1 滚动发布"
  patch_web_rollout "$source" "$ROLLING_WEB_STRATEGY" "$template" || return 1
  wait_for_rollout "$source_name" "$nonce" || return 1
  probe_candidate || return 1
  wait_endpoints "$candidate_uids" || return 1
  observe_web || return 1
  current=$(get_deployment "$source_name") || return 1
  jq -e --arg nonce "$nonce" '.spec.template.metadata.annotations["youban.dev/deploy-id"] == $nonce' \
    <<<"$current" >/dev/null || return 1
  patch_web_rollout "$current" "$previous_strategy"
}

recover_rolling_web() {
  local current previous_template previous_strategy
  current=$(get_deployment "$source_name") || return 1
  previous_template=$(jq -c '.spec.template' <<<"$source") || return 1
  previous_strategy=$(jq -ce '.spec.strategy' <<<"$source") || return 1
  if jq -e --slurpfile template <(printf '%s\n' "$previous_template") \
    --argjson strategy "$previous_strategy" \
    '.spec.template == $template[0] and .spec.strategy == $strategy' <<<"$current" >/dev/null; then
    return 0
  fi
  jq -e --arg nonce "$nonce" '.spec.template.metadata.annotations["youban.dev/deploy-id"] == $nonce' \
    <<<"$current" >/dev/null || return 1
  patch_web_rollout "$current" "$ROLLING_WEB_STRATEGY" "$previous_template" || return 1
  wait_for_rollout "$source_name" "" || return 1
  probe_candidate || return 1
  wait_endpoints "$candidate_uids" || return 1
  probe_public || return 1
  current=$(get_deployment "$source_name") || return 1
  jq -e --slurpfile template <(printf '%s\n' "$previous_template") '.spec.template == $template[0]' \
    <<<"$current" >/dev/null || return 1
  patch_web_rollout "$current" "$previous_strategy"
}

promote_worker() {
  local candidate
  candidate=$(stage_candidate 0) || return 1
  target_uid=$(jq -er '.metadata.uid' <<<"$candidate") || return 1
  wait_for_rollout "$target_name" "$nonce" || return 1
  source_scale_attempted=true
  scale_deployment "$source_name" "$source_replicas" 0 "$source_uid" || return 1
  wait_pods_gone "$source_selector" || return 1
  require_ready_node "$source_node" || return 1
  scale_deployment "$target_name" 0 "$replicas" "$target_uid" "$nonce" || return 1
  wait_for_rollout "$target_name" "$nonce" || return 1
  echo "[$role] 已交接 $source_name → $target_name"
}

recover_role() {
  local service current pods target_node=""
  if [[ "$role" == web ]]; then
    if [[ "$web_rollout_attempted" == true ]]; then
      recover_rolling_web
      return "$?"
    fi
    [[ "$web_switch_attempted" == true ]] || return 0
    service=$(kube_request "$WEB_SERVICE" 15) || return 1
    current=$(jq -c '.spec.selector' <<<"$service") || return 1
    if [[ "$current" == "$old_service_selector" ]]; then return 0; fi
    [[ "$current" == "$next_service_selector" ]] || return 1
    # The old color is intentionally kept running throughout promotion.
    switch_web "$next_service_selector" "$old_service_selector" || return 1
    wait_endpoints "$baseline_uids" || return 1
    probe_public || return 1
  elif [[ "$source_scale_attempted" == true ]]; then
    if [[ -n "$source_node" ]]; then
      pods=$(pod_list "$target_selector") || return 1
      target_node=$(jq -r '.items[0].spec.nodeName // ""' <<<"$pods") || return 1
      require_ready_node "$target_node" || return 1
    fi
    scale_deployment "$target_name" "$replicas" 0 "$target_uid" "$nonce" || return 1
    wait_pods_gone "$target_selector" || return 1
    require_ready_node "$source_node" && require_ready_node "$target_node" || return 1
    # Do not restart any old worker if its predecessor process has not exited.
    current=$(get_deployment "$source_name") || return 1
    if jq -e '.spec.replicas == 0' <<<"$current" >/dev/null; then
      wait_pods_gone "$source_selector" || return 1
    fi
    scale_deployment "$source_name" 0 "$source_replicas" "$source_uid" || return 1
    wait_for_rollout "$source_name" "" || return 1
  fi
}

deploy_role() {
  local role="$1" role_started=$SECONDS
  local source source_name source_uid source_replicas source_selector target target_name target_selector target_uid
  local web service_uid old_service_selector next_service_selector baseline_uids candidate_uids
  local web_switch_attempted=false web_rollout_attempted=false source_scale_attempted=false source_node="" replicas nonce
  trap 'trap - EXIT; exit 143' TERM
  trap '
    rc=$?
    if (( rc != 0 )); then
      if recover_role; then echo "[$role] 旧版未切换或已恢复" >&2
      else
        echo "[$role] 自动恢复失败，请立即按发布手册检查 $source_name 和 $target_name" >&2
        rc=3
      fi
    fi
    printf "[%s] 部署%s，耗时 %s 秒\n" "$role" "$([[ $rc == 0 ]] && echo 成功 || echo 失败)" "$((SECONDS - role_started))"
    exit "$rc"
  ' EXIT
  echo "[$role] 开始红蓝部署"
  if ! prepare_role "$role"; then
    return 1
  fi
  replicas="$source_replicas"
  if [[ -z "$ENV_JSON" ]]; then
    replicas=1
    [[ "$role" != web ]] || replicas=2
  fi
  nonce="$(date -u +%FT%TZ)-$$-$RANDOM"
  if [[ "$role" == web ]]; then
    if [[ "$WEB_PARALLEL_VERIFIED" == true ]]; then
      promote_web || exit $?
    else
      roll_web || exit $?
    fi
  else
    promote_worker || exit $?
  fi
  exit 0
}

LOCK_UID=""
safely_finished=false
cleanup_release() {
  local rc=$? cleanup_failed=false
  if [[ -n "$LOCK_UID" && "$safely_finished" == true ]]; then
    if ! jq -cn --arg uid "$LOCK_UID" '{apiVersion:"v1",kind:"DeleteOptions",preconditions:{uid:$uid}}' |
      json_request "$CORE/configmaps/$LOCK_NAME" DELETE >/dev/null; then
      echo "发布锁清理失败，请核对 $LOCK_NAME" >&2
      cleanup_failed=true
    fi
  elif [[ -n "$LOCK_UID" ]]; then
    echo "发布异常中断，已保留集群发布锁 $LOCK_NAME 供复查" >&2
  fi
  rm -f -- "$KUBE_TMP/config" "$KUBE_TMP/ca" "$KUBE_TMP/cert" "$KUBE_TMP/key"
  rmdir "$KUBE_TMP"
  if [[ "$cleanup_failed" == true && "$rc" == 0 ]]; then exit 1; fi
  return "$rc"
}
trap cleanup_release EXIT
finish_interrupted() {
  trap '' INT TERM
  local pid
  while IFS= read -r pid; do
    kill -TERM "$pid" 2>/dev/null || true
  done < <(jobs -pr)
  echo "发布已中断，请检查已提交到 TKE 的变更和各角色副本" >&2
  exit "$1"
}
trap 'finish_interrupted 130' INT
trap 'finish_interrupted 143' TERM

release_owner="$(hostname)-$$-$(date -u +%FT%TZ)"
if ! LOCK_UID=$(jq -cn --arg name "$LOCK_NAME" --arg owner "$release_owner" '
  {apiVersion:"v1",kind:"ConfigMap",metadata:{name:$name},data:{owner:$owner}}
' | json_request "$CORE/configmaps" POST | jq -er '.metadata.uid'); then
  echo "无法获取发布锁；若已有发布，请先核对其状态" >&2
  if current_lock=$(kube_request "$CORE/configmaps/$LOCK_NAME" 8); then
    jq -r '"锁持有者：\(.data.owner // "未知")，创建时间：\(.metadata.creationTimestamp // "未知")"' <<<"$current_lock" >&2
  fi
  exit 1
fi
DEPLOYMENT_LIST=$(kube_request "$DEPLOYMENTS" 15)
HPA_LIST=$(kube_request "/apis/autoscaling/v2/namespaces/$NAMESPACE/horizontalpodautoscalers" 15)

failed=0
recovery_failed=false
wait_role() {
  local rc
  if wait "$1"; then return 0; else rc=$?; fi
  failed=1
  if (( rc == 3 || rc > 128 )); then recovery_failed=true; fi
}
pids=()
for role in "${ROLES[@]}"; do
  deploy_role "$role" &
  pids+=("$!")
done
for pid in "${pids[@]}"; do
  wait_role "$pid"
done

if [[ "$recovery_failed" == false ]]; then safely_finished=true; fi
if (( failed )); then
  echo "并发部署结束，部分角色失败，总耗时 $((SECONDS - STARTED)) 秒" >&2
  exit 1
fi
echo "部署成功，总耗时 $((SECONDS - STARTED)) 秒"
