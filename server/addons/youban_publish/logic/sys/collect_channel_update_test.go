package sys

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestCollectChannelUpdateFromWebhook(t *testing.T) {
	chat := models.Chat{ID: -1004444998796, Type: models.ChatTypeChannel, Title: "fixture"}
	member := &models.ChatMemberUpdated{
		Chat: chat, NewChatMember: models.ChatMember{Type: models.ChatMemberTypeAdministrator},
	}
	for _, tc := range []struct {
		name   string
		update *models.Update
		ok     bool
		member bool
		self   bool
	}{
		{"channel", &models.Update{ChannelPost: &models.Message{ID: 12, Chat: chat}}, true, false, false},
		{"edited channel", &models.Update{EditedChannelPost: &models.Message{ID: 12, Chat: chat}}, true, false, false},
		{"message", &models.Update{Message: &models.Message{ID: 12, Chat: chat}}, true, false, false},
		{"edited message", &models.Update{EditedMessage: &models.Message{ID: 12, Chat: chat}}, true, false, false},
		{"own administrator change", &models.Update{MyChatMember: member}, true, true, true},
		{"other administrator change", &models.Update{ChatMember: member}, true, true, false},
		{"nil", nil, false, false, false},
		{"no chat", &models.Update{}, false, false, false},
		{"private message", &models.Update{Message: &models.Message{Chat: models.Chat{ID: 123, Type: models.ChatTypePrivate}}}, false, false, false},
		{"private membership", &models.Update{MyChatMember: &models.ChatMemberUpdated{Chat: models.Chat{ID: 123, Type: models.ChatTypePrivate}}}, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := collectChannelUpdateFromWebhook(tc.update)
			if ok != tc.ok {
				t.Fatalf("ok=%v want=%v", ok, tc.ok)
			}
			if ok && (got.Chat.ID != chat.ID || (got.Membership != nil) != tc.member || got.SelfMember != tc.self) {
				t.Fatalf("unexpected event: %+v", got)
			}
		})
	}
	for _, status := range []models.ChatMemberType{
		models.ChatMemberTypeMember, models.ChatMemberTypeAdministrator,
		models.ChatMemberTypeLeft, models.ChatMemberTypeBanned,
	} {
		got, ok := collectChannelUpdateFromWebhook(&models.Update{MyChatMember: &models.ChatMemberUpdated{
			Chat: chat, NewChatMember: models.ChatMember{Type: status},
		}})
		if !ok || !got.SelfMember || got.Membership.NewChatMember.Type != status {
			t.Fatalf("lost own membership status %q", status)
		}
	}
}

func TestCollectChannelDiscoveryOnlyUpdates(t *testing.T) {
	chat := models.Chat{ID: -1004444998796, Type: models.ChatTypeSupergroup}
	for _, update := range []*models.Update{
		{ChatJoinRequest: &models.ChatJoinRequest{Chat: chat}},
		{MessageReaction: &models.MessageReactionUpdated{Chat: chat}},
		{MessageReactionCount: &models.MessageReactionCountUpdated{Chat: chat}},
		{ChatBoost: &models.ChatBoostUpdated{Chat: chat}},
		{RemovedChatBoost: &models.ChatBoostRemoved{Chat: chat}},
		{Message: &models.Message{Chat: chat, NewChatMembers: []models.User{{ID: 123}}}},
		{Message: &models.Message{Chat: chat, LeftChatMember: &models.User{ID: 123}}},
	} {
		event, ok := collectChannelUpdateFromWebhook(update)
		if !ok || !event.DiscoveryOnly || event.SelfMember || event.Chat.ID != chat.ID {
			t.Fatalf("wrong discovery-only event: %+v ok=%v", event, ok)
		}
	}
}
