package sys

import "strings"

func (s telegramJobLifecycle) String() string      { return string(s) }
func (s telegramDispatchState) String() string     { return string(s) }
func (s telegramSendPhaseState) String() string    { return string(s) }
func jobDispatch(v DispatchStatus) *DispatchStatus { return &v }
func jobPhase(v SendPhase) *SendPhase              { return &v }
func jobStatusStrings(values []JobStatus) []string {
	r := make([]string, len(values))
	for i, v := range values {
		r[i] = v.String()
	}
	return r
}

// Public state aliases are intentionally backed by the types already used by
// the publish worker. This lets new code use strong names without requiring a
// flag-day migration of database models and existing call sites.
type JobStatus = telegramJobLifecycle
type DispatchStatus = telegramDispatchState
type SendPhase = telegramSendPhaseState
type BlockReason = telegramChannelBlockReason

const (
	JobStatusPending    JobStatus = telegramLifecyclePending
	JobStatusSending    JobStatus = telegramLifecycleSending
	JobStatusRetryWait  JobStatus = telegramLifecycleRetryWait
	JobStatusUnknown    JobStatus = telegramLifecycleConfirming
	JobStatusSent       JobStatus = telegramLifecycleSent
	JobStatusFailed     JobStatus = telegramLifecycleFailed
	JobStatusSuperseded JobStatus = telegramLifecycleSuperseded

	DispatchStatusIdle       DispatchStatus = telegramDispatchIdle
	DispatchStatusQueued     DispatchStatus = telegramDispatchQueued
	DispatchStatusProcessing DispatchStatus = telegramDispatchProcessing
	DispatchStatusDone       DispatchStatus = telegramDispatchDone

	SendPhaseEmpty             SendPhase = telegramSendPhaseStateEmpty
	SendPhaseCleanupProcessing SendPhase = telegramSendPhaseStateCleanupProcessing
	SendPhaseCleanupConfirmed  SendPhase = telegramSendPhaseStateCleanupConfirmed
	SendPhaseDisplaySending    SendPhase = telegramSendPhaseStateDisplaySending
	SendPhaseDisplayConfirmed  SendPhase = telegramSendPhaseStateDisplayConfirmed
	SendPhaseVerifySending     SendPhase = telegramSendPhaseStateVerifySending
	SendPhaseVerifyConfirmed   SendPhase = telegramSendPhaseStateVerifyConfirmed
	SendPhaseCompletedNoVerify SendPhase = telegramSendPhaseStateCompletedNoVerify
	SendPhaseTimeoutCancelled  SendPhase = telegramSendPhaseStateTimeoutCancelled

	BlockReasonNone          BlockReason = telegramChannelBlockNone
	BlockReasonMedia         BlockReason = telegramChannelBlockMedia
	BlockReasonAccount       BlockReason = telegramChannelBlockAccount
	BlockReasonUnknownResult BlockReason = telegramChannelBlockUnknownResult
)

func ParseJobStatus(value string) (JobStatus, bool) {
	s := JobStatus(strings.TrimSpace(value))
	return s, s.Valid()
}

func ParseDispatchStatus(value string) (DispatchStatus, bool) {
	s := DispatchStatus(strings.TrimSpace(value))
	return s, s.Valid()
}

func ParseSendPhase(value string) (SendPhase, bool) {
	s := SendPhase(strings.TrimSpace(value))
	return s, s.Valid()
}

func ParseBlockReason(value string) (BlockReason, bool) {
	s := BlockReason(strings.TrimSpace(value))
	switch s {
	case BlockReasonNone, BlockReasonMedia, BlockReasonAccount, BlockReasonUnknownResult:
		return s, true
	default:
		return s, false
	}
}

func (s telegramChannelBlockReason) IsBlocking() bool { return s != telegramChannelBlockNone }
func (s telegramChannelBlockReason) Valid() bool {
	_, ok := ParseBlockReason(string(s))
	return ok
}

func (s telegramJobLifecycle) IsRetryable() bool { return s.NeedsRetry() }
func (s telegramDispatchState) IsRetryable() bool {
	return s == telegramDispatchIdle || s == telegramDispatchQueued
}
func (s telegramSendPhaseState) IsTerminal() bool {
	return s == telegramSendPhaseStateVerifyConfirmed ||
		s == telegramSendPhaseStateCompletedNoVerify ||
		s == telegramSendPhaseStateTimeoutCancelled
}
func (s telegramSendPhaseState) IsRetryable() bool {
	return s == telegramSendPhaseStateCleanupProcessing ||
		s == telegramSendPhaseStateDisplaySending ||
		s == telegramSendPhaseStateVerifySending
}
func (s telegramSendPhaseState) IsBlocking() bool { return s.OccupiesChannelExecution() }

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
		telegramSendPhaseStateTimeoutCancelled,
		SendPhaseRelayPreparing, SendPhaseRelayReady, SendPhasePackageSending, SendPhasePackageConfirmed:
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

// Relay phases share the existing persisted send_phase column.
const (
	SendPhaseRelayPreparing   SendPhase = "relay_preparing"
	SendPhaseRelayReady       SendPhase = "relay_ready"
	SendPhasePackageSending   SendPhase = "package_sending"
	SendPhasePackageConfirmed SendPhase = "package_confirmed"
)
