package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestCyclePublishIdentifiers(t *testing.T) {
	operationNo := cyclePublishOperationNo(12, 34, 56)
	if operationNo != "cycle_batch:12:34:56" {
		t.Fatalf("unexpected operation number: %s", operationNo)
	}
	if !isCycleBatchOperation(operationNo) {
		t.Fatalf("operation should be recognized as cycle batch: %s", operationNo)
	}
	if cyclePublishOperationNo(12, 34, 57) == operationNo {
		t.Fatal("different channels must not share a cycle operation number")
	}
}

func TestDueProfileCycleOperationNo(t *testing.T) {
	operationNo := dueProfileCycleOperationNo(12, 3, 34, 56)
	if operationNo != "cycle_batch:due:12:3:34:56" {
		t.Fatalf("unexpected due operation number: %s", operationNo)
	}
	if !isCycleBatchOperation(operationNo) {
		t.Fatalf("due operation should reuse cycle delivery chain: %s", operationNo)
	}
}

func TestCalculateProfileCycleAt(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	base := gtime.New(time.Date(2026, 7, 31, 13, 25, 55, 0, location))
	withoutClock := calculateProfileCycleAt(3, "", base, false)
	wantWithoutClock := time.Date(2026, 8, 3, 13, 25, 55, 0, location)
	if withoutClock == nil || !withoutClock.Time.Equal(wantWithoutClock) {
		t.Fatalf("cycle without clock = %v, want %v", withoutClock, wantWithoutClock)
	}
	withClock := calculateProfileCycleAt(3, "17:30", base, false)
	wantWithClock := time.Date(2026, 8, 3, 17, 30, 0, 0, location)
	if withClock == nil || !withClock.Time.Equal(wantWithClock) {
		t.Fatalf("cycle with clock = %v, want %v", withClock, wantWithClock)
	}
	develop := calculateProfileCycleAt(3, "17:30", base, true)
	if develop == nil || !develop.Time.Equal(base.Add(3*time.Second).Time) {
		t.Fatalf("develop cycle = %v, want %v", develop, base.Add(3*time.Second))
	}
}

func TestProfileCycleOverdueAtDeterministic(t *testing.T) {
	now := gtime.New(time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC))
	first := profileCycleOverdueAt(now, 123, 45)
	second := profileCycleOverdueAt(now, 123, 45)
	if first == nil || second == nil || !first.Equal(second) {
		t.Fatalf("overdue spread must be deterministic: %v %v", first, second)
	}
	if first.Before(now) || first.After(now.Add(profileCycleOverdueSpreadWindow)) {
		t.Fatalf("overdue spread out of range: %v", first)
	}
}

func TestSameProfileCycleConfig(t *testing.T) {
	base := profileCycleChannelConfig{Id: 1, Enabled: 1, Days: 3, PublishTime: "17:30", Status: 1, Direction: "up"}
	if !sameProfileCycleConfig(base, base) {
		t.Fatal("same enabled cycle config must be stable")
	}
	changedDays := base
	changedDays.Days = 4
	if sameProfileCycleConfig(base, changedDays) {
		t.Fatal("changed cycle days must trigger reschedule")
	}
	disabled := base
	disabled.Enabled = 0
	if sameProfileCycleConfig(base, disabled) {
		t.Fatal("enabled state change must trigger reschedule")
	}
	otherDisabled := disabled
	otherDisabled.Days = 10
	if !sameProfileCycleConfig(disabled, otherDisabled) {
		t.Fatal("disabled configs do not require repeated schedule calculation")
	}
}

func TestParseCycleClock(t *testing.T) {
	tests := []struct {
		value  string
		hour   int
		minute int
		ok     bool
	}{
		{value: "09:30", hour: 9, minute: 30, ok: true},
		{value: " 23:59 ", hour: 23, minute: 59, ok: true},
		{value: "24:00", ok: false},
		{value: "09", ok: false},
	}
	for _, test := range tests {
		hour, minute, ok := parseCycleClock(test.value)
		if hour != test.hour || minute != test.minute || ok != test.ok {
			t.Fatalf("parseCycleClock(%q) = (%d, %d, %v), want (%d, %d, %v)", test.value, hour, minute, ok, test.hour, test.minute, test.ok)
		}
	}
}
