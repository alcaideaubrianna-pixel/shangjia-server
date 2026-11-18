package sys

import (
	"context"
	"os"
	"testing"
	"time"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
)

// The bound marker must suppress discovery writes without caching permission.
func TestCollectChannelBoundMarkerPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	t.Cleanup(func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.Background()
	if !cache.Initialized() {
		adapter := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
		original := g.Cfg().MustGet(ctx, "cache.adapter").Val()
		if err := adapter.Set("cache.adapter", "memory"); err != nil {
			t.Fatal(err)
		}
		cache.SetAdapter(ctx)
		if err := adapter.Set("cache.adapter", original); err != nil {
			t.Fatal(err)
		}
	}
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previous) })
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			"CREATE TEMP TABLE hg_youban_publish_collect_channel (id bigint,tenant_id bigint,telegram_chat_id text,title text,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_collect_account_channel (id bigint,tenant_id bigint,account_id bigint,channel_id bigint,source_id bigint,collect_enabled int,status text,approval_status text,disabled_reason text NOT NULL DEFAULT '',updated_at timestamp,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_collect_bot_channel (id bigserial PRIMARY KEY,tenant_id bigint,bot_id bigint,channel_id bigint,status text,bot_role text,last_seen_at timestamp,last_update_id bigint,removed_at timestamp,removed_reason text,created_at timestamp,updated_at timestamp,deleted_at timestamp,UNIQUE(tenant_id,bot_id,channel_id)) ON COMMIT DROP",
			"INSERT INTO hg_youban_publish_collect_bot_channel(id,tenant_id,bot_id,channel_id,status,updated_at) VALUES (1,56,109,738019,'discovered',NOW())",
			"CREATE TEMP TABLE hg_youban_publish_collect_source (id bigint,tenant_id bigint,bot_id bigint,source_chat_id text,channel_id bigint,deleted_at timestamp,updated_at timestamp) ON COMMIT DROP",
			"INSERT INTO hg_youban_publish_collect_channel VALUES (738019,56,'-1004444998796','fixture',NULL)",
			"INSERT INTO hg_youban_publish_collect_account_channel(id,tenant_id,account_id,channel_id,source_id,collect_enabled,status,approval_status,updated_at,deleted_at) VALUES (1,56,561,738019,1370,1,'enabled','approved',NOW(),NULL)",
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		key := publishconsts.CollectChannelBoundKeyPrefix + collectChannelAccessRelationKey(56, 561, 1370, "4444998796") + ":bot:109"
		if err := cache.Instance().Set(ctx, key, true, 5*time.Minute); err != nil {
			return err
		}
		source := gdb.Record{
			"id": g.NewVar(1370), "tenant_id": g.NewVar(56),
			"account_id": g.NewVar(561), "bot_id": g.NewVar(109),
		}
		// Both Chat ID forms must hit the same marker. Missing metadata-write
		// columns in the fixture make an accidental discovery write fail.
		service := &sSysPublish{}
		for _, tc := range []struct {
			name, chatID, status, approval string
			enabled                        int
			want                           bool
		}{
			{"enabled", "-1004444998796", "enabled", "approved", 1, true},
			{"short alias", "4444998796", "enabled", "approved", 1, true},
			{"manual disable", "-1004444998796", "disabled", "approved", 0, false},
			{"manual reject", "-1004444998796", "enabled", "rejected", 1, false},
			{"review pending", "-1004444998796", "pending", "pending", 0, false},
		} {
			if _, err := tx.Exec("UPDATE hg_youban_publish_collect_account_channel SET status=$1,approval_status=$2,collect_enabled=$3", tc.status, tc.approval, tc.enabled); err != nil {
				return err
			}
			service.refreshCollectSourceCache(ctx)
			got, err := service.ensureBotCollectChannelApproval(ctx, source, &collectorin.CollectorDelivery{SourceChatID: tc.chatID})
			if err != nil || got != tc.want {
				t.Fatalf("%s: allowed=%v err=%v want=%v", tc.name, got, err, tc.want)
			}
		}
		if _, err := tx.Exec("UPDATE hg_youban_publish_collect_account_channel SET status='enabled',approval_status='approved',collect_enabled=1"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_channel
			(id,tenant_id,telegram_chat_id,title) VALUES (738020,56,'4444998796','short alias')`); err != nil {
			return err
		}
		checkApproval := func(t *testing.T, chat string, want bool) {
			t.Helper()
			service.refreshCollectSourceCache(ctx)
			got, err := service.ensureBotCollectChannelApproval(ctx, source, &collectorin.CollectorDelivery{SourceChatID: chat})
			if err != nil || got != want {
				t.Fatalf("chat %q: allowed=%v err=%v want=%v", chat, got, err, want)
			}
		}
		t.Run("bound_cache_ignores_foreign_blacklist_markers", func(t *testing.T) {
			for _, tc := range []struct {
				name                string
				tenantID, accountID int64
			}{
				{"other account", 56, 562},
				{"other tenant same channel", 57, 561},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_account_channel
						(id,tenant_id,account_id,channel_id,source_id,collect_enabled,status,approval_status,disabled_reason)
						VALUES(2,$1,$2,738020,0,0,'disabled','rejected',$3)`, tc.tenantID, tc.accountID, introFeeChannelBlacklistReason); err != nil {
						t.Fatal(err)
					}
					checkApproval(t, "-1004444998796", true)
					checkApproval(t, "4444998796", true)
					if _, err := tx.Exec(`DELETE FROM hg_youban_publish_collect_account_channel WHERE id=2`); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
		t.Run("blacklist_blocks_bound_and_uncached_bot_approval", func(t *testing.T) {
			for _, channelID := range []int64{738019, 738020} {
				for _, status := range []string{"disabled", "enabled", "pending"} {
					if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_account_channel
						(id,tenant_id,account_id,channel_id,source_id,collect_enabled,status,approval_status,disabled_reason,updated_at)
						VALUES(3,56,561,$1,0,1,$2,'approved',$3,'2026-11-15 01:00:00')`,
						channelID, status, introFeeChannelBlacklistReason); err != nil {
						t.Fatal(err)
					}
					for _, bound := range []bool{true, false} {
						for _, chat := range []string{"-1004444998796", "4444998796"} {
							if bound {
								if err := cache.Instance().Set(ctx, key, true, 5*time.Minute); err != nil {
									t.Fatal(err)
								}
							} else if _, err := cache.Instance().Remove(ctx, key); err != nil {
								t.Fatal(err)
							}
							// Clear rejection decisions between calls so each assertion
							// exercises the bound resolver or uncached database check.
							checkApproval(t, chat, false)
						}
					}
					if _, err := tx.Exec(`DELETE FROM hg_youban_publish_collect_account_channel WHERE id=3`); err != nil {
						t.Fatal(err)
					}
					if err := cache.Instance().Set(ctx, key, true, 5*time.Minute); err != nil {
						t.Fatal(err)
					}
					checkApproval(t, "-1004444998796", true)
					checkApproval(t, "4444998796", true)
				}
			}
		})
		if _, err := tx.Exec("UPDATE hg_youban_publish_collect_bot_channel SET status='bot_removed'"); err != nil {
			return err
		}
		if err := service.touchCollectBotChannel(ctx, 56, 109, "-1004444998796", gtime.Now()); err != nil {
			return err
		}
		botRow, err := tx.GetOne("SELECT status FROM hg_youban_publish_collect_bot_channel WHERE tenant_id=56 AND bot_id=109 AND channel_id=738019")
		if err != nil {
			return err
		}
		if botRow["status"].String() != "bot_removed" {
			t.Fatal("ordinary channel-cache touch revived removed BOT")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
