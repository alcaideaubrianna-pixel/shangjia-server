package domain

import "testing"

func TestDeliveryStatusTransitions(t *testing.T) {
	for _, tc := range []struct {
		from, to DeliveryStatus
		want     bool
	}{
		{DeliveryPending, DeliveryProcessing, true},
		{DeliveryProcessing, DeliveryCompleted, true},
		{DeliveryProcessing, DeliveryFailedRetry, true},
		{DeliveryFailedRetry, DeliveryProcessing, true},
		{DeliveryCompleted, DeliveryProcessing, false},
		{DeliveryDead, DeliveryProcessing, false},
		{DeliveryStatus("unknown"), DeliveryProcessing, false},
	} {
		if got := CanTransition(tc.from, tc.to); got != tc.want {
			t.Fatalf("CanTransition(%q, %q)=%v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestRouteScopes(t *testing.T) {
	accountRoute := Route{Pool: PoolRealtime, Priority: PriorityRealtime, AccountID: 12}
	if !accountRoute.Valid() || !accountRoute.AccountScoped() || accountRoute.SourceScoped() {
		t.Fatal("account route classification is invalid")
	}

	sourceRoute := Route{Pool: PoolRealtime, Priority: PriorityUrgent, SourceID: 351}
	if !sourceRoute.Valid() || sourceRoute.AccountScoped() || !sourceRoute.SourceScoped() {
		t.Fatal("source route classification is invalid")
	}

	historyRoute := Route{Pool: PoolHistory, Priority: PriorityNormal, SourceID: 1370}
	if !historyRoute.History() || historyRoute.Realtime() {
		t.Fatal("history route classification is invalid")
	}
}

func TestIDValidity(t *testing.T) {
	if EventID(0).Valid() || !DeliveryID(1).Valid() || AccountID(-1).Valid() || !AccountID(0).Valid() {
		t.Fatal("ID validity rules are incorrect")
	}
}

func TestCollectSubmitResultAndBoundaryValues(t *testing.T) {
	result := CollectSubmitResult{
		Outcome:        CollectSubmitOutcomeQueued,
		Stage:          CollectSubmitStageQueue,
		Reason:         CollectSubmitReasonQueued,
		DeliveryID:     DeliveryID(12),
		CollectEventID: EventID(34),
	}
	if !result.Valid() || !result.Outcome.IsSuccess() || !result.Outcome.IsTerminal() {
		t.Fatal("queued submit result should be valid, successful, and terminal")
	}
}
