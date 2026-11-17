package consts

// CollectPairOutcome separates a durable binding from a definitive rejection.
// An unresolved transaction must return a retry/error, never a successful skip.
type CollectPairOutcome uint8

const (
	CollectPairUnresolved CollectPairOutcome = iota
	CollectPairBound
	CollectPairAlreadyBound
	CollectPairRejected
	CollectPairNeedsRepair
)

func (o CollectPairOutcome) IsBound() bool {
	return o == CollectPairBound || o == CollectPairAlreadyBound
}
