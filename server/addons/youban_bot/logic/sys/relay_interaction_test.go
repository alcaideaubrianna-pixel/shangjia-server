package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
)

func relayInteractionFixture(t *testing.T) (context.Context, *sSysBot, <-chan string) {
	t.Helper()
	ctx := relayTestDatabase(t)
	for _, name := range []string{botTable, userTable, featureTable, "hg_youban_bot_account_bind"} {
		relayTestBaseTable(t, ctx, name)
	}
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS test_relay_user_unique ON hg_youban_bot_user(bot_id,telegram_user_id)"); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRegistry.Ctx(ctx).Data(do.YoubanBotRegistry{Id: 1, BotToken: "123:fake", BotUserId: "123", BotName: "Relay", BotType: "two_way", TenantId: 1, AccountId: 99, Status: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotFeature.Ctx(ctx).Data(do.YoubanBotFeature{FeatureKey: consts.RelayFeatureKey, Status: 1, ConfigJson: "{}"}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 1, TenantId: 1, AccountId: 99, Enabled: 1, AuditDatabase: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	texts := make(chan string, 50)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		texts <- r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/answerCallbackQuery") {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":99}}`))
	}))
	t.Cleanup(server.Close)
	client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	s.relayOwnedBots = relayTestOwnedBackendBots
	s.relayAccountForUser = func(ctx context.Context, id int64) (*botProfileAccount, error) {
		if id == 12 {
			return &botProfileAccount{TenantId: 1, AccountId: 99, App: "api"}, nil
		}
		if id == 23 {
			return &botProfileAccount{TenantId: 1, AccountId: 99, App: "api"}, nil
		}
		return nil, fmt.Errorf("unbound")
	}

	s.relayClientForToken = func(context.Context, string) (*tgbot.Bot, error) { return client, nil }
	return ctx, s, texts
}

func TestRelayDedicatedBotIdentityAndOldMenus(t *testing.T) {
	ctx, s, texts := relayInteractionFixture(t)
	// Pausing the global switch must not turn a dedicated Bot back into the main Bot.
	if _, err := dao.YoubanBotFeature.Ctx(ctx).Where(dao.YoubanBotFeature.Columns().FeatureKey, consts.RelayFeatureKey).Data(do.YoubanBotFeature{Status: 2}).Update(); err != nil {
		t.Fatal(err)
	}
	menus, err := (&relayGatewayFeature{bot: s}).Menus(ctx, gatewayservice.BotContext{Bindings: []gatewayservice.BotBinding{{Owner: "youban_bot", ReferenceID: 1}}})
	if err != nil || len(menus.Items) != 3 || relayMenuHas(menus, "profile") || relayMenuHas(menus, "relayadmin") {
		t.Fatalf("dedicated Bot inherited main menus: %+v %v", menus, err)
	}
	for _, text := range []string{"双向机器人管理", "资料管理", "/profile", "/start", "hello"} {
		msg := &models.Message{ID: 1, Text: text, From: &models.User{ID: 12}, Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}}
		handled, err := s.handleRelayUpdate(ctx, 1, &models.Update{Message: msg})
		if !handled || err != nil {
			t.Fatalf("dedicated message escaped routing: %q %v %v", text, handled, err)
		}
		reply := <-texts
		if strings.Contains(reply, "小灰机上架") || reply == "" {
			t.Fatalf("main Bot welcome leaked: %s", reply)
		}
	}
	count, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(dao.YoubanBotRelayDelivery.Columns().Kind, consts.RelayKindChat).Count()
	if err != nil || count != 0 {
		t.Fatal("menu input was forwarded", count, err)
	}
	if _, err := dao.YoubanBotRegistry.Ctx(ctx).WherePri(1).Data(do.YoubanBotRegistry{IsDefault: 1}).Update(); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.relayIdentity(ctx, 1)
	if err != nil || cfg != nil {
		t.Fatal("default main Bot acquired relay identity", err)
	}
}

