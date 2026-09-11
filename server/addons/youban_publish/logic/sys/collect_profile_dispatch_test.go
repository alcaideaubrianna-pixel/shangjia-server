package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectDispatchResumeActionForStatus(t *testing.T) {
	tests := []struct {
		status string
		want   collectDispatchResumeAction
	}{
		{status: sysin.CollectDispatchStatusPending, want: collectDispatchResumeNoop},
		{status: sysin.CollectDispatchStatusFailed, want: collectDispatchResumeSubmit},
		{status: sysin.CollectDispatchStatusReviewing, want: collectDispatchResumeNoop},
		{status: sysin.CollectDispatchStatusSent, want: collectDispatchResumeNoop},
		{status: sysin.CollectDispatchStatusSkipped, want: collectDispatchResumeNoop},
		{status: "unknown", want: collectDispatchResumeInvalid},
	}
	for _, test := range tests {
		if got := collectDispatchResumeActionForStatus(test.status); got != test.want {
			t.Fatalf("status %q action = %d, want %d", test.status, got, test.want)
		}
	}
}
