package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestCollectLocalWindowRemainingUsesWallClock(t *testing.T) {
	now := time.Now()
	databaseValue := gtime.NewFromTime(time.Date(
		now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, time.UTC,
	))
	remaining := collectLocalWindowRemaining(databaseValue, 3*time.Minute)
	if remaining < 2*time.Minute+50*time.Second || remaining > 3*time.Minute {
		t.Fatalf("expected local wall-clock delay near 3m, got %s", remaining)
	}
}

func TestCollectLocalWindowRemainingDue(t *testing.T) {
	now := time.Now().Add(-4 * time.Minute)
	databaseValue := gtime.NewFromTime(time.Date(
		now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, time.UTC,
	))
	if remaining := collectLocalWindowRemaining(databaseValue, 3*time.Minute); remaining != 0 {
		t.Fatalf("expected due window, got %s", remaining)
	}
}
