package sysin

// Internal service contracts. Tokens/access hashes must never reach public DTOs.
type BotManagerChannel struct {
	Id              int64
	TgAccountId     int64
	TargetChatId    string
	ChannelTitle    string
	ChannelUsername string
	AccessHash      string
}

type BotManagerBot struct {
	Id          int64
	BotName     string
	BotUsername string
	BotToken    string `json:"-"`
}

type BotManagerProjection struct {
	TenantId    int64
	AccountId   int64
	ChannelId   int64
	BotId       int64
	BotName     string
	BotUsername string
	BotToken    string `json:"-"`
	Remove      bool
}
