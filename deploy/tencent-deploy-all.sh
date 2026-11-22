#!/usr/bin/env bash
set -euo pipefail

# Docker Swarm deployment entry point.
# TKE/Kubernetes deployment logic lives in tencent-tke-deploy-legacy.sh.
exec "$(dirname "$0")/swarm-deploy-local.sh" "$@"
