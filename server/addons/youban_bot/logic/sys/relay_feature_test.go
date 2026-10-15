package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"

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
	// SQLite schemas share the default group name while each test uses a new file.
	// Invalidate metadata cached before this test created its tables.
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
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
