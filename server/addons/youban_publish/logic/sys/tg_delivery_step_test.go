package sys

import "testing"

func TestTelegramDeliveryStepsForPhase(t *testing.T) {
	tests := []struct {
		phase    string
		expected map[string]string
	}{
		{phase: telegramSendPhaseCleanupProcessing, expected: map[string]string{telegramDeliveryStepCleanup: telegramDeliveryStepRunning}},
		{phase: telegramSendPhaseDisplaySending, expected: map[string]string{telegramDeliveryStepCleanup: telegramDeliveryStepSucceeded, telegramDeliveryStepDisplay: telegramDeliveryStepRunning}},
		{phase: telegramSendPhaseVerifySending, expected: map[string]string{telegramDeliveryStepDisplay: telegramDeliveryStepSucceeded, telegramDeliveryStepVerify: telegramDeliveryStepRunning}},
		{phase: telegramSendPhaseVerifyConfirmed, expected: map[string]string{telegramDeliveryStepVerify: telegramDeliveryStepSucceeded, telegramDeliveryStepFinalize: telegramDeliveryStepSucceeded}},
		{phase: telegramSendPhaseCompletedNoVerify, expected: map[string]string{telegramDeliveryStepDisplay: telegramDeliveryStepSucceeded, telegramDeliveryStepVerify: telegramDeliveryStepSkipped, telegramDeliveryStepFinalize: telegramDeliveryStepSucceeded}},
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
