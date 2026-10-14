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

func TestCollectContentHasVerificationRequiresVerifyVideo(t *testing.T) {
	tests := []struct {
		name    string
		content *collectContentResult
		want    bool
	}{
		{name: "nil"},
		{name: "display video", content: &collectContentResult{Media: []collectMediaItem{{Type: "video", Purpose: "display"}}}},
		{name: "verify image", content: &collectContentResult{Media: []collectMediaItem{{Type: "image", Purpose: "verify"}}}},
		{name: "verify video", content: &collectContentResult{Media: []collectMediaItem{{Type: "VIDEO", Purpose: "VERIFY"}}}, want: true},
	}
	for _, test := range tests {
		if got := collectContentHasVerification(test.content); got != test.want {
			t.Fatalf("%s: got %t want %t", test.name, got, test.want)
		}
	}
}

func TestLateVerificationRepairOperationIsStableAndNotManualRepublish(t *testing.T) {
	operationNo := lateVerificationRepairOperationNo(123, 456)
	if operationNo != "verify-repair:123:profile:456" {
		t.Fatalf("unexpected operation number: %s", operationNo)
	}
	if isManualProfilePublishOperation(operationNo) {
		t.Fatal("late verification repair must not replace existing display messages")
	}
}
