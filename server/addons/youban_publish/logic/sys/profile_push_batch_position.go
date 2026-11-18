package sys

type profilePushQueueItem struct {
	ProfileID int64 `json:"profileId" orm:"profile_id"`
	AccountID int64 `json:"accountId" orm:"account_id"`
	Order     int64 `json:"order" orm:"batch_order"`
	Ordinal   int64 `json:"ordinal" orm:"ordinal"`
	Eligible  bool  `json:"eligible" orm:"eligible"`
}

func profilePushQueueRank(items []profilePushQueueItem, profileID, accountID int64) profilePushBatchRank {
	var rank profilePushBatchRank
	var targetOrdinal int64
	for _, item := range items {
		if item.ProfileID == profileID && item.AccountID == accountID && item.Eligible {
			rank.Eligible = 1
			targetOrdinal = item.Ordinal
			rank.InCurrentRun = targetOrdinal > 0
			break
		}
		if item.Eligible {
			rank.BeforeTarget++
			if item.Ordinal > 0 {
				rank.SelectedAhead++
			}
		}
	}
	if rank.Eligible == 0 {
		return profilePushBatchRank{}
	}
	if rank.InCurrentRun {
		rank.SelectedAhead = 0
		for _, item := range items {
			if item.Eligible && item.Ordinal > 0 && item.Ordinal < targetOrdinal {
				rank.SelectedAhead++
			}
		}
	}
	return rank
}

// Counts describe one bounded channel queue snapshot, never a fixed round.
type profilePushBatchRank struct {
	BeforeTarget  int64 `json:"beforeTarget"`
	SelectedAhead int64 `json:"selectedAhead"`
	Eligible      int64 `json:"eligible"`
	InCurrentRun  bool  `json:"inCurrentRun"`
}

type profileBatchPushPosition struct {
	AheadCount       int64
	RoundRemaining   int64
	WaitingNextRound bool
	InCurrentRun     bool
}

func profilePushBatchPosition(rank profilePushBatchRank, activeBatchSize int64) profileBatchPushPosition {
	if rank.InCurrentRun {
		return profileBatchPushPosition{AheadCount: max(rank.SelectedAhead, 0), InCurrentRun: true}
	}
	return profileBatchPushPosition{AheadCount: max(rank.BeforeTarget-max(rank.SelectedAhead, 0), 0)}
}

// -1 means an actual, unsent snapshot member. Others start at the next
// sending day when a run is active, irrespective of their numeric rank.
func profilePushBatchDayOffset(batchSize int, rank profilePushBatchRank, activeBatchSize int64) (int64, bool) {
	if batchSize <= 0 || rank.Eligible == 0 {
		return 0, false
	}
	position := profilePushBatchPosition(rank, activeBatchSize)
	if position.InCurrentRun {
		return -1, true
	}
	return position.AheadCount / int64(batchSize), true
}
