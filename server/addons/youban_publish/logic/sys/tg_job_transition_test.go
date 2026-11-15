package sys

import (
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

func TestTelegramJobTransitionLongDispatchError(t *testing.T) {
	message := strings.Repeat("数据库错误", 200)
	_, data, err := telegramJobTransitionUpdate(telegramJobTransition{
		From: []string{telegramJobStatusSending}, To: telegramJobStatusRetryWait,
		ErrorMessage: message, Additional: g.Map{"last_dispatch_error": message},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := data["last_dispatch_error"].(string); len([]rune(got)) > 512 {
		t.Fatalf("dispatch error exceeds varchar(512): %d", len([]rune(got)))
	}
	if data["error_message"] != message {
		t.Fatal("full error must remain available in text field")
	}
}

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

func TestTelegramJobLifecyclePredicates(t *testing.T) {
	for _, status := range []string{telegramJobStatusSent, telegramJobStatusFailed, telegramJobStatusSuperseded} {
		if !telegramJobLifecycleTerminal(status) {
			t.Fatalf("status %q should be terminal", status)
		}
	}
	for _, status := range []string{telegramJobStatusSending, telegramJobStatusConfirming} {
		if telegramJobLifecycleCanBeOvertaken(status) || !telegramJobLifecycleOccupiesChannel(status) {
			t.Fatalf("status %q has inconsistent channel lifecycle predicates", status)
		}
	}
	for _, status := range []string{telegramJobStatusPending, telegramJobStatusRetryWait} {
		if !telegramJobLifecycleNeedsRetry(status) || !telegramJobLifecycleCanBeOvertaken(status) {
			t.Fatalf("status %q should be retryable and overtaken", status)
		}
	}
	if !telegramJobLifecycleNeedsReconcile(telegramJobStatusConfirming) {
		t.Fatal("unknown/confirming status should require reconciliation")
	}
}

func TestTelegramJobSourceClassification(t *testing.T) {
	cases := []struct {
		operation string
		priority  int
		want      telegramJobSource
	}{
		{operation: "collect:123", want: telegramSourceCollect},
		{operation: "cycle_batch:1:2:3", want: telegramSourceCycle},
		{operation: "full_push:1:2", want: telegramSourceFullPush},
		{operation: "history:1", want: telegramSourceHistory},
		{operation: "down:1", want: telegramSourceCleanup},
		{operation: "manual:1", priority: tgJobPriorityInteractive, want: telegramSourceManual},
	}
	for _, test := range cases {
		if got := telegramJobSourceOf(test.operation, test.priority); got != test.want {
			t.Fatalf("operation=%q priority=%d got=%q want=%q", test.operation, test.priority, got, test.want)
		}
	}
}
