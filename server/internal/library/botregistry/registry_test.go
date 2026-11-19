package botregistry

import (
	"strings"
	"testing"
)

func TestTelegramBotID(t *testing.T) {
	for _, token := range []string{"123:original", " 00123:rotated "} {
		if id, err := TelegramBotID(token); err != nil || id != 123 {
			t.Fatal("expected stable numeric identity")
		}
	}
	for _, token := range []string{"0:private", "123:", "-1:private", "+1:private", "1x:private", "9223372036854775808:private", "123"} {
		if id, err := TelegramBotID(token); id != 0 || err == nil || strings.Contains(err.Error(), token) {
			t.Fatal("expected invalid identity without token disclosure")
		}
	}
}
