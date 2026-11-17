package domain

// EventStatus is the lifecycle state of a collector event.
type EventStatus string

const (
	EventReceived    EventStatus = "received"
	EventProcessing  EventStatus = "processing"
	EventReady       EventStatus = "ready"
	EventIgnored     EventStatus = "ignored"
	EventFailedRetry EventStatus = "failed_retry"
	EventDead        EventStatus = "dead"
)

func (s EventStatus) Valid() bool {
	switch s {
	case EventReceived, EventProcessing, EventReady, EventIgnored, EventFailedRetry, EventDead:
		return true
	default:
		return false
	}
}

// DeliveryStatus is the lifecycle state of a collector delivery.
type DeliveryStatus string

const (
	DeliveryPending     DeliveryStatus = "pending"
	DeliveryProcessing  DeliveryStatus = "processing"
	DeliveryCompleted   DeliveryStatus = "completed"
	DeliveryFailedRetry DeliveryStatus = "failed_retry"
	DeliveryDead        DeliveryStatus = "dead"
	DeliveryQuarantined DeliveryStatus = "quarantined"
)

func (s DeliveryStatus) Valid() bool {
	switch s {
	case DeliveryPending, DeliveryProcessing, DeliveryCompleted, DeliveryFailedRetry, DeliveryDead, DeliveryQuarantined:
		return true
	default:
		return false
	}
}

func (s DeliveryStatus) Terminal() bool {
	return s == DeliveryCompleted || s == DeliveryDead || s == DeliveryQuarantined
}

func (s DeliveryStatus) Retryable() bool {
	return s == DeliveryPending || s == DeliveryFailedRetry
}

// CanTransition reports whether a delivery lifecycle transition is permitted.
func CanTransition(from, to DeliveryStatus) bool {
	if !from.Valid() || !to.Valid() || from == to || from.Terminal() {
		return false
	}
	switch from {
	case DeliveryPending:
		return to == DeliveryProcessing || to == DeliveryQuarantined
	case DeliveryProcessing:
		return to == DeliveryCompleted || to == DeliveryFailedRetry || to == DeliveryDead || to == DeliveryQuarantined
	case DeliveryFailedRetry:
		return to == DeliveryProcessing || to == DeliveryQuarantined
	default:
		return false
	}
}
