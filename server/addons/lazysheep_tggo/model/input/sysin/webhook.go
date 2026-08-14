package sysin

const WebhookUpdateTopic = "lazysheep_tggo_webhook_update"

type WebhookUpdateTask struct {
	BotKey  string `json:"botKey"`
	Payload []byte `json:"payload"`
}
