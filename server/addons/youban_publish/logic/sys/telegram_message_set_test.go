package sys

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTelegramDeleteFallbackNextRunDoesNotChainBacklog(t *testing.T) {
	started := time.Now()
	next := telegramDeleteFallbackNextRunAt(context.Background(), 39, 123)
	if next.Before(started.Add(10*time.Second)) || next.After(started.Add(3*time.Minute)) {
		t.Fatalf("fallback next run = %s, expected within roughly 10s-3m", next)
	}
}

func TestIsTelegramBotRemovedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "kicked", err: errors.New("forbidden: bot was kicked from the channel"), want: true},
		{name: "blocked", err: errors.New("forbidden: bot was blocked by the user"), want: true},
		{name: "chat missing", err: errors.New("bad request: chat not found"), want: true},
		{name: "rate limited", err: errors.New("too many requests: retry after 5"), want: false},
		{name: "permission", err: errors.New("not enough rights to delete messages"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isTelegramBotRemovedError(test.err); got != test.want {
				t.Fatalf("isTelegramBotRemovedError() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestIsTelegramMessagePermanentlyUndeletableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "cannot delete", err: errors.New("bad request: message can't be deleted"), want: true},
		{name: "unicode apostrophe", err: errors.New("bad request: message can’t be deleted"), want: true},
		{name: "missing", err: errors.New("bad request: message to delete not found"), want: false},
		{name: "rate limited", err: errors.New("too many requests: retry after 5"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isTelegramMessagePermanentlyUndeletableError(test.err); got != test.want {
				t.Fatalf("isTelegramMessagePermanentlyUndeletableError() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestIsTelegramDeleteRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "http2 goaway", err: errors.New("http2: Transport received Server's graceful shutdown GOAWAY"), want: true},
		{name: "bad gateway", err: errors.New("502 Bad Gateway"), want: true},
		{name: "rate limited", err: errors.New("Too Many Requests: retry after 11"), want: true},
		{name: "permission", err: errors.New("message can't be deleted"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isTelegramDeleteRetryableError(test.err); got != test.want {
				t.Fatalf("isTelegramDeleteRetryableError() = %v, want %v", got, test.want)
			}
		})
	}
}
