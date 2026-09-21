package sys

import "testing"

func TestTerminalMediaProcessingStatuses(t *testing.T) {
	statuses := terminalMediaProcessingStatuses()
	want := map[string]bool{
		mediaProcessingReady:  true,
		mediaProcessingFailed: true,
	}
	if len(statuses) != len(want) {
		t.Fatalf("terminal status count = %d, want %d", len(statuses), len(want))
	}
	for _, status := range statuses {
		if !want[status] {
			t.Fatalf("unexpected terminal media status %q", status)
		}
	}
}
