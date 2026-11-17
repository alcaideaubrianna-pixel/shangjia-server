package sys

type profileBatchPushPosition struct {
	AheadCount       int64
	RoundRemaining   int64
	WaitingNextRound bool
	InCurrentRun     bool
}

// Position describes scheduled profile occurrences, not distinct profiles or
// Redis tasks. A profile outside this round waits for the remaining occurrences
// of this round and then its predecessors in the next round.
// Call only after profilePushBatchDayOffset has accepted an eligible rank.
func profilePushBatchPosition(rank profilePushBatchRank, activeBatchSize int64) profileBatchPushPosition {
	if activeBatchSize < 0 {
		activeBatchSize = 0
	}
	if rank.InCurrentRound > 0 {
		if activeBatchSize > 0 && rank.AheadInRound < activeBatchSize {
			return profileBatchPushPosition{
				AheadCount: max(rank.AheadInRound, 0), InCurrentRun: true,
			}
		}
		return profileBatchPushPosition{AheadCount: max(rank.AheadInRound-activeBatchSize, 0)}
	}
	remaining := max(rank.RoundLeft-activeBatchSize, 0)
	return profileBatchPushPosition{
		AheadCount:     remaining + max(rank.BeforeTarget, 0),
		RoundRemaining: remaining, WaitingNextRound: true,
	}
}
