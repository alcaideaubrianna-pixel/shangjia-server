#!/usr/bin/env bash
set -euo pipefail

# Deploy one immutable image tag to all SAE roles in a safe order.
# Usage: ./deploy/sae-deploy-all.sh e5b746c [web|background|collector|media|publish|scheduler|account ...]

REGION_ID="${SAE_REGION_ID:-ap-southeast-1}"
ACR_INSTANCE_ID="${SAE_ACR_INSTANCE_ID:-cri-wx0a7j1fo37ad8mo}"
IMAGE_REPOSITORY="${SAE_IMAGE_REPOSITORY:-xxxxxx-registry-vpc.ap-southeast-1.cr.aliyuncs.com/xiaohuiji/youban-server}"
MEDIA_FALLBACK_CDN_URL="${SAE_MEDIA_FALLBACK_CDN_URL:-https://awdaffwaffawf-1480411854.cos.ap-hongkong.myqcloud.com}"
PUBLISH_QUEUE_CONCURRENCY="${SAE_PUBLISH_QUEUE_CONCURRENCY:-6}"
PUBLISH_BULK_CONCURRENCY="${SAE_PUBLISH_BULK_CONCURRENCY:-8}"
ABORT_STUCK_CHANGE_ORDER="${SAE_ABORT_STUCK_CHANGE_ORDER:-false}"
TAG="${1:-}"

if [[ -z "$TAG" || ! "$TAG" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "usage: $0 <image-tag>" >&2
  exit 2
fi
command -v aliyun >/dev/null || { echo "aliyun CLI is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

IMAGE_URL="${IMAGE_REPOSITORY}:${TAG}"
repo_path="${IMAGE_REPOSITORY#*/}"
repo_id=$(aliyun cr ListRepository --RegionId "$REGION_ID" --InstanceId "$ACR_INSTANCE_ID" --RepoName "${repo_path##*/}" --RepoNamespaceName "${repo_path%/*}" --PageSize 100 | jq -r '.Repositories[0].RepoId // empty')
[[ -n "$repo_id" ]] || { echo "ACR repository not found: $IMAGE_REPOSITORY" >&2; exit 1; }
tag_exists=$(aliyun cr ListRepoTag --RegionId "$REGION_ID" --InstanceId "$ACR_INSTANCE_ID" --RepoId "$repo_id" --PageSize 100 | jq -r --arg tag "$TAG" '[.Images[]? | select(.Tag == $tag)] | length')
[[ "$tag_exists" == "1" ]] || { echo "ACR image tag not found: $IMAGE_URL" >&2; exit 1; }

declare -A APP_IDS=(
  [web]=b4565e3c-6d08-4297-bf8b-037884d54ee4
  [background]=d12e3d86-e368-48ae-bac1-bd41b3a08ef2
  [collector]=0deb4862-2af5-4c0f-9e82-ffa67364843e
  [media]=a34c2ae5-6e59-428b-8aa5-1277cbecc9ae
  [publish]=2fb5d3db-0975-4dea-b086-805d255131fa
  [scheduler]=596e5f62-0715-499e-ba71-46f00c9869d3
  [account]=f8d4a7a9-d30f-46a5-89ac-dc57eb48fb4f
)
declare -A COMMAND_ARGS=(
  [web]='["runtime"]'
  [background]='["background-worker"]'
  [collector]='["collector-worker"]'
  [media]='["media-worker"]'
  [publish]='["publish-worker"]'
  [scheduler]='["scheduler"]'
  [account]='["account"]'
)

if (( $# > 1 )); then
  ROLES=("${@:2}")
else
  ROLES=(web background collector media publish scheduler account)
fi
for role in "${ROLES[@]}"; do
  [[ -n "${APP_IDS[$role]:-}" ]] || { echo "unknown role: $role" >&2; exit 2; }
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

for role in "${ROLES[@]}"; do
  app_id="${APP_IDS[$role]}"
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
  envs=$(jq -r '.Data.Envs // "[]"' <<<"$cfg" | jq --arg fallback "$MEDIA_FALLBACK_CDN_URL" --arg queue "$PUBLISH_QUEUE_CONCURRENCY" --arg bulk "$PUBLISH_BULK_CONCURRENCY" '
    def upsert($name;$value): if any(.[]; .name == $name) then map(if .name == $name then .value = $value else . end) else . + [{"name":$name,"value":$value}] end;
    upsert("YOUBAN_PUBLISH_MEDIA_CACHE_FALLBACK_CDN_BASE_URL";$fallback)
    | if $ENV_ROLE == "publish" then upsert("YOUBAN_PUBLISH_QUEUE_CONCURRENCY";$queue) | upsert("YOUBAN_PUBLISH_BULK_CONCURRENCY";$bulk) else . end
  ' --arg ENV_ROLE "$role" | jq -c '.')
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
  aliyun sae DeployApplication \
    --RegionId "$REGION_ID" \
    --AppId "$app_id" \
    --AcrInstanceId "$ACR_INSTANCE_ID" \
    --ImageUrl "$IMAGE_URL" \
    --CommandArgs "${COMMAND_ARGS[$role]}" \
    --Replicas "$replicas" \
    --MaxSurgeInstances "$max_surge" \
    --MinReadyInstances "$min_ready" \
    --Envs "$envs" \
    --ChangeOrderDesc "deploy $TAG ($role)" \
    --Deploy true >/dev/null
  wait_ready "$role" "$app_id" "$replicas"
  echo "ready $role"
done

echo "deployed $IMAGE_URL"
