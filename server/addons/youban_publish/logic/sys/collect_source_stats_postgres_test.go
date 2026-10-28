package sys

import (
	"context"
	"os"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectSourceSuccessTotalsLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, query := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,event_id bigint,status text) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_dispatch(tenant_id,account_id,source_id,event_id,status) SELECT 38,512,109,n,'sent' FROM generate_series(1,36) n`,
			// Several rules/targets for one event still count once. Failed,
			// skipped, pending, other sources/accounts/tenants must not count.
			`INSERT INTO hg_youban_publish_collect_dispatch(tenant_id,account_id,source_id,event_id,status) VALUES
			 (38,512,109,1,'sent'),(38,512,109,2,'sent'),
			 (38,512,109,37,'pending'),(38,512,109,38,'failed'),(38,512,109,39,'skipped'),
			 (39,512,109,40,'sent'),(38,999,109,41,'sent'),(38,512,110,42,'sent')`,
		} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		list := []*sysin.CollectSourceModel{
			{Id: 109, TenantId: 38, AccountId: 512, EventTotal: 0, SuccessTotal: 0},
			{Id: 111, TenantId: 38, AccountId: 512, EventTotal: 10, SuccessTotal: 99},
			{Id: 110, TenantId: 39, AccountId: 512, SuccessTotal: 7},
			nil,
		}
		if err := fillCollectSourceSuccessTotals(ctx, list, 38, 512); err != nil {
			return err
		}
		if list[0].SuccessTotal != 36 || list[0].EventTotal != 0 {
			t.Fatalf("cleaned event logs hid historical imports: %+v", list[0])
		}
		if list[1].SuccessTotal != 0 || list[2].SuccessTotal != 7 {
			t.Fatalf("incorrect empty/source ownership handling: %+v", list)
		}
		// Repeat reads are idempotent, with no workers or external calls.
		return fillCollectSourceSuccessTotals(ctx, list, 38, 512)
	})
	if err != nil {
		t.Fatal(err)
	}
}
