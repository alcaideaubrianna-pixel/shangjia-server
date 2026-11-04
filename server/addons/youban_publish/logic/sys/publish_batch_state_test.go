package sys

import "testing"

func TestPublishBatchOperationNoUpperBound(t *testing.T) {
	tests := []struct {
		prefix string
		upper  string
		valid  bool
	}{
		{prefix: "cycle_batch:123:", upper: "cycle_batch:123;", valid: true},
		{prefix: "full_push:12:345:", upper: "full_push:12:345;", valid: true},
		{prefix: "", valid: false},
		{prefix: "cycle_batch:123", valid: false},
	}
	for _, test := range tests {
		t.Run(test.prefix, func(t *testing.T) {
			upper, ok := publishBatchOperationNoUpperBound(test.prefix)
			if ok != test.valid || upper != test.upper {
				t.Fatalf("upper bound = %q, valid = %v; want %q, %v", upper, ok, test.upper, test.valid)
			}
		})
	}

	for _, prefix := range []string{"cycle_batch:123:", "full_push:12:345:"} {
		upper, _ := publishBatchOperationNoUpperBound(prefix)
		for _, operationNo := range []string{prefix, prefix + "1:88", prefix + "999"} {
			if operationNo < prefix || operationNo >= upper {
				t.Errorf("operation number %q is outside [%q, %q)", operationNo, prefix, upper)
			}
		}
	}
}

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
