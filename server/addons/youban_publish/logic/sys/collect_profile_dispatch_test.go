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

func TestLateVerificationRepairOperationIsStableFullRepublish(t *testing.T) {
	operationNo := lateVerificationRepublishOperationNo(123, 456)
	if operationNo != "verify-misbinding-repair:123:profile:456" {
		t.Fatalf("unexpected operation number: %s", operationNo)
	}
	if isInteractiveProfilePublishOperation(operationNo) {
		t.Fatal("late verification repair is automated and must not use the interactive fast path")
	}
}

func TestShouldQueueLateVerificationOnlyForOnlineProfile(t *testing.T) {
	if !shouldQueueLateVerification(1) {
		t.Fatal("online profile must allow late verification")
	}
	for _, status := range []int{0, 2, 3} {
		if shouldQueueLateVerification(status) {
			t.Fatalf("offline status %d must not trigger historical republish", status)
		}
	}
}
