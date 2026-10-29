package sys

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
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

func TestProfilePermissionHidesCustomerRemarkExceptCreator(t *testing.T) {
	for _, permission := range []string{sysin.ProfilePermissionShared, sysin.ProfilePermissionVisitor} {
		item := &sysin.ProfileModel{CustomerRemark: "private"}
		markProfilePermission(item, permission)
		if item.CustomerRemark != "" {
			t.Fatalf("permission %q exposed customer remark", permission)
		}
	}
	item := &sysin.ProfileModel{CustomerRemark: "private"}
	markProfilePermission(item, sysin.ProfilePermissionCreator)
	if item.CustomerRemark != "private" {
		t.Fatal("creator lost customer remark")
	}
	item = &sysin.ProfileModel{CustomerRemark: "private"}
	markProfilePermission(item, sysin.ProfilePermissionAdmin)
	if item.CustomerRemark != "private" {
		t.Fatal("tenant admin lost customer remark")
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

func TestProfileFilterOptionsLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_STORAGE_PG") != "1" {
		t.Skip("requires local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, q := range []string{
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint PRIMARY KEY,tenant_id bigint,nickname text,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_channel(id bigint PRIMARY KEY,tenant_id bigint,telegram_chat_id text,title text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_account_channel(account_id bigint,tenant_id bigint,channel_id bigint,source_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_chat_id text,title text,source_type text,bot_id bigint,bot_collect_scope text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_bot_channel_cache(tenant_id bigint,bot_id bigint,chat_id text,chat_title text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_channel(id bigint PRIMARY KEY,tenant_id bigint,channel_title text,publish_direction text,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_account VALUES(8,7,'本人',1,NULL),(9,7,'另一账户',1,NULL),(10,10,'其他租户',1,NULL)`,
			`INSERT INTO hg_youban_publish_collect_channel VALUES(100,7,'-100100','关联频道',NULL)`,
			`INSERT INTO hg_youban_publish_collect_account_channel VALUES(8,7,100,101,NULL)`,
			`INSERT INTO hg_youban_publish_collect_source VALUES(102,7,9,'-100102','已删历史频道','account',0,'',now()),(103,7,9,'','Bot来源','bot',64,'all',NULL),(104,10,10,'-100104','禁止频道','account',0,'',NULL)`,
			`INSERT INTO hg_youban_publish_collect_source VALUES(105,7,9,'-100103','Bot频道','account',0,'',NULL),(106,7,9,'103','Bot频道','account',0,'',now()),(107,7,9,'-100107','Bot频道','account',0,'',NULL),(108,7,9,'','另一个Bot规则','bot',64,'all',NULL)`,
			`INSERT INTO hg_youban_publish_bot_channel_cache VALUES(7,64,'-100103','Bot频道',NULL),(10,64,'-100999','跨租户缓存',NULL)`,
			`INSERT INTO hg_youban_publish_channel VALUES(1,7,'推送频道','up',NULL),(2,10,'禁止推送频道','up',NULL)`,
		} {
			if _, e := tx.Exec(q); e != nil {
				return e
			}
		}
		s := NewSysPublish()
		cap := &sysin.AccountCapabilityModel{TenantId: 7, AccountId: 8, AccountType: "uploader", ProfileDetailViewEnabled: 1}
		account := &sysin.AccountModel{TenantId: 7, Id: 8}
		for _, tc := range []struct {
			selected int64
			want     int
		}{{0, 4}, {8, 1}, {9, 3}} {
			res, e := s.profileFilterOptions(ctx, account, cap, tc.selected)
			if e != nil {
				return e
			}
			if len(res.Accounts) != 2 || len(res.SourceChannels) != tc.want || len(res.PushChannels) != 1 {
				t.Fatalf("selected%d options%+v source%+v", tc.selected, res, res.SourceChannels)
			}
			seen := map[string]bool{}
			for _, ch := range res.SourceChannels {
				key := canonicalCollectSourceChatID(ch.SourceChatId)
				if seen[key] || ch.SourceId != 0 {
					t.Fatalf("duplicate channel or rule-scoped option: %+v", ch)
				}
				seen[key] = true
				if ch.SourceChatId == "-100104" || ch.SourceChatId == "-100999" {
					t.Fatal("cross tenant options")
				}
			}
		}
		if _, e := s.profileFilterOptions(ctx, account, cap, 10); e == nil {
			t.Fatal("cross tenant account accepted")
		}
		cap.ProfileDetailViewEnabled = 0
		res, e := s.profileFilterOptions(ctx, account, cap, 0)
		if e != nil {
			return e
		}
		if len(res.Accounts) != 1 || len(res.SourceChannels) != 1 {
			t.Fatalf("revoked options %+v", res)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
