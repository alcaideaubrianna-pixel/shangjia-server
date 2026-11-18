package sys

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// This fixture deliberately has no source configuration table. Authorization
// must resolve the event's actual chat, never the source's configured channel.
func TestCollectEventChannelAccessLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	previousDB := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres",
	}})
	t.Cleanup(func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		defer tx.Ctx(ctx)
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_channel(
				id bigint PRIMARY KEY, tenant_id bigint, telegram_chat_id text,
				title text, deleted_at timestamp
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_account_channel(
				id bigint PRIMARY KEY, tenant_id bigint, account_id bigint,
				channel_id bigint, source_id bigint, collect_enabled smallint,
				status text, approval_status text, disabled_reason text NOT NULL DEFAULT '', updated_at timestamp,
				deleted_at timestamp,
				UNIQUE(tenant_id,account_id,channel_id,source_id)
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_bot_channel(
				id bigint PRIMARY KEY, tenant_id bigint, bot_id bigint,
				channel_id bigint, status text, updated_at timestamp,
				deleted_at timestamp,
				UNIQUE(tenant_id,bot_id,channel_id)
			) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_channel(id,tenant_id,telegram_chat_id,title) VALUES
				(1279,78,'-1003856373273','configured disabled channel'),
				(1280,78,'-1003763752459','actual event channel'),
				(1281,78,'3763752459','actual short alias'),
				(2280,79,'-1003763752459','other tenant channel')`,
			`INSERT INTO hg_youban_publish_collect_account_channel
				(id,tenant_id,account_id,channel_id,source_id,collect_enabled,status,approval_status,updated_at) VALUES
				(1,78,615,1279,351,0,'disabled','rejected','2026-11-17 01:00:00'),
				(2,78,615,1280,351,1,'enabled','approved','2026-11-16 01:00:00'),
				(3,78,615,1280,352,0,'disabled','rejected','2026-11-18 01:00:00'),
				(4,79,615,2280,351,0,'disabled','rejected','2026-11-18 01:00:00'),
				(5,78,616,1280,351,0,'disabled','rejected','2026-11-18 01:00:00')`,
			`INSERT INTO hg_youban_publish_collect_bot_channel
				(id,tenant_id,bot_id,channel_id,status,updated_at) VALUES
				(101,78,64,1279,'removed','2026-11-19 01:00:00'),
				(102,78,64,1280,'discovered','2026-11-17 01:00:00'),
				(103,78,64,1281,'removed','2026-11-16 01:00:00'),
				(104,78,65,1280,'removed','2026-11-19 01:00:00'),
				(105,79,64,1280,'removed','2026-11-19 01:00:00')`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		event := func(chat string) gdb.Record {
			return gdb.Record{
				"id": gvar.New(19182174), "tenant_id": gvar.New(78),
				"account_id": gvar.New(0), "source_id": gvar.New(351),
				"source_chat_id": gvar.New(chat),
				// Decoys represent the unrelated source configuration.
				"channel_id": gvar.New(1279), "source_channel_id": gvar.New(1279),
			}
		}
		check := func(t *testing.T, chat string, allowed bool, channelID, relationID int64) {
			t.Helper()
			got, err := resolveCollectEventChannelAccess(ctx, event(chat), 615)
			if err != nil {
				t.Fatalf("resolve actual chat %q: %v", chat, err)
			}
			if got.Allowed != allowed || got.ChannelID != channelID || got.Relation.Id != relationID {
				t.Fatalf("chat %q: got %+v; want allowed=%v channel=%d relation=%d",
					chat, got, allowed, channelID, relationID)
			}
			if got.Relation.SourceId != 351 {
				t.Fatalf("relation escaped source scope: %+v", got.Relation)
			}
		}
		t.Run("actual_channel_not_configured_channel_and_foreign_denials_ignored", func(t *testing.T) {
			check(t, "-1003763752459", true, 1280, 2)
			check(t, "3763752459", true, 1280, 2)
		})
		t.Run("channel_blacklist_overrides_enabled_source_and_removal_restores_allow", func(t *testing.T) {
			for _, markerChannelID := range []int64{1280, 1281} {
				for _, status := range []string{"disabled", "enabled", "pending"} {
					if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_account_channel
						(id,tenant_id,account_id,channel_id,source_id,collect_enabled,status,approval_status,disabled_reason,updated_at)
						VALUES(10,78,615,$1,0,1,$2,'approved',$3,'2026-11-15 01:00:00')`,
						markerChannelID, status, introFeeChannelBlacklistReason); err != nil {
						t.Fatal(err)
					}
					for _, chat := range []string{"-1003763752459", "3763752459"} {
						// The older account marker must override the newer enabled
						// source relation without replacing its identity.
						check(t, chat, false, 1280, 2)
					}
					if _, err := tx.Exec(`DELETE FROM hg_youban_publish_collect_account_channel WHERE id=10`); err != nil {
						t.Fatal(err)
					}
					check(t, "-1003763752459", true, 1280, 2)
					check(t, "3763752459", true, 1280, 2)
				}
			}
		})
		t.Run("unrelated_blacklist_markers_do_not_block_enabled_source", func(t *testing.T) {
			for _, tc := range []struct {
				name                           string
				tenantID, accountID, channelID int64
				sourceID                       int64
				reason                         string
			}{
				{"other account", 78, 616, 1281, 0, introFeeChannelBlacklistReason},
				{"other tenant same channel", 79, 615, 1281, 0, introFeeChannelBlacklistReason},
				{"configured channel", 78, 615, 1279, 0, introFeeChannelBlacklistReason},
				{"specific source is not account marker", 78, 615, 1281, 352, introFeeChannelBlacklistReason},
				{"different reason", 78, 615, 1281, 0, "manual_disable"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_account_channel
						(id,tenant_id,account_id,channel_id,source_id,collect_enabled,status,approval_status,disabled_reason)
						VALUES(11,$1,$2,$3,$4,0,'disabled','rejected',$5)`,
						tc.tenantID, tc.accountID, tc.channelID, tc.sourceID, tc.reason); err != nil {
						t.Fatal(err)
					}
					check(t, "-1003763752459", true, 1280, 2)
					check(t, "3763752459", true, 1280, 2)
					if _, err := tx.Exec(`DELETE FROM hg_youban_publish_collect_account_channel WHERE id=11`); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
		t.Run("missing_source_subscription_follows_default_discovery_policy", func(t *testing.T) {
			got, err := resolveCollectEventChannelAccess(ctx, event("-1003763752459"), 617)
			if err != nil || !got.Allowed || got.Relation.Id != 0 || got.ChannelID != 1280 {
				t.Fatalf("missing relation must follow discovery policy: %+v err=%v", got, err)
			}
		})
		checkBot := func(t *testing.T, chat string, botID, relationID, channelID int64, status string) {
			t.Helper()
			access, err := resolveCollectEventChannelAccess(ctx, event(chat), 615)
			if err != nil {
				t.Fatalf("resolve actual chat for Bot: %v", err)
			}
			// Both actual aliases must survive resolution; configured channel
			// 1279 and other tenants must never enter the Bot relation lookup.
			seen := make(map[int64]bool)
			for _, id := range access.ChannelIDs {
				seen[id] = true
			}
			if len(access.ChannelIDs) != 2 || !seen[1280] || !seen[1281] {
				t.Fatalf("unexpected actual alias IDs: %v", access.ChannelIDs)
			}
			got, err := resolveCollectEventBotChannel(ctx, 78, botID, access)
			if err != nil {
				t.Fatalf("resolve actual Bot channel: %v", err)
			}
			if got.Id != relationID || got.ChannelId != channelID || got.Status != status {
				t.Fatalf("got Bot relation %+v; want id=%d channel=%d status=%q",
					got, relationID, channelID, status)
			}
		}
		t.Run("bot_configured_removed_and_foreign_relations_do_not_override_actual", func(t *testing.T) {
			checkBot(t, "-1003763752459", 64, 102, 1280, "discovered")
			checkBot(t, "3763752459", 64, 102, 1280, "discovered")
		})
		t.Run("bot_missing_relation_returns_zero_without_error", func(t *testing.T) {
			checkBot(t, "-1003763752459", 66, 0, 0, "")
		})
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_bot_channel
			SET updated_at='2026-11-17 02:00:00' WHERE id=103`); err != nil {
			return err
		}
		t.Run("bot_latest_actual_alias_removed_is_visible", func(t *testing.T) {
			checkBot(t, "-1003763752459", 64, 103, 1281, "removed")
			checkBot(t, "3763752459", 64, 103, 1281, "removed")
		})
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_bot_channel
			SET updated_at='2026-11-17 03:00:00' WHERE id=102`); err != nil {
			return err
		}
		t.Run("bot_newest_discovered_overrides_older_alias_removed", func(t *testing.T) {
			checkBot(t, "3763752459", 64, 102, 1280, "discovered")
		})
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_bot_channel
			SET updated_at='2026-11-17 03:00:00' WHERE id=103`); err != nil {
			return err
		}
		t.Run("bot_equal_timestamp_uses_descending_relation_id", func(t *testing.T) {
			checkBot(t, "-1003763752459", 64, 103, 1281, "removed")
		})
		t.Run("actual_disabled_channel_still_denied", func(t *testing.T) {
			check(t, "-1003856373273", false, 1279, 1)
		})
		t.Run("unknown_or_empty_chat_never_falls_back", func(t *testing.T) {
			for _, chat := range []string{"", "-1004999999999", "4999999999"} {
				got, err := resolveCollectEventChannelAccess(ctx, event(chat), 615)
				if err == nil || got.Allowed || got.ChannelID != 0 {
					t.Errorf("chat %q must fail without configured-channel fallback: got %+v err=%v", chat, got, err)
				}
			}
		})
		// A newer manual denial on the short alias overrides the older allow,
		// even when the incoming chat uses the full Bot API ID.
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_account_channel
			(id,tenant_id,account_id,channel_id,source_id,collect_enabled,status,approval_status,updated_at)
			VALUES(6,78,615,1281,351,0,'disabled','rejected','2026-11-17 02:00:00')`); err != nil {
			return err
		}
		t.Run("newest_alias_denial_wins", func(t *testing.T) {
			check(t, "-1003763752459", false, 1281, 6)
			check(t, "3763752459", false, 1281, 6)
		})
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_account_channel
			SET updated_at='2026-11-17 03:00:00' WHERE id=2`); err != nil {
			return err
		}
		t.Run("newer_allow_wins_over_older_denial", func(t *testing.T) {
			check(t, "3763752459", true, 1280, 2)
		})
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_account_channel
			SET updated_at='2026-11-17 03:00:00' WHERE id=6`); err != nil {
			return err
		}
		t.Run("equal_timestamp_uses_descending_relation_id", func(t *testing.T) {
			check(t, "-1003763752459", false, 1281, 6)
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
