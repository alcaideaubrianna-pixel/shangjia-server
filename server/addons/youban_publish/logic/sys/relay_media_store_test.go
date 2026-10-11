package sys

import (
	"testing"
	"time"
)

func TestRelayLeaseAvailable(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		status     string
		leaseUntil time.Time
		want       bool
	}{
		{name: "active lease", status: "uploading", leaseUntil: now.Add(time.Minute), want: true},
		{name: "expired lease", status: "uploading", leaseUntil: now.Add(-time.Second), want: false},
		{name: "missing lease", status: "uploading", want: false},
		{name: "ready record", status: "ready", leaseUntil: now.Add(time.Minute), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relayLeaseAvailable(tt.status, tt.leaseUntil, now); got != tt.want {
				t.Fatalf("relayLeaseAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}
