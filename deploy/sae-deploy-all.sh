#!/usr/bin/env bash
set -euo pipefail

# The deployment helper uses Bash functions and arrays.  Re-exec explicitly
# when invoked through `sh`/another shell so background role workers see the
# same function definitions instead of failing with "deploy_role: command not
# found".
if [[ -z "${BASH_VERSION:-}" ]]; then
  exec bash "$0" "$@"
fi

usage() {
  cat <<'HELP'
阿里云 SAE 部署工具

用法：
  deploy/sae-deploy-all.sh <image-tag> [角色 ...]
  deploy/sae-deploy-all.sh --env-file <env.json> [角色 ...]
  deploy/sae-deploy-all.sh --help
  deploy/sae-deploy-all.sh -h

命令：
  <image-tag>            检查 ACR 镜像标签和摘要，然后部署指定角色。
  --env-file <env.json>  合并 ENV，保留各角色当前镜像，并重新部署。
                        JSON 必须为字符串键值对象，例如：
                        {"YOUBAN_PUBLISH_HISTORY_CONCURRENCY":"4"}
  -h, --help            显示帮助并退出，不访问云服务。

角色：
  web         Web/API 与 Bot runtime 入口（2 个副本）
  collector   采集消息处理
  background  后台任务
  media       媒体下载与处理
  publish     Telegram 消息推送
  scheduler   定时调度
  account     Telegram 账户会话（严格单实例）

不指定角色时，按以下顺序部署全部角色：
  web collector background media publish scheduler account
指定角色时，仅按传入顺序部署这些角色。
镜像部署模式会同步脚本管理的 ENV 和副本数；除 web 外均为 1 个副本。
ENV-only 模式保留当前副本数，account 使用单实例切换策略。

示例：
  deploy/sae-deploy-all.sh ee36a708 web
  deploy/sae-deploy-all.sh ee36a708 collector publish
  deploy/sae-deploy-all.sh ee36a708
  deploy/sae-deploy-all.sh --env-file /path/to/env.json media publish

环境变量（可选）：
  SAE_REGION_ID                             地域，默认 ap-southeast-1
  SAE_ACR_INSTANCE_ID                       ACR 实例
  SAE_IMAGE_URL                             临时使用的完整镜像地址（例如 ghcr.io/...），跳过 ACR 校验
  SAE_IMAGE_REPOSITORY                      镜像仓库
  SAE_MEDIA_FALLBACK_CDN_URL                 媒体回源 CDN
  SAE_PUBLISH_QUEUE_CONCURRENCY              推送并发，默认 3
  SAE_PUBLISH_BULK_CONCURRENCY               批量推送并发，默认 4
  SAE_HISTORY_PENDING_EVENT_LIMIT            历史待处理事件上限，默认 240
  SAE_HISTORY_CONCURRENCY                    历史并发，默认 4
  SAE_MEDIA_ACCOUNT_CONCURRENCY              媒体账户并发，默认 8
  SAE_MEDIA_SOURCE_CONCURRENCY               媒体来源并发，默认 4
  SAE_FACEPP_CONCURRENCY                     Face++ 并发，默认 1
  SAE_ALIYUN_MATTING_CONCURRENCY              阿里云抠图并发，默认 5
  SAE_TENCENT_MATTING_CONCURRENCY             腾讯云抠图并发，默认 4
  SAE_FAPIHUB_CONCURRENCY                    FAPIHUB 并发，默认 2
  SAE_FAPIHUB_RATE_LIMIT_PER_MINUTE           FAPIHUB 每分钟限额，默认 150
  SAE_DB_HOST_OVERRIDE                      覆盖数据库地址
  SAE_REDIS_ADDRESS_OVERRIDE                覆盖 Redis 地址
  SAE_ASSOCIATE_EIP_OVERRIDE=true            account 部署时关联 EIP
  SAE_ANTISCAN_FALLBACK_PROVIDER              防扫图备用服务商
  SAE_PUBLISH_MEDIA_PERMANENT_FAILURE_ENABLED 永久媒体失败处理，默认 true
  SAE_DISABLE_SLS=true                      关闭 SLS
  SAE_ENABLE_SLS=true                       启用 SLS（与关闭选项互斥）
  SAE_ABORT_STUCK_CHANGE_ORDER=true          中止并回滚进行中的变更，再部署
  ALIBABA_CLOUD_PROFILE                     ENV-only 模式使用的 CLI profile

部署依赖：aliyun CLI、jq，以及已配置的云访问凭据。
HELP
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

if [[ "${1:-}" == "--env-file" ]]; then
  exec "$(dirname "$0")/sae-env-update.sh" "$@"
fi

# Deploy one immutable image tag to all SAE roles in a safe order.
# Usage: ./deploy/sae-deploy-all.sh e5b746c [web|background|collector|media|publish|scheduler|account ...]

REGION_ID="${SAE_REGION_ID:-ap-southeast-1}"
ACR_INSTANCE_ID="${SAE_ACR_INSTANCE_ID:-cri-wx0a7j1fo37ad8mo}"
IMAGE_REPOSITORY="${SAE_IMAGE_REPOSITORY:-xxxxxx-registry-vpc.ap-southeast-1.cr.aliyuncs.com/xiaohuiji/youban-server}"
EXTERNAL_IMAGE_URL="${SAE_IMAGE_URL:-}"
MEDIA_FALLBACK_CDN_URL="${SAE_MEDIA_FALLBACK_CDN_URL:-https://awdaffwaffawf-1480411854.cos.ap-hongkong.myqcloud.com}"
PUBLISH_QUEUE_CONCURRENCY="${SAE_PUBLISH_QUEUE_CONCURRENCY:-3}"
PUBLISH_BULK_CONCURRENCY="${SAE_PUBLISH_BULK_CONCURRENCY:-4}"
HISTORY_PENDING_EVENT_LIMIT="${SAE_HISTORY_PENDING_EVENT_LIMIT:-240}"
HISTORY_CONCURRENCY="${SAE_HISTORY_CONCURRENCY:-4}"
FACEPP_CONCURRENCY="${SAE_FACEPP_CONCURRENCY:-1}"
ALIYUN_MATTING_CONCURRENCY="${SAE_ALIYUN_MATTING_CONCURRENCY:-5}"
TENCENT_MATTING_CONCURRENCY="${SAE_TENCENT_MATTING_CONCURRENCY:-4}"
FAPIHUB_CONCURRENCY="${SAE_FAPIHUB_CONCURRENCY:-2}"
FAPIHUB_RATE_LIMIT_PER_MINUTE="${SAE_FAPIHUB_RATE_LIMIT_PER_MINUTE:-150}"
MEDIA_ACCOUNT_CONCURRENCY="${SAE_MEDIA_ACCOUNT_CONCURRENCY:-8}"
MEDIA_SOURCE_CONCURRENCY="${SAE_MEDIA_SOURCE_CONCURRENCY:-4}"
DB_HOST_OVERRIDE="${SAE_DB_HOST_OVERRIDE:-}"
REDIS_ADDRESS_OVERRIDE="${SAE_REDIS_ADDRESS_OVERRIDE:-}"
ASSOCIATE_EIP_OVERRIDE="${SAE_ASSOCIATE_EIP_OVERRIDE:-}"
ANTISCAN_FALLBACK_PROVIDER="${SAE_ANTISCAN_FALLBACK_PROVIDER:-}"
MEDIA_PERMANENT_FAILURE_ENABLED="${SAE_PUBLISH_MEDIA_PERMANENT_FAILURE_ENABLED:-true}"
DISABLE_SLS="${SAE_DISABLE_SLS:-false}"
ENABLE_SLS="${SAE_ENABLE_SLS:-false}"
ABORT_STUCK_CHANGE_ORDER="${SAE_ABORT_STUCK_CHANGE_ORDER:-false}"
TAG="${1:-}"

if [[ -z "$TAG" || ! "$TAG" =~ ^[A-Za-z0-9._-]+$ ]]; then
  usage >&2
  exit 2
fi
command -v aliyun >/dev/null || { echo "aliyun CLI is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
if [[ "$DISABLE_SLS" == "true" && "$ENABLE_SLS" == "true" ]]; then
  echo "SAE_DISABLE_SLS and SAE_ENABLE_SLS cannot both be true" >&2
  exit 2
fi

if [[ -n "$EXTERNAL_IMAGE_URL" ]]; then
  IMAGE_URL="$EXTERNAL_IMAGE_URL"
  IMAGE_DIGEST=""
  echo "using external image: $IMAGE_URL"
else
  IMAGE_URL="${IMAGE_REPOSITORY}:${TAG}"
  repo_path="${IMAGE_REPOSITORY#*/}"
  repo_id=$(aliyun cr ListRepository --RegionId "$REGION_ID" --InstanceId "$ACR_INSTANCE_ID" --RepoName "${repo_path##*/}" --RepoNamespaceName "${repo_path%/*}" --PageSize 100 | jq -r '.Repositories[0].RepoId // empty')
  [[ -n "$repo_id" ]] || { echo "ACR repository not found: $IMAGE_REPOSITORY" >&2; exit 1; }
  tag_exists=$(aliyun cr ListRepoTag --RegionId "$REGION_ID" --InstanceId "$ACR_INSTANCE_ID" --RepoId "$repo_id" --PageSize 100 | jq -r --arg tag "$TAG" '[.Images[]? | select(.Tag == $tag)] | length')
  [[ "$tag_exists" == "1" ]] || { echo "ACR image tag not found: $IMAGE_URL" >&2; exit 1; }
  IMAGE_DIGEST=$(aliyun cr GetRepoTag --RegionId "$REGION_ID" --InstanceId "$ACR_INSTANCE_ID" --RepoId "$repo_id" --Tag "$TAG" | jq -r '.Digest // empty')
  [[ -n "$IMAGE_DIGEST" ]] || { echo "ACR image digest not found: $IMAGE_URL" >&2; exit 1; }
fi

app_id_for_role() {
  case "$1" in
    web) printf '%s' 'b4565e3c-6d08-4297-bf8b-037884d54ee4' ;;
    background) printf '%s' 'd12e3d86-e368-48ae-bac1-bd41b3a08ef2' ;;
    collector) printf '%s' '0deb4862-2af5-4c0f-9e82-ffa67364843e' ;;
    media) printf '%s' 'a34c2ae5-6e59-428b-8aa5-1277cbecc9ae' ;;
    publish) printf '%s' '2fb5d3db-0975-4dea-b086-805d255131fa' ;;
    scheduler) printf '%s' '596e5f62-0715-499e-ba71-46f00c9869d3' ;;
    account) printf '%s' 'f8d4a7a9-d30f-46a5-89ac-dc57eb48fb4f' ;;
    *) return 1 ;;
  esac
}

