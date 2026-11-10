package sys

import "testing"

func TestDecideProfileMediaReadiness(t *testing.T) {
	tests := []struct {
		name     string
		expected int
		actual   int
		pending  int
		failed   int
		ready    bool
	}{
		{name: "intentional text only", expected: 0, actual: 0, ready: true},
		{name: "operation not persisted yet", expected: 3, actual: 0, ready: false},
		{name: "media task pending", expected: 3, actual: 3, pending: 3, ready: false},
		{name: "media task processing", expected: 3, actual: 3, pending: 1, ready: false},
		{name: "failed media", expected: 3, actual: 3, failed: 1, ready: false},
		{name: "partial media rows", expected: 3, actual: 2, ready: false},
		{name: "all media ready", expected: 3, actual: 3, ready: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, _ := decideProfileMediaReadiness(tt.expected, tt.actual, tt.pending, tt.failed)
			if ready != tt.ready {
				t.Fatalf("ready=%t, want %t", ready, tt.ready)
			}
		})
	}
}
