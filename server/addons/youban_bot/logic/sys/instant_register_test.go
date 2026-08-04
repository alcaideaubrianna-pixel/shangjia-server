package sys

import (
	"errors"
	"strings"
	"testing"
)

func TestInstantRegisterMessageContainsClickableURL(t *testing.T) {
	message := instantRegisterMessage(
		"<b>注册链接已生成</b>",
		"WXEG0QDKBGPK",
		"2026-08-04 15:01:51",
		"https://example.com/auth/register?inviteCode=WXEG0QDKBGPK&from=tg",
		"打开注册页面",
		true,
	)
	for _, expected := range []string{
		`<a href="https://example.com/auth/register?inviteCode=WXEG0QDKBGPK&amp;from=tg">打开注册页面</a>`,
		"<code>WXEG0QDKBGPK</code>",
		"2026-08-04 15:01:51",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("message should contain %q, got %q", expected, message)
		}
	}
}

func TestInstantRegisterMessageUsesPlainLocalURL(t *testing.T) {
	message := instantRegisterMessage(
		"注册链接已生成",
		"7AUKTNEGI2U2",
		"2026-08-04 15:18:36",
		"http://localhost:5999/auth/register?inviteCode=7AUKTNEGI2U2",
		"打开注册页面",
		false,
	)
	if strings.Contains(message, "<a href=") {
		t.Fatalf("local URL should not use Telegram anchor: %q", message)
	}
	if !strings.Contains(message, "http://localhost:5999/auth/register?inviteCode=7AUKTNEGI2U2") {
		t.Fatalf("local URL should remain visible in message: %q", message)
	}
}

func TestTelegramInlineButtonURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{url: "https://example.com/auth/register", want: "https://example.com/auth/register"},
		{url: "http://localhost:5999/auth/register?inviteCode=ABC", want: "http://127.0.0.1.nip.io:5999/auth/register?inviteCode=ABC"},
		{url: "http://127.0.0.1:5999/auth/register", want: "http://127.0.0.1.nip.io:5999/auth/register"},
		{url: "http://192.168.1.10/auth/register", want: ""},
		{url: "/auth/register", want: ""},
	}
	for _, test := range tests {
		if got := telegramInlineButtonURL(test.url); got != test.want {
			t.Fatalf("telegramInlineButtonURL(%q)=%q, want %q", test.url, got, test.want)
		}
	}
}

func TestTelegramInlineButtonURLInvalid(t *testing.T) {
	if !telegramInlineButtonURLInvalid(errors.New("Bad Request: inline keyboard button URL is invalid: Wrong HTTP URL")) {
		t.Fatal("Telegram invalid button URL error should be recognized")
	}
	if telegramInlineButtonURLInvalid(errors.New("request timeout")) {
		t.Fatal("unrelated errors should not be recognized as invalid button URL")
	}
}
