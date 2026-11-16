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
