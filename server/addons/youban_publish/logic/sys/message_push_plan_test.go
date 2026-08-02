package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestFirstMessagePushPlanRunAt(t *testing.T) {
	now := messagePushPlanTestTime(2026, time.August, 2, 12, 0, 0)
	got := firstMessagePushPlanRunAt([]string{"18:00:00", "09:00:00"}, now)
	assertMessagePushPlanTime(t, got, 2026, time.August, 2, 18, 0, 0)

	now = messagePushPlanTestTime(2026, time.August, 2, 20, 0, 0)
	got = firstMessagePushPlanRunAt([]string{"18:00:00", "09:00:00"}, now)
	assertMessagePushPlanTime(t, got, 2026, time.August, 3, 9, 0, 0)
}

func TestNextMessagePushPlanRunAtKeepsSameExecutionDay(t *testing.T) {
	scheduledAt := messagePushPlanTestTime(2026, time.August, 2, 9, 0, 0)
	now := messagePushPlanTestTime(2026, time.August, 2, 9, 5, 0)
	got := nextMessagePushPlanRunAt([]string{"09:00:00", "18:00:00"}, 3, scheduledAt, now)
	assertMessagePushPlanTime(t, got, 2026, time.August, 2, 18, 0, 0)
}

func TestNextMessagePushPlanRunAtAdvancesIntervalDays(t *testing.T) {
	scheduledAt := messagePushPlanTestTime(2026, time.August, 2, 18, 0, 0)
	now := messagePushPlanTestTime(2026, time.August, 2, 18, 2, 0)
	got := nextMessagePushPlanRunAt([]string{"09:00:00", "18:00:00"}, 3, scheduledAt, now)
	assertMessagePushPlanTime(t, got, 2026, time.August, 5, 9, 0, 0)
}

func TestNextMessagePushPlanRunAtSkipsExpiredSlots(t *testing.T) {
	scheduledAt := messagePushPlanTestTime(2026, time.August, 2, 18, 0, 0)
	now := messagePushPlanTestTime(2026, time.August, 5, 10, 0, 0)
	got := nextMessagePushPlanRunAt([]string{"09:00:00", "18:00:00"}, 3, scheduledAt, now)
	assertMessagePushPlanTime(t, got, 2026, time.August, 5, 18, 0, 0)

	now = messagePushPlanTestTime(2026, time.August, 6, 10, 0, 0)
	got = nextMessagePushPlanRunAt([]string{"09:00:00", "18:00:00"}, 3, scheduledAt, now)
	assertMessagePushPlanTime(t, got, 2026, time.August, 8, 9, 0, 0)
}

func TestNextMessagePushPlanRunAtDefaultsToDaily(t *testing.T) {
	scheduledAt := messagePushPlanTestTime(2026, time.August, 2, 18, 0, 0)
	now := messagePushPlanTestTime(2026, time.August, 2, 18, 1, 0)
	got := nextMessagePushPlanRunAt([]string{"18:00:00"}, 0, scheduledAt, now)
	assertMessagePushPlanTime(t, got, 2026, time.August, 3, 18, 0, 0)
}

func messagePushPlanTestTime(year int, month time.Month, day, hour, minute, second int) *gtime.Time {
	return gtime.NewFromTime(time.Date(year, month, day, hour, minute, second, 0, time.Local))
}

func assertMessagePushPlanTime(t *testing.T, got *gtime.Time, year int, month time.Month, day, hour, minute, second int) {
	t.Helper()
	if got == nil {
		t.Fatal("expected scheduled time, got nil")
	}
	want := time.Date(year, month, day, hour, minute, second, 0, time.Local)
	if !got.Time.Equal(want) {
		t.Fatalf("scheduled time = %s, want %s", got.Time.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}