func TestRelayFirstUseOutboxDedupAndPermission(t *testing.T) {
	ctx, s, texts := relayInteractionFixture(t)
	for _, id := range []string{"21", "22"} {
		if _, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Data(do.YoubanBotRelayAdmin{BotId: 1, UserId: id, Username: id}).Insert(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Data(do.YoubanBotRelayAdmin{BotId: 1, UserId: "", Username: "@pending"}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotAccountBind.Ctx(ctx).Data(do.YoubanBotAccountBind{Id: 99, AccountId: 99, App: "api", TelegramUserId: "23", Status: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	cfg := &entity.YoubanBotRelayConfig{BotId: 1, TenantId: 1, AccountId: 99, Enabled: 1}
	binding := &entity.YoubanBotRelayBinding{Id: 10, BotId: 1, PairKey: "12:13", LeftChatId: "12", RightChatId: "13", LeftName: "A", RightName: "B"}
	if _, err := dao.YoubanBotRelayBinding.Ctx(ctx).Data(do.YoubanBotRelayBinding{Id: 10, BotId: 1, PairKey: "12:13", LeftChatId: "12", RightChatId: "13", LeftName: "A", RightName: "B"}).Insert(); err != nil {
		t.Fatal(err)
	}
	msg := &models.Message{ID: 1, Text: "private content", Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}, From: &models.User{ID: 12, Username: "alice", FirstName: "<Alice>"}}
	for i := 0; i < 3; i++ {
		msg.ID++
		if err := s.enqueueRelayFirstUse(ctx, cfg, msg, binding); err != nil {
			t.Fatal(err)
		}
	}
	c := dao.YoubanBotRelayDelivery.Columns()
	var rows []*entity.YoubanBotRelayDelivery
	if err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.Kind, consts.RelayKindNotice).OrderAsc(c.Id).Scan(&rows); err != nil || len(rows) != 3 {
		t.Fatalf("notice duplicate/pending-admin leak: %d %v", len(rows), err)
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(rows[0].RawJson), &payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload.Text, "private content") || !strings.Contains(payload.Text, "&lt;Alice&gt;") {
		t.Fatal("notification leaked content or invalid HTML")
	}
	var sendRow, revokeRow *entity.YoubanBotRelayDelivery
	for _, row := range rows {
		if row.TargetChatId == "21" {
			sendRow = row
		}
		if row.TargetChatId == "22" {
			revokeRow = row
		}
	}
	if sendRow == nil || revokeRow == nil {
		t.Fatal("confirmed recipients missing")
	}
	ids, status, err := s.sendRelayDeliveries(ctx, []*entity.YoubanBotRelayDelivery{sendRow})
	if err != nil || status != consts.RelaySent || len(ids) != 1 {
		t.Fatal("notice not sent", status, err)
	}
	if text := <-texts; !strings.Contains(text, "绑定用户开始使用") {
		t.Fatal("notice not sent with sendMessage", text)
	}
	if _, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Where(do.YoubanBotRelayAdmin{BotId: 1, UserId: revokeRow.TargetChatId}).Delete(); err != nil {
		t.Fatal(err)
	}
	_, status, err = s.sendRelayDeliveries(ctx, []*entity.YoubanBotRelayDelivery{revokeRow})
	if err != nil || status != consts.RelayCancelled {
		t.Fatal("revoked admin received queued notice", status, err)
	}
	// A new binding creates a fresh, independently deduplicated lifecycle.
	binding.Id = 11
	if err := s.enqueueRelayFirstUse(ctx, cfg, msg, binding); err != nil {
		t.Fatal(err)
	}
	count, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.Kind, consts.RelayKindNotice).Count()
	if err != nil || count != 5 {
		t.Fatal("new binding did not reset notice", count, err)
	}
}

func TestRelayFirstUseConcurrentDedup(t *testing.T) {
	ctx, s, _ := relayInteractionFixture(t)
	if _, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Data(do.YoubanBotRelayAdmin{BotId: 1, UserId: "21", Username: "21"}).Insert(); err != nil {
		t.Fatal(err)
	}
	cfg := &entity.YoubanBotRelayConfig{BotId: 1, TenantId: 1, AccountId: 99}
	binding := &entity.YoubanBotRelayBinding{Id: 10, PairKey: "12:13"}
	msg := &models.Message{ID: 2, Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}, From: &models.User{ID: 12}}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.enqueueRelayFirstUse(ctx, cfg, msg, binding); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	count, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(dao.YoubanBotRelayDelivery.Columns().Kind, consts.RelayKindNotice).Count()
	if err != nil || count != 1 {
		t.Fatal("concurrent first use duplicated", count, err)
	}
}

func TestRelayPendingAdminStartConfirmsIdentity(t *testing.T) {
	ctx, s, texts := relayInteractionFixture(t)
	if _, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Data(do.YoubanBotRelayAdmin{BotId: 1, UserId: "", Username: "@alice"}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(dao.YoubanBotRelayConfig.Columns().BotId, 1).Data(do.YoubanBotRelayConfig{Enabled: 0}).Update(); err != nil {
		t.Fatal(err)
	}
	msg := &models.Message{ID: 1, Text: "/start", From: &models.User{ID: 12, Username: "Alice"}, Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}}
	handled, err := s.handleRelayUpdate(ctx, 1, &models.Update{Message: msg})
	if !handled || err != nil {
		t.Fatal("paused Bot cannot confirm pending admin", handled, err)
	}
	text := <-texts
	if !strings.Contains(text, "管理员身份已确认") || !strings.Contains(text, "/help") || !strings.Contains(text, "已停用") {
		t.Fatal("admin onboarding missing confirmation/state", text)
	}
	count, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Where(do.YoubanBotRelayAdmin{BotId: 1, UserId: "12"}).Count()
	if err != nil || count != 1 {
		t.Fatal("pending admin unresolved", count, err)
	}
}

func TestRelayManagementButtonsRecheckPermissions(t *testing.T) {
	ctx, s, texts := relayInteractionFixture(t)
	if _, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Data(do.YoubanBotRelayAdmin{BotId: 1, UserId: "12", Username: "12"}).Insert(); err != nil {
		t.Fatal(err)
	}
	q := &models.CallbackQuery{ID: "button", From: models.User{ID: 12}, Data: "rm:1 停用", Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: 1, Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}}}}
	handled, err := s.handleRelayManagementCallback(ctx, 1, q)
	if !handled || err != nil {
		t.Fatal("management callback failed", handled, err)
	}
	<-texts // answerCallbackQuery
	text := <-texts
	if !strings.Contains(text, "确认 停用") {
		t.Fatal("disable did not require confirmation", text)
	}
	cfg, err := relayConfig(ctx, 1)
	if err != nil || cfg.Enabled != 1 {
		t.Fatal("unconfirmed click disabled Bot", err)
	}
	if _, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Where(do.YoubanBotRelayAdmin{BotId: 1, UserId: "12"}).Delete(); err != nil {
		t.Fatal(err)
	}
	s.relayAccountForUser = func(context.Context, int64) (*botProfileAccount, error) {
		return &botProfileAccount{TenantId: 2, AccountId: 100, App: "api"}, nil
	}
	q.Data = "rm:1 确认 停用"
	handled, err = s.handleRelayManagementCallback(ctx, 1, q)
	if !handled || err != nil {
		t.Fatal("revoked callback failed", handled, err)
	}
	<-texts
	text = <-texts
	if !strings.Contains(text, "无权管理其他租户账户") {
		t.Fatal("stale button bypassed permission", text)
	}
	cfg, err = relayConfig(ctx, 1)
	if err != nil || cfg.Enabled != 1 {
		t.Fatal("revoked admin disabled Bot", err)
	}
}

