package domain

// CollectSubmitStage identifies the collector stage that produced a submit result.
type CollectSubmitStage string

const (
	CollectSubmitStageValidation  CollectSubmitStage = "validation"
	CollectSubmitStageSource      CollectSubmitStage = "source"
	CollectSubmitStagePersistence CollectSubmitStage = "persistence"
	CollectSubmitStageDispatch    CollectSubmitStage = "dispatch"
	CollectSubmitStageQueue       CollectSubmitStage = "queue"
)

func (s CollectSubmitStage) Valid() bool {
	switch s {
	case CollectSubmitStageValidation, CollectSubmitStageSource,
		CollectSubmitStagePersistence, CollectSubmitStageDispatch, CollectSubmitStageQueue:
		return true
	default:
		return false
	}
}

// CollectSubmitReason describes why a collector delivery was accepted or stopped.
type CollectSubmitReason string

const (
	CollectSubmitReasonAccepted           CollectSubmitReason = "accepted"
	CollectSubmitReasonGlobalDisabled     CollectSubmitReason = "global_disabled"
	CollectSubmitReasonSourceMissing      CollectSubmitReason = "source_missing"
	CollectSubmitReasonSourceDisabled     CollectSubmitReason = "source_disabled"
	CollectSubmitReasonSourceTypeMismatch CollectSubmitReason = "source_type_mismatch"
	CollectSubmitReasonApprovalBlocked    CollectSubmitReason = "approval_blocked"
	CollectSubmitReasonSelfLoop           CollectSubmitReason = "self_loop"
	CollectSubmitReasonNoRule             CollectSubmitReason = "no_rule"
	CollectSubmitReasonDeduplicated       CollectSubmitReason = "deduplicated"
	CollectSubmitReasonWaitingMedia       CollectSubmitReason = "waiting_media"
	CollectSubmitReasonRetryableFailure   CollectSubmitReason = "retryable_failure"
	CollectSubmitReasonPermanentFailure   CollectSubmitReason = "permanent_failure"
	CollectSubmitReasonCreated            CollectSubmitReason = "created"
	CollectSubmitReasonQueued             CollectSubmitReason = "queued"
)

func (r CollectSubmitReason) Valid() bool {
	switch r {
	case CollectSubmitReasonAccepted, CollectSubmitReasonGlobalDisabled,
		CollectSubmitReasonSourceMissing, CollectSubmitReasonSourceDisabled,
		CollectSubmitReasonSourceTypeMismatch, CollectSubmitReasonApprovalBlocked,
		CollectSubmitReasonSelfLoop, CollectSubmitReasonNoRule,
		CollectSubmitReasonDeduplicated, CollectSubmitReasonWaitingMedia,
		CollectSubmitReasonRetryableFailure, CollectSubmitReasonPermanentFailure,
		CollectSubmitReasonCreated, CollectSubmitReasonQueued:
		return true
	default:
		return false
	}
}

// CollectSubmitOutcome is the externally meaningful result of collector submission.
type CollectSubmitOutcome string

const (
	CollectSubmitOutcomeAccepted CollectSubmitOutcome = "accepted"
	CollectSubmitOutcomeSkipped  CollectSubmitOutcome = "skipped"
	CollectSubmitOutcomeDeferred CollectSubmitOutcome = "deferred"
	CollectSubmitOutcomeCreated  CollectSubmitOutcome = "created"
	CollectSubmitOutcomeQueued   CollectSubmitOutcome = "queued"
	CollectSubmitOutcomeFailed   CollectSubmitOutcome = "failed"
)

func (o CollectSubmitOutcome) Valid() bool {
	switch o {
	case CollectSubmitOutcomeAccepted, CollectSubmitOutcomeSkipped,
		CollectSubmitOutcomeDeferred, CollectSubmitOutcomeCreated,
		CollectSubmitOutcomeQueued, CollectSubmitOutcomeFailed:
		return true
	default:
		return false
	}
}

func (o CollectSubmitOutcome) IsSuccess() bool {
	return o == CollectSubmitOutcomeAccepted || o == CollectSubmitOutcomeCreated || o == CollectSubmitOutcomeQueued
}

func (o CollectSubmitOutcome) IsTerminal() bool {
	return o == CollectSubmitOutcomeAccepted || o == CollectSubmitOutcomeSkipped ||
		o == CollectSubmitOutcomeCreated || o == CollectSubmitOutcomeQueued
}

// CollectSubmitResult is the strict collector-to-service submission contract.
type CollectSubmitResult struct {
	Outcome CollectSubmitOutcome
	Stage   CollectSubmitStage
	Reason  CollectSubmitReason
	Detail  string

	DeliveryID     DeliveryID
	CollectEventID EventID
	ProfileID      int64
	DispatchID     int64
	JobID          int64

	Retryable bool
}

func (r CollectSubmitResult) Valid() bool {
	return r.Outcome.Valid() && r.Stage.Valid() && r.Reason.Valid()
}
