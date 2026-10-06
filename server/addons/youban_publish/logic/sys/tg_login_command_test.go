package sys

import (
	"testing"
)

func TestTelegramLoginPasswordCommandKeyIsScoped(t *testing.T) {
	first := telegramLoginPasswordCommandKey(" token ", 42)
	if first != "youban_publish:tg_login_password:42:token" {
		t.Fatalf("unexpected command key: %q", first)
	}
	if first == telegramLoginPasswordCommandKey("token", 43) {
		t.Fatal("password command key must be scoped by account")
	}
}
