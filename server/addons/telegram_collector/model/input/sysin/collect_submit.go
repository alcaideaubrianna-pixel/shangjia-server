package sysin

import "hotgo/addons/telegram_collector/domain"

// CollectSubmitResult is the transport boundary representation used by sysin.
// Domain code should use domain.CollectSubmitResult instead.
type CollectSubmitResult struct {
	Outcome domain.CollectSubmitOutcome `json:"outcome"`
	Stage   domain.CollectSubmitStage   `json:"stage"`
	Reason  domain.CollectSubmitReason  `json:"reason"`
	Detail  string                      `json:"detail,omitempty"`

	DeliveryID     domain.DeliveryID `json:"deliveryId,omitempty"`
	CollectEventID domain.EventID    `json:"collectEventId,omitempty"`
	ProfileID      domain.ProfileID  `json:"profileId,omitempty"`
	DispatchID     domain.DispatchID `json:"dispatchId,omitempty"`
	JobID          domain.TGJobID    `json:"jobId,omitempty"`

	Retryable bool `json:"retryable"`
}

func (r CollectSubmitResult) ToDomain() (domain.CollectSubmitResult, bool) {
	result := domain.CollectSubmitResult{
		Outcome:        r.Outcome,
		Stage:          r.Stage,
		Reason:         r.Reason,
		Detail:         r.Detail,
		DeliveryID:     domain.DeliveryID(r.DeliveryID),
		CollectEventID: domain.EventID(r.CollectEventID),
		ProfileID:      r.ProfileID,
		DispatchID:     r.DispatchID,
		JobID:          r.JobID,
		Retryable:      r.Retryable,
	}
	return result, result.Valid()
}

func CollectSubmitResultFromDomain(result domain.CollectSubmitResult) CollectSubmitResult {
	return CollectSubmitResult{
		Outcome:        result.Outcome,
		Stage:          result.Stage,
		Reason:         result.Reason,
		Detail:         result.Detail,
		DeliveryID:     int64(result.DeliveryID),
		CollectEventID: int64(result.CollectEventID),
		ProfileID:      result.ProfileID,
		DispatchID:     result.DispatchID,
		JobID:          result.JobID,
		Retryable:      result.Retryable,
	}
}
