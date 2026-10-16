package sys

import (
	"context"
	"errors"
	"testing"

	"hotgo/addons/youban_tg_bot_gateway/service"
)

type clientLookupProvider struct {
	recordingGatewayProvider
	items []service.BotBinding
	err   error
}

func (p *clientLookupProvider) ListEnabledBots(context.Context) ([]service.BotBinding, error) {
	return p.items, p.err
}

func TestClientBindingsIsolateUnrelatedProviderFailure(t *testing.T) {
	broken := &clientLookupProvider{err: errors.New("missing provider column")}
	healthy := &clientLookupProvider{items: []service.BotBinding{{Token: "123:test"}}}
	for _, providers := range [][]service.Provider{{broken, healthy}, {healthy, broken}} {
		bindings, err := loadClientBindings(context.Background(), tokenKey("123:test"), providers)
		if err != nil || len(bindings[tokenKey("123:test")]) != 1 {
			t.Fatalf("healthy Bot unavailable: bindings=%d err=%v", len(bindings), err)
		}
	}
	if _, err := loadClientBindings(context.Background(), tokenKey("missing"), []service.Provider{broken, healthy}); err == nil {
		t.Fatal("missing Bot must not be authorized")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loadClientBindings(ctx, tokenKey("123:test"), []service.Provider{broken, healthy}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}
