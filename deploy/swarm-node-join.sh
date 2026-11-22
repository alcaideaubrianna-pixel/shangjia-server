#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "用法：$0 <节点 SSH 地址> <角色> [节点 SSH 密钥]" >&2
  echo "示例：$0 ubuntu@43.160.230.10 collector /Users/ley/.ssh/id_hk_rsa" >&2
  exit 2
fi

NODE="$1"
ROLE="$2"
KEY="${3:-${SWARM_MANAGER_SSH_KEY:-/Users/ley/.ssh/id_hk_rsa}}"
MANAGER="${SWARM_MANAGER_SSH_TARGET:-ubuntu@43.160.212.235}"
[[ "$ROLE" =~ ^(collector|publish|scheduler|background|media|web|worker)$ ]] || {
  echo "不支持的节点角色：$ROLE" >&2
  exit 2
}

SSH=(ssh -i "$KEY" -o BatchMode=yes -o StrictHostKeyChecking=accept-new)
MANAGER_SSH=("${SSH[@]}" "$MANAGER")
NODE_SSH=("${SSH[@]}" "$NODE")
join_args=$("${MANAGER_SSH[@]}" 'sudo docker swarm join-token worker -q')
manager_ip=$("${MANAGER_SSH[@]}" 'hostname -I | awk "{print \$1}"')

"${NODE_SSH[@]}" 'set -e
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sudo sh
  sudo usermod -aG docker "$USER" || true
fi
sudo systemctl enable --now docker'

"${NODE_SSH[@]}" "sudo docker swarm join --token '$join_args' '$manager_ip:2377'"
node_name=$("${NODE_SSH[@]}" 'hostname')
"${MANAGER_SSH[@]}" "sudo docker node update --label-add role='$ROLE' '$node_name' >/dev/null"

# New Swarm nodes also need the manager's private registry credentials.
registry_config=$("${MANAGER_SSH[@]}" 'sudo base64 -w0 /root/.docker/config.json')
"${NODE_SSH[@]}" "sudo mkdir -p /root/.docker; printf '%s' '$registry_config' | base64 -d | sudo tee /root/.docker/config.json >/dev/null; sudo chmod 600 /root/.docker/config.json"

# Lighthouse firewall rules are required for Swarm monitoring and future node discovery.
node_ip="${NODE##*@}"
if command -v tccli >/dev/null 2>&1 && [[ "$node_ip" =~ ^[0-9]+(\.[0-9]+){3}$ ]]; then
  instance_id=$(tccli lighthouse DescribeInstances --region ap-singapore \
    --Filters "[{\"Name\":\"public-ip-address\",\"Values\":[\"$node_ip\"]}]" |
    jq -er '(.Response // .).InstanceSet[0].InstanceId')
  tccli lighthouse CreateFirewallRules --region ap-singapore --InstanceId "$instance_id" \
    --FirewallRules '[
      {"Protocol":"TCP","Port":"2377,7946,19100,18080","CidrBlock":"10.3.0.0/16","Action":"ACCEPT","FirewallRuleDescription":"Swarm and monitoring internal access"},
      {"Protocol":"UDP","Port":"7946,4789","CidrBlock":"10.3.0.0/16","Action":"ACCEPT","FirewallRuleDescription":"Swarm overlay internal access"}
    ]' >/dev/null
fi

if [[ "${SWARM_AUTO_LOGGING:-true}" == true ]] && [[ -x "$(dirname "$0")/cls-loglistener-install.sh" ]]; then
  "$(dirname "$0")/cls-loglistener-install.sh" "$node_ip"
fi
echo "节点已加入 Swarm：$node_name role=$ROLE"
