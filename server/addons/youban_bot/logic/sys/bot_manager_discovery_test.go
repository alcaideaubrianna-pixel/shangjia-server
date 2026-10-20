package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	_ "hotgo/addons/telegram_collector/logic/sys"
	cs "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

type managerDiscoveryCatalog struct{ ps.ISysPublish }

func (managerDiscoveryCatalog) BotManagerCatalog(context.Context, int64, int64) ([]*pi.BotManagerChannel, []*pi.BotManagerBot, error) {
	return []*pi.BotManagerChannel{{Id: 1, TgAccountId: 7, TargetChatId: "-100123", AccessHash: "456", ChannelTitle: "Test channel"}}, nil, nil
}

func TestBotManagerMissingMembershipSyncAndRealSubmission(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires local PostgreSQL for production Submit SQL and row locks")
	}
	ctx, s, st := managerTestDB(t)
	requests := managerTestHTTP(t, ctx, s)
	previous := managerPreviousPublish()
	ps.RegisterSysPublish(managerDiscoveryCatalog{})
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	managerTestDraft(t, ctx, st, 1)
	for i := 0; i < 2; i++ {
		if err := s.managerMembers(ctx, st, 0, 1, 1); err != nil {
			t.Fatal(err)
		}
		if text := <-requests; !strings.Contains(text, "正在同步 1 个") || strings.Contains(text, "bm:tasks:") {
			t.Fatal("missing sync feedback or removed task entry returned", text)
		}
	}
	var tasks []*entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).Scan(&tasks); err != nil || len(tasks) != 1 {
		t.Fatal("repeated missing-data clicks must deduplicate", tasks, err)
	}
	ddl, err := os.ReadFile("../../../telegram_collector/resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	db := dao.YoubanBotManagerTask.DB()
	if _, err := db.Exec(ctx, "DROP TABLE IF EXISTS hg_tg_collector_account_task"); err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(ddl), ";") {
		if strings.Contains(statement, `CREATE TABLE IF NOT EXISTS "hg_tg_collector_account_task"`) {
			if _, err := db.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	// This uses the actual collector Submit implementation, not the fake used
	// by UI tests. It catches missing task-type allowlist entries before release.
	if err := s.managerTick(ctx); err != nil {
		t.Fatal(err)
	}
	var task entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(tasks[0].Id).Scan(&task); err != nil || task.Status != "submitted" || task.AccountTaskId <= 0 {
		t.Fatalf("sync not dispatched: task=%+v err=%v", task, err)
	}
	accountTask, err := cs.AccountTasks().Get(ctx, task.AccountTaskId)
	if err != nil || accountTask.TaskType != managerTaskType || accountTask.AccountID != 7 {
		t.Fatal("wrong durable Account task", accountTask, err)
	}
}

func TestBotManagerListUsesChannelMembershipNotTokenOwnership(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	requests := managerTestHTTP(t, ctx, s)
	managerTestChannel(t, ctx, 1, 4, 0, 2)
	for _, b := range []do.YoubanBotManaged{
		{Id: 1, TenantId: 1, AccountId: 99, TelegramBotId: 1001, BotUsername: "external_bot", Status: 1},
		{Id: 2, TenantId: 1, AccountId: 99, TelegramBotId: 1002, BotUsername: "credential_only_bot", BotToken: "1002:test", Status: 1},
		{Id: 3, TenantId: 2, AccountId: 99, TelegramBotId: 1003, BotUsername: "other_tenant_bot", Status: 1},
	} {
		if _, err := dao.YoubanBotManaged.Ctx(ctx).Data(b).Insert(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dao.YoubanBotManagedMember.Ctx(ctx).Data(do.YoubanBotManagedMember{TenantId: 1, AccountId: 99, ManagedBotId: 1, ChannelId: 1, MemberStatus: "admin"}).Insert(); err != nil {
		t.Fatal(err)
	}
	if err := s.managerList(ctx, st, 0, 1); err != nil {
		t.Fatal(err)
	}
	text := <-requests
	if !strings.Contains(text, "external_bot") || strings.Contains(text, "credential_only_bot") || strings.Contains(text, "other_tenant_bot") || strings.Contains(text, "bm:tasks:") {
		t.Fatal("list is not channel-scoped", text)
	}
	if err := s.managerCallback(ctx, st, 0, managerCommand{Action: "bot", ID: 1}); err != nil {
		t.Fatal(err)
	}
	text = <-requests
	if strings.Contains(text, "bm:token:") || strings.Contains(text, "bm:delete:") || !strings.Contains(text, "bm:members:") {
		t.Fatal("Bot detail still contains credential/configuration management", text)
	}
	if _, err := dao.YoubanBotManagedMember.Ctx(ctx).WherePri(1).Data(do.YoubanBotManagedMember{MemberStatus: "removed"}).Update(); err != nil {
		t.Fatal(err)
	}
	n, err := managerChannelBotsModel(ctx, st).Count()
	if err != nil || n != 0 {
		t.Fatal("removed Bot still appears in list", n, err)
	}
}
