package sys

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"

	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
)

func TestCollectorFeatureSkipsNonMessageUpdates(t *testing.T) {
	feature := &botCollectorFeature{}
	for _, update := range []*models.Update{
		nil,
		{MyChatMember: &models.ChatMemberUpdated{}},
		{ChatMember: &models.ChatMemberUpdated{}},
		{ChatJoinRequest: &models.ChatJoinRequest{}},
		{CallbackQuery: &models.CallbackQuery{}},
		{MessageReaction: &models.MessageReactionUpdated{}},
	} {
		// No bindings, cache or database are needed for lifecycle events.
		handled, err := feature.HandleUpdate(context.Background(), gatewayservice.BotContext{}, update)
		if handled || err != nil {
			t.Fatalf("non-message update must bypass collection: handled=%v err=%v", handled, err)
		}
	}
}
