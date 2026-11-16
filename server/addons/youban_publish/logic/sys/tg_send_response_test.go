package sys

import (
	"context"
	"errors"
	"testing"
)

func TestAmbiguousBotResponseCannotImmediatelySwitchTransport(t *testing.T) {
	s := &sSysPublish{}
	job := telegramJobRecord{RetryCount: 10}
	for _, cause := range []error{
		context.DeadlineExceeded,
		context.Canceled,
		errors.New("connection reset by peer"),
		errors.New("error decode response body for method sendMediaGroup"),
		errTelegramDeliveryUncertain,
	} {
		if s.shouldFallbackAfterBotFailure(context.Background(), job, cause) {
			t.Fatalf("ambiguous result must be reconciled before transport fallback: %v", cause)
		}
	}
}
