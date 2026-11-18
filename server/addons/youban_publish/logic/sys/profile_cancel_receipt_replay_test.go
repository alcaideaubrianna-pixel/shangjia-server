package sys

import "testing"

func TestCanceledAttemptReceiptReplayRequiresDeletedConfirmedEvidence(t *testing.T) {
	deleted := map[int64]struct{}{11: {}, 12: {}}
	complete := telegramDeliveryAttempt{
		Status: telegramAttemptStatusConfirmed, ResponseReceived: 1, ExpectedCount: 2,
	}
	messages := []*telegramSentMessage{{MessageId: 11}, {MessageId: 12}}
	if canceledAttemptReceiptNeedsReplay(complete, messages, deleted) {
		t.Fatal("complete confirmed and deleted evidence must not be replayed")
	}
	if !canceledAttemptReceiptNeedsReplay(complete, messages, map[int64]struct{}{11: {}}) {
		t.Fatal("a live or missing message still needs capture and cleanup")
	}
	if !canceledAttemptReceiptNeedsReplay(complete, messages[:1], deleted) {
		t.Fatal("partial evidence must retain the original recovery path")
	}
	for _, status := range []string{"sending", "unknown", "superseded"} {
		attempt := complete
		attempt.Status = status
		if !canceledAttemptReceiptNeedsReplay(attempt, messages, deleted) {
			t.Fatalf("%s must not be treated as a complete confirmed receipt", status)
		}
	}
	noResponse := complete
	noResponse.ResponseReceived = 0
	if !canceledAttemptReceiptNeedsReplay(noResponse, messages, deleted) {
		t.Fatal("deleted IDs alone cannot prove a response was received")
	}
	if !canceledAttemptReceiptNeedsReplay(complete, []*telegramSentMessage{nil, {MessageId: 12}}, deleted) {
		t.Fatal("invalid message identities cannot suppress recovery")
	}
	zeroExpected := complete
	zeroExpected.ExpectedCount = 0
	if !canceledAttemptReceiptNeedsReplay(zeroExpected, messages, deleted) {
		t.Fatal("an unknown expected count cannot suppress recovery")
	}
	if !canceledAttemptReceiptNeedsReplay(complete, nil, deleted) {
		t.Fatal("empty evidence cannot suppress recovery")
	}
	if !canceledAttemptReceiptNeedsReplay(complete, []*telegramSentMessage{{MessageId: 0}, {MessageId: 12}}, deleted) {
		t.Fatal("a non-positive message ID cannot suppress recovery")
	}
}
