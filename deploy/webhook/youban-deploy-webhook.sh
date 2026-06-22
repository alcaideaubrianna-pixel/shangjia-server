#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/youban}"
SERVER_IMAGE_PREFIX="${SERVER_IMAGE_PREFIX:-ghcr.io/mjiadfwaff-bot/youban-server:}"
H5_IMAGE_PREFIX="${H5_IMAGE_PREFIX:-ghcr.io/mjiadfwaff-bot/youban-h5:}"
DOCKER_CONFIG_DIR="${DOCKER_CONFIG_DIR:-/home/ubuntu/.docker}"

image="${1:-}"
if [[ -z "$image" ]]; then
  echo "missing image" >&2
  exit 2
fi

env_key=""
case "$image" in
  ${SERVER_IMAGE_PREFIX}*) env_key="YOUBAN_SERVER_IMAGE" ;;
  ${H5_IMAGE_PREFIX}*) env_key="YOUBAN_H5_IMAGE" ;;
  *)
    echo "image is not allowed: $image" >&2
    exit 2
    ;;
esac

cd "$APP_DIR"

if sudo grep -q "^${env_key}=" .env; then
  sudo sed -i "s#^${env_key}=.*#${env_key}=$image#" .env
else
  echo "${env_key}=$image" | sudo tee -a .env >/dev/null
fi
sudo ./render-config.sh
sudo env DOCKER_CONFIG="$DOCKER_CONFIG_DIR" docker compose pull
sudo env DOCKER_CONFIG="$DOCKER_CONFIG_DIR" docker compose up -d --force-recreate
sudo docker image prune -f >/dev/null
