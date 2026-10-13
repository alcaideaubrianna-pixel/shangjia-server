package sys

import "testing"

func TestTelegramOutboxJobStillNeedsDispatch(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		dispatchStatus string
		want           bool
	}{
		{name: "pending idle", status: "pending", dispatchStatus: tgDispatchStatusIdle, want: true},
		{name: "retry empty", status: "failed_retry", dispatchStatus: "", want: true},
		{name: "unknown idle", status: "unknown", dispatchStatus: tgDispatchStatusIdle, want: true},
		{name: "queued", status: "pending", dispatchStatus: tgDispatchStatusQueued, want: false},
		{name: "processing", status: "pending", dispatchStatus: tgDispatchStatusProcessing, want: false},
		{name: "sent", status: "sent", dispatchStatus: tgDispatchStatusDone, want: false},
		{name: "failed", status: "failed", dispatchStatus: tgDispatchStatusDone, want: false},
		{name: "superseded", status: "superseded", dispatchStatus: tgDispatchStatusDone, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramOutboxJobStillNeedsDispatch(test.status, test.dispatchStatus); got != test.want {
				t.Fatalf("telegramOutboxJobStillNeedsDispatch(%q, %q)=%t, want %t", test.status, test.dispatchStatus, got, test.want)
			}
		})
	}
}
