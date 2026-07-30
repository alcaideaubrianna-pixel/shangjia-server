package sys

import (
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

func TestCollectMediaShouldReconnectAccount(t *testing.T) {
	for _, test := range []struct {
		message string
		want    bool
	}{
		{message: "rpc error: FILE_MIGRATE", want: true},
		{message: "get next chunk: DC is closed", want: true},
		{message: "file_reference_expired", want: false},
		{message: "unsupported media", want: false},
	} {
		if got := collectMediaShouldReconnectAccount(errors.New(test.message)); got != test.want {
			t.Fatalf("message %q: got %t want %t", test.message, got, test.want)
		}
	}
}
