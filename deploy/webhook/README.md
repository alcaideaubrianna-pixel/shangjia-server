# Youban Deploy Webhook

Minimal deploy webhook for GitHub Actions.

Required GitHub Actions secrets:

- `YOUBAN_DEPLOY_URL`: `http://YOUR_SERVER:9088/deploy`
- `YOUBAN_DEPLOY_TOKEN`: same token as `/opt/youban/deploy-webhook.env`

Server files:

- `/opt/youban/deploy-webhook.py`
- `/opt/youban/deploy-webhook.sh`
- `/opt/youban/deploy-webhook.env`
- `/etc/systemd/system/youban-deploy-webhook.service`

Example env:

```env
DEPLOY_WEBHOOK_HOST=0.0.0.0
DEPLOY_WEBHOOK_PORT=9088
DEPLOY_WEBHOOK_TOKEN=change-me
DEPLOY_SCRIPT=/opt/youban/deploy-webhook.sh
IMAGE_PREFIX=ghcr.io/mjiadfwaff-bot/youban-server:
APP_DIR=/opt/youban
DOCKER_CONFIG_DIR=/home/ubuntu/.docker
```
