package sys

import (
	"errors"
	"testing"
	"time"
)

func TestCollectHistoryPauseErrorProvidesAccountTaskRetryDelay(t *testing.T) {
	want := 45 * time.Second
	err := &collectHistoryPauseError{delay: want, err: errors.New("backpressure")}
	if got := err.AccountTaskRetryDelay(); got != want {
		t.Fatalf("AccountTaskRetryDelay() = %s, want %s", got, want)
	}
}

func TestShouldLogCollectHistoryBackpressureLimitsPerSource(t *testing.T) {
	collectHistoryBackpressureLogs.Lock()
	collectHistoryBackpressureLogs.last = make(map[int64]time.Time)
	collectHistoryBackpressureLogs.Unlock()
	now := time.Now()
	if !shouldLogCollectHistoryBackpressure(95, now) {
		t.Fatal("first log should be allowed")
	}
	if shouldLogCollectHistoryBackpressure(95, now.Add(30*time.Second)) {
		t.Fatal("second log inside interval should be suppressed")
	}
	if !shouldLogCollectHistoryBackpressure(95, now.Add(time.Minute)) {
		t.Fatal("log after interval should be allowed")
	}
	if !shouldLogCollectHistoryBackpressure(86, now.Add(30*time.Second)) {
		t.Fatal("different source should have an independent interval")
	}
}
