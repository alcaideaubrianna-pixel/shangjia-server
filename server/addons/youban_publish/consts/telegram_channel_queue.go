package consts

// Telegram 频道可靠领取使用与现有三 lane 相同的 Redis 端点。
const (
	TelegramChannelProcessingPrefix   = "youban:tg:channel:processing:"
	TelegramChannelProcessingDueKey   = "youban:tg:channel:processing_due"
	TelegramChannelRehydrateLockKey   = "youban:tg:channel:rehydrate_lock"
	TelegramChannelRehydrateCursorKey = "youban:tg:channel:rehydrate_cursor"
)
