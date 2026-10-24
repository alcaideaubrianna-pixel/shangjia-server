package sys

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/os/gcache"

	"hotgo/internal/library/cache"
)

func TestTelegramLoginCodeCommandCrossInstance(t *testing.T) {
	ctx := context.Background()
	if !cache.Initialized() {
		cache.SetAdapter(ctx)
	}
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previous) })

	producer := new(sSysPublish)
	consumer := new(sSysPublish)
	if err := producer.submitTelegramLoginCode(ctx, " token ", 42, "12345"); err != nil {
		t.Fatalf("submit code: %v", err)
	}
	code, err := consumer.consumeTelegramLoginCode(ctx, "token", 42)
	if err != nil {
		t.Fatalf("consume code: %v", err)
	}
	if code != "12345" {
		t.Fatalf("code=%q, want 12345", code)
	}
	if code, err = consumer.consumeTelegramLoginCode(ctx, "token", 42); err != nil || code != "" {
		t.Fatalf("command must be consumed once: code=%q err=%v", code, err)
	}
}

func TestTelegramLoginCodeCommandKeyIsScoped(t *testing.T) {
	first := telegramLoginCodeCommandKey(" token ", 42)
	if first != "youban_publish:tg_login_code:42:token" {
		t.Fatalf("unexpected command key: %q", first)
	}
	if first == telegramLoginCodeCommandKey("token", 43) {
		t.Fatal("code command key must be scoped by account")
	}
}

func TestTelegramLoginPasswordCommandKeyIsScoped(t *testing.T) {
	first := telegramLoginPasswordCommandKey(" token ", 42)
	if first != "youban_publish:tg_login_password:42:token" {
		t.Fatalf("unexpected command key: %q", first)
	}
	if first == telegramLoginPasswordCommandKey("token", 43) {
		t.Fatal("password command key must be scoped by account")
	}
}
