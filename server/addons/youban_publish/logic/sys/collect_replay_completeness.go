package sys

import "sort"

// collectReplayMissingMessageIDs compares channel message identities, never
// receiver tenants or Bot IDs. A shared fact can have been retained by any Bot.
func collectReplayMissingMessageIDs(factIDs, candidateIDs []int64) []int64 {
	present := make(map[int64]bool, len(candidateIDs))
	for _, id := range candidateIDs {
		present[id] = true
	}
	missing := make([]int64, 0)
	for _, id := range factIDs {
		if id > 0 && !present[id] {
			missing = append(missing, id)
			present[id] = true
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	return missing
}
