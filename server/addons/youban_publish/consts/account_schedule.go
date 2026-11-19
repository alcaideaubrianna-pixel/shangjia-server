package consts

const (
	AccountScheduleDirectoryLeaseKey = "youban_publish:account_schedule:directory_lease"
	AccountScheduleRedisPrefix       = "youban_publish:{account_schedule_v2}"
	AccountCycleRepairPrefix         = "youban_publish:cycle:repair:"
	QueueProfileCyclePrepareTask     = "youban_publish:profile_cycle_prepare"
	QueueAccountScheduleTask         = "youban_publish:account_schedule"
	AccountScheduleDirectoryNextKey  = "youban_publish:account_schedule:directory_next"
	AccountScheduleCursorKey         = "youban_publish:account_schedule:cursor"
	AccountScheduleLeasePrefix       = "youban_publish:account_schedule:lease:"
	AccountScheduleMaintenancePrefix = "youban_publish:account_schedule:maintenance:"
)
