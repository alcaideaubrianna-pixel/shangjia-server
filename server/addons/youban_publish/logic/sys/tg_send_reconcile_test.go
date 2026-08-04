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
	if telegramSendPhaseHasDisplay(telegramSendPhaseDisplaySending) {
		t.Fatal("unconfirmed display phase must be reconciled before reuse")
	}
}
