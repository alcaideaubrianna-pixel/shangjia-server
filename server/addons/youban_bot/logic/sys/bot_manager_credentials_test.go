package sys

import (
	"context"
	"os"
	"testing"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

type managerStaleCatalog struct{ ps.ISysPublish }

func (managerStaleCatalog) BotManagerCatalog(context.Context, int64, int64) ([]*pi.BotManagerChannel, []*pi.BotManagerBot, error) {
	return nil, []*pi.BotManagerBot{{Id: 7, BotToken: "1001:stale", BotUsername: "test_bot"}}, nil
}

func TestBotManagerStaleSyncCannotOverwriteRotatedCredentials(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("catalog synchronization uses PostgreSQL row locks")
	}
	ctx, s, st := managerTestDB(t)
	managerTestDraft(t, ctx, st, 1)
	if _, err := dao.YoubanBotManaged.Ctx(ctx).WherePri(1).Data(do.YoubanBotManaged{BotToken: "1001:rotated"}).Update(); err != nil {
		t.Fatal(err)
	}
	previous := managerPreviousPublish()
	ps.RegisterSysPublish(managerStaleCatalog{})
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	if err := s.managerSync(ctx, st); err != nil {
		t.Fatal(err)
	}
	var row entity.YoubanBotManaged
	if err := dao.YoubanBotManaged.Ctx(ctx).WherePri(1).Scan(&row); err != nil {
		t.Fatal(err)
	}
	if row.BotToken != "1001:rotated" || row.PublishBotId != 7 {
		t.Fatalf("stale snapshot overwrote credentials or lost linkage: %+v", row)
	}
	_, _ = dao.YoubanBotManaged.Ctx(ctx).WherePri(1).Data(do.YoubanBotManaged{BotToken: ""}).Update()
	if err := s.managerSync(ctx, st); err != nil {
		t.Fatal(err)
	}
	if err := dao.YoubanBotManaged.Ctx(ctx).WherePri(1).Scan(&row); err != nil || row.BotToken != "1001:stale" {
		t.Fatal("identity-only discovery did not import configured Token", err)
	}
}
