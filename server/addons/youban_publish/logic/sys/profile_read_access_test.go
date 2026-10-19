package sys

import (
	"context"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
	"os"
	"testing"
)

func TestProfileDetailReadDoesNotGrantWrite(t *testing.T) {
	cap := &sysin.AccountCapabilityModel{AccountId: 8, TenantId: 7, AccountType: "uploader", ProfileDetailViewEnabled: 1}
	item := &sysin.ProfileModel{AccountId: 9, TenantId: 7}
	markSharedProfilePermission(item, cap)
	if !item.CanViewDetail || item.CanEdit || item.Permission != sysin.ProfilePermissionVisitor {
		t.Fatalf("read escalated write: %+v", item)
	}
	scope, err := profileWriteAccountScope(cap)
	if err != nil || scope != 8 {
		t.Fatalf("write scope %d %v", scope, err)
	}
	item.TenantId = 10
	markSharedProfilePermission(item, cap)
	if item.CanViewDetail {
		t.Fatal("cross tenant detail permission")
	}
	cap.ProfileDetailViewEnabled = 0
	item.TenantId = 7
	markSharedProfilePermission(item, cap)
	if item.CanViewDetail {
		t.Fatal("revoked permission retained")
	}
	item.AccountId = 8
	markSharedProfilePermission(item, cap)
	if !item.CanViewDetail || !item.CanEdit {
		t.Fatal("creator regression")
	}
	cap.SharedResourceEnabled = 1
	item.AccountId = 9
	markSharedProfilePermission(item, cap)
	if !item.CanViewDetail || !item.CanEdit {
		t.Fatal("shared regression")
	}
}

func TestProfileDetailReadLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_STORAGE_PG") != "1" {
		t.Skip("requires local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, q := range []string{
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint PRIMARY KEY,tenant_id bigint,account_type text,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account_setting(account_id bigint,tenant_id bigint,profile_detail_view_enabled int,shared_resource_enabled int,telegram_binding_enabled int,group_push_enabled int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_account VALUES(8,7,'uploader',1,NULL),(9,7,'uploader',1,NULL),(10,10,'uploader',1,NULL)`,
			`INSERT INTO hg_youban_publish_account_setting VALUES(8,7,1,0,0,0,NULL)`,
			`INSERT INTO hg_youban_publish_profile_state VALUES(1,7,8,NULL),(2,7,9,NULL),(3,10,10,NULL)`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		s := NewSysPublish()
		cap, err := s.activeAccountCapability(ctx, 7, 8)
		if err != nil {
			return err
		}
		ids, err := s.readableProfileAccountIds(ctx, cap, 9)
		if err != nil || len(ids) != 1 || ids[0] != 9 {
			t.Fatalf("selected scope %v %v", ids, err)
		}
		if _, err = s.readableProfileAccountIds(ctx, cap, 10); err == nil {
			t.Fatal("cross tenant account")
		}
		for _, tc := range []struct {
			id   int64
			want int64
			fail bool
		}{{0, 8, false}, {1, 0, false}, {2, 0, false}, {3, 0, true}, {999, 0, true}} {
			got, e := s.profileRecordAccountScope(ctx, 7, 8, tc.id)
			if (e != nil) != tc.fail || (!tc.fail && got != tc.want) {
				t.Fatalf("record scope id=%d got=%d err=%v", tc.id, got, e)
			}
		}
		if _, err = tx.Exec(`UPDATE hg_youban_publish_account_setting SET profile_detail_view_enabled=0 WHERE account_id=8`); err != nil {
			return err
		}
		if _, err = s.profileRecordAccountScope(ctx, 7, 8, 2); err == nil {
			t.Fatal("revocation not immediate")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProfilePushChannelFilterLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_STORAGE_PG") != "1" {
		t.Skip("requires local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, q := range []string{
			`CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint PRIMARY KEY,tenant_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_channel(profile_id bigint,tenant_id bigint,channel_id bigint,is_manual int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE INDEX ON hg_youban_publish_profile_channel(tenant_id,profile_id,is_manual,channel_id)`,
			`CREATE TEMP TABLE hg_youban_publish_channel(id bigint PRIMARY KEY,tenant_id bigint,publish_direction text,status int,is_default_selected int,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_profile_state VALUES(1,7,NULL),(2,7,NULL),(3,7,NULL),(4,10,NULL)`,
			`INSERT INTO hg_youban_publish_profile_channel VALUES(2,7,20,1,NULL),(3,7,10,1,now()),(4,10,10,1,NULL)`,
			`INSERT INTO hg_youban_publish_channel VALUES(10,7,'up',1,1,NULL),(20,7,'up',1,0,NULL),(30,10,'up',1,1,NULL)`,
		} {
			if _, e := tx.Exec(q); e != nil {
				return e
			}
		}
		for _, tc := range []struct {
			channel int64
			want    []int64
		}{{10, []int64{1, 3}}, {20, []int64{2}}, {30, nil}} {
			var rows []struct{ ProfileId int64 }
			mod := g.DB().Model(publishProfileStateTable+" ps").Ctx(ctx).Where("ps.tenant_id", 7)
			mod = applyProfilePushChannelFilter(mod, &sysin.ProfileListInp{PushChannelId: tc.channel}, "ps.tenant_id", "ps.profile_id")
			if e := mod.Fields("ps.profile_id").OrderAsc("ps.profile_id").Scan(&rows); e != nil {
				return e
			}
			if len(rows) != len(tc.want) {
				t.Fatalf("channel%d rows%+v want%v", tc.channel, rows, tc.want)
			}
			for i, row := range rows {
				if row.ProfileId != tc.want[i] {
					t.Fatalf("channel%d rows%+v", tc.channel, rows)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
