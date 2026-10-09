package sys

import "testing"

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
