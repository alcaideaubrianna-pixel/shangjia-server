# Local Bot API media test

This command sends an explicit media set through a chosen Telegram Bot API
endpoint. It does not initialize HotGo, PostgreSQL, Redis, queues, webhooks, or
the Telegram Account runtime.

Use a dedicated test bot. Telegram requires calling `logOut` on the cloud Bot
API before moving a bot to a local Bot API server. Do not point a production bot
at both endpoints.

## Manifest

```json
{
  "display": [
    {"source": "https://cdn.example.test/1.jpg", "filename": "1.jpg"},
    {"source": "/absolute/path/2.jpg", "filename": "2.jpg"}
  ],
  "verify": {
    "source": "https://cdn.example.test/verify.mp4",
    "filename": "verify.mp4"
  }
}
```

`display` must contain 2 to 10 photos. Sources may be HTTPS URLs or local
paths. Files are downloaded to a private temporary directory before upload.

## Run

Expose the remote local Bot API server only on loopback, for example:

```bash
ssh -N -L 127.0.0.1:18081:127.0.0.1:8081 user@bot-api-host
```

Set the token without passing it as a command-line argument:

```bash
export YOUBAN_TG_LOCAL_TEST_TOKEN='test-bot-token'
go run ./tools/tg_local_send_test \
  --endpoint http://127.0.0.1:18081 \
  --chat-id @test_channel \
  --manifest /absolute/path/media.json \
  --repeat 3 \
  --timeout 30m \
  --cleanup=true
```

The command fails unless every album response contains exactly the expected
number of messages. Successful rounds delete only the message IDs returned by
that round. If an API call times out with an unknown result, inspect the test
channel by the printed unique marker before running another round.
