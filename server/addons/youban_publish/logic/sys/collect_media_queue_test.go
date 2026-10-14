package sys

import "testing"

func TestCollectMediaQueueShardKey(t *testing.T) {
	tests := []struct {
		name    string
		payload collectMediaQueuePayload
		want    int64
	}{
		{name: "account source", payload: collectMediaQueuePayload{TgAccountId: 74, SourceId: 364}, want: 74},
		{name: "bot source", payload: collectMediaQueuePayload{SourceId: 351}, want: 351},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectMediaQueueShardKey(test.payload); got != test.want {
				t.Fatalf("collectMediaQueueShardKey()=%d, want %d", got, test.want)
			}
		})
	}
}

func TestNormalizeCollectMediaOwnerConcurrency(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{input: 0, want: 1},
		{input: 2, want: 2},
		{input: 99, want: collectMediaMaxOwnerConcurrency},
	}
	for _, test := range tests {
		if got := normalizeCollectMediaOwnerConcurrency(test.input); got != test.want {
			t.Fatalf("normalizeCollectMediaOwnerConcurrency(%d)=%d, want %d", test.input, got, test.want)
		}
	}
}
