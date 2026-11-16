package sys

// CollectSubmitStage identifies the stage that produced a collector submit
// result. Keeping the stage typed prevents callers from mixing delivery,
// persistence, and dispatch labels in structured logs.
type CollectSubmitStage string

const (
	CollectSubmitStageValidation  CollectSubmitStage = "validation"
	CollectSubmitStageSource      CollectSubmitStage = "source"
	CollectSubmitStagePersistence CollectSubmitStage = "persistence"
	CollectSubmitStageDispatch    CollectSubmitStage = "dispatch"
	CollectSubmitStageQueue       CollectSubmitStage = "queue"
)

// CollectSubmitReason describes why a collector delivery was accepted,
// skipped, deferred, or rejected. Values are intentionally stable for metrics,
// SLS queries, and recovery decisions.
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

// CollectSubmitOutcome is the externally meaningful result of handling one
// collector delivery. It is separate from database status: a delivery may be
// skipped or deferred without being a processing failure.
type CollectSubmitOutcome string

const (
	CollectSubmitOutcomeAccepted CollectSubmitOutcome = "accepted"
	CollectSubmitOutcomeSkipped  CollectSubmitOutcome = "skipped"
	CollectSubmitOutcomeDeferred CollectSubmitOutcome = "deferred"
	CollectSubmitOutcomeCreated  CollectSubmitOutcome = "created"
	CollectSubmitOutcomeQueued   CollectSubmitOutcome = "queued"
	CollectSubmitOutcomeFailed   CollectSubmitOutcome = "failed"
)

// CollectSubmitResult is the standalone result contract for collector
// submission. IDs are optional because validation and source filtering can
// finish before an event, profile, dispatch, or queue job exists.
type CollectSubmitResult struct {
	Outcome CollectSubmitOutcome
	Stage   CollectSubmitStage
	Reason  CollectSubmitReason
	Detail  string

	DeliveryID     int64
	CollectEventID int64
	ProfileID      int64
	DispatchID     int64
	JobID          int64

	Retryable bool
}

// IsTerminal reports whether automatic retry should not be inferred from the
// result. Deferred and failed results remain retry candidates.
func (r CollectSubmitResult) IsTerminal() bool {
	switch r.Outcome {
	case CollectSubmitOutcomeSkipped, CollectSubmitOutcomeCreated,
		CollectSubmitOutcomeQueued, CollectSubmitOutcomeAccepted:
		return true
	default:
		return false
	}
}

// IsSuccess reports whether the submission reached an accepted, created, or
// queued outcome. Skipped and deferred outcomes remain distinct from success.
func (r CollectSubmitResult) IsSuccess() bool {
	return r.Outcome == CollectSubmitOutcomeAccepted ||
		r.Outcome == CollectSubmitOutcomeCreated ||
		r.Outcome == CollectSubmitOutcomeQueued
}
