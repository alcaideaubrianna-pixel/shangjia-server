package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

func TestValidateTelegramJobTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    []string
		to      string
		wantErr bool
	}{
		{name: "claim pending", from: []string{telegramJobStatusPending}, to: telegramJobStatusSending},
		{name: "claim retry", from: []string{telegramJobStatusRetryWait}, to: telegramJobStatusSending},
		{name: "send success", from: []string{telegramJobStatusSending}, to: telegramJobStatusSent},
		{name: "send retry", from: []string{telegramJobStatusSending}, to: telegramJobStatusRetryWait},
		{name: "send uncertain", from: []string{telegramJobStatusSending}, to: telegramJobStatusConfirming},
		{name: "confirm with account fallback", from: []string{telegramJobStatusConfirming}, to: telegramJobStatusSending},
		{name: "late confirmation", from: []string{telegramJobStatusFailed}, to: telegramJobStatusRetryWait},
		{name: "manually recover failed", from: []string{telegramJobStatusFailed}, to: telegramJobStatusPending},
		{name: "claim failed down notification", from: []string{telegramJobStatusFailed}, to: telegramJobStatusSending},
		{name: "terminal regression", from: []string{telegramJobStatusSent}, to: telegramJobStatusPending, wantErr: true},
		{name: "late retry reports sent", from: []string{telegramJobStatusRetryWait}, to: telegramJobStatusSent},
		{name: "missing source", from: nil, to: telegramJobStatusSent, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTelegramJobTransition(test.from, test.to)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateTelegramJobTransition(%v, %q) error=%v, wantErr=%t", test.from, test.to, err, test.wantErr)
			}
		})
	}
}

func TestTelegramJobTransitionUpdate(t *testing.T) {
	from, data, err := telegramJobTransitionUpdate(telegramJobTransition{
		From:         []string{telegramJobStatusSending},
		To:           telegramJobStatusRetryWait,
		RetryDelay:   time.Minute,
		ErrorMessage: "temporary failure",
		Additional:   g.Map{"retry_count": 2},
	})
	if err != nil {
		t.Fatalf("telegramJobTransitionUpdate returned error: %v", err)
	}
	if len(from) != 1 || from[0] != telegramJobStatusSending {
		t.Fatalf("unexpected source statuses: %v", from)
	}
	if data["status"] != telegramJobStatusRetryWait || data["dispatch_status"] != tgDispatchStatusIdle {
		t.Fatalf("unexpected transition state: %#v", data)
	}
	if data["next_retry_at"] == nil || data["retry_count"] != 2 || data["error_message"] != "temporary failure" {
		t.Fatalf("transition metadata missing: %#v", data)
	}

	_, _, err = telegramJobTransitionUpdate(telegramJobTransition{
		From:       []string{telegramJobStatusSending},
		To:         telegramJobStatusSent,
		Additional: g.Map{"status": telegramJobStatusFailed},
	})
	if err == nil {
		t.Fatal("expected status override to be rejected")
	}
}
