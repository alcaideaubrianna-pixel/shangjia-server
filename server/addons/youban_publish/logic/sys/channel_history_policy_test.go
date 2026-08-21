package sys

import "testing"

func TestShouldDeleteChannelHistory(t *testing.T) {
	tests := []struct {
		name            string
		preserveHistory bool
		messageCount    int
		want            bool
	}{
		{name: "preserve messages", preserveHistory: true, messageCount: 1, want: false},
		{name: "delete messages", preserveHistory: false, messageCount: 1, want: true},
		{name: "nothing to delete", preserveHistory: false, messageCount: 0, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldDeleteChannelHistory(test.preserveHistory, test.messageCount); got != test.want {
				t.Fatalf("shouldDeleteChannelHistory() = %v, want %v", got, test.want)
			}
		})
	}
}
