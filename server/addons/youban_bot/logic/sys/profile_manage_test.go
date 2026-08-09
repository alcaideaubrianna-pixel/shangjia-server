package sys

import (
	"errors"
	"testing"
	"time"
)

func TestIsInvalidTelegramMediaReference(t *testing.T) {
	if !isInvalidTelegramMediaReference(errors.New("bad request: wrong file identifier/HTTP URL specified")) {
		t.Fatal("expected invalid Telegram media reference to be detected")
	}
	if isInvalidTelegramMediaReference(errors.New("request timeout")) {
		t.Fatal("unexpectedly classified timeout as invalid media reference")
	}
}

func TestProfileMediaGroupIdleWaitUsesLastMessageTime(t *testing.T) {
	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.Local)
	pending := profilePendingMediaGroup{CreatedAt: now.Add(-10 * time.Minute).Unix(), UpdatedAt: now.Add(-2 * time.Minute).UnixNano()}
	if wait := profileMediaGroupIdleWait(pending, now); wait != time.Minute {
		t.Fatalf("unexpected idle wait: got %s, want %s", wait, time.Minute)
	}
}

func TestProfileMediaGroupIdleWaitReturnsReadyAfterDebounce(t *testing.T) {
	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.Local)
	pending := profilePendingMediaGroup{UpdatedAt: now.Add(-profileMediaGroupDebounce).UnixNano()}
	if wait := profileMediaGroupIdleWait(pending, now); wait != 0 {
		t.Fatalf("expected media group to be ready, got wait %s", wait)
	}
}
