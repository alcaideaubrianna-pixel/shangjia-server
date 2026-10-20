package sys

import (
	"testing"
	"time"
)

func TestTelegramReconcileRetryDelayWaitsForSafeTakeover(t *testing.T) {
	now := time.Date(2026, 10, 20, 16, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		created time.Time
		want    time.Duration
	}{
		{name: "fresh interruption", created: now, want: 5*time.Minute + time.Second},
		{name: "near boundary", created: now.Add(-4*time.Minute - 36*time.Second), want: 25 * time.Second},
		{name: "at boundary", created: now.Add(-5 * time.Minute), want: 0},
		{name: "past boundary", created: now.Add(-6 * time.Minute), want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramReconcileRetryDelay(test.created, now); got != test.want {
				t.Fatalf("retry delay = %s, want %s", got, test.want)
			}
		})
	}
}

func TestTelegramReconcileRetryErrorExposesAccountTaskDelay(t *testing.T) {
	err := &telegramReconcileRetryError{delay: 24 * time.Second}
	if got := err.AccountTaskRetryDelay(); got != 24*time.Second {
		t.Fatalf("account task retry delay = %s", got)
	}
}
