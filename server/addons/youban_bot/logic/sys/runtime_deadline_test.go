package sys

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestExpiredBotUpdateStopsBeforeDatabase(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	bot := NewSysBot()
	update := &models.Update{ID: 1, Message: &models.Message{ID: 1, Text: "开始使用", Chat: models.Chat{ID: 1, Type: models.ChatTypePrivate}}}
	if err := bot.handleUpdate(ctx, 1, update); !errors.Is(err, context.Canceled) {
		t.Fatalf("handleUpdate error=%v", err)
	}
	if _, err := bot.featureConfigs(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("featureConfigs error=%v", err)
	}
	if _, err := bot.dispatchFeature(ctx, 1, update.Message, update.Message.Text); !errors.Is(err, context.Canceled) {
		t.Fatalf("dispatchFeature error=%v", err)
	}
}
