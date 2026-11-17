package sys

const (
	collectMediaDefaultSourceConcurrency  = 2
	collectMediaDefaultAccountConcurrency = 4
	collectMediaMaxOwnerConcurrency       = 16
)

// Preserve the existing shared key space and total owner limit. The last slot
// is realtime-reserved only when there is capacity for both classes. Bulk must
// never borrow it, even when its normal slots are full. With limit=1 both
// classes still compete for slot 0: compatibility/progress takes precedence
// over reservation, so realtime isolation cannot be guaranteed at that limit.
func collectMediaOwnerSlotOrder(eventID int64, limit int, bulk bool) []int {
	count := normalizeCollectMediaOwnerConcurrency(limit)
	if bulk && count > 1 {
		count--
	}
	start := int(eventID % int64(count))
	if start < 0 {
		start += count
	}
	slots := make([]int, count)
	for offset := range slots {
		slots[offset] = (start + offset) % count
	}
	return slots
}

func normalizeCollectMediaOwnerConcurrency(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > collectMediaMaxOwnerConcurrency {
		return collectMediaMaxOwnerConcurrency
	}
	return limit
}
