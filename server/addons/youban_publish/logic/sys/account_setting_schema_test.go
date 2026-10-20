package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestAccountSettingSaveAfterOnlineMigration(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_STORAGE_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.Background()
	t.Cleanup(func() { _ = g.DB().GetCore().ClearTableFields(ctx, publishAccountSettingTable) })
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, query := range []string{
			`CREATE TEMP TABLE hg_youban_publish_account_setting (
			 id bigserial PRIMARY KEY, tenant_id bigint, account_id bigint,
			 enable_suffix int, suffix_content text, enable_title_mark int, mark_mode text,
			 number_source text, custom_mark_text text, mark_position text,
			 shared_resource_enabled int, telegram_binding_enabled int, group_push_enabled int,
			 created_by bigint, updated_by bigint, created_at timestamp, updated_at timestamp, deleted_at timestamp
			) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_account_setting(tenant_id,account_id) VALUES(78,616),(79,617)`,
		} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}
		// Simulate a Web instance that cached the schema before the online migration.
		if _, err := g.DB().TableFields(ctx, publishAccountSettingTable); err != nil {
			return err
		}
		s := NewSysPublish()
		if err := s.saveAccountSetting(ctx, 78, 612, &sysin.AccountSettingSaveInp{AccountId: 616, ProfileDetailViewEnabled: 1}); err == nil || !strings.Contains(err.Error(), "尚未升级") {
			t.Fatalf("missing migration must fail explicitly, got %v", err)
		}
		if _, err := tx.Exec(`ALTER TABLE hg_youban_publish_account_setting ADD COLUMN profile_detail_view_enabled smallint NOT NULL DEFAULT 0`); err != nil {
			return err
		}
		for _, enabled := range []int{1, 0, 1} {
			if err := s.saveAccountSetting(ctx, 78, 612, &sysin.AccountSettingSaveInp{AccountId: 616, ProfileDetailViewEnabled: enabled}); err != nil {
				return err
			}
			got, err := s.accountSetting(ctx, 78, 616)
			if err != nil {
				return err
			}
			if got.ProfileDetailViewEnabled != enabled {
				t.Fatalf("saved %d, reloaded %d", enabled, got.ProfileDetailViewEnabled)
			}
		}
		other, err := s.accountSetting(ctx, 79, 617)
		if err != nil {
			return err
		}
		if other.ProfileDetailViewEnabled != 0 {
			t.Fatal("changed another tenant's permission")
		}
		// A first-time settings row must retain the permission too.
		if err := s.saveAccountSetting(ctx, 78, 612, &sysin.AccountSettingSaveInp{AccountId: 618, ProfileDetailViewEnabled: 1}); err != nil {
			return err
		}
		got, err := s.accountSetting(ctx, 78, 618)
		if err != nil {
			return err
		}
		if got.ProfileDetailViewEnabled != 1 {
			t.Fatal("new settings lost permission")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
