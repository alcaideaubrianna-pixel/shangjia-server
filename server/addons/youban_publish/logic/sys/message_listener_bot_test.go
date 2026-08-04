package sys

import "testing"

func TestListenerBotChatType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "group", want: "group"},
		{input: "supergroup", want: "group"},
		{input: "channel", want: "channel"},
		{input: "private", want: ""},
		{input: "", want: ""},
	}
	for _, tt := range tests {
		if got := listenerBotChatType(tt.input); got != tt.want {
			t.Fatalf("listenerBotChatType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
