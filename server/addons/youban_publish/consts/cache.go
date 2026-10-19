package consts

const (
	AntiScanHashHistoryCacheKeyPrefix = "youban_publish:anti_scan:history:"
	AntiScanHashHistoryLockKeyPrefix  = "youban_publish:anti_scan:history_lock:"
	CollectProcessScheduleKeyPrefix   = "youban_publish:collect:process:schedule:"
	CollectProcessThrottleKeyPrefix   = "youban_publish:collect:process:throttle:"
	CollectMediaRecoveryKeyPrefix     = "youban_publish:collect:media:recovery:"
	CollectMediaCompletionKeyPrefix   = "youban_publish:collect:media:completion:"
	DuplicateScanSessionKeyPrefix     = "youban_publish:duplicate_scan:session:"
	DuplicateScanBatchKeyPrefix       = "youban_publish:duplicate_scan:batch:"
	DuplicateScanWorkKeyPrefix        = "youban_publish:duplicate_scan:work:"
	DuplicateScanResultKeyPrefix      = "youban_publish:duplicate_scan:result:v4:"
	PublishAccountActivityKeyPrefix   = "youban_publish:account:activity:"
	MediaPHashScopeSizeKeyPrefix      = "youban_publish:media_phash_lsh:scope_size:"
)
