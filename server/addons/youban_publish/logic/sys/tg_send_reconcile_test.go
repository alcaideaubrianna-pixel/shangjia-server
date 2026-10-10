package sys

import (
	"errors"
	"strings"
	"testing"
)

func TestTelegramJobPhaseMarkerStableAndDistinct(t *testing.T) {
	display := telegramJobPhaseMarker(13776, "display")
	if display != telegramJobPhaseMarker(13776, "display") {
		t.Fatal("marker must remain stable across retries")
	}
	if display == telegramJobPhaseMarker(13776, "verify") {
		t.Fatal("display and verify markers must differ")
	}
	if strings.Contains(display, "13776") {
		t.Fatal("marker must not expose job id")
	}
}

func TestTelegramAmbiguousDeliveryError(t *testing.T) {
	for _, message := range []string{"context deadline exceeded", "HTTP/2 GOAWAY", "cannot rewind body after connection loss", "closed pipe"} {
		if !isTelegramAmbiguousDeliveryError(errors.New(message)) {
			t.Fatalf("expected ambiguous error: %s", message)
		}
	}
	if isTelegramAmbiguousDeliveryError(errors.New("Bad Request: chat not found")) {
		t.Fatal("permanent API errors must not enter reconciliation")
	}
	if !isTelegramAmbiguousDeliveryError(telegramDeliveryUncertainError(errors.New("保存消息记录失败"))) {
		t.Fatal("post-delivery persistence errors must enter reconciliation")
	}
}

func TestTelegramSendPhaseHasDisplay(t *testing.T) {
	if !telegramSendPhaseHasDisplay(telegramSendPhaseVerifySending) {
		t.Fatal("verify phase must not resend display media")
	}
	if !telegramSendPhaseHasDisplay(telegramSendPhaseVerifyConfirmed) {
		t.Fatal("confirmed verify phase must never resend display media")
	}
	if !telegramSendPhaseHasDisplay(telegramSendPhaseCompletedNoVerify) {
		t.Fatal("completed without verify phase must never resend display media")
	}
	if telegramSendPhaseHasDisplay(telegramSendPhaseDisplaySending) {
		t.Fatal("unconfirmed display phase must be reconciled before reuse")
	}
}

func TestTelegramSendPhaseHasCleanup(t *testing.T) {
	completed := []string{
		telegramSendPhaseCleanupConfirmed,
		telegramSendPhaseDisplaySending,
		telegramSendPhaseDisplayConfirmed,
		telegramSendPhaseVerifySending,
		telegramSendPhaseVerifyConfirmed,
		telegramSendPhaseCompletedNoVerify,
	}
	for _, phase := range completed {
		if !telegramSendPhaseHasCleanup(phase) {
			t.Fatalf("phase %q must include completed cleanup", phase)
		}
	}
	for _, phase := range []string{"", telegramSendPhaseCleanupProcessing} {
		if telegramSendPhaseHasCleanup(phase) {
			t.Fatalf("phase %q must require cleanup", phase)
		}
	}
}

func TestTelegramJobCompletionError(t *testing.T) {
	tests := []struct {
		name               string
		phase              string
		verifyMediaCount   int
		verifyMessageCount int
		wantErr            bool
	}{
		{name: "display only truthful phase", phase: telegramSendPhaseCompletedNoVerify},
		{name: "legacy false verified phase", phase: telegramSendPhaseVerifyConfirmed, wantErr: true},
		{name: "verify media without message", phase: telegramSendPhaseVerifyConfirmed, verifyMediaCount: 1, wantErr: true},
		{name: "verify message without confirmed phase", phase: telegramSendPhaseDisplayConfirmed, verifyMediaCount: 1, verifyMessageCount: 1, wantErr: true},
		{name: "verified publish", phase: telegramSendPhaseVerifyConfirmed, verifyMediaCount: 1, verifyMessageCount: 1},
		{name: "late verify invalidates display only completion", phase: telegramSendPhaseCompletedNoVerify, verifyMediaCount: 1, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := telegramJobCompletionError(test.phase, test.verifyMediaCount, test.verifyMessageCount)
			if (err != nil) != test.wantErr {
				t.Fatalf("telegramJobCompletionError() err=%v, wantErr=%t", err, test.wantErr)
			}
		})
	}
}

func TestTelegramCleanupPhaseRetriesWithoutDeliveryReconcile(t *testing.T) {
	if !telegramSendPhaseIsCleanup(telegramSendPhaseCleanupProcessing) {
		t.Fatal("cleanup processing phase must be recognized as cleanup")
	}
	if telegramSendPhaseIsCleanup(telegramSendPhaseDisplaySending) {
		t.Fatal("display delivery must not be classified as cleanup")
	}
}
