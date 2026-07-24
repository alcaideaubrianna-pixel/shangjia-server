package sys

import "testing"

func TestNeedMediaRepairCount(t *testing.T) {
	tests := []struct {
		name               string
		expectedMediaCount int
		actualMediaCount   int
		want               bool
	}{
		{name: "no media", expectedMediaCount: 0, actualMediaCount: 0, want: false},
		{name: "empty actual", expectedMediaCount: 3, actualMediaCount: 0, want: true},
		{name: "same count", expectedMediaCount: 2, actualMediaCount: 2, want: false},
		{name: "different count", expectedMediaCount: 2, actualMediaCount: 1, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needMediaRepairCount(tt.expectedMediaCount, tt.actualMediaCount)
			if got != tt.want {
				t.Fatalf("unexpected result: got %v want %v", got, tt.want)
			}
		})
	}
}
