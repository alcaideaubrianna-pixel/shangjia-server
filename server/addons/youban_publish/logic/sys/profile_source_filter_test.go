package sys

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestProfileSourceChannelFilterLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_STORAGE_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, query := range []string{
			`CREATE TEMP TABLE channel_filter_profiles(id bigint,profile_id bigint,tenant_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(profile_id bigint,source_id bigint,event_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_event(id bigint,source_chat_id text) ON COMMIT DROP`,
			`INSERT INTO channel_filter_profiles VALUES(1,1,7),(2,2,7),(3,3,7),(4,4,8)`,
			`INSERT INTO hg_youban_publish_collect_event VALUES(1,'-100100'),(2,'100'),(3,'-100200'),(4,'-100100')`,
			`INSERT INTO hg_youban_publish_collect_dispatch VALUES(1,11,1),(1,12,1),(2,12,2),(3,13,3),(4,11,4)`,
		} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}
		for _, field := range []string{"p.id", "p.profile_id"} {
			for _, tc := range []struct {
				source int64
				chat   string
				want   []int64
			}{
				{0, "-100100", []int64{1, 2}}, {0, "100", []int64{1, 2}},
				{11, "-100100", []int64{1}}, {12, "", []int64{1, 2}},
				{0, "-100200", []int64{3}}, {0, "-100999", nil}, {0, "", []int64{1, 2, 3}},
			} {
				mod := tx.Model("channel_filter_profiles p").Where("p.tenant_id", 7)
				mod = applyProfileCollectionSourceFilter(mod, &sysin.ProfileListInp{CollectSourceId: tc.source, CollectSourceChatId: tc.chat}, field)
				var rows []struct{ Id int64 }
				if err := mod.Fields("p.id").OrderAsc("p.id").Scan(&rows); err != nil {
					return err
				}
				var got []int64
				for _, row := range rows {
					got = append(got, row.Id)
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("field=%s source=%d chat=%s got=%v want=%v", field, tc.source, tc.chat, got, tc.want)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
