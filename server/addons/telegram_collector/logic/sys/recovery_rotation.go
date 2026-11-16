package sys

import "sync/atomic"

var collectorRecoveryBranchTurn atomic.Uint64

func collectorRecoveryBranchOrder(branches []string) []string {
	if len(branches) == 0 {
		return nil
	}
	start := int((collectorRecoveryBranchTurn.Add(1) - 1) % uint64(len(branches)))
	ordered := make([]string, 0, len(branches))
	ordered = append(ordered, branches[start:]...)
	return append(ordered, branches[:start]...)
}
