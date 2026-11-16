package sysin

import "hotgo/addons/telegram_collector/domain"

// CollectSubmitResult is the transport boundary representation used by sysin.
// Domain code should use domain.CollectSubmitResult instead.
type CollectSubmitResult struct {
	Outcome string `json:"outcome"`
	Stage   string `json:"stage"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail,omitempty"`

	DeliveryID     int64 `json:"deliveryId,omitempty"`
	CollectEventID int64 `json:"collectEventId,omitempty"`
	ProfileID      int64 `json:"profileId,omitempty"`
	DispatchID     int64 `json:"dispatchId,omitempty"`
	JobID          int64 `json:"jobId,omitempty"`

	Retryable bool `json:"retryable"`
}

func (r CollectSubmitResult) ToDomain() (domain.CollectSubmitResult, bool) {
	result := domain.CollectSubmitResult{
		Outcome:        domain.CollectSubmitOutcome(r.Outcome),
		Stage:          domain.CollectSubmitStage(r.Stage),
		Reason:         domain.CollectSubmitReason(r.Reason),
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
		Outcome:        string(result.Outcome),
		Stage:          string(result.Stage),
		Reason:         string(result.Reason),
		Detail:         result.Detail,
		DeliveryID:     int64(result.DeliveryID),
		CollectEventID: int64(result.CollectEventID),
		ProfileID:      result.ProfileID,
		DispatchID:     result.DispatchID,
		JobID:          result.JobID,
		Retryable:      result.Retryable,
	}
}
