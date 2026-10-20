package sysin

import "github.com/gogf/gf/v2/os/gtime"

// ProfileOriginModel is an immutable source observation, not a publishing target.
type ProfileOriginModel struct {
	Id              int64       `json:"id"`
	TenantId        int64       `json:"-"`
	AccountId       int64       `json:"-"`
	ProfileId       int64       `json:"-"`
	OriginKey       string      `json:"-"`
	OriginKind      string      `json:"originKind"`
	CollectMethod   string      `json:"collectMethod"`
	SourceId        int64       `json:"sourceId"`
	EventId         int64       `json:"eventId"`
	BotId           int64       `json:"botId"`
	TgAccountId     int64       `json:"tgAccountId"`
	FollowAccountId int64       `json:"followAccountId"`
	ChatIdRaw       string      `json:"chatIdRaw"`
	ChatId          string      `json:"chatId"`
	MessageId       int64       `json:"messageId"`
	GroupedId       string      `json:"groupedId"`
	ChannelTitle    string      `json:"channelTitle"`
	ChannelUsername string      `json:"channelUsername"`
	ReceivedAt      *gtime.Time `json:"receivedAt"`
	EvidenceKind    string      `json:"evidenceKind"`
	Completeness    string      `json:"completeness"`
	CreatedAt       *gtime.Time `json:"createdAt"`
}

type ProfileOriginBackfillResult struct {
	Skipped  int   `json:"skipped"`
	Scanned  int   `json:"scanned"`
	Origins  int   `json:"origins"`
	Complete int   `json:"complete"`
	Partial  int   `json:"partial"`
	Unknown  int   `json:"unknown"`
	Inserted int64 `json:"inserted"`
	NextId   int64 `json:"nextId"`
	Applied  bool  `json:"applied"`
}
