package sys

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	twdao "hotgo/addons/youban_two_way_bot/internal/dao"
	"hotgo/addons/youban_two_way_bot/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/model"
	"hotgo/utility/botregistry"
)

func creationFixture(t *testing.T) context.Context {
	t.Helper()
	link := os.Getenv("YOUBAN_TWOWAY_TEST_PG")
	if link == "" {
		t.Skip("requires disposable local youban_twoway_test")
	}
	if !strings.Contains(link, "127.0.0.1") || !strings.HasSuffix(link, "/youban_twoway_test") {
		t.Fatal("refusing non-test database")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Link: link}})
	ctx := context.WithValue(context.Background(), consts.ContextHTTPKey, &model.Context{User: &model.Identity{Id: 30, App: consts.AppApi}})
	if _, err := g.DB().Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	if err := botregistry.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE hg_youban_publish_account(id bigint,tenant_id bigint,account_type text,username text,nickname text,status int,deleted_at timestamp)`,
		`INSERT INTO hg_youban_publish_account VALUES(30,3,'admin','owner','Owner',1,NULL),(31,3,'admin','other','Other',1,NULL)`,
		`CREATE TABLE hg_youban_publish_tg_account(id bigint,tenant_id bigint,session_key text,status text,deleted_at timestamp)`,
		`INSERT INTO hg_youban_publish_tg_account VALUES(8,3,'session','authorized',NULL)`,
	} {
		if _, err := g.DB().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx
}
func TestExistingTwoWayCreationAndEditPreserveGroups(t *testing.T) {
	ctx := creationFixture(t)
	for i, existing := range []string{"", "-100selected"} {
		calls, syncs := 0, 0
		id := fmt.Sprint(100 + i)
		s := NewSysTwoWayBot()
		s.creation = &botCreationHooks{
			token: func(context.Context, string) (string, string, string, error) { return id, "test_bot", "Test", nil },
			group: func(_ context.Context, a *publishAccount, tg *publishTgAccount, group, username string) (*setupGroupResult, error) {
				calls++
				if a.Id != 30 || tg.Id != 8 || group != existing || username != "test_bot" {
					t.Fatal("creation group inputs changed")
				}
				return &setupGroupResult{SupergroupId: "-10099", AccessHash: 999, Title: "Management", InviteLink: "invite"}, nil
			},
			sync: func(_ context.Context, botID, tenantID int64) error {
				syncs++
				if botID <= 0 || tenantID != 3 {
					t.Fatal("runtime identity wrong")
				}
				return nil
			},
		}
		in := &sysin.BotSaveInp{TgAccountId: 8, BotToken: id + ":token", Name: "Dedicated", WelcomeMessage: "Welcome", ExistingGroupId: existing}
		if err := s.AdminBotSave(ctx, in); err != nil {
			t.Fatal(err)
		}
		row, err := s.botById(ctx, in.Id, 3)
		if err != nil {
			t.Fatal(err)
		}
		if row.AccountId != 30 || row.BotType != "two_way" || row.BotUserId != id || row.SupergroupId != "-10099" || row.SupergroupAccessHash != "999" || row.WelcomeMessage != "Welcome" {
			t.Fatal("created Bot lost identity/group fields")
		}
		disabled := sysin.TwoWayBotStatusDisabled
		edit := &sysin.BotSaveInp{Id: in.Id, TgAccountId: 8, Name: "Renamed", WelcomeMessage: "New welcome", Status: &disabled}
		if err := s.AdminBotSave(ctx, edit); err != nil {
			t.Fatal(err)
		}
		row, err = s.botById(ctx, in.Id, 3)
		if err != nil {
			t.Fatal(err)
		}
		if calls != 1 || syncs != 2 || row.SupergroupId != "-10099" || row.Status != 2 || row.BotName != "Renamed" || botModel(row, "").Status != 0 {
			t.Fatal("edit recreated group or changed original API semantics")
		}
		if err := s.AdminBotSave(ctx, &sysin.BotSaveInp{TgAccountId: 8, BotToken: id + ":token"}); err == nil {
			t.Fatal("duplicate Bot creation accepted")
		}
		if calls != 1 {
			t.Fatal("duplicate Bot created an orphan management group")
		}
	}
}
func TestTwoWayListKeywordCannotEscapeOwnerScope(t *testing.T) {
	ctx := creationFixture(t)
	if _, err := g.DB().Exec(ctx, `INSERT INTO hg_youban_bot_registry(tenant_id,account_id,bot_type,bot_user_id,bot_username) VALUES(3,30,'two_way','100','owned'),(3,31,'two_way','101','needle'),(9,90,'two_way','102','needle')`); err != nil {
		t.Fatal(err)
	}
	s := NewSysTwoWayBot()
	rows, count, err := s.AdminBotList(ctx, &sysin.BotListInp{Keyword: "needle"})
	if err != nil || count != 0 || len(rows) != 0 {
		t.Fatal("keyword OR bypassed account/tenant filter", count, err)
	}
	var ids []int64
	if err := twdao.YoubanBotRegistry.Ctx(ctx).Where(twdao.YoubanBotRegistry.Columns().AccountId, 31).Fields("id").Scan(&ids); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ownedBotByID(ctx, ids[0], &publishAccount{Id: 30, TenantId: 3}); err == nil {
		t.Fatal("same tenant different account allowed")
	}
}
