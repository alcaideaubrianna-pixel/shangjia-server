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
	Enabled         int      `json:"autoDeleteEnabled"`
	Keywords        []string `json:"keywords"`
	Rules           []string `json:"rules"`
	DefaultKeywords []string `json:"defaultKeywords"`
	CustomKeywords  []string `json:"customKeywords"`
	DefaultRules    []string `json:"defaultRules"`
	CustomRules     []string `json:"customRules"`
}

type CloudResourceConfig struct {
	MattingProvider       string `json:"mattingProvider"`
	AliyunAccessKeyId     string `json:"aliyunAccessKeyId"`
	AliyunAccessKeySecret string `json:"aliyunAccessKeySecret"`
	AliyunEndpoint        string `json:"aliyunEndpoint"`
	TencentVisionEnabled  int    `json:"tencentVisionEnabled"`
	TencentCloudSite      string `json:"tencentCloudSite"`
	TencentSecretId       string `json:"tencentSecretId"`
	TencentSecretKey      string `json:"tencentSecretKey"`
	TencentRegion         string `json:"tencentRegion"`
	TencentMattingBucket  string `json:"tencentMattingBucket"`
	TencentMattingPath    string `json:"tencentMattingPath"`
	TencentBdaEndpoint    string `json:"tencentBdaEndpoint"`
	TencentIaiEndpoint    string `json:"tencentIaiEndpoint"`
	FapiHubEnabled        int    `json:"fapiHubEnabled"`
	FapiHubApiKey         string `json:"fapiHubApiKey"`
	FapiHubEndpoint       string `json:"fapiHubEndpoint"`
	FapiHubModel          string `json:"fapiHubModel"`
	FacePlusApiKey        string `json:"facePlusApiKey"`
	FacePlusApiSecret     string `json:"facePlusApiSecret"`
	FacePlusEndpoint      string `json:"facePlusEndpoint"`
	FacePlusConcurrency   int    `json:"facePlusConcurrency"`
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
	BackgroundTexturePreset    string `json:"backgroundTexturePreset"`
	BackgroundTextureImage     string `json:"backgroundTextureImage"`
	MaskEnabled                int    `json:"maskEnabled"`
	MaskMode                   string `json:"maskMode"`
	MaskCount                  int    `json:"maskCount"`
	QrText                     string `json:"qrText"`
	StickerOpacity             int    `json:"stickerOpacity"`
	StickerImage               string `json:"stickerImage"`
	MaskItemsJson              string `json:"maskItemsJson"`
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
