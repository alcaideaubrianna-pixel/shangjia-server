package model

// CollectEventStatus is the persisted lifecycle status of a collection event.
type CollectEventStatus string

const (
	CollectEventStatusPending      CollectEventStatus = "pending"
	CollectEventStatusGroupCollect CollectEventStatus = "group_collecting"
	CollectEventStatusWaitingOrder CollectEventStatus = "waiting_order"
	CollectEventStatusPrechecked   CollectEventStatus = "prechecked"
	CollectEventStatusMediaPending CollectEventStatus = "media_pending"
	CollectEventStatusMediaReady   CollectEventStatus = "media_ready"
	CollectEventStatusDispatched   CollectEventStatus = "dispatched"
	CollectEventStatusProcessed    CollectEventStatus = "processed"
	CollectEventStatusIgnored      CollectEventStatus = "ignored"
	CollectEventStatusFailed       CollectEventStatus = "failed"
)

func (s CollectEventStatus) String() string { return string(s) }

func (s CollectEventStatus) Valid() bool {
	switch s {
	case CollectEventStatusPending, CollectEventStatusGroupCollect, CollectEventStatusWaitingOrder,
		CollectEventStatusPrechecked, CollectEventStatusMediaPending, CollectEventStatusMediaReady,
		CollectEventStatusDispatched, CollectEventStatusProcessed, CollectEventStatusIgnored,
		CollectEventStatusFailed:
		return true
	default:
		return false
	}
}

func ParseCollectEventStatus(value string) (CollectEventStatus, bool) {
	status := CollectEventStatus(value)
	return status, status.Valid()
}

// CollectDispatchStatus is the persisted lifecycle status of a collection dispatch.
type CollectDispatchStatus string

const (
	CollectDispatchStatusPending   CollectDispatchStatus = "pending"
	CollectDispatchStatusReviewing CollectDispatchStatus = "reviewing"
	CollectDispatchStatusSent      CollectDispatchStatus = "sent"
	CollectDispatchStatusSkipped   CollectDispatchStatus = "skipped"
	CollectDispatchStatusFailed    CollectDispatchStatus = "failed"
)

func (s CollectDispatchStatus) String() string { return string(s) }

func (s CollectDispatchStatus) Valid() bool {
	switch s {
	case CollectDispatchStatusPending, CollectDispatchStatusReviewing, CollectDispatchStatusSent,
		CollectDispatchStatusSkipped, CollectDispatchStatusFailed:
		return true
	default:
		return false
	}
}

func ParseCollectDispatchStatus(value string) (CollectDispatchStatus, bool) {
	status := CollectDispatchStatus(value)
	return status, status.Valid()
}

// CollectProcessOutboxStatus is the lease and delivery status of a process outbox row.
type CollectProcessOutboxStatus string

const (
	CollectProcessOutboxStatusPending    CollectProcessOutboxStatus = "pending"
	CollectProcessOutboxStatusProcessing CollectProcessOutboxStatus = "processing"
	CollectProcessOutboxStatusDelivered  CollectProcessOutboxStatus = "delivered"
)

func (s CollectProcessOutboxStatus) String() string { return string(s) }

func (s CollectProcessOutboxStatus) Valid() bool {
	switch s {
	case CollectProcessOutboxStatusPending, CollectProcessOutboxStatusProcessing, CollectProcessOutboxStatusDelivered:
		return true
	default:
		return false
	}
}

func ParseCollectProcessOutboxStatus(value string) (CollectProcessOutboxStatus, bool) {
	status := CollectProcessOutboxStatus(value)
	return status, status.Valid()
}

// TGAttemptStatus is the persisted lifecycle status of a Telegram delivery attempt.
type TGAttemptStatus string

const (
	TGAttemptStatusSending    TGAttemptStatus = "sending"
	TGAttemptStatusAwaiting   TGAttemptStatus = "awaiting_webhook"
	TGAttemptStatusConfirmed  TGAttemptStatus = "confirmed"
	TGAttemptStatusRetryWait  TGAttemptStatus = "retry_wait"
	TGAttemptStatusSuperseded TGAttemptStatus = "superseded"
)

func (s TGAttemptStatus) String() string { return string(s) }

func (s TGAttemptStatus) Valid() bool {
	switch s {
	case TGAttemptStatusSending, TGAttemptStatusAwaiting, TGAttemptStatusConfirmed,
		TGAttemptStatusRetryWait, TGAttemptStatusSuperseded:
		return true
	default:
		return false
	}
}

func ParseTGAttemptStatus(value string) (TGAttemptStatus, bool) {
	status := TGAttemptStatus(value)
	return status, status.Valid()
}

// TGOutboxStatus is the lease and delivery status of a Telegram outbox row.
type TGOutboxStatus string

const (
	TGOutboxStatusPending    TGOutboxStatus = "pending"
	TGOutboxStatusProcessing TGOutboxStatus = "processing"
	TGOutboxStatusDelivered  TGOutboxStatus = "delivered"
)

func (s TGOutboxStatus) String() string { return string(s) }

func (s TGOutboxStatus) Valid() bool {
	switch s {
	case TGOutboxStatusPending, TGOutboxStatusProcessing, TGOutboxStatusDelivered:
		return true
	default:
		return false
	}
}

func ParseTGOutboxStatus(value string) (TGOutboxStatus, bool) {
	status := TGOutboxStatus(value)
	return status, status.Valid()
}
