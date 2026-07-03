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

type CloudResourceConfig struct {
	TencentVisionEnabled int    `json:"tencentVisionEnabled"`
	TencentSecretId      string `json:"tencentSecretId"`
	TencentSecretKey     string `json:"tencentSecretKey"`
	TencentRegion        string `json:"tencentRegion"`
	TencentBdaEndpoint   string `json:"tencentBdaEndpoint"`
	TencentIaiEndpoint   string `json:"tencentIaiEndpoint"`
}

type AntiScanConfig struct {
	Enabled                    int    `json:"antiScanEnabled"`
	DefaultNewNoteEnabled      int    `json:"defaultNewNoteEnabled"`
	ExistingBatchEnabled       int    `json:"existingBatchEnabled"`
	ForceBeforeSendEnabled     int    `json:"forceBeforeSendEnabled"`
	AllowSingleOverrideEnabled int    `json:"allowSingleOverrideEnabled"`
	MetadataStripEnabled       int    `json:"metadataStripEnabled"`
	ResizeEnabled              int    `json:"resizeEnabled"`
	ResizeScale                int    `json:"resizeScale"`
	CropEnabled                int    `json:"cropEnabled"`
	CropPercent                int    `json:"cropPercent"`
	PortraitBackgroundEnabled  int    `json:"portraitBackgroundEnabled"`
	BackgroundReplaceEnabled   int    `json:"backgroundReplaceEnabled"`
	BackgroundBlurEnabled      int    `json:"backgroundBlurEnabled"`
	BackgroundTextureEnabled   int    `json:"backgroundTextureEnabled"`
	MaskEnabled                int    `json:"maskEnabled"`
	MaskMode                   string `json:"maskMode"`
	MaskCount                  int    `json:"maskCount"`
	QrText                     string `json:"qrText"`
	StickerOpacity             int    `json:"stickerOpacity"`
	StickerImage               string `json:"stickerImage"`
	WatermarkEnabled           int    `json:"watermarkEnabled"`
	ProfileNoWatermarkEnabled  int    `json:"profileNoWatermarkEnabled"`
	WatermarkFontSize          int    `json:"watermarkFontSize"`
	WatermarkOpacity           int    `json:"watermarkOpacity"`
	WatermarkText              string `json:"watermarkText"`
	StickerText                string `json:"stickerText"`
	NoiseEnabled               int    `json:"noiseEnabled"`
	NoiseStrength              int    `json:"noiseStrength"`
	CompressionEnabled         int    `json:"compressionEnabled"`
	CompressionQuality         int    `json:"compressionQuality"`
	JpegQualityControlEnabled  int    `json:"jpegQualityControlEnabled"`
	ColorJitterEnabled         int    `json:"colorJitterEnabled"`
	ColorJitterStrength        int    `json:"colorJitterStrength"`
	SharpenBlurEnabled         int    `json:"sharpenBlurEnabled"`
	SharpenBlurMode            string `json:"sharpenBlurMode"`
	SharpenBlurStrength        int    `json:"sharpenBlurStrength"`
}
