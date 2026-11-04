package sys

// The workflow tables deliberately separate business progress from transport
// progress. A batch/item is what users see; an execution is one attempt by a
// Bot or protocol account; outbox remains only the delivery mechanism.

const (
	taskBatchTypeCollect      = "collect"
	taskBatchTypeCyclePublish = "cycle_publish"
	taskBatchTypeFullPublish  = "full_publish"
	taskBatchTypeManual       = "manual_publish"

	taskStatusPending   = "pending"
	taskStatusRunning   = "running"
	taskStatusSuccess   = "success"
	taskStatusFailed    = "failed"
	taskStatusUnknown   = "unknown"
	taskStatusCancelled = "cancelled"

	taskExecutionQueued  = "queued"
	taskExecutionRunning = "running"
	taskExecutionSuccess = "success"
	taskExecutionFailed  = "failed"
	taskExecutionExpired = "expired"
)

// taskBatchSummary is intentionally small and maps directly to the daily
// report counters. It can be built from task_item rows or maintained
// incrementally when an item reaches a terminal state.
type taskBatchSummary struct {
	Total     int
	Pending   int
	Running   int
	Success   int
	Failed    int
	Unknown   int
	Cancelled int
}

func (s taskBatchSummary) status() string {
	if s.Total == 0 {
		return taskStatusPending
	}
	if s.Running > 0 || s.Pending > 0 {
		return taskStatusRunning
	}
	if s.Unknown > 0 {
		return taskStatusUnknown
	}
	if s.Success == s.Total {
		return taskStatusSuccess
	}
	if s.Cancelled == s.Total {
		return taskStatusCancelled
	}
	return taskStatusFailed
}
