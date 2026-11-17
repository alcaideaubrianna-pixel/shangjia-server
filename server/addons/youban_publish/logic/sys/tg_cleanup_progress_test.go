package sys

import (
	"context"
	"errors"
	"testing"
)

type cleanupProgressTestSettler struct {
	called bool
	jobID  int64
	err    error
}

func (s *cleanupProgressTestSettler) settleProfileCleanupProgress(ctx context.Context, job telegramJobRecord, payload tgQueuePayload) error {
	s.called = true
	s.jobID = job.Id
	return s.err
}

func TestTelegramCleanupProgressWithoutExtension(t *testing.T) {
	for _, operation := range []string{"", "unpublish:10:1", "cancel:10:1"} {
		err := settleTelegramCleanupProgress(context.Background(), struct{}{}, telegramJobRecord{Id: 123},
			tgQueuePayload{CleanupOperationNo: operation})
		if err != nil {
			t.Fatalf("legacy cleanup %q requires optional extension: %v", operation, err)
		}
	}
	if err := settleTelegramCleanupProgress(context.Background(), struct{}{}, telegramJobRecord{Id: 123},
		tgQueuePayload{ProgressOperationNo: "unpublish:10:1"}); err == nil {
		t.Fatal("progress task acknowledged without its extension")
	}
}

func TestTelegramCleanupProgressDelegatesReceiptAndError(t *testing.T) {
	want := errors.New("receipt not committed")
	settler := &cleanupProgressTestSettler{err: want}
	got := settleTelegramCleanupProgress(context.Background(), settler, telegramJobRecord{Id: 123},
		tgQueuePayload{CleanupOperationNo: "unpublish:10:1"})
	if !settler.called || settler.jobID != 123 || !errors.Is(got, want) {
		t.Fatalf("extension receipt/error lost: called=%v job=%d err=%v", settler.called, settler.jobID, got)
	}
}
