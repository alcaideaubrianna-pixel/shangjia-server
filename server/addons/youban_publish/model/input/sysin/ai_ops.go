package sysin

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
