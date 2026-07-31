package sys

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestAccountCollectCircuitBackoff(t *testing.T) {
	service := NewSysPublish()

	first := service.openAccountCollectCircuit(nil, 7, errors.New("DC is closed"))
	if first < 4*time.Second || first > 6*time.Second {
		t.Fatalf("unexpected first circuit delay: %s", first)
	}
	second := service.openAccountCollectCircuit(nil, 7, errors.New("DC is closed"))
	if second < 9*time.Second || second > 16*time.Second {
		t.Fatalf("unexpected second circuit delay: %s", second)
	}
	if delay, blocked := service.accountCollectCircuitBlocked(7); !blocked || delay <= 0 {
		t.Fatalf("account circuit should be blocked, delay=%s blocked=%t", delay, blocked)
	}

	service.closeAccountCollectCircuit(7)
	if _, blocked := service.accountCollectCircuitBlocked(7); blocked {
		t.Fatal("account circuit should be closed after a successful connection")
	}
}

func TestAccountCollectCircuitTransientStateDoesNotStopWorkerStart(t *testing.T) {
	service := NewSysPublish()
	service.openAccountCollectCircuit(nil, 8, errors.New("DC is closed"))
	if !service.accountCollectCircuitShouldStart(8) {
		t.Fatal("transient connection state should allow worker restart")
	}
	service.openAccountCollectCircuit(nil, 8, errors.New("AUTH_BYTES_INVALID"))
	if service.accountCollectCircuitShouldStart(8) {
		t.Fatal("permanent auth state should stop worker start")
	}
}

func TestAccountCollectCircuitJSONRoundTrip(t *testing.T) {
	original := accountCollectCircuit{
		failures:     3,
		blockedUntil: time.Now().Add(time.Minute).Round(time.Millisecond),
		status:       "reconnecting",
		lastMessage:  "DC is closed",
		recoveries:   2,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "{}" {
		t.Fatal("circuit state must not serialize to an empty object")
	}
	var restored accountCollectCircuit
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.failures != original.failures || restored.status != original.status || restored.lastMessage != original.lastMessage || restored.recoveries != original.recoveries {
		t.Fatalf("restored circuit state mismatch: %#v", restored)
	}
}

func TestCollectMediaShouldReconnectAccount(t *testing.T) {
	for _, test := range []struct {
		message string
		want    bool
	}{
		{message: "rpc error: FILE_MIGRATE", want: false},
		{message: "get next chunk: DC is closed", want: true},
		{message: "file_reference_expired", want: false},
		{message: "unsupported media", want: false},
	} {
		if got := collectMediaShouldReconnectAccount(errors.New(test.message)); got != test.want {
			t.Fatalf("message %q: got %t want %t", test.message, got, test.want)
		}
	}
}
