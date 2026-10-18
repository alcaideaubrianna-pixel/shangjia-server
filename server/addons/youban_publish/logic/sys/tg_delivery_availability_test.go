package sys

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestTelegramDeliveryAvailabilityLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_PERMISSION_LOCAL_PG") != "1" {
		t.Skip("requires isolated local PostgreSQL")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, query := range []string{
			"CREATE TEMP TABLE " + publishTgAccountTable + " (id bigint,tenant_id bigint,status text,session_key text,deleted_at timestamp,deleted_by bigint)",
			"CREATE TEMP TABLE " + publishBotTable + " (id bigint,tenant_id bigint,tg_account_id bigint,status int,bot_token text,deleted_at timestamp)",
			"CREATE TEMP TABLE " + publishChannelTable + " (id bigint,tenant_id bigint,tg_account_id bigint,status int,deleted_at timestamp)",
			"CREATE TEMP TABLE " + publishTgChannelTable + " (id bigint,tenant_id bigint,tg_account_id bigint)",
			"CREATE TEMP TABLE " + publishCollectSourceTable + " (id bigint,tenant_id bigint,tg_account_id bigint,deleted_at timestamp)",
			"CREATE TEMP TABLE " + publishTgJobTable + " (id bigint,tenant_id bigint,channel_id bigint,bot_id bigint,push_mode text,status text,send_phase text,target_chat_id text DEFAULT '-100123',dispatch_status text DEFAULT 'idle',next_retry_at timestamp,created_at timestamp DEFAULT NOW(),operation_no text DEFAULT 'collect:1')",
			"CREATE TEMP TABLE " + publishTgOutboxTable + " (id bigint,job_id bigint,attempt_count int,available_at timestamp,event_type text,status text,lease_expires_at timestamp)",
			"INSERT INTO " + publishTgOutboxTable + " VALUES (1,1,0,NOW()-INTERVAL '1 hour','publish','pending',NULL)",
			"INSERT INTO " + publishTgAccountTable + " VALUES (81,87,'authorized','old',NOW(),0),(82,87,'authorized','valid',NULL,0),(83,88,'authorized','other',NULL,0),(84,87,'expired','',NULL,0)",
			"INSERT INTO " + publishBotTable + " VALUES (80,87,0,1,'test-token',NULL),(81,87,0,2,'disabled',NULL),(82,88,0,1,'other-tenant',NULL),(83,0,0,1,'global',NULL),(84,87,0,1,'',NULL),(85,87,0,1,'deleted',NOW())",
			"INSERT INTO " + publishChannelTable + " VALUES (256,87,81,1,NULL)",
		} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}
		for _, test := range []struct {
			name      string
			bot       int64
			mode      string
			available bool
		}{
			{"Bot ignores deleted protocol account", 80, "bot", true},
			{"legacy default Bot mode", 80, "", true},
			{"explicit protocol cannot use Bot bypass", 80, "account", false},
			{"disabled Bot", 81, "bot", false},
			{"cross tenant Bot", 82, "bot", false},
			{"global Bot", 83, "bot", true},
			{"empty token", 84, "bot", false},
			{"deleted Bot", 85, "bot", false},
		} {
			if _, err := tx.Exec("TRUNCATE " + publishTgJobTable); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO "+publishTgJobTable+" (id,tenant_id,channel_id,bot_id,push_mode,status,send_phase) VALUES (1,87,256,?,?,'pending','')", test.bot, test.mode); err != nil {
				return err
			}
			_, available, err := checkTelegramJobDeliveryAvailability(ctx, db, telegramJobRecord{Id: 1, TenantId: 87, ChannelId: 256})
			if available != test.available || (test.available && err != nil) || (!test.available && !errors.Is(err, errTelegramPublishAccountMissing)) {
				t.Fatalf("%s: available=%v err=%v", test.name, available, err)
			}
			row, err := tx.GetOne("SELECT " + telegramChannelAvailableAccountSQL("j") + " AS eligible FROM " + publishTgJobTable + " j")
			if err != nil {
				return err
			}
			if !row["eligible"].Bool() {
				t.Fatalf("%s: missing account must reach diagnostic gate", test.name)
			}
			rows, err := tx.GetAll(telegramDueOutboxPostgresSQL(), time.Now(), "publish", time.Now(), "pending", "processing", time.Now(), 10)
			if err != nil {
				return err
			}
			if len(rows) != 1 {
				t.Fatalf("%s: Outbox did not select channel head", test.name)
			}
		}
		for _, id := range []int64{81, 83} {
			_, err := checkTelegramProtocolAccount(ctx, db, 87, id)
			if !errors.Is(err, errTelegramPublishAccountMissing) {
				t.Fatalf("deleted/cross tenant account %d: %v", id, err)
			}
		}
		if ok, err := checkTelegramProtocolAccount(ctx, db, 87, 82); !ok || err != nil {
			t.Fatalf("authorized account: %v %v", ok, err)
		}
		if ok, err := checkTelegramProtocolAccount(ctx, db, 87, 84); ok || err != nil {
			t.Fatalf("expired account: %v %v", ok, err)
		}
		if _, err := tx.Exec("UPDATE " + publishChannelTable + " SET tg_account_id=82"); err != nil {
			return err
		}
		if err := removeTelegramAccountsWithDB(ctx, db, []int64{82}, 87, 628); err == nil || !strings.Contains(err.Error(), "上架频道") {
			t.Fatalf("referenced account deletion should fail: %v", err)
		}
		row, err := tx.GetOne("SELECT deleted_at FROM " + publishTgAccountTable + " WHERE id=82")
		if err != nil {
			return err
		}
		if !row["deleted_at"].IsNil() {
			t.Fatal("blocked deletion changed account")
		}
		if _, err := tx.Exec("DELETE FROM " + publishChannelTable); err != nil {
			return err
		}
		for _, reference := range []struct{ table, name string }{
			{publishCollectSourceTable, "采集配置"},
			{publishBotTable, "机器人配置"},
		} {
			if _, err := tx.Exec("INSERT INTO " + reference.table + " (id,tenant_id,tg_account_id) VALUES (100,87,82)"); err != nil {
				return err
			}
			if err := removeTelegramAccountsWithDB(ctx, db, []int64{82}, 87, 628); err == nil || !strings.Contains(err.Error(), reference.name) {
				t.Fatalf("%s reference was not protected: %v", reference.name, err)
			}
			if _, err := tx.Exec("DELETE FROM " + reference.table + " WHERE id=100"); err != nil {
				return err
			}
		}
		if err := removeTelegramAccountsWithDB(ctx, db, []int64{82}, 88, 628); err != nil {
			return err
		}
		row, err = tx.GetOne("SELECT deleted_at FROM " + publishTgAccountTable + " WHERE id=82")
		if err != nil {
			return err
		}
		if !row["deleted_at"].IsNil() {
			t.Fatal("cross tenant deletion changed account")
		}
		if err := removeTelegramAccountsWithDB(ctx, db, []int64{82}, 87, 628); err != nil {
			return err
		}
		row, err = tx.GetOne("SELECT deleted_at,deleted_by FROM " + publishTgAccountTable + " WHERE id=82")
		if err != nil {
			return err
		}
		if row["deleted_at"].IsNil() || row["deleted_by"].Int64() != 628 {
			t.Fatal("unreferenced account was not deleted with audit owner")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
