package sys

type telegramMediaItem struct {
	Id            int64  `json:"id"`
	MediaType     string `json:"mediaType"`
	Purpose       string `json:"purpose"`
	FileUrl       string `json:"fileUrl"`
	PosterUrl     string `json:"posterUrl"`
	TgFileId      string `json:"tgFileId"`
	TgThumbFileId string `json:"tgThumbFileId"`
	SortIndex     int    `json:"sortIndex"`
}

type telegramSentMessage struct {
	MessageId    int64
	MediaGroupId string
	Purpose      string
	MediaId      int64
	TgFileId     string
}

type telegramJobRecord struct {
	Id           int64  `json:"id"`
	TaskId       int64  `json:"taskId"`
	TenantId     int64  `json:"tenantId"`
	AccountId    int64  `json:"accountId"`
	ProfileId    int64  `json:"profileId"`
	BotId        int64  `json:"botId"`
	TargetChatId string `json:"targetChatId"`
	RetryCount   int    `json:"retryCount"`
}
