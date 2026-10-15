package sys

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"
	coreconsts "hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
)

func relayTestBaseTable(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	ddl, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	prefix := "CREATE TABLE IF NOT EXISTS " + name + " ("
	start := strings.Index(string(ddl), prefix)
	if start < 0 {
		t.Fatal("missing test table " + name)
	}
	statement := strings.Split(string(ddl)[start:], ";")[0]
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		statement = strings.ReplaceAll(statement, "bigserial PRIMARY KEY", "integer PRIMARY KEY AUTOINCREMENT")
	}
	db, _ := gdb.Instance()
	if _, err = db.Exec(ctx, statement); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "DELETE FROM "+name); err != nil {
		t.Fatal(err)
	}
}

func TestRelayFeatureListsUseAuthoritativeBotSettings(t *testing.T) {
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, botTable)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_bot(id,bot_name,bot_username,status)VALUES(1,'Test','test_bot',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 1, OwnerId: 99, Enabled: 1, BlockLinks: 1, AuditDatabase: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayRule.Ctx(ctx).Data(do.YoubanBotRelayRule{BotId: 1, Kind: consts.RelayRuleKeyword, Value: "new", Label: "New"}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayBinding.Ctx(ctx).Data(do.YoubanBotRelayBinding{BotId: 1, PairKey: "12:13", LeftChatId: "12", RightChatId: "13", LeftName: "A", RightName: "B"}).Insert(); err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	m := &sysin.FeatureModel{ConfigSchema: featureConfigSchema(consts.RelayFeatureKey), ConfigValues: map[string]interface{}{"botId": 1, "keywords": "stale", "blockLinks": 0}}
	if err := s.relayFeatureSchema(ctx, m); err != nil {
		t.Fatal(err)
	}
	values := featureConfigValues(m.ConfigJson)
	if values["keywords"] != "New" || values["bindings"] != "12 13" || values["blockLinks"] != float64(1) {
		t.Fatalf("stale feature data: %s", m.ConfigJson)
	}
}

func TestRelaySettingsAndListsAreAtomic(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires PostgreSQL row locks")
	}
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, botTable)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_bot(id,bot_name,status)VALUES(1,'Test',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 1, OwnerId: 99, Enabled: 1, BlockLinks: 0, AuditDatabase: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, coreconsts.ContextHTTPKey, &model.Context{})
	contexts.SetUser(ctx, &model.Identity{Id: 99, App: coreconsts.AppAdmin})
	s := NewSysBot()
	raw, _ := json.Marshal(map[string]interface{}{"botId": 1, "enabled": 1, "blockLinks": 1, "auditDatabase": 1, "admins": "12\n13", "keywords": "new", "bindings": "invalid binding"})
	if err := s.saveRelayFeatureSettings(ctx, string(raw)); err == nil {
		t.Fatal("invalid binding accepted")
	}
	cfg, err := relayConfig(ctx, 1)
	if err != nil || cfg.BlockLinks != 0 {
		t.Fatalf("partial settings committed: %+v %v", cfg, err)
	}
	var admins []*entity.YoubanBotRelayAdmin
	if err := dao.YoubanBotRelayAdmin.Ctx(ctx).Scan(&admins); err != nil || len(admins) != 0 {
		t.Fatal("partial admin update committed")
	}
	words, err := s.relayKeywords(ctx, 1)
	if err != nil || len(words) != 0 {
		t.Fatal("partial keyword update committed")
	}
}

func TestRelayObserverReusesDirectoryWithoutChangingPrivileges(t *testing.T) {
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, userTable)
	relayTestBaseTable(t, ctx, channelCacheTable)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS test_relay_user_unique ON hg_youban_bot_user(bot_id,telegram_user_id)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS test_relay_group_unique ON hg_youban_bot_channel_cache(bot_id,chat_id)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotUser.Ctx(ctx).Data(do.YoubanBotUser{BotId: 1, TelegramUserId: "12", MessageCount: 7, IsSuperAdmin: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	msg := &models.Message{ID: 3, Text: "hello", From: &models.User{ID: 12, Username: "alice", FirstName: "Alice"}, Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}}
	for i := 0; i < 2; i++ {
		if err := s.relayObserveChat(ctx, 1, msg); err != nil {
			t.Fatal(err)
		}
	}
	var user *entity.YoubanBotUser
	if err := dao.YoubanBotUser.Ctx(ctx).Where(do.YoubanBotUser{BotId: 1, TelegramUserId: "12"}).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if user.TelegramUsername != "alice" || user.MessageCount != 7 || user.IsSuperAdmin != 1 {
		t.Fatalf("observer changed counters/privileges: %+v", user)
	}
	peer, err := s.relayKnownUser(ctx, 1, "@alice")
	if err != nil || peer.ID != "12" {
		t.Fatalf("directory resolved table primary key as Telegram identity: %+v %v", peer, err)
	}
}
