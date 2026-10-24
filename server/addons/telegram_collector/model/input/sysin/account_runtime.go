package sysin

const (
	AccountChannelAccessLeft      = "left"
	AccountChannelAccessRemoved   = "removed"
	AccountChannelAccessForbidden = "forbidden"
)

type AccountChannelAccessEvent struct {
	ChannelID int64  `json:"channelId"`
	UserID    int64  `json:"userId"`
	Date      int    `json:"date"`
	Title     string `json:"title"`
	Reason    string `json:"reason"`
}

type AccountRuntimeBinding struct {
	AccountID int64
	Signature string
	Sources   []AccountRuntimeSource
	Payload   any
}

type AccountRuntimeSource struct {
	TenantID  int64
	AccountID int64
	SourceID  int64
	ChatID    string
}
