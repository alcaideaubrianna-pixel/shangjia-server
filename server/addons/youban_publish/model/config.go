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

type PublishConfig struct {
	CyclePublishEnabled    int    `json:"cyclePublishEnabled"`
	CyclePublishDays       int    `json:"cyclePublishDays"`
	CyclePublishTime       string `json:"cyclePublishTime"`
	SkipDownChannelEnabled int    `json:"skipDownChannelEnabled"`
	SendIntervalSeconds    int    `json:"sendIntervalSeconds"`
	SendWindowEnabled      int    `json:"sendWindowEnabled"`
	SendWindowStart        string `json:"sendWindowStart"`
	SendWindowEnd          string `json:"sendWindowEnd"`
	FailureStrategy        string `json:"failureStrategy"`
	RetryEnabled           int    `json:"retryEnabled"`
	MaxRetryCount          int    `json:"maxRetryCount"`
	RetryIntervalMinutes   int    `json:"retryIntervalMinutes"`
	DefaultAntiScanEnabled int    `json:"defaultAntiScanEnabled"`
}

type AutoDeleteConfig struct {
	Enabled  int      `json:"autoDeleteEnabled"`
	BotIds   []int64  `json:"botIds"`
	Keywords []string `json:"keywords"`
}

type AntiScanConfig struct {
	Enabled                   int    `json:"antiScanEnabled"`
	DefaultNewNoteEnabled     int    `json:"defaultNewNoteEnabled"`
	MetadataStripEnabled      int    `json:"metadataStripEnabled"`
	PortraitBackgroundEnabled int    `json:"portraitBackgroundEnabled"`
	BackgroundReplaceEnabled  int    `json:"backgroundReplaceEnabled"`
	MaskMode                  string `json:"maskMode"`
	MaskCount                 int    `json:"maskCount"`
	QrText                    string `json:"qrText"`
	StickerOpacity            int    `json:"stickerOpacity"`
	StickerImage              string `json:"stickerImage"`
	WatermarkEnabled          int    `json:"watermarkEnabled"`
	WatermarkText             string `json:"watermarkText"`
	StickerText               string `json:"stickerText"`
	NoiseEnabled              int    `json:"noiseEnabled"`
	NoiseStrength             int    `json:"noiseStrength"`
	CompressionEnabled        int    `json:"compressionEnabled"`
	CompressionQuality        int    `json:"compressionQuality"`
	ColorJitterEnabled        int    `json:"colorJitterEnabled"`
}
