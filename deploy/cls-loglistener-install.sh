#!/usr/bin/env bash
set -euo pipefail

if [[ $# -eq 0 ]]; then
  set -- 43.160.212.235 43.160.221.143 43.160.224.192 43.160.202.7 43.160.210.163 43.160.212.139 43.160.201.198
fi

KEY="${SWARM_MANAGER_SSH_KEY:-/Users/ley/.ssh/id_hk_rsa}"
REGION="${CLS_REGION:-ap-singapore}"
LABEL="${CLS_MACHINE_GROUP_LABEL:-xiaohuiji-swarm}"
OPERATOR_URL="${CLS_LOGLISTENER_OPERATOR_URL:-https://mirrors.tencentyun.com/install/cls/script/loglistener/loglistener_operator}"
SID=$(jq -er '.secretId' "${TCCLI_CREDENTIAL_FILE:-$HOME/.tccli/default.credential}")
SK=$(jq -er '.secretKey' "${TCCLI_CREDENTIAL_FILE:-$HOME/.tccli/default.credential}")

for host in "$@"; do
  echo "[$host] 安装或检查 LogListener"
  ssh -i "$KEY" -o BatchMode=yes -o StrictHostKeyChecking=accept-new "ubuntu@$host" \
    "SID='$SID' SK='$SK' OPERATOR_URL='$OPERATOR_URL' LABEL='$LABEL' REGION='$REGION' sudo -E bash -s" <<'REMOTE'
set -e
if systemctl is-active --quiet loglistener; then
  systemctl check loglistener
  exit 0
fi
curl -fsSL "$OPERATOR_URL" -o /tmp/loglistener_operator
chmod 700 /tmp/loglistener_operator
systemctl stop loglistener 2>/dev/null || true
if [ -d /opt/loglistener ]; then rm -rf /opt/loglistener; fi
printf 'yes\n' | /tmp/loglistener_operator install \
  -s "$SID" -k "$SK" -r "$REGION" -p 443 -l "$LABEL" \
  --switch_endpoint --skip_os_support_check
systemctl enable --now loglistener
systemctl check loglistener
REMOTE
  echo "[$host] LogListener 已就绪"
done
