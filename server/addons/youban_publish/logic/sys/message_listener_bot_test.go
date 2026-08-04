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

func TestListenerBotNotifyInputKeepsSourceBot(t *testing.T) {
	in := listenerBotNotifyInput(17, "-100123456", "绑定成功")
	if in.BotId != 17 {
		t.Fatalf("notify bot id = %d, want 17", in.BotId)
	}
	if in.ChatId != "-100123456" {
		t.Fatalf("notify chat id = %q, want -100123456", in.ChatId)
	}
	if in.Text != "绑定成功" {
		t.Fatalf("notify text = %q, want 绑定成功", in.Text)
	}
}
