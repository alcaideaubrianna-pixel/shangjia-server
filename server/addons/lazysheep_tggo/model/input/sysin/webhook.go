package sysin

const WebhookUpdateTopic = "lazysheep_tggo_webhook_update"

type WebhookUpdateTask struct {
	ReceivedAtUnixNano int64  `json:"receivedAtUnixNano,omitempty"`
	BotKey             string `json:"botKey"`
	Payload            []byte `json:"payload"`
}
