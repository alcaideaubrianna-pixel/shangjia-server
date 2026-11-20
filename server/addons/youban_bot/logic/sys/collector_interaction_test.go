package sys

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"

	pi "hotgo/addons/youban_publish/model/input/sysin"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
)

func TestCollectorGatewayPassesThroughChannelAndGroupUpdates(t *testing.T) {
	feature := &collectorGatewayFeature{}
	bot := gatewayservice.BotContext{Bindings: []gatewayservice.BotBinding{
		{Owner: "youban_publish", ReferenceID: 1},
	}}
	group := models.Chat{Type: models.ChatTypeSupergroup}
	cases := map[string]*models.Update{
		"channel": {ChannelPost: &models.Message{Chat: models.Chat{Type: models.ChatTypeChannel}}},
		"group":   {Message: &models.Message{Chat: group}},
		"group callback": {CallbackQuery: &models.CallbackQuery{
			Message: models.MaybeInaccessibleMessage{Message: &models.Message{Chat: group}},
		}},
		"inaccessible group callback": {CallbackQuery: &models.CallbackQuery{
			Message: models.MaybeInaccessibleMessage{InaccessibleMessage: &models.InaccessibleMessage{Chat: group}},
		}},
	}
	for name, update := range cases {
		t.Run(name, func(t *testing.T) {
			handled, err := feature.HandleUpdate(context.Background(), bot, update)
			if handled || err != nil {
				t.Fatalf("expected provider dispatch, got handled=%v err=%v", handled, err)
			}
		})
	}
}

func TestCollectorNoteActionsRejectLegacyOperations(t *testing.T) {
	for _, data := range []string{
		"pf:up:A00001",
		"pf:send:A00001", "pf:channels:A00001", "ch:list", "cm:list:1",
	} {
		if collectorCallbackAction(data) != "" {
			t.Errorf("allowed unsupported callback %q", data)
		}
	}
	for _, data := range []string{
		"pf:list:2", "pf:search", "pf:askedit", "pf:create", "pf:publishup:A00001",
		"pf:down:A00001", "pf:delconfirm:A00001", "pf:preview:A00001",
	} {
		if collectorCallbackAction(data) != data {
			t.Errorf("rejected supported callback %q", data)
		}
	}
}

func TestCollectorReplyKeyboard(t *testing.T) {
	for _, tc := range []struct {
		waiting bool
		buttons []string
	}{
		{true, []string{"注册申请"}},
		{false, []string{"上架资料", "资料管理"}},
	} {
		markup := collectorReplyKeyboard(tc.waiting)
		if len(markup.Keyboard) != 1 || len(markup.Keyboard[0]) != len(tc.buttons) {
			t.Fatalf("unexpected keyboard for waiting=%v: %+v", tc.waiting, markup.Keyboard)
		}
		for i, text := range tc.buttons {
			if markup.Keyboard[0][i].Text != text {
				t.Errorf("button %d = %q, want %q", i, markup.Keyboard[0][i].Text, text)
			}
		}
	}
}

func TestCollectorProfileCardHidesUnrelatedActions(t *testing.T) {
	markup := collectorNoteMarkup(profileCardMarkup("A00001", "view"))
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if collectorCallbackAction(button.CallbackData) == "" {
				t.Errorf("rendered unsupported callback %q", button.CallbackData)
			}
		}
	}

}

func TestCollectorCommandsDoNotEnterLegacyActions(t *testing.T) {
	for _, text := range []string{"/send A00001", "/admin", "下架 A00001", "取消推送", "频道管理"} {
		command, _ := botCommandAndArgs(text)
		if action := collectorMessageAction(command, text); action != "" {
			t.Errorf("%q routed to %q", text, action)
		}
	}
}

func TestCollectorOwnedNoteActions(t *testing.T) {
	for _, tc := range []struct {
		status int
		review bool
		label  string
	}{
		{2, false, "上架"}, {2, true, "提交审核"}, {1, true, "下架"},
		{pi.ProfileStatusReviewPending, true, "审核中"},
	} {
		note := &pi.NoteModel{ProfileModel: pi.ProfileModel{Status: tc.status, ProfileNo: "A00001"}}
		buttons := collectorOwnedNoteMarkup(note, tc.review).InlineKeyboard
		if buttons[0][0].Text != tc.label {
			t.Fatalf("status %d label = %q, want %q", tc.status, buttons[0][0].Text, tc.label)
		}
		for _, row := range buttons {
			for _, button := range row {
				if collectorCallbackAction(button.CallbackData) == "" {
					t.Errorf("unsupported callback %q", button.CallbackData)
				}
			}
		}
	}
}
