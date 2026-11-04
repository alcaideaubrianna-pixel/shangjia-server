#!/usr/bin/env bash
set -euo pipefail

REGION_ID="${SAE_REGION_ID:-ap-southeast-1}"
ACR_INSTANCE_ID="${SAE_ACR_INSTANCE_ID:-cri-wx0a7j1fo37ad8mo}"
PROFILE_ARGS=()
[[ -n "${ALIBABA_CLOUD_PROFILE:-}" ]] && PROFILE_ARGS+=(--profile "$ALIBABA_CLOUD_PROFILE")

if [[ "${1:-}" != "--env-file" || -z "${2:-}" ]]; then
  echo "usage: $0 --env-file <env.json> [web background collector media publish scheduler account ...]" >&2
  exit 2
fi
ENV_FILE="$2"
[[ -f "$ENV_FILE" ]] || { echo "env file not found: $ENV_FILE" >&2; exit 2; }
command -v aliyun >/dev/null || { echo "aliyun CLI is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
jq -e 'type == "object" and all(.[]; type == "string")' "$ENV_FILE" >/dev/null || {
  echo "env.json must be a JSON object with string values" >&2; exit 2;
}

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

roles=(web collector background media publish scheduler account)
if (( $# > 2 )); then roles=("${@:3}"); fi
for role in "${roles[@]}"; do app_id_for_role "$role" >/dev/null || { echo "unknown role: $role" >&2; exit 2; }; done

wait_ready() {
  local role="$1" app_id="$2"
  for _ in {1..60}; do
    status=$(aliyun sae DescribeApplicationStatus --RegionId "$REGION_ID" --AppId "$app_id" "${PROFILE_ARGS[@]}")
    [[ "$(jq -r '.Data.LastChangeOrderStatus // ""' <<<"$status")" == "FAIL" ]] && { echo "$role update failed" >&2; return 1; }
    if [[ "$(jq -r '.Data.CurrentStatus // ""' <<<"$status")" == "RUNNING" && "$(jq -r '.Data.SubStatus // ""' <<<"$status")" == "NORMAL" && "$(jq -r '.Data.LastChangeOrderStatus // ""' <<<"$status")" == "SUCCESS" ]]; then return 0; fi
    sleep 5
  done
  echo "$role did not become healthy" >&2; return 1
}

for role in "${roles[@]}"; do
  app_id=$(app_id_for_role "$role")
  cfg=$(aliyun sae DescribeApplicationConfig --RegionId "$REGION_ID" --AppId "$app_id" "${PROFILE_ARGS[@]}")
  image=$(jq -r '.Data.ImageUrl // empty' <<<"$cfg")
  [[ -n "$image" ]] || { echo "$role has no current image" >&2; exit 1; }
  envs=$(jq -r '.Data.Envs // "[]"' <<<"$cfg" | jq --slurpfile overrides "$ENV_FILE" '
    reduce ($overrides[0] | to_entries[]) as $item (.;
      if any(.[]; .name == $item.key) then map(if .name == $item.key then .value = $item.value else . end)
      else . + [{name:$item.key,value:$item.value}] end) | .[]' 2>/dev/null | jq -s -c '.')
  replicas=$(jq -r '.Data.Replicas // 1' <<<"$cfg")
  max_surge=$(jq -r '.Data.MaxSurgeInstances // 1' <<<"$cfg")
  min_ready=$(jq -r '.Data.MinReadyInstances // 1' <<<"$cfg")
  if [[ "$role" == "account" ]]; then max_surge=0; min_ready=0; fi
  command_args=$(jq -r '.Data.CommandArgs // "[]"' <<<"$cfg")
  echo "updating ENV only: $role"
  aliyun sae DeployApplication --RegionId "$REGION_ID" --AppId "$app_id" --AcrInstanceId "$ACR_INSTANCE_ID" \
    --ImageUrl "$image" --CommandArgs "$command_args" --Replicas "$replicas" \
    --MaxSurgeInstances "$max_surge" --MinReadyInstances "$min_ready" \
    --Envs "$envs" --ChangeOrderDesc "ENV-only update ($role)" --Deploy true "${PROFILE_ARGS[@]}" >/dev/null
  wait_ready "$role" "$app_id"
  echo "ready: $role"
done
