package model

type TelegramConfig struct {
	AppId             int    `json:"appId"`
	AppHash           string `json:"appHash"`
	ProxyUrl          string `json:"proxyUrl"`
	BotRuntimeMode    string `json:"botRuntimeMode"`
	WebhookBaseUrl    string `json:"webhookBaseUrl"`
	WebhookSecret     string `json:"webhookSecret"`
	DefaultTargetChat string `json:"defaultTargetChat"`
}

type AccountConfig struct {
	DefaultRoleId int64 `json:"defaultRoleId"`
	DefaultDeptId int64 `json:"defaultDeptId"`
}
