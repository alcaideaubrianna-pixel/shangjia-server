# PocketPing 本地部署

本目录用于本地部署 PocketPing bridge-server，替代本地 Chatwoot。

```bash
mkdir -p ../../third_party
git clone --depth 1 https://github.com/Ruwad-io/pocketping.git ../../third_party/pocketping
cp .env.example .env
docker compose up -d
```

访问健康检查：

```text
http://localhost:3001/health
```

Telegram 接入需要：

- 通过 BotFather 创建 bot，填写 `TELEGRAM_BOT_TOKEN`
- 创建开启 Topics 的 Telegram supergroup，填写 supergroup id 到 `TELEGRAM_CHAT_ID`
- 将 bot 加为管理员，并授予 Manage Topics 权限

当前本地 bridge-server 会按 HotGo 会话自动拆分 Telegram 话题：

- 一个 HotGo 聊天会话对应一个 Telegram forum topic
- 客户发起新会话时自动调用 Telegram `createForumTopic`
- 客户后续消息会带 `message_thread_id` 发到同一个 topic
- 客服在该 topic 内回复后，bridge-server 会按 topic 映射回 HotGo 的 `pocketping_session_id`
- topic 映射持久化到 `TELEGRAM_TOPIC_MAP_PATH`，Docker 默认挂载到 `/data/telegram-topic-map.json`

HotGo 通过 `youbanChat.pocketPing` 配置访问该 bridge-server。APP 前端继续请求 HotGo 的 `/api/youban_chat/chat/*`，不直接依赖 PocketPing 内部接口。
