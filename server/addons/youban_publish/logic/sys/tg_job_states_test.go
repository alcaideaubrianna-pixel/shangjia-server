package sys

import "testing"

func TestStrongTelegramJobStates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parse func(string) (string, bool)
		value string
		want  string
	}{
		{"job", func(v string) (string, bool) { s, ok := ParseJobStatus(v); return string(s), ok }, " sent ", "sent"},
		{"dispatch", func(v string) (string, bool) { s, ok := ParseDispatchStatus(v); return string(s), ok }, "queued", "queued"},
		{"phase", func(v string) (string, bool) { s, ok := ParseSendPhase(v); return string(s), ok }, "verify_confirmed", "verify_confirmed"},
		{"block", func(v string) (string, bool) { s, ok := ParseBlockReason(v); return string(s), ok }, "media_not_ready", "media_not_ready"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.parse(tc.value)
			if !ok || got != tc.want {
				t.Fatalf("parse(%q)=(%q,%v), want (%q,true)", tc.value, got, ok, tc.want)
			}
		})
	}
	if _, ok := ParseJobStatus("invalid"); ok {
		t.Fatal("invalid job status accepted")
	}
}

func TestStrongTelegramJobStatePredicates(t *testing.T) {
	if !JobStatusSent.IsTerminal() || JobStatusSent.IsRetryable() {
		t.Fatal("sent status predicates are incorrect")
	}
	if !JobStatusRetryWait.IsRetryable() || JobStatusRetryWait.IsTerminal() {
		t.Fatal("retry status predicates are incorrect")
	}
	if !DispatchStatusQueued.IsRetryable() || !SendPhaseDisplaySending.IsRetryable() {
		t.Fatal("retryable state predicates are incorrect")
	}
	if !SendPhaseVerifyConfirmed.IsTerminal() || !SendPhaseDisplaySending.IsBlocking() {
		t.Fatal("send phase predicates are incorrect")
	}
	if !BlockReasonMedia.IsBlocking() || BlockReasonNone.IsBlocking() {
		t.Fatal("block reason predicates are incorrect")
	}
}
