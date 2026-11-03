package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountIngressRetriesBeforeAcknowledgement(t *testing.T) {
	calls := 0
	event := &sysin.AccountMessageEvent{TgAccountID: 9, SourceID: 12, SourceMessageID: 42}
	err := persistAccountMessage(context.Background(), event, func(ctx context.Context, got *sysin.AccountMessageEvent) error {
		if got != event {
			t.Fatal("retry changed event identity")
		}
		calls++
		if calls < 3 {
			return errors.New("temporary dependency failure")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestAccountIngressCancellationDoesNotAcknowledge(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	started := time.Now()
	err := persistAccountMessage(ctx, &sysin.AccountMessageEvent{}, func(context.Context, *sysin.AccountMessageEvent) error {
		calls++
		cancel()
		return errors.New("database unavailable")
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || time.Since(started) > time.Second {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
