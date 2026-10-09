package sys

import "testing"

func TestFairCollectRecoverySourceOffset(t *testing.T) {
	tests := []struct {
		name        string
		sourceCount int
		limit       int
		round       int64
		want        int
	}{
		{name: "first window", sourceCount: 157, limit: 100, round: 0, want: 0},
		{name: "second window", sourceCount: 157, limit: 100, round: 1, want: 100},
		{name: "wraps windows", sourceCount: 157, limit: 100, round: 2, want: 0},
		{name: "short input", sourceCount: 2, limit: 100, round: 1, want: 0},
		{name: "invalid limit", sourceCount: 157, limit: 0, round: 1, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fairCollectRecoverySourceOffset(tt.sourceCount, tt.limit, tt.round); got != tt.want {
				t.Fatalf("fairCollectRecoverySourceOffset(%d, %d, %d) = %d, want %d", tt.sourceCount, tt.limit, tt.round, got, tt.want)
			}
		})
	}
}

func TestCollectRecoveryPerSourceLimit(t *testing.T) {
	tests := []struct {
		name        string
		totalLimit  int
		sourceCount int
		want        int
	}{
		{name: "all candidates get one slot", totalLimit: 100, sourceCount: 100, want: 1},
		{name: "quota shared across sources", totalLimit: 100, sourceCount: 25, want: 4},
		{name: "quota capped for few sources", totalLimit: 100, sourceCount: 2, want: 10},
		{name: "more sources than slots", totalLimit: 3, sourceCount: 10, want: 1},
		{name: "invalid total limit", totalLimit: 0, sourceCount: 10, want: 1},
		{name: "invalid source count", totalLimit: 100, sourceCount: 0, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collectRecoveryPerSourceLimit(tt.totalLimit, tt.sourceCount); got != tt.want {
				t.Fatalf("collectRecoveryPerSourceLimit(%d, %d) = %d, want %d", tt.totalLimit, tt.sourceCount, got, tt.want)
			}
		})
	}
}
