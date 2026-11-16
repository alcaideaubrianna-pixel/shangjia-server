package sys

import "testing"

func TestTelegramPhaseWriteCannotRegressConfirmedDelivery(t *testing.T) {
	contains := func(target, source SendPhase) bool {
		for _, candidate := range telegramPhaseWriteSources(target) {
			if candidate == source {
				return true
			}
		}
		return false
	}
	for _, target := range []SendPhase{SendPhaseEmpty, SendPhaseCleanupProcessing, SendPhaseCleanupConfirmed, SendPhaseDisplaySending, SendPhaseDisplayConfirmed, SendPhaseRelayReady} {
		if contains(target, SendPhaseVerifyConfirmed) || contains(target, SendPhaseCompletedNoVerify) {
			t.Fatalf("late %s write can regress a completed delivery", target)
		}
	}
	if contains(SendPhaseCleanupConfirmed, SendPhaseDisplayConfirmed) || contains(SendPhaseDisplayConfirmed, SendPhaseVerifySending) {
		t.Fatal("late phase write can regress an in-flight verification")
	}
	if !contains(SendPhaseVerifyConfirmed, SendPhaseDisplayConfirmed) || !contains(SendPhaseCompletedNoVerify, SendPhaseDisplayConfirmed) || !contains(SendPhaseVerifyConfirmed, SendPhaseVerifyConfirmed) {
		t.Fatal("valid completion or idempotent confirmation was rejected")
	}
	if !contains(SendPhaseRelayReady, SendPhasePackageSending) {
		t.Fatal("legacy pre-confirmation relay retry was removed")
	}
}
