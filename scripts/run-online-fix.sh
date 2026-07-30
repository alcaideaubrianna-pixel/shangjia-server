#!/usr/bin/env bash

set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "Usage: $0 <hotgo arguments...>" >&2
  echo "Example: $0 up -m=fix -a1=materialImportMediaRepair -a2=436 -a3=5000" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
server_dir="${repo_root}/server"

fix_host="${YOUBAN_FIX_HOST:-124.156.175.163}"
fix_user="${YOUBAN_FIX_USER:-ubuntu}"
fix_key="${YOUBAN_FIX_KEY:-${HOME}/.ssh/id_xiaohuiji}"
fix_container="${YOUBAN_FIX_CONTAINER:-youban-server}"

if [[ ! -f "${fix_key}" ]]; then
  echo "SSH key not found: ${fix_key}" >&2
  exit 1
fi

run_id="$(date +%Y%m%d%H%M%S)-$$"
local_dir="$(mktemp -d "${TMPDIR:-/tmp}/youban-online-fix.XXXXXX")"
local_binary="${local_dir}/hotgo-fix"
local_archive="${local_binary}.gz"
remote_archive="/tmp/hotgo-fix-${run_id}.gz"
remote_binary="/tmp/hotgo-fix-${run_id}"
container_binary="/tmp/hotgo-fix-${run_id}"
remote_target="${fix_user}@${fix_host}"

ssh_options=(
  -o BatchMode=yes
  -o ConnectTimeout=15
  -o StrictHostKeyChecking=no
  -i "${fix_key}"
)

uploaded=0

cleanup() {
  local exit_code=$?
  rm -rf "${local_dir}"
  if [[ ${uploaded} -eq 1 ]]; then
    ssh "${ssh_options[@]}" "${remote_target}" \
      "sudo -n docker exec '${fix_container}' rm -f '${container_binary}' >/dev/null 2>&1 || true; rm -f '${remote_archive}' '${remote_binary}'" \
      >/dev/null 2>&1 || true
  fi
  exit "${exit_code}"
}
trap cleanup EXIT INT TERM

printf 'Building temporary linux/amd64 repair binary...\n'
if [[ "${YOUBAN_FIX_BUILD_MODE:-docker}" == "native" ]]; then
  (
    cd "${server_dir}"
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
      go build -trimpath -ldflags='-s -w' -o "${local_binary}" .
  )
else
  if ! command -v docker >/dev/null 2>&1; then
    echo "Docker is required for the default linux/amd64 CGO build." >&2
    exit 1
  fi
  docker run --rm --platform linux/amd64 \
    -e CGO_ENABLED=1 \
    -e GOOS=linux \
    -e GOARCH=amd64 \
    -v "${repo_root}:/src" \
    -v "${local_dir}:/out" \
    -v youban-online-fix-go-mod:/go/pkg/mod \
    -v youban-online-fix-go-build:/root/.cache/go-build \
    -w /src/server \
    golang:1.25-bookworm \
    go build -trimpath -ldflags='-s -w' -o /out/hotgo-fix .
fi
gzip -9 "${local_binary}"

printf 'Uploading temporary binary to %s...\n' "${remote_target}"
scp "${ssh_options[@]}" "${local_archive}" "${remote_target}:${remote_archive}"
uploaded=1

printf -v escaped_args ' %q' "$@"
remote_command="set -e; gzip -dc '${remote_archive}' > '${remote_binary}'; chmod 700 '${remote_binary}'; sudo -n docker cp '${remote_binary}' '${fix_container}:${container_binary}'; sudo -n docker exec '${fix_container}' sh -c 'cd /app && chmod 700 ${container_binary} && ${container_binary}${escaped_args}'"

printf 'Executing temporary repair binary in container %s...\n' "${fix_container}"
ssh "${ssh_options[@]}" "${remote_target}" "${remote_command}"
printf 'Online repair command completed successfully.\n'
