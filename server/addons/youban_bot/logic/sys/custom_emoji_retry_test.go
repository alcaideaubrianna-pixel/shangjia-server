package sys

import (
	"context"
	"errors"
	"io"
	"testing"

	"hotgo/addons/youban_bot/model/input/sysin"
)

func TestRetryTelegramTransientRetriesEOF(t *testing.T) {
	attempts := 0
	result, err := retryTelegramTransient(context.Background(), "test", 3, func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", io.EOF
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("retryTelegramTransient() error = %v", err)
	}
	if result != "ok" {
		t.Fatalf("retryTelegramTransient() result = %q, want %q", result, "ok")
	}
	if attempts != 3 {
		t.Fatalf("retryTelegramTransient() attempts = %d, want 3", attempts)
	}
}

func TestRetryTelegramTransientStopsOnPermanentError(t *testing.T) {
	wantErr := errors.New("bad request")
	attempts := 0
	_, err := retryTelegramTransient(context.Background(), "test", 3, func() (string, error) {
		attempts++
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("retryTelegramTransient() error = %v, want %v", err, wantErr)
	}
	if attempts != 1 {
		t.Fatalf("retryTelegramTransient() attempts = %d, want 1", attempts)
	}
}

func TestRetryTelegramTransientStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	_, err := retryTelegramTransient(ctx, "test", 3, func() (string, error) {
		attempts++
		cancel()
		return "", io.EOF
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("retryTelegramTransient() error = %v, want %v", err, context.Canceled)
	}
	if attempts != 1 {
		t.Fatalf("retryTelegramTransient() attempts = %d, want 1", attempts)
	}
}

func TestOrderedCustomEmojiModelsKeepsRequestedOrder(t *testing.T) {
	resolved := map[string]*sysin.CustomEmojiModel{
		"emoji-1": {EmojiId: "emoji-1"},
		"emoji-2": {EmojiId: "emoji-2"},
	}
	result := orderedCustomEmojiModels([]string{"emoji-2", "missing", "emoji-1"}, resolved)
	if len(result) != 2 {
		t.Fatalf("orderedCustomEmojiModels() length = %d, want 2", len(result))
	}
	if result[0].EmojiId != "emoji-2" || result[1].EmojiId != "emoji-1" {
		t.Fatalf("orderedCustomEmojiModels() order = [%s, %s]", result[0].EmojiId, result[1].EmojiId)
	}
}
