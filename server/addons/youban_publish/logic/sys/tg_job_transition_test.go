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
		From: []JobStatus{telegramJobStatusSending}, To: telegramJobStatusRetryWait,
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
		from    []JobStatus
		to      JobStatus
		wantErr bool
	}{
		{name: "claim pending", from: []JobStatus{telegramJobStatusPending}, to: telegramJobStatusSending},
		{name: "claim retry", from: []JobStatus{telegramJobStatusRetryWait}, to: telegramJobStatusSending},
		{name: "send success", from: []JobStatus{telegramJobStatusSending}, to: telegramJobStatusSent},
		{name: "send retry", from: []JobStatus{telegramJobStatusSending}, to: telegramJobStatusRetryWait},
		{name: "send uncertain", from: []JobStatus{telegramJobStatusSending}, to: telegramJobStatusConfirming},
		{name: "confirm with account fallback", from: []JobStatus{telegramJobStatusConfirming}, to: telegramJobStatusSending},
		{name: "late confirmation", from: []JobStatus{telegramJobStatusFailed}, to: telegramJobStatusRetryWait},
		{name: "manually recover failed", from: []JobStatus{telegramJobStatusFailed}, to: telegramJobStatusPending},
		{name: "claim failed down notification", from: []JobStatus{telegramJobStatusFailed}, to: telegramJobStatusSending},
		{name: "terminal regression", from: []JobStatus{telegramJobStatusSent}, to: telegramJobStatusPending, wantErr: true},
		{name: "late retry reports sent", from: []JobStatus{telegramJobStatusRetryWait}, to: telegramJobStatusSent},
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
		From:         []JobStatus{telegramJobStatusSending},
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
		From:       []JobStatus{telegramJobStatusSending},
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

func TestTelegramJobTransitionProtectedColumns(t *testing.T) {
	for _, field := range []string{"status", "dispatch_status", "send_phase", " STATUS ", "SEND_PHASE"} {
		_, _, err := telegramJobTransitionUpdate(telegramJobTransition{From: []JobStatus{JobStatusSending}, To: JobStatusRetryWait, Additional: g.Map{field: "invalid"}})
		if err == nil {
			t.Fatalf("protected field %q accepted", field)
		}
	}
}
func TestTelegramJobTransitionTypedFields(t *testing.T) {
	for _, phase := range []SendPhase{SendPhaseEmpty, SendPhaseDisplayConfirmed, SendPhaseRelayPreparing, SendPhaseRelayReady, SendPhasePackageSending, SendPhasePackageConfirmed} {
		_, data, err := telegramJobTransitionUpdate(telegramJobTransition{From: []JobStatus{JobStatusSending}, To: JobStatusRetryWait, SendPhase: jobPhase(phase), DispatchStatus: jobDispatch(DispatchStatusIdle)})
		if err != nil || data["send_phase"] != phase.String() || data["dispatch_status"] != "idle" {
			t.Fatalf("phase %q data=%v err=%v", phase, data, err)
		}
	}
	for _, in := range []telegramJobTransition{
		{From: []JobStatus{JobStatusSending}, To: JobStatusRetryWait, SendPhase: jobPhase(SendPhase("invalid"))},
		{From: []JobStatus{JobStatusSending}, To: JobStatusRetryWait, DispatchStatus: jobDispatch(DispatchStatus("invalid"))},
		{From: []JobStatus{JobStatusSending}, To: JobStatusSent, DispatchStatus: jobDispatch(DispatchStatusIdle)},
	} {
		if _, _, err := telegramJobTransitionUpdate(in); err == nil {
			t.Fatalf("invalid typed transition accepted: %+v", in)
		}
	}
}

func TestTelegramMediaFallbackTransitionPreservesProgress(t *testing.T) {
	for _, tc := range []struct {
		purpose                 string
		snapshot, current, want SendPhase
	}{
		{"display", SendPhaseCleanupConfirmed, SendPhaseDisplaySending, SendPhaseEmpty},
		{"verify", SendPhaseDisplayConfirmed, SendPhaseVerifySending, SendPhaseDisplayConfirmed},
	} {
		in := telegramMediaFallbackTransition(telegramJobRecord{Id: 42, SendPhase: tc.snapshot}, tc.purpose)
		_, data, err := telegramJobTransitionUpdate(in)
		if err != nil {
			t.Fatal(err)
		}
		if in.JobId != 42 || len(in.SendPhases) != 1 || in.SendPhases[0] != tc.current {
			t.Fatalf("missing CAS predicate: %+v", in)
		}
		if data["status"] != "failed_retry" || data["dispatch_status"] != "idle" || data["send_phase"] != tc.want.String() || data["next_retry_at"] == nil {
			t.Fatalf("incorrect fallback state: %v", data)
		}
	}
}
func TestTelegramJobTransitionRejectsUnknownPhasePredicate(t *testing.T) {
	_, _, err := telegramJobTransitionUpdate(telegramJobTransition{From: []JobStatus{JobStatusSending}, To: JobStatusRetryWait, SendPhases: []SendPhase{SendPhase("invalid")}})
	if err == nil {
		t.Fatal("invalid phase predicate accepted")
	}
}
