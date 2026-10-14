package sys

import "testing"

func TestTelegramDeliveryStepsForPhase(t *testing.T) {
	tests := []struct {
		phase    string
		expected map[string]string
	}{
		{phase: telegramSendPhaseCleanupProcessing, expected: map[string]string{telegramDeliveryStepCleanup: telegramDeliveryStepRunning}},
		{phase: telegramSendPhaseDisplaySending, expected: map[string]string{telegramDeliveryStepCleanup: telegramDeliveryStepSucceeded, telegramDeliveryStepDisplay: telegramDeliveryStepPending}},
		{phase: telegramSendPhaseVerifySending, expected: map[string]string{telegramDeliveryStepDisplay: telegramDeliveryStepPending, telegramDeliveryStepVerify: telegramDeliveryStepPending}},
		{phase: telegramSendPhaseVerifyConfirmed, expected: map[string]string{telegramDeliveryStepVerify: telegramDeliveryStepPending, telegramDeliveryStepFinalize: telegramDeliveryStepPending}},
		{phase: telegramSendPhaseCompletedNoVerify, expected: map[string]string{telegramDeliveryStepDisplay: telegramDeliveryStepPending, telegramDeliveryStepVerify: telegramDeliveryStepSkipped, telegramDeliveryStepFinalize: telegramDeliveryStepPending}},
	}
	for _, test := range tests {
		t.Run(test.phase, func(t *testing.T) {
			actual := make(map[string]string)
			for _, step := range telegramDeliveryStepsForPhase(test.phase) {
				actual[step.StepKey] = step.Status
			}
			if len(actual) != 4 {
				t.Fatalf("step count=%d, want 4", len(actual))
			}
			for step, status := range test.expected {
				if actual[step] != status {
					t.Fatalf("step %s status=%s, want %s", step, actual[step], status)
				}
			}
		})
	}
}

func TestTelegramDeliveryStepForAttempt(t *testing.T) {
	tests := []struct {
		phase, attemptStatus string
		step, status         string
	}{
		{phase: "display", attemptStatus: telegramAttemptStatusSending, step: telegramDeliveryStepDisplay, status: telegramDeliveryStepRunning},
		{phase: "verify", attemptStatus: telegramAttemptStatusWaiting, step: telegramDeliveryStepVerify, status: telegramDeliveryStepConfirming},
		{phase: "verify", attemptStatus: telegramAttemptStatusConfirmed, step: telegramDeliveryStepVerify, status: telegramDeliveryStepSucceeded},
		{phase: "display", attemptStatus: telegramAttemptStatusRetryWait, step: telegramDeliveryStepDisplay, status: telegramDeliveryStepRetryWait},
		{phase: "cleanup", attemptStatus: telegramAttemptStatusSending},
	}
	for _, test := range tests {
		step, status := telegramDeliveryStepForAttempt(telegramDeliveryAttempt{Phase: test.phase, Status: test.attemptStatus})
		if step != test.step || status != test.status {
			t.Fatalf("phase=%s attempt=%s got=(%s,%s), want=(%s,%s)", test.phase, test.attemptStatus, step, status, test.step, test.status)
		}
	}
}
