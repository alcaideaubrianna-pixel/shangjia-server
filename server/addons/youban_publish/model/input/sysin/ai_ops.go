package sysin

type AIOpsTelegramMessageRefInp struct {
	ChatId    string `json:"chatId" dc:"Telegram频道Chat ID"`
	MessageId int64  `json:"messageId" dc:"Telegram消息ID"`
}

type AIOpsTelegramOfflineInp struct {
	URLs       []string                     `json:"urls" dc:"Telegram消息链接"`
	Messages   []AIOpsTelegramMessageRefInp `json:"messages" dc:"Telegram频道和消息ID"`
	ProfileIds []int64                      `json:"profileIds" dc:"资料ID"`
	ProfileNos []string                     `json:"profileNos" dc:"资料编号"`
}

type AIOpsTelegramOfflineItem struct {
	Target        string  `json:"target"`
	Status        string  `json:"status"`
	Message       string  `json:"message"`
	ProfileId     int64   `json:"profileId"`
	ProfileNo     string  `json:"profileNo"`
	JobIds        []int64 `json:"jobIds"`
	Offline       bool    `json:"offline"`
	CleanupQueued bool    `json:"cleanupQueued"`
}

type AIOpsTelegramOfflineResult struct {
	Total          int                         `json:"total"`
	CleanupQueued  int                         `json:"cleanupQueued"`
	AlreadyDeleted int                         `json:"alreadyDeleted"`
	NotFound       int                         `json:"notFound"`
	Failed         int                         `json:"failed"`
	Items          []*AIOpsTelegramOfflineItem `json:"items"`
}

type AIOpsTelegramMessageContextInp struct {
	URL       string `json:"url" dc:"Telegram消息地址"`
	ChatId    string `json:"chatId" dc:"Telegram频道Chat ID"`
	MessageId int64  `json:"messageId" dc:"Telegram消息ID"`
}

type AIOpsTelegramMessageContextModel struct {
	Found                  bool   `json:"found"`
	MatchType              string `json:"matchType"`
	MessageURL             string `json:"messageUrl"`
	MessageUsername        string `json:"messageUsername"`
	MessageChatId          string `json:"messageChatId"`
	MessageId              int64  `json:"messageId"`
	ProfileId              int64  `json:"profileId"`
	ProfileNo              string `json:"profileNo"`
	ProfileStatus          int    `json:"profileStatus"`
	TenantId               int64  `json:"tenantId"`
	AccountId              int64  `json:"accountId"`
	AccountName            string `json:"accountName"`
	IsCollected            bool   `json:"isCollected"`
	SourceType             string `json:"sourceType"`
	JobId                  int64  `json:"jobId"`
	JobStatus              string `json:"jobStatus"`
	JobSendPhase           string `json:"jobSendPhase"`
	JobDispatchStatus      string `json:"jobDispatchStatus"`
	JobError               string `json:"jobError"`
	JobDispatchError       string `json:"jobDispatchError"`
	OperationNo            string `json:"operationNo"`
	LatestAttemptPhase     string `json:"latestAttemptPhase"`
	LatestAttemptStatus    string `json:"latestAttemptStatus"`
	LatestAttemptError     string `json:"latestAttemptError"`
	SentDisplayCount       int    `json:"sentDisplayCount"`
	SentVerifyCount        int    `json:"sentVerifyCount"`
	ChannelId              int64  `json:"channelId"`
	ChannelTitle           string `json:"channelTitle"`
	CollectEventId         int64  `json:"collectEventId"`
	CollectEventStatus     string `json:"collectEventStatus"`
	CollectSourceId        int64  `json:"collectSourceId"`
	CollectSourceTitle     string `json:"collectSourceTitle"`
	CollectSourceUsername  string `json:"collectSourceUsername"`
	CollectSourceChatId    string `json:"collectSourceChatId"`
	CollectSourceMessageId int64  `json:"collectSourceMessageId"`
	CollectSourceURL       string `json:"collectSourceUrl"`
	CollectRuleId          int64  `json:"collectRuleId"`
	CollectRuleName        string `json:"collectRuleName"`
	CollectRuleStatus      int    `json:"collectRuleStatus"`
	DisplayMediaCount      int    `json:"displayMediaCount"`
	VerifyMediaCount       int    `json:"verifyMediaCount"`
	MaterialGroupStatus    string `json:"materialGroupStatus"`
	Diagnosis              string `json:"diagnosis"`
}
