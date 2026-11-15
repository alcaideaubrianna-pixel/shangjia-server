package sys

import "testing"

func TestTelegramJobStateEnums(t *testing.T) {
	if !telegramLifecycleStateOf(" sending ").OccupiesChannelExecution() {
		t.Fatal("sending must occupy channel execution")
	}
	if telegramLifecycleStateOf("failed_retry").OccupiesChannelExecution() {
		t.Fatal("failed_retry must not occupy channel execution")
	}
	if !telegramLifecycleStateOf("unknown").NeedsReconcile() {
		t.Fatal("unknown must require reconciliation")
	}
	if !telegramLifecycleStateOf("sent").IsTerminal() {
		t.Fatal("sent must be terminal")
	}
	if telegramLifecycleStateOf("invalid").Valid() {
		t.Fatal("invalid lifecycle must be rejected")
	}
}

func TestTelegramDispatchStateEnums(t *testing.T) {
	if !telegramDispatchStateOfValue("queued").IsInFlight() {
		t.Fatal("queued must be in flight")
	}
	if !telegramDispatchStateOfValue("processing").IsInFlight() {
		t.Fatal("processing must be in flight")
	}
	if !telegramDispatchStateOfValue("done").IsTerminal() {
		t.Fatal("done must be terminal")
	}
	if telegramDispatchStateOfValue("invalid").Valid() {
		t.Fatal("invalid dispatch state must be rejected")
	}
}

func TestTelegramSendPhaseStateEnums(t *testing.T) {
	if !telegramSendPhaseStateOf("display_sending").OccupiesChannelExecution() {
		t.Fatal("display_sending must occupy channel execution")
	}
	if telegramSendPhaseStateOf("verify_confirmed").OccupiesChannelExecution() {
		t.Fatal("verify_confirmed must not occupy channel execution")
	}
	if !telegramSendPhaseStateOf("verify_confirmed").IsConfirmed() {
		t.Fatal("verify_confirmed must be confirmed")
	}
}

func TestTelegramJobOccupiesChannelExecution(t *testing.T) {
	tests := []struct {
		name                    string
		status, phase, dispatch string
		want                    bool
	}{
		{name: "waiting retry", status: "failed_retry", dispatch: "idle", want: false},
		{name: "sending lifecycle", status: "sending", dispatch: "idle", want: true},
		{name: "unknown requires reconcile", status: "unknown", dispatch: "idle", want: true},
		{name: "active send phase", status: "pending", phase: "verify_sending", dispatch: "idle", want: true},
		{name: "processing dispatch", status: "pending", dispatch: "processing", want: true},
		{name: "completed", status: "sent", phase: "completed_without_verify", dispatch: "done", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramJobOccupiesChannelExecution(test.status, test.phase, test.dispatch); got != test.want {
				t.Fatalf("telegramJobOccupiesChannelExecution(%q,%q,%q)=%t, want %t",
					test.status, test.phase, test.dispatch, got, test.want)
			}
		})
	}
}

func TestTelegramJobSourcePriority(t *testing.T) {
	if telegramJobSourceStateOf("collect:1", 50).Priority() >= telegramJobSourceStateOf("cycle_batch:1", 50).Priority() {
		t.Fatal("collect must have higher priority than cycle")
	}
	if telegramJobSourceStateOf("cycle_batch:1", 50).Priority() >= telegramJobSourceStateOf("full_push:1", 50).Priority() {
		t.Fatal("cycle must have higher priority than full push")
	}
}
