package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestParseAIOpsTelegramMessageURL(t *testing.T) {
	tests := []struct {
		raw, username, chat string
		message             int64
	}{
		{"https://t.me/KKbaoyang01/428685", "KKbaoyang01", "", 428685},
		{"https://t.me/c/4204753279/217733?single", "", "-1004204753279", 217733},
	}
	for _, test := range tests {
		ref, err := parseAIOpsTelegramMessageURL(test.raw)
		if err != nil {
			t.Fatalf("parse %q: %v", test.raw, err)
		}
		if ref.Username != test.username || ref.ChatId != test.chat || ref.MessageId != test.message {
			t.Fatalf("parse %q = %+v", test.raw, ref)
		}
	}
}

func TestParseAIOpsTelegramMessageContextInput(t *testing.T) {
	tests := []struct {
		name      string
		input     *sysin.AIOpsTelegramMessageContextInp
		username  string
		chatId    string
		messageId int64
	}{
		{name: "url", input: &sysin.AIOpsTelegramMessageContextInp{URL: "https://t.me/demo/12?single"}, username: "demo", messageId: 12},
		{name: "chat and message", input: &sysin.AIOpsTelegramMessageContextInp{ChatId: "4204753279", MessageId: 13}, chatId: "-1004204753279", messageId: 13},
		{name: "message only", input: &sysin.AIOpsTelegramMessageContextInp{MessageId: 14}, messageId: 14},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref, _, err := parseAIOpsTelegramMessageContextInput(test.input)
			if err != nil {
				t.Fatal(err)
			}
			if ref.Username != test.username || ref.ChatId != test.chatId || ref.MessageId != test.messageId {
				t.Fatalf("unexpected ref: %+v", ref)
			}
		})
	}
}

func TestParseAIOpsTelegramMessageContextInputRequiresTarget(t *testing.T) {
	if _, _, err := parseAIOpsTelegramMessageContextInput(&sysin.AIOpsTelegramMessageContextInp{}); err == nil {
		t.Fatal("empty input unexpectedly succeeded")
	}
}

func TestParseAIOpsTelegramMessageURLRejectsInvalidInput(t *testing.T) {
	for _, raw := range []string{
		"145606",
		"https://example.com/rzk881/145606",
		"https://t.me/rzk881/not-a-message",
		"https://t.me/c/not-a-channel/145606",
	} {
		if _, err := parseAIOpsTelegramMessageURL(raw); err == nil {
			t.Fatalf("parse %q unexpectedly succeeded", raw)
		}
	}
}
