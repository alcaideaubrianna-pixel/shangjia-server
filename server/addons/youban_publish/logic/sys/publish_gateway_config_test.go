package sys

import (
	"strings"
	"testing"

	"hotgo/addons/youban_publish/model"
)

func TestTelegramGatewayRuntimeConfigDefaultPreservesOfficialRuntime(t *testing.T) {
	conf := &model.TelegramConfig{
		BotRuntimeApiUrl: "https://api.telegram.org",
		BotApiServerUrl:  "http://local-bot-api.internal",
	}
	got, err := telegramGatewayRuntimeConfig(conf)
	if err != nil {
		t.Fatal(err)
	}
	if got.LocalBotEnabled || got.RuntimeServerURL != conf.BotRuntimeApiUrl || got.MediaServerURL != conf.BotApiServerUrl {
		t.Fatalf("unexpected default runtime config: %+v", got)
	}
}

func TestTelegramGatewayRuntimeConfigLocalModeUsesOneEndpoint(t *testing.T) {
	conf := &model.TelegramConfig{
		BotRuntimeApiUrl: "https://api.telegram.org",
		BotApiServerUrl:  "http://local-bot-api.internal",
		LocalBotEnabled:  true,
	}
	got, err := telegramGatewayRuntimeConfig(conf)
	if err != nil {
		t.Fatal(err)
	}
	if !got.LocalBotEnabled || got.RuntimeServerURL != conf.BotApiServerUrl || got.MediaServerURL != conf.BotApiServerUrl {
		t.Fatalf("local mode endpoints were not unified: %+v", got)
	}
}

func TestTelegramGatewayRuntimeConfigRejectsMissingLocalEndpoint(t *testing.T) {
	_, err := telegramGatewayRuntimeConfig(&model.TelegramConfig{LocalBotEnabled: true})
	if err == nil || !strings.Contains(err.Error(), "YOUBAN_TELEGRAM_BOT_API_URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}
