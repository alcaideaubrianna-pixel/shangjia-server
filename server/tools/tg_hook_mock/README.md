# Local Telegram Hook Mock

This server is a local-only Telegram Bot API replacement. It never contacts Telegram.

```bash
go run ./tools/tg_hook_mock --scenario ./tools/local-regression/scenarios/display_verify_success.json
```

Point the local runtime at the printed URL, for example:

```text
YOUBAN_TELEGRAM_RUNTIME_API_URL=http://127.0.0.1:19100
YOUBAN_TELEGRAM_BOT_API_URL=http://127.0.0.1:19100
```

Supported actions are `success`, `error`, and `timeout`. Rules match the Telegram method name, including `sendMediaGroup`, `sendVideo`, `deleteMessage`, and `getWebhookInfo`.
