package sysin

type CollectSourceOptionModel struct {
	Id           int64  `json:"id" dc:"采集源ID"`
	SourceId     int64  `json:"sourceId" dc:"采集源ID"`
	SourceChatId string `json:"sourceChatId" dc:"采集频道ID"`
	Label        string `json:"label" dc:"采集频道显示名称"`
	Username     string `json:"username" dc:"采集频道用户名"`
}
