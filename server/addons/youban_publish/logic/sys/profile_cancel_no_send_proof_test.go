package sys

import (
	"testing"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestCanceledAttemptDefiniteRejectionSurvivesSuperseding(t *testing.T) {
	for _, proof := range []string{
		"bot_definite_rejection", "bot_preparation_failed_before_send",
		"protocol_definite_rejection", "protocol_preparation_failed_before_send",
	} {
		for _, status := range []string{telegramAttemptStatusRetryWait, telegramAttemptStatusSuperseded} {
			attempt := telegramDeliveryAttempt{Id: 1, Status: status, ErrorMessage: proof}
			if !canceledAttemptHasDefiniteNoSendProof(attempt, 0) {
				t.Fatalf("lost canonical no-send proof after lifecycle change: %s/%s", status, proof)
			}
		}
	}
	base := telegramDeliveryAttempt{Id: 1, Status: telegramAttemptStatusSuperseded, ErrorMessage: "bot_definite_rejection"}
	for _, tc := range []struct {
		name  string
		alter func(*telegramDeliveryAttempt)
		audit int
	}{
		{"audited message", func(*telegramDeliveryAttempt) {}, 1},
		{"received response", func(a *telegramDeliveryAttempt) { a.ResponseReceived = 1 }, 0},
		{"confirmed message", func(a *telegramDeliveryAttempt) { a.ConfirmedCount = 1 }, 0},
		{"pending webhook", func(a *telegramDeliveryAttempt) { a.WebhookDeadline = gtime.Now() }, 0},
		{"unknown lifecycle", func(a *telegramDeliveryAttempt) { a.Status = "unknown" }, 0},
		{"missing attempt", func(a *telegramDeliveryAttempt) { a.Id = 0 }, 0},
		{"unproven error", func(a *telegramDeliveryAttempt) { a.ErrorMessage = "request timed out" }, 0},
		{"substring is not proof", func(a *telegramDeliveryAttempt) { a.ErrorMessage = "unexpected bot_definite_rejection" }, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempt := base
			tc.alter(&attempt)
			if canceledAttemptHasDefiniteNoSendProof(attempt, tc.audit) {
				t.Fatal("uncertain or contradictory evidence must keep cancellation waiting")
			}
		})
	}
}
