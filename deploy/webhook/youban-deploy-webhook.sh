#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/youban}"
IMAGE_PREFIX="${IMAGE_PREFIX:-ghcr.io/mjiadfwaff-bot/youban-server:}"
DOCKER_CONFIG_DIR="${DOCKER_CONFIG_DIR:-/home/ubuntu/.docker}"

image="${1:-}"
if [[ -z "$image" ]]; then
  echo "missing image" >&2
  exit 2
fi

case "$image" in
  ${IMAGE_PREFIX}*) ;;
  *)
    echo "image is not allowed: $image" >&2
    exit 2
    ;;
esac

cd "$APP_DIR"

if sudo grep -q '^YOUBAN_SERVER_IMAGE=' .env; then
  sudo sed -i "s#^YOUBAN_SERVER_IMAGE=.*#YOUBAN_SERVER_IMAGE=$image#" .env
else
  echo "YOUBAN_SERVER_IMAGE=$image" | sudo tee -a .env >/dev/null
fi
sudo ./render-config.sh
sudo env DOCKER_CONFIG="$DOCKER_CONFIG_DIR" docker compose pull
sudo env DOCKER_CONFIG="$DOCKER_CONFIG_DIR" docker compose up -d --force-recreate
sudo docker image prune -f >/dev/null
