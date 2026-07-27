package sys

import (
	"testing"

	"hotgo/addons/youban_tg_bot_gateway/service"
)

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