func TestRelayBlockedAndCommandMessagesDoNotNotify(t *testing.T) {
	ctx, s, _ := relayInteractionFixture(t)
	if _, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Data(do.YoubanBotRelayAdmin{BotId: 1, UserId: "21", Username: "21"}).Insert(); err != nil {
		t.Fatal(err)
	}
	cfg := &entity.YoubanBotRelayConfig{BotId: 1, TenantId: 1, AccountId: 99, Enabled: 1, AuditDatabase: 1}
	binding := &entity.YoubanBotRelayBinding{Id: 10, BotId: 1, PairKey: "12:13", LeftChatId: "12", RightChatId: "13"}
	msg := &models.Message{ID: 1, Text: "/help", Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}, From: &models.User{ID: 12}}
	if err := s.enqueueRelayMessage(ctx, cfg, "Relay", msg, binding, "13", "", true); err != nil {
		t.Fatal(err)
	}
	msg.ID = 2
	msg.Text = "blocked"
	if err := s.enqueueRelayMessage(ctx, cfg, "Relay", msg, binding, "13", "blocked", false); err != nil {
		t.Fatal(err)
	}
	c := dao.YoubanBotRelayDelivery.Columns()
	count, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.Kind, consts.RelayKindNotice).Count()
	if err != nil || count != 0 {
		t.Fatal("command or blocked message created notice", count, err)
	}
	msg.ID = 3
	msg.Text = "hello"
	if err := s.enqueueRelayMessage(ctx, cfg, "Relay", msg, binding, "13", "", false); err != nil {
		t.Fatal(err)
	}
	count, err = dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.Kind, consts.RelayKindNotice).Count()
	if err != nil || count != 1 {
		t.Fatal("first ordinary message did not create notice", count, err)
	}
}

func TestRelayAuditControlsStayOutOfDedicatedHelp(t *testing.T) {
	ctx, s, _ := relayInteractionFixture(t)
	cfg, err := relayConfig(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"聊天存档 关闭", "聊天存档 开启", "关闭转发"} {
		handled, _, err := s.relayManagementAudit(ctx, cfg, command)
		if !handled || err != nil {
			t.Fatal("management audit control failed", command, err)
		}
	}
	handled, _, err := s.relayManagementAudit(ctx, cfg, "转发群 123")
	if !handled || err == nil {
		t.Fatal("private user accepted as audit destination")
	}
	for _, word := range []string{"审计设置", "聊天存档", "转发群"} {
		if strings.Contains(relayAdminHelp, word) {
			t.Fatal("audit controls leaked into relay help", word)
		}
	}
}

func relayTestOwnedBackendBots(ctx context.Context, tenantID, accountID, sourceID int64, lock bool) ([]*publishsysin.RelayOwnedBotModel, error) {
	if sourceID <= 0 {
		sourceID = 1
	}
	token := "123:fake"
	if sourceID == 2 {
		token = "456:other"
	}
	return []*publishsysin.RelayOwnedBotModel{{BotModel: publishsysin.BotModel{BotType: "two_way", Id: sourceID, TenantId: tenantID, BotToken: token, Status: 1}, AccountId: accountID, CreatedBy: accountID}}, nil
}
