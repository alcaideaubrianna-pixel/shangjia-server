package sys

import "testing"

func TestTaskBatchSummaryStatus(t *testing.T) {
	tests := []struct {
		name string
		in   taskBatchSummary
		want string
	}{
		{"empty", taskBatchSummary{}, taskStatusPending},
		{"pending", taskBatchSummary{Total: 3, Pending: 2, Success: 1}, taskStatusRunning},
		{"running", taskBatchSummary{Total: 3, Running: 1, Success: 2}, taskStatusRunning},
		{"unknown", taskBatchSummary{Total: 2, Success: 1, Unknown: 1}, taskStatusUnknown},
		{"success", taskBatchSummary{Total: 2, Success: 2}, taskStatusSuccess},
		{"cancelled", taskBatchSummary{Total: 2, Cancelled: 2}, taskStatusCancelled},
		{"failed", taskBatchSummary{Total: 2, Success: 1, Failed: 1}, taskStatusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.status(); got != tt.want {
				t.Fatalf("status()=%q, want %q", got, tt.want)
			}
		})
	}
}
