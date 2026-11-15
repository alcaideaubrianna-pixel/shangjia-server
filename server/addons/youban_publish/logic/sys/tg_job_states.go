package sys

import "strings"

const (
	telegramSendPhaseStateEmpty             telegramSendPhaseState = ""
	telegramSendPhaseStateCleanupProcessing telegramSendPhaseState = "cleanup_processing"
	telegramSendPhaseStateCleanupConfirmed  telegramSendPhaseState = "cleanup_confirmed"
	telegramSendPhaseStateDisplaySending    telegramSendPhaseState = "display_sending"
	telegramSendPhaseStateDisplayConfirmed  telegramSendPhaseState = "display_confirmed"
	telegramSendPhaseStateVerifySending     telegramSendPhaseState = "verify_sending"
	telegramSendPhaseStateVerifyConfirmed   telegramSendPhaseState = "verify_confirmed"
	telegramSendPhaseStateCompletedNoVerify telegramSendPhaseState = "completed_without_verify"
	telegramSendPhaseStateTimeoutCancelled  telegramSendPhaseState = "timeout_cancelled"
)

func (s telegramJobLifecycle) Valid() bool {
	switch s {
	case telegramLifecyclePending, telegramLifecycleSending, telegramLifecycleRetryWait,
		telegramLifecycleConfirming, telegramLifecycleSent, telegramLifecycleFailed,
		telegramLifecycleSuperseded:
		return true
	default:
		return false
	}
}

func (s telegramJobLifecycle) IsTerminal() bool {
	return s == telegramLifecycleSent || s == telegramLifecycleFailed || s == telegramLifecycleSuperseded
}

func (s telegramJobLifecycle) OccupiesChannelExecution() bool {
	return s == telegramLifecycleSending || s == telegramLifecycleConfirming
}

func (s telegramJobLifecycle) CanBeOvertaken() bool {
	return s == telegramLifecyclePending || s == telegramLifecycleRetryWait
}

func (s telegramJobLifecycle) NeedsRetry() bool {
	return s == telegramLifecyclePending || s == telegramLifecycleRetryWait
}

func (s telegramJobLifecycle) NeedsReconcile() bool {
	return s == telegramLifecycleConfirming
}

func (s telegramDispatchState) Valid() bool {
	switch s {
	case telegramDispatchIdle, telegramDispatchQueued, telegramDispatchProcessing, telegramDispatchDone:
		return true
	default:
		return false
	}
}

func (s telegramDispatchState) IsInFlight() bool {
	return s == telegramDispatchQueued || s == telegramDispatchProcessing
}

func (s telegramDispatchState) IsTerminal() bool {
	return s == telegramDispatchDone
}

func (s telegramJobSource) Valid() bool {
	switch s {
	case telegramSourceManual, telegramSourceInteractive, telegramSourceCycle,
		telegramSourceFullPush, telegramSourceCollect, telegramSourceHistory,
		telegramSourceCleanup, telegramSourceUnknown:
		return true
	default:
		return false
	}
}

// Priority returns the scheduling order shared by all job producers.
// Lower values are dispatched first.
func (s telegramJobSource) Priority() int {
	switch s {
	case telegramSourceManual, telegramSourceInteractive:
		return tgJobPriorityInteractive
	case telegramSourceCollect:
		return 10
	case telegramSourceCycle:
		return 30
	case telegramSourceFullPush:
		return 50
	case telegramSourceHistory:
		return 70
	case telegramSourceCleanup:
		return 5
	default:
		return 100
	}
}

func (s telegramSendPhaseState) Valid() bool {
	switch s {
	case telegramSendPhaseStateEmpty, telegramSendPhaseStateCleanupProcessing,
		telegramSendPhaseStateCleanupConfirmed, telegramSendPhaseStateDisplaySending,
		telegramSendPhaseStateDisplayConfirmed, telegramSendPhaseStateVerifySending,
		telegramSendPhaseStateVerifyConfirmed, telegramSendPhaseStateCompletedNoVerify,
		telegramSendPhaseStateTimeoutCancelled:
		return true
	default:
		return false
	}
}

func (s telegramSendPhaseState) OccupiesChannelExecution() bool {
	switch s {
	case telegramSendPhaseStateCleanupProcessing, telegramSendPhaseStateDisplaySending,
		telegramSendPhaseStateVerifySending:
		return true
	default:
		return false
	}
}

func (s telegramSendPhaseState) IsConfirmed() bool {
	switch s {
	case telegramSendPhaseStateCleanupConfirmed, telegramSendPhaseStateDisplayConfirmed,
		telegramSendPhaseStateVerifyConfirmed, telegramSendPhaseStateCompletedNoVerify:
		return true
	default:
		return false
	}
}

func telegramLifecycleStateOf(value string) telegramJobLifecycle {
	return telegramJobLifecycle(strings.TrimSpace(value))
}

func telegramSendPhaseStateOf(value string) telegramSendPhaseState {
	return telegramSendPhaseState(strings.TrimSpace(value))
}

func telegramDispatchStateOfValue(value string) telegramDispatchState {
	return telegramDispatchState(strings.TrimSpace(value))
}

func telegramJobSourceStateOf(operationNo string, priority int) telegramJobSource {
	return telegramJobSourceOf(operationNo, priority)
}

// telegramJobOccupiesChannelExecution centralizes the cross-field rule.
// A task owns the channel while its lifecycle or send phase is actively
// executing, or while its lifecycle is awaiting an uncertain Telegram result.
func telegramJobOccupiesChannelExecution(status, sendPhase, dispatchStatus string) bool {
	return telegramLifecycleStateOf(status).OccupiesChannelExecution() ||
		telegramLifecycleStateOf(status).NeedsReconcile() ||
		telegramSendPhaseStateOf(sendPhase).OccupiesChannelExecution() ||
		telegramDispatchStateOfValue(dispatchStatus) == telegramDispatchProcessing
}
