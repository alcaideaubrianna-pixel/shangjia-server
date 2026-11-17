package consts

const (
	// PushEngineCacheKeyPrefix 是账户推送引擎灰度配置缓存键前缀。
	PushEngineCacheKeyPrefix            = "youban_publish:push_engine:"
	CollectSourceStatsLockKey           = "youban_publish:collect:stats:lock"
	AccountChannelAccessNotifyKeyPrefix = "youban_publish:account:channel_access:notify:"
	AntiScanHashHistoryCacheKeyPrefix   = "youban_publish:anti_scan:history:"
	AntiScanHashHistoryLockKeyPrefix    = "youban_publish:anti_scan:history_lock:"
	CollectProcessScheduleKeyPrefix     = "youban_publish:collect:process:schedule:"
	CollectProcessThrottleKeyPrefix     = "youban_publish:collect:process:throttle:"
	// CollectProcessRuleQueueKeyPrefix 是采集规则逻辑队列的键前缀。
	CollectProcessRuleQueueKeyPrefix = "youban_publish:collect:rule:queue:"
	CollectMediaRecoveryKeyPrefix    = "youban_publish:collect:media:recovery:"
	CollectMediaCompletionKeyPrefix  = "youban_publish:collect:media:completion:"
	CollectBotChannelTouchKeyPrefix  = "youban_publish:collect:bot_channel:touch:"
	// CollectChannelBoundKeyPrefix 仅标记已建立关系，不缓存采集授权决定。
	CollectChannelBoundKeyPrefix    = "youban_publish:collect:channel:bound:v1:"
	DuplicateScanSessionKeyPrefix   = "youban_publish:duplicate_scan:session:"
	DuplicateScanBatchKeyPrefix     = "youban_publish:duplicate_scan:batch:"
	DuplicateScanWorkKeyPrefix      = "youban_publish:duplicate_scan:work:"
	DuplicateScanResultKeyPrefix    = "youban_publish:duplicate_scan:result:v4:"
	PublishAccountActivityKeyPrefix = "youban_publish:account:activity:"
	MediaPHashScopeSizeKeyPrefix    = "youban_publish:media_phash_lsh:scope_size:"
	ProfileMediaValidationKeyPrefix = "youban_publish:profile_media_validation:"
	// TelegramReadyStreamDedupeKeyPrefix 防止多个 Outbox dispatcher 在租约边界重复写入同一就绪事件。
	TelegramReadyStreamDedupeKeyPrefix = "youban_publish:tg:ready:dedupe:"
	// TelegramJobRecoveryKeyPrefix 防止多个恢复器在同一轮重复恢复同一 TG Job。
	TelegramJobRecoveryKeyPrefix    = "youban_publish:tg:job:recovery:"
	TelegramOutboxFairScanCursorKey = "youban_publish:tg:outbox:fair_scan:cursor:v1"
	TelegramOutboxFairScanLockKey   = "youban_publish:tg:outbox:fair_scan:lock:v1"
)
