package sys

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
)

func TestRelayHelpRequiresConfiguredAdminAndPrivateChat(t *testing.T) {
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, botTable)
	relayTestBaseTable(t, ctx, featureTable)
	relayTestBaseTable(t, ctx, "hg_youban_bot_account_bind")
	if _, err := dao.YoubanBotBot.Ctx(ctx).Data(do.YoubanBotBot{Id: 1, BotToken: "123:fake", BotName: "Test", Status: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Data(do.YoubanBotRelayAdmin{BotId: 1, UserId: "12", Username: ""}).Insert(); err != nil {
		t.Fatal(err)
	}
	texts := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		texts <- r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()
	client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	s.relayClientForToken = func(context.Context, string) (*tgbot.Bot, error) { return client, nil }
	cfg := &entity.YoubanBotRelayConfig{BotId: 1, TenantId: 1, AccountId: 99, SourceBotId: 1, OwnerId: 99}
	for _, tc := range []struct {
		user int64
		chat models.ChatType
		want string
		help bool
	}{
		{13, models.ChatTypePrivate, "仅配置的管理员", false},
		{12, models.ChatTypeGroup, "私聊", false},
		{12, models.ChatTypePrivate, "双向机器人 · 管理帮助", true},
	} {
		msg := &models.Message{ID: 1, From: &models.User{ID: tc.user}, Chat: models.Chat{ID: tc.user, Type: tc.chat}}
		if err := s.handleRelayCommand(ctx, cfg, msg, relayCommand{Name: "/help"}); err != nil {
			t.Fatal(err)
		}
		text := <-texts
		if !strings.Contains(text, tc.want) || strings.Contains(text, "绑定 @alice @bob") != tc.help {
			t.Fatalf("unauthorized help disclosure: %s", text)
		}
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 1, TenantId: 1, AccountId: 99, SourceBotId: 1, OwnerId: 99, Enabled: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
}
