package sysin

import (
	"testing"

	"hotgo/addons/telegram_collector/domain"
)

func TestCollectSubmitResultConversion(t *testing.T) {
	want := domain.CollectSubmitResult{
		Outcome:        domain.CollectSubmitOutcomeQueued,
		Stage:          domain.CollectSubmitStageQueue,
		Reason:         domain.CollectSubmitReasonQueued,
		Detail:         "dispatch queued",
		DeliveryID:     domain.DeliveryID(7),
		CollectEventID: domain.EventID(9),
		Retryable:      false,
	}

	got, ok := CollectSubmitResultFromDomain(want).ToDomain()
	if !ok {
		t.Fatal("boundary conversion rejected a valid domain result")
	}
	if got != want {
		t.Fatalf("round trip mismatch: got %#v, want %#v", got, want)
	}
}

func TestCollectSubmitResultConversionRejectsUnknownValues(t *testing.T) {
	if _, ok := (CollectSubmitResult{
		Outcome: "unknown",
		Stage:   string(domain.CollectSubmitStageQueue),
		Reason:  string(domain.CollectSubmitReasonQueued),
	}).ToDomain(); ok {
		t.Fatal("boundary conversion accepted an unknown outcome")
	}
}

func TestCollectorBoundaryRoutesAndValidation(t *testing.T) {
	event := RawUpdateEvent{TenantID: 1, SourceID: 351, AccountID: 0, SourceType: SourceTypeBot, Priority: EventPriorityUrgent}
	if !event.ValidateBoundary() || !event.Route().SourceScoped() || !event.Route().Realtime() {
		t.Fatal("bot event should produce a valid realtime source route")
	}

	message := AccountMessageEvent{
		TenantID: 1, AccountID: 2, SourceID: 1370, TgAccountID: 3,
		SourceChatID: "-1001", SourceMessageID: 4, SourceUniqueKey: "event-4",
	}
	if !message.ValidateBoundary() || !message.Route().AccountScoped() {
		t.Fatal("account message should produce a valid account route")
	}
	invalidAccountEvent := RawUpdateEvent{TenantID: 1, SourceID: 351, SourceType: SourceTypeAccount}
	if invalidAccountEvent.ValidateBoundary() {
		t.Fatal("account event without account id should be rejected")
	}
}
