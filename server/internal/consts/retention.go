package consts

import "time"

// Task execution detail logs share one retention policy across runtime modules.
// Set YOUBAN_TASK_LOG_RETENTION_DAYS to override the three-day default (1-3650).
const (
	TaskLogRetentionDaysEnv              = "YOUBAN_TASK_LOG_RETENTION_DAYS"
	DefaultTaskExecutionLogRetentionDays = 3
	TaskExecutionLogCleanupBatchSize     = 1000
	TaskExecutionLogCleanupInterval      = time.Minute
)
