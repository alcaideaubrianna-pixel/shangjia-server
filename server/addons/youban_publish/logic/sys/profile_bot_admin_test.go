package sys

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestProfileWriteAccountScope(t *testing.T) {
	for _, tc := range []struct {
		cap   *sysin.AccountCapabilityModel
		scope int64
		fail  bool
	}{
		{nil, 0, true},
		{&sysin.AccountCapabilityModel{AccountId: 8, AccountType: "admin"}, 0, true},
		{&sysin.AccountCapabilityModel{TenantId: 7, AccountId: 8, AccountType: "admin"}, 0, false},
		{&sysin.AccountCapabilityModel{TenantId: 7, AccountId: 8, AccountType: "uploader"}, 8, false},
		{&sysin.AccountCapabilityModel{TenantId: 7, AccountId: 8, AccountType: "uploader", SharedResourceEnabled: 1}, 0, false},
	} {
		scope, err := profileWriteAccountScope(tc.cap)
		if (err != nil) != tc.fail || scope != tc.scope {
			t.Fatalf("scope=%d err=%v case=%+v", scope, err, tc)
		}
	}
}

func TestProfileAdminOwnerGroupsLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_STORAGE_PG") != "1" {
		t.Skip("requires local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_content_profile(id bigint PRIMARY KEY,profile_no text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tenant(id bigint PRIMARY KEY) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint PRIMARY KEY,tenant_id bigint,account_type text,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account_setting(account_id bigint,tenant_id bigint,shared_resource_enabled int,telegram_binding_enabled int,group_push_enabled int,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_content_profile VALUES(643999,'FNYA5991185',NULL),(1,'OWN1',NULL),(2,'OTHER2',NULL)`,
			`INSERT INTO hg_youban_publish_account VALUES(8,7,'admin',1,NULL),(139,7,'uploader',1,NULL),(200,9,'uploader',1,NULL)`,
			`INSERT INTO hg_youban_publish_profile_state VALUES(643999,7,139,NULL),(1,7,8,NULL),(2,9,200,NULL),(3,7,139,now())`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		s := NewSysPublish()
		admin := &sysin.AccountCapabilityModel{TenantId: 7, AccountId: 8, AccountType: "admin"}
		groups, err := s.profileOwnerGroups(ctx, []int64{643999}, nil, admin)
		if err != nil || len(groups) != 1 || groups[0].AccountId != 139 {
			t.Fatalf("production regression: groups=%+v err=%v", groups, err)
		}
		for _, ids := range [][]int64{{2}, {3}, {643999, 2}, {643999, 999999999}} {
			if _, err := s.profileOwnerGroups(ctx, ids, nil, admin); err == nil {
				t.Fatalf("unauthorized ids accepted: %v", ids)
			}
		}
		uploader := &sysin.AccountCapabilityModel{TenantId: 7, AccountId: 8, AccountType: "uploader"}
		if _, err := s.profileOwnerGroups(ctx, []int64{643999}, nil, uploader); err == nil {
			t.Fatal("uploader gained admin scope")
		}
		uploader.SharedResourceEnabled = 1
		if _, err := s.profileOwnerGroups(ctx, []int64{643999}, nil, uploader); err != nil {
			t.Fatal(err)
		}
		if _, err := s.profileOwnerGroups(ctx, []int64{2}, nil, uploader); err == nil {
			t.Fatal("shared uploader crossed tenant")
		}
		owner, err := s.editableProfileOwnerAccountId(ctx, 643999, "", admin)
		if err != nil || owner != 139 {
			t.Fatalf("edit owner=%d err=%v", owner, err)
		}
		// Resolve the actual persisted role, not the AccountType supplied by a client.
		capability, err := s.activeAccountCapability(ctx, 7, 8)
		if err != nil {
			return err
		}
		scope, err := profileWriteAccountScope(capability)
		if err != nil || scope != 0 {
			t.Fatalf("persisted admin scope=%d err=%v", scope, err)
		}
		id, err := s.botResolveProfileId(ctx, capability.TenantId, scope, "FNYA5991185", false)
		if err != nil || id != 643999 {
			t.Fatalf("admin profile resolution id=%d err=%v", id, err)
		}
		if _, err := s.BotProfileStatus(ctx, &sysin.BotProfileStatusInp{TenantId: 7, AccountId: 8, AccountType: "admin", Nos: []string{"OTHER2"}, Status: 2}); err == nil {
			t.Fatal("status crossed tenant")
		}
		if _, err := s.BotProfileEdit(ctx, &sysin.BotProfileEditInp{TenantId: 7, AccountId: 8, ProfileNo: "OTHER2", Title: "blocked"}); err == nil {
			t.Fatal("edit crossed tenant")
		}
		if err := s.BotProfileDelete(ctx, 7, 8, &sysin.ProfileDeleteInp{Ids: []int64{2}}); err == nil {
			t.Fatal("delete crossed tenant")
		}
		if _,err:=s.BotProfileStatus(ctx,&sysin.BotProfileStatusInp{TenantId:7,AccountId:139,AccountType:"admin",Nos:[]string{"OWN1"},Status:2});err==nil {t.Fatal("forged admin role accepted")}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
