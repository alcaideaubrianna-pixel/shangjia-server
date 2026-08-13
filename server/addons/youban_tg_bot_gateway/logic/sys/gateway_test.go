package sys

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot/models"

	"hotgo/addons/youban_tg_bot_gateway/service"
)

type failingGatewayFeature struct{}

func (f *failingGatewayFeature) Key() string   { return "failing-test-feature" }
func (f *failingGatewayFeature) Priority() int { return 100 }
func (f *failingGatewayFeature) Menus(context.Context, service.BotContext) (service.FeatureMenus, error) {
	return service.FeatureMenus{}, nil
}
func (f *failingGatewayFeature) HandleUpdate(context.Context, service.BotContext, *models.Update) (bool, error) {
	return false, errors.New("feature failed")
}

type recordingGatewayProvider struct{ called bool }

func (p *recordingGatewayProvider) Name() string { return "recording-test-provider" }
func (p *recordingGatewayProvider) ListEnabledBots(context.Context) ([]service.BotBinding, error) {
	return nil, nil
}
func (p *recordingGatewayProvider) HandleUpdate(context.Context, service.BotBinding, *models.Update) error {
	p.called = true
	return nil
}

func TestGatewayRefreshCoalescesSignals(t *testing.T) {
	gateway := NewGateway()
	for range 3 {
		if err := gateway.Refresh(context.Background()); err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
	}
	if got := len(gateway.refresh); got != 1 {
		t.Fatalf("refresh signals = %d, want 1", got)
	}
}

func TestRuntimeModeForSystem(t *testing.T) {
	tests := []struct {
		name       string
		systemMode string
		config     *service.RuntimeConfig
		want       string
	}{
		{name: "develop auto uses polling", systemMode: "develop", config: &service.RuntimeConfig{Mode: "auto", WebhookBaseURL: "https://example.com"}, want: "pull"},
		{name: "production auto uses webhook", systemMode: "production", config: &service.RuntimeConfig{Mode: "auto", WebhookBaseURL: "https://example.com"}, want: "webhook"},
		{name: "production auto without public url uses polling", systemMode: "production", config: &service.RuntimeConfig{Mode: "auto", WebhookBaseURL: "http://localhost:8000"}, want: "pull"},
		{name: "invalid explicit webhook uses polling", systemMode: "production", config: &service.RuntimeConfig{Mode: "webhook", WebhookBaseURL: ""}, want: "pull"},
		{name: "explicit polling remains polling", systemMode: "production", config: &service.RuntimeConfig{Mode: "polling"}, want: "pull"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeModeForSystem(test.systemMode, test.config); got != test.want {
				t.Fatalf("runtimeModeForSystem() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNormalizeMenuItems(t *testing.T) {
	commands, signature, err := normalizeMenuItems([]service.MenuItem{
		{Command: "/chat", Description: "双向聊天", Order: 20},
		{Command: "cooperation", Description: "平台合作", Order: 10},
		{Command: "chat", Description: "双向聊天", Order: 30},
	})
	if err != nil {
		t.Fatalf("normalizeMenuItems() error = %v", err)
	}
	if len(commands) != 2 || commands[0].Command != "cooperation" || commands[1].Command != "chat" {
		t.Fatalf("normalizeMenuItems() commands = %#v", commands)
	}
	if signature != "cooperation=平台合作\nchat=双向聊天" {
		t.Fatalf("normalizeMenuItems() signature = %q", signature)
	}
}

func TestNormalizeMenuItemsRejectsConflict(t *testing.T) {
	_, _, err := normalizeMenuItems([]service.MenuItem{
		{Command: "chat", Description: "双向聊天"},
		{Command: "/chat", Description: "其他功能"},
	})
	if err == nil {
		t.Fatal("normalizeMenuItems() expected conflict error")
	}
}

func TestDispatchRunsProviderWhenFeatureFails(t *testing.T) {
	feature := &failingGatewayFeature{}
	provider := &recordingGatewayProvider{}
	service.RegisterFeature(feature)
	service.RegisterProvider(provider)
	gateway := NewGateway()
	gateway.bindings["test-key"] = []service.BotBinding{{Owner: provider.Name()}}

	err := gateway.dispatch(t.Context(), "test-key", &models.Update{ID: 1})
	if err == nil {
		t.Fatal("dispatch() expected feature error")
	}
	if !provider.called {
		t.Fatal("dispatch() did not run provider after feature failure")
	}
}
