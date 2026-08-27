package sys

import (
	"errors"
	"testing"
	"time"
)

func TestTelegramDeleteFallbackRetryErrorProvidesDelay(t *testing.T) {
	want := 25 * time.Second
	err := &telegramDeleteFallbackRetryError{cause: errors.New("FLOOD_WAIT_20"), delay: want}
	if got := err.AccountTaskRetryDelay(); got != want {
		t.Fatalf("AccountTaskRetryDelay() = %s, want %s", got, want)
	}
	if err.Error() != "FLOOD_WAIT_20" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestTelegramDeleteFallbackConstantsRemainLowPriority(t *testing.T) {
	if telegramDeleteFallbackPriority != -10 {
		t.Fatalf("fallback priority = %d, want -10", telegramDeleteFallbackPriority)
	}
	if telegramDeleteFallbackInterval != time.Minute {
		t.Fatalf("fallback interval = %s", telegramDeleteFallbackInterval)
	}
}

func TestTelegramDeleteFallbackRetryDelayBacksOff(t *testing.T) {
	wants := []time.Duration{time.Minute, time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 30 * time.Minute, 30 * time.Minute}
	for attempt, want := range wants {
		if got := telegramDeleteFallbackRetryDelay(attempt); got != want {
			t.Fatalf("attempt %d delay = %s, want %s", attempt, got, want)
		}
	}
}
