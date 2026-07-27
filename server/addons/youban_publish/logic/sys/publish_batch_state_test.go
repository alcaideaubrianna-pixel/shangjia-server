package sys

import "testing"

func TestResolvePublishBatchState(t *testing.T) {
	tests := []struct {
		name    string
		counts  publishBatchJobCounts
		done    bool
		status  string
		message string
	}{
		{name: "pending", counts: publishBatchJobCounts{Total: 2, Pending: 1, Sent: 1}},
		{name: "all sent", counts: publishBatchJobCounts{Total: 2, Sent: 2}, done: true, status: "completed"},
		{name: "partial failed", counts: publishBatchJobCounts{Total: 2, Sent: 1, Failed: 1}, done: true, status: "partial_failed", message: "TG任务完成但存在失败：成功1，失败1"},
		{name: "all failed", counts: publishBatchJobCounts{Total: 2, Failed: 2}, done: true, status: "failed", message: "TG任务完成但存在失败：成功0，失败2"},
		{name: "empty batch", counts: publishBatchJobCounts{}, done: true, status: "completed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			done, status, message, err := resolvePublishBatchState(test.counts)
			if err != nil {
				t.Fatalf("resolve batch state: %v", err)
			}
			if done != test.done || status != test.status || message != test.message {
				t.Fatalf("unexpected state: done=%v status=%q message=%q", done, status, message)
			}
		})
	}
}
