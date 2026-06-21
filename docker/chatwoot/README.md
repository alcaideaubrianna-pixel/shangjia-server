# Chatwoot 本地部署

本项目聊天链路已切换到 `docker/pocketping`。本目录保留旧 Chatwoot PoC compose，仅用于历史排查，不再作为本地默认聊天服务启动。

当前本地调试请使用：

```bash
cd ../pocketping
cp .env.example .env
docker compose up -d
```