command_args_for_role() {
  case "$1" in
    web) printf '%s' '["runtime"]' ;;
    background) printf '%s' '["background-worker"]' ;;
    collector) printf '%s' '["collector-worker"]' ;;
    media) printf '%s' '["media-worker"]' ;;
    publish) printf '%s' '["publish-worker"]' ;;
    scheduler) printf '%s' '["scheduler"]' ;;
    account) printf '%s' '["account"]' ;;
    *) return 1 ;;
  esac
}

if (( $# > 1 )); then
  ROLES=("${@:2}")
else
  ROLES=(web collector background media publish scheduler account)
fi
for role in "${ROLES[@]}"; do
  app_id_for_role "$role" >/dev/null || { echo "unknown role: $role" >&2; exit 2; }
done

wait_ready() {
  local role="$1" app_id="$2" expected_instances="$3" attempts=60 data
  for ((i=1; i<=attempts; i++)); do
    data=$(aliyun sae DescribeApplicationStatus --RegionId "$REGION_ID" --AppId "$app_id")
    if [[ "$(jq -r '.Data.LastChangeOrderStatus // ""' <<<"$data")" == "FAIL" ]]; then
      echo "$role deployment failed" >&2
      return 1
    fi
    if [[ "$(jq -r '.Data.CurrentStatus // ""' <<<"$data")" == "RUNNING" && "$(jq -r '.Data.SubStatus // ""' <<<"$data")" == "NORMAL" && "$(jq -r '.Data.RunningInstances // 0' <<<"$data")" -ge 1 && "$(jq -r '.Data.LastChangeOrderStatus // ""' <<<"$data")" == "SUCCESS" ]]; then
      return 0
    fi
    sleep 5
  done
  echo "$role deployment did not become healthy" >&2
  return 1
}

deploy_role() {
  local role="$1"
  app_id="$(app_id_for_role "$role")"
  echo "deploying $role"
  current=$(aliyun sae DescribeApplicationStatus --RegionId "$REGION_ID" --AppId "$app_id")
  if [[ "$(jq -r '.Data.LastChangeOrderRunning // false' <<<"$current")" == "true" ]]; then
    change_order_id=$(jq -r '.Data.LastChangeOrderId // empty' <<<"$current")
    if [[ "$ABORT_STUCK_CHANGE_ORDER" != "true" ]]; then
      echo "$role has an active change order ($change_order_id); refusing concurrent deployment" >&2
      exit 1
    fi
    echo "aborting active change order $change_order_id for $role via deployment script"
    aliyun sae AbortChangeOrder --ChangeOrderId "$change_order_id" --Rollback true >/dev/null
    sleep 5
  fi
  cfg=$(aliyun sae DescribeApplicationConfig --RegionId "$REGION_ID" --AppId "$app_id")
  envs=$(jq -r '.Data.Envs // "[]"' <<<"$cfg" | jq --arg fallback "$MEDIA_FALLBACK_CDN_URL" --arg permanent "$MEDIA_PERMANENT_FAILURE_ENABLED" --arg queue "$PUBLISH_QUEUE_CONCURRENCY" --arg bulk "$PUBLISH_BULK_CONCURRENCY" --arg image "$IMAGE_URL" --arg digest "$IMAGE_DIGEST" '
    def upsert($name;$value): if any(.[]; .name == $name) then map(if .name == $name then .value = $value else . end) else . + [{"name":$name,"value":$value}] end;
    upsert("YOUBAN_PUBLISH_MEDIA_CACHE_FALLBACK_CDN_BASE_URL";$fallback)
    | upsert("YOUBAN_PUBLISH_MEDIA_PERMANENT_FAILURE_ENABLED";$permanent)
    | upsert("APP_IMAGE_REFERENCE";$image)
    | upsert("APP_IMAGE_DIGEST";$digest)
    | if $DB_HOST_OVERRIDE != "" then upsert("YOUBAN_DB_HOST";$DB_HOST_OVERRIDE) else . end
    | if $REDIS_ADDRESS_OVERRIDE != "" then upsert("YOUBAN_REDIS_ADDRESS";$REDIS_ADDRESS_OVERRIDE) else . end
    | if ($ENV_ROLE == "publish" or $ENV_ROLE == "media") then upsert("YOUBAN_PUBLISH_MEDIA_ACCOUNT_CONCURRENCY";$MEDIA_ACCOUNT_CONCURRENCY) | upsert("YOUBAN_PUBLISH_MEDIA_SOURCE_CONCURRENCY";$MEDIA_SOURCE_CONCURRENCY) else . end
    | if ($ENV_ROLE == "publish" or $ENV_ROLE == "account") then upsert("YOUBAN_PUBLISH_HISTORY_PENDING_EVENT_LIMIT";$history_limit) | upsert("YOUBAN_PUBLISH_HISTORY_CONCURRENCY";$history_concurrency) else . end
    | if $ENV_ROLE == "publish" then upsert("YOUBAN_PUBLISH_QUEUE_CONCURRENCY";$queue) | upsert("YOUBAN_PUBLISH_BULK_CONCURRENCY";$bulk) | upsert("YOUBAN_FACEPP_CONCURRENCY";$FACEPP_CONCURRENCY) | upsert("YOUBAN_ALIYUN_MATTING_CONCURRENCY";$ALIYUN_MATTING_CONCURRENCY) | upsert("YOUBAN_TENCENT_MATTING_CONCURRENCY";$TENCENT_MATTING_CONCURRENCY) | upsert("YOUBAN_FAPIHUB_CONCURRENCY";$FAPIHUB_CONCURRENCY) | upsert("YOUBAN_FAPIHUB_RATE_LIMIT_PER_MINUTE";$FAPIHUB_RATE_LIMIT_PER_MINUTE) | upsert("YOUBAN_ANTISCAN_FALLBACK_PROVIDER";$ANTISCAN_FALLBACK_PROVIDER) else . end
  ' --arg ENV_ROLE "$role" --arg DB_HOST_OVERRIDE "$DB_HOST_OVERRIDE" --arg REDIS_ADDRESS_OVERRIDE "$REDIS_ADDRESS_OVERRIDE" --arg MEDIA_ACCOUNT_CONCURRENCY "$MEDIA_ACCOUNT_CONCURRENCY" --arg MEDIA_SOURCE_CONCURRENCY "$MEDIA_SOURCE_CONCURRENCY" --arg FACEPP_CONCURRENCY "$FACEPP_CONCURRENCY" --arg ALIYUN_MATTING_CONCURRENCY "$ALIYUN_MATTING_CONCURRENCY" --arg TENCENT_MATTING_CONCURRENCY "$TENCENT_MATTING_CONCURRENCY" --arg FAPIHUB_CONCURRENCY "$FAPIHUB_CONCURRENCY" --arg FAPIHUB_RATE_LIMIT_PER_MINUTE "$FAPIHUB_RATE_LIMIT_PER_MINUTE" --arg ANTISCAN_FALLBACK_PROVIDER "$ANTISCAN_FALLBACK_PROVIDER" --arg history_limit "$HISTORY_PENDING_EVENT_LIMIT" --arg history_concurrency "$HISTORY_CONCURRENCY" | jq -c '.')
  replicas=1
  max_surge=1
  min_ready=1
  if [[ "$role" == "account" ]]; then
    # Account is a strict singleton; never overlap protocol consumers.
    max_surge=0
    min_ready=0
  elif [[ "$role" == "web" ]]; then
    replicas=2
  fi
  deploy_args=(
    aliyun sae DeployApplication
    --RegionId "$REGION_ID" \
    --AppId "$app_id" \
    --ImageUrl "$IMAGE_URL" \
    --CommandArgs "$(command_args_for_role "$role")" \
    --Replicas "$replicas" \
    --MaxSurgeInstances "$max_surge" \
    --MinReadyInstances "$min_ready" \
    --Envs "$envs" \
    --ChangeOrderDesc "deploy $TAG ($role)" \
    --Deploy true
  )
  if [[ -z "$EXTERNAL_IMAGE_URL" ]]; then
    deploy_args+=(--AcrInstanceId "$ACR_INSTANCE_ID")
  fi
  if [[ "$role" == "account" && "$ASSOCIATE_EIP_OVERRIDE" == "true" ]]; then
    deploy_args+=(--AssociateEip true)
  fi
  if [[ "$DISABLE_SLS" == "true" ]]; then
    deploy_args+=(--SlsConfigs "")
  elif [[ "$ENABLE_SLS" == "true" ]]; then
    deploy_args+=(--SlsConfigs '[{"logDir":"","logType":"stdout"}]')
  fi
  "${deploy_args[@]}" >/dev/null
  wait_ready "$role" "$app_id" "$replicas"
  echo "ready $role"
}

# Deploy requested roles concurrently.  Each role owns its SAE application and
# readiness polling, so serialising them only adds the full rollout time of
# every role.  Keep account's singleton safety inside its own deployment
# options; a failure in any background deployment must fail this script.
DEPLOY_PIDS=()
DEPLOY_ROLES=()
for role in "${ROLES[@]}"; do
  deploy_role "$role" &
  DEPLOY_PIDS+=("$!")
  DEPLOY_ROLES+=("$role")
done

deploy_failed=0
for i in "${!DEPLOY_PIDS[@]}"; do
  pid="${DEPLOY_PIDS[$i]}"
  role="${DEPLOY_ROLES[$i]}"
  if ! wait "$pid"; then
    echo "$role deployment failed" >&2
    deploy_failed=1
  fi
done
if (( deploy_failed != 0 )); then
  exit 1
fi

echo "deployed $IMAGE_URL"
