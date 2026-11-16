package domain

// DeliveryStage identifies the collector boundary at which processing ended.
type DeliveryStage string

const (
	StageDecode         DeliveryStage = "decode"
	StagePersist        DeliveryStage = "persist"
	StageMatch          DeliveryStage = "match"
	StageReview         DeliveryStage = "review"
	StagePublishEnqueue DeliveryStage = "publish_enqueue"
	StageDelivery       DeliveryStage = "delivery"
)

func (s DeliveryStage) Valid() bool {
	switch s {
	case StageDecode, StagePersist, StageMatch, StageReview, StagePublishEnqueue, StageDelivery:
		return true
	default:
		return false
	}
}

// DeliveryOutcome is the result exposed by the collector-to-publish boundary.
type DeliveryOutcome string

const (
	OutcomeStored          DeliveryOutcome = "stored"
	OutcomeQueuedForReview DeliveryOutcome = "queued_for_review"
	OutcomeRetryable       DeliveryOutcome = "retryable"
	OutcomePermanent       DeliveryOutcome = "permanent"
)

func (o DeliveryOutcome) Valid() bool {
	switch o {
	case OutcomeStored, OutcomeQueuedForReview, OutcomeRetryable, OutcomePermanent:
		return true
	default:
		return false
	}
}

func (o DeliveryOutcome) Retryable() bool { return o == OutcomeRetryable }
