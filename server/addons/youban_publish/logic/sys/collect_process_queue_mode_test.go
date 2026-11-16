package sys

import "testing"

func TestNormalizeCollectProcessOutboxMode(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{"", "redis_first"},
		{"redis_first", "redis_first"},
		{" REDIS_FIRST ", "redis_first"},
		{"postgres_outbox", "postgres_outbox"},
		{"outbox", "postgres_outbox"},
		{"legacy", "postgres_outbox"},
		{"unknown", "redis_first"},
	} {
		if got := normalizeCollectProcessOutboxMode(test.input); got != test.want {
			t.Fatalf("mode %q: got %q, want %q", test.input, got, test.want)
		}
	}
}

func TestCollectProcessOutboxBoundary(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		payload collectProcessQueuePayload
		want    bool
	}{
		{name: "redis first realtime", mode: "redis_first", payload: collectProcessQueuePayload{EventId: 1, History: false}, want: false},
		{name: "redis first history", mode: "redis_first", payload: collectProcessQueuePayload{EventId: 2, History: true}, want: false},
		{name: "redis first old lease", mode: "redis_first", payload: collectProcessQueuePayload{EventId: 3, OutboxOwner: "legacy-owner"}, want: true},
		{name: "explicit postgres mode", mode: "postgres_outbox", payload: collectProcessQueuePayload{EventId: 4}, want: true},
		{name: "legacy alias", mode: "legacy", payload: collectProcessQueuePayload{EventId: 5}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectProcessShouldPersistOutboxMode(test.mode, test.payload); got != test.want {
				t.Fatalf("persist outbox=%t, want %t", got, test.want)
			}
		})
	}
}
