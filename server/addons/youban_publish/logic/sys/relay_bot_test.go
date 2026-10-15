package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// Uses an isolated connection group/schema to verify the real cross-plugin query
// without touching normal tables or relying on account resolver test hooks.
func TestRelayOwnedBotsRequireTenantAccountAndCreator(t *testing.T) {
	link := os.Getenv("YOUBAN_RELAY_TEST_PG")
	if link == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	if !strings.Contains(link, "127.0.0.1") || !strings.HasSuffix(link, "/youban_relay_test") {
		t.Fatal("refusing non-test database")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Link: link}})
	ctx := context.Background()
	db := g.DB()
	// Roll back all fixture DDL and rows. The table uses the real DAO schema.
	err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("CREATE TEMP TABLE hg_youban_bot_registry (id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,created_by bigint,status int,bot_type varchar(32) DEFAULT 'two_way',bot_name text,bot_username text,bot_token text,remark text,created_at timestamp,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP"); err != nil {
			return err
		}
		if _, err := tx.Exec("CREATE TEMP TABLE hg_youban_publish_account(id bigint,tenant_id bigint,status int,deleted_at timestamp) ON COMMIT DROP"); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO hg_youban_publish_account(id,tenant_id,status)VALUES(100,10,1),(101,10,1)"); err != nil {
			return err
		}
		if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_bot_registry(id,tenant_id,account_id,created_by,status) VALUES(1,10,100,100,1),(2,10,101,101,1),(3,11,100,100,1),(4,10,100,999,1),(5,10,100,100,2),(6,10,0,100,1)`); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO hg_youban_bot_registry(id,tenant_id,account_id,created_by,status,bot_type)VALUES(8,10,100,100,1,'normal')"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_bot_registry(id,tenant_id,account_id,created_by,status,deleted_at)VALUES(7,10,100,100,1,NOW())`); err != nil {
			return err
		}
		s := &sSysPublish{}
		rows, err := s.RelayOwnedBots(ctx, 10, 100, 0, false)
		if err != nil {
			return err
		}
		if len(rows) != 2 || rows[0].Id != 1 || rows[1].Id != 4 {
			t.Fatalf("cross tenant/account/creator or inactive Bot leaked: %+v", rows)
		}
		rows, err = s.RelayOwnedBots(ctx, 10, 100, 2, true)
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatal("guessed source ID bypassed account")
		}
		if _, err := tx.Exec("UPDATE hg_youban_publish_account SET status=2 WHERE id=100"); err != nil {
			return err
		}
		rows, err = s.RelayOwnedBots(ctx, 10, 100, 1, false)
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatal("disabled account retained selectable Bot")
		}
		if _, err = s.RelayOwnedBots(ctx, 0, 100, 0, false); err == nil {
			t.Fatal("missing tenant accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
