package sys

import "github.com/gogf/gf/v2/os/gtime"

type telegramMediaItem struct {
	Id                int64  `json:"id"`
	AttachmentId      int64  `json:"attachmentId"`
	MediaType         string `json:"mediaType"`
	MustSend          bool   `json:"mustSend"`
	Purpose           string `json:"purpose"`
	FileUrl           string `json:"fileUrl"`
	PosterUrl         string `json:"posterUrl"`
	StoragePath       string `json:"storagePath"`
	PosterStoragePath string `json:"posterStoragePath"`
	TgFileId          string `json:"tgFileId"`
	TgThumbFileId     string `json:"tgThumbFileId"`
	AssetHash         string `json:"assetHash"`
	SortIndex         int    `json:"sortIndex"`
	VideoWidth        int    `json:"videoWidth"`
	VideoHeight       int    `json:"videoHeight"`
	VideoDuration     int    `json:"videoDuration"`
	AntiScanEnabled   bool   `json:"antiScanEnabled"`
	AntiScanSeed      int64  `json:"antiScanSeed"`
	ForceUpload       bool   `json:"-"`
	ProtectedHashKey  string `json:"-"`
	ProtectedPHash    uint64 `json:"-"`
	ProtectedDHash    uint64 `json:"-"`
}

type telegramSentMessage struct {
	MessageId        int64
	MediaGroupId     string
	Purpose          string
	MediaId          int64
	TgFileId         string
	AssetHash        string
	ProtectedHashKey string
	ProtectedPHash   uint64
	ProtectedDHash   uint64
}

type telegramCopyMediaRef struct {
	ChatId    string
	MessageId int
}

type telegramJobRecord struct {
	Id                     int64       `json:"id"`
	TaskId                 int64       `json:"taskId"`
	OperationNo            string      `json:"operationNo"`
	TenantId               int64       `json:"tenantId"`
	AccountId              int64       `json:"accountId"`
	ProfileId              int64       `json:"profileId"`
	ChannelId              int64       `json:"channelId"`
	BotId                  int64       `json:"botId"`
	PushMode               string      `json:"pushMode"`
	Status                 string      `json:"status"`
	TargetChatId           string      `json:"targetChatId"`
	CollectEventId         int64       `json:"collectEventId"`
	CollectSourceId        int64       `json:"collectSourceId"`
	CollectSourceChatId    string      `json:"collectSourceChatId"`
	CollectSourceMessageId int64       `json:"collectSourceMessageId"`
	RetryCount             int         `json:"retryCount"`
	Priority               int         `json:"priority"`
	AsynqTaskId            string      `json:"asynqTaskId"`
	QueueName              string      `json:"queueName"`
	DispatchStatus         string      `json:"dispatchStatus"`
	DispatchedAt           *gtime.Time `json:"dispatchedAt"`
	DispatchCount          int         `json:"dispatchCount"`
	SendPhase              string      `json:"sendPhase"`
	ReconcileCount         int         `json:"reconcileCount"`
	SentAt                 *gtime.Time `json:"sentAt"`
	CycleEnabled           int         `json:"cycleEnabled"`
	CycleDays              int         `json:"cycleDays"`
	CyclePublishTime       string      `json:"cyclePublishTime"`
	NextCycleAt            *gtime.Time `json:"nextCycleAt"`
	CreatedAt              *gtime.Time `json:"createdAt"`
}
