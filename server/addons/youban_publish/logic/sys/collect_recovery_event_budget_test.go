package sys

import (
	"testing"
	"time"
)

func TestCollectEventRecoveryTimeBudgetReservesLaterLanes(t *testing.T) {
	remaining := 15 * time.Second
	for lanes := 8; lanes > 0; lanes-- {
		source, branch := collectEventRecoveryTimeBudget(remaining, lanes)
		if source <= 0 || branch <= 0 || source > 5*time.Second || branch*3 > source {
			t.Fatalf("invalid deadline allocation: source=%s branch=%s", source, branch)
		}
		remaining -= source
		if remaining <= 0 {
			t.Fatal("an early source exhausted later lanes' query budget")
		}
	}
}

func TestCollectEventRecoveryBudgets(t *testing.T) {
	for budget := 0; budget <= 100; budget++ {
		recent, history, failed := collectEventRecoveryBudgets(budget)
		if recent+history+failed != budget || recent < 0 || history < 0 || failed < 0 {
			t.Fatalf("budget %d: invalid allocation %d/%d/%d", budget, recent, history, failed)
		}
		if budget >= 3 && (recent == 0 || history == 0 || failed == 0) {
			t.Fatalf("budget %d starves a recovery class", budget)
		}
	}
}
