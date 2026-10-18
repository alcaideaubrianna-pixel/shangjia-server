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

func TestProviderBindingsIsolateFailureAndKeepPreviousBindings(t *testing.T) {
	broken := &clientLookupProvider{
		recordingGatewayProvider: recordingGatewayProvider{},
		err:                      errors.New("missing addon column"),
	}
	healthy := &clientLookupProvider{
		recordingGatewayProvider: recordingGatewayProvider{},
		items:                    []service.BotBinding{{Owner: "publish", ReferenceID: 7, Token: "123:publish"}},
	}
	broken.recordingGatewayProvider.name = "two-way"
	healthy.recordingGatewayProvider.name = "publish"
	previous := map[string][]service.BotBinding{
		tokenKey("456:relay"): {{Owner: "two-way", ReferenceID: 9, Token: "456:relay"}},
	}

	bindings, err := loadProviderBindings(context.Background(), []service.Provider{broken, healthy}, previous)
	if err != nil {
		t.Fatalf("loadProviderBindings() error = %v", err)
	}
	if len(bindings[tokenKey("123:publish")]) != 1 {
		t.Fatal("healthy publish binding was not loaded")
	}
	if len(bindings[tokenKey("456:relay")]) != 1 {
		t.Fatal("failed provider's previous binding was not retained")
	}
}

func TestProviderBindingsFailsWhenEveryProviderFails(t *testing.T) {
	broken := &clientLookupProvider{err: errors.New("database unavailable")}
	if _, err := loadProviderBindings(context.Background(), []service.Provider{broken}, nil); err == nil {
		t.Fatal("all-provider failure must be reported")
	}
}
