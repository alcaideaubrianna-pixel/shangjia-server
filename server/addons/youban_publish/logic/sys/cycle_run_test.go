package sys

import "testing"

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
