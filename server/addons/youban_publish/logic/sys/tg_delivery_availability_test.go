package sys

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
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
			"CREATE TEMP TABLE " + publishTgJobTable + " (id bigint,tenant_id bigint,channel_id bigint,bot_id bigint,push_mode text,status text,send_phase text,target_chat_id text DEFAULT '-100123',dispatch_status text DEFAULT 'idle',next_retry_at timestamp,created_at timestamp DEFAULT NOW(),operation_no text DEFAULT 'collect:1',priority int DEFAULT 50)",
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
			rows, err := tx.GetAll(telegramDueOutboxPostgresSQL(), "publish", time.Now(), "pending", "processing", time.Now(), time.Now(), "publish", time.Now(), "pending", "processing", time.Now())
			if err != nil {
				return err
			}
			if len(rows) != 1 {
				t.Fatalf("%s: Outbox did not select channel head", test.name)
			}
		}
		// Bot delivery must also survive an expired or unbound protocol account.
		for _, accountID := range []int64{84, 0} {
			if _, err := tx.Exec("UPDATE "+publishChannelTable+" SET tg_account_id=?", accountID); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE " + publishTgJobTable + " SET bot_id=80,push_mode='bot'"); err != nil {
				return err
			}
			_, available, err := checkTelegramJobDeliveryAvailability(ctx, db, telegramJobRecord{Id: 1, TenantId: 87, ChannelId: 256})
			if !available || err != nil {
				t.Fatalf("Bot with protocol account %d: available=%v err=%v", accountID, available, err)
			}
			rows, err := tx.GetAll(telegramDueOutboxPostgresSQL(), "publish", time.Now(), "pending", "processing", time.Now(), time.Now(), "publish", time.Now(), "pending", "processing", time.Now())
			if err != nil {
				return err
			}
			if len(rows) != 1 {
				t.Fatalf("Bot with protocol account %d: Outbox blocked", accountID)
			}
		}
		// Exercise the complete PostgreSQL Outbox head query with the backlog
		// shape observed in production: an untouched historical AI repair must
		// yield to a later interactive BOT publish, but becomes a hard barrier as
		// soon as its worker starts.
		if _, err := tx.Exec("TRUNCATE " + publishTgJobTable + "," + publishTgOutboxTable); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgJobTable + ` (id,tenant_id,channel_id,bot_id,push_mode,status,send_phase,operation_no,created_at) VALUES
			(1,87,256,80,'bot','pending','','ai-republish:old',NOW()-INTERVAL '1 day'),
			(2,87,256,80,'bot','pending','','bot_publish:new',NOW()-INTERVAL '1 minute')`); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgOutboxTable + ` VALUES
			(1,1,0,NOW()-INTERVAL '1 day','publish','pending',NULL),
			(2,2,0,NOW()-INTERVAL '1 minute','publish','pending',NULL)`); err != nil {
			return err
		}
		rows, err := tx.GetAll(telegramDueOutboxPostgresSQL(), "publish", time.Now(), "pending", "processing", time.Now(), time.Now(), "publish", time.Now(), "pending", "processing", time.Now())
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["job_id"].Int64() != 2 {
			t.Fatalf("interactive BOT publish was not selected ahead of untouched AI backlog: %+v", rows)
		}
		if _, err := tx.Exec("UPDATE " + publishTgJobTable + " SET status='sending',send_phase='display_sending' WHERE id=1"); err != nil {
			return err
		}
		rows, err = tx.GetAll(telegramDueOutboxPostgresSQL(), "publish", time.Now(), "pending", "processing", time.Now(), time.Now(), "publish", time.Now(), "pending", "processing", time.Now())
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatalf("started operation must hold the channel while its worker owns delivery: %+v", rows)
		}
		// Across channels, interactive work must be selected before an older
		// cycle. An unavailable channel is still returned so validation can fail
		// its head explicitly and release that channel's successor.
		if _, err := tx.Exec("TRUNCATE " + publishTgJobTable + "," + publishTgOutboxTable); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgJobTable + ` (id,tenant_id,channel_id,bot_id,push_mode,status,send_phase,operation_no,created_at,priority) VALUES
			(11,87,301,80,'bot','pending','','cycle_batch:old',NOW()-INTERVAL '1 day',90),
			(12,87,302,80,'bot','pending','','profile:manual',NOW()-INTERVAL '1 minute',1),
			(13,87,303,84,'bot','pending','','profile:invalid-bot',NOW()-INTERVAL '2 minutes',1),
			(14,87,302,80,'bot','pending','','profile:manual-next',NOW(),1)`); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgOutboxTable + ` VALUES
			(11,11,0,NOW()-INTERVAL '1 day','publish','pending',NULL),
			(12,12,0,NOW()-INTERVAL '1 minute','publish','pending',NULL),
			(13,13,0,NOW()-INTERVAL '2 minutes','publish','pending',NULL),
			(14,14,0,NOW(),'publish','pending',NULL)`); err != nil {
			return err
		}
		rows, err = tx.GetAll(telegramDueOutboxPostgresSQL(), "publish", time.Now(), "pending", "processing", time.Now(), time.Now(), "publish", time.Now(), "pending", "processing", time.Now())
		if err != nil {
			return err
		}
		if len(rows) != 3 || rows[0]["job_id"].Int64() != 13 || rows[1]["job_id"].Int64() != 12 || rows[2]["job_id"].Int64() != 11 {
			t.Fatalf("cross-channel priority/isolation failed: %+v", rows)
		}
		// A scheduler pass is bounded by channel heads, not a global row limit.
		// All 1000 channels must be represented while the second job in each
		// channel remains behind its head.
		if _, err := tx.Exec("TRUNCATE " + publishTgJobTable + "," + publishTgOutboxTable); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgJobTable + ` (id,tenant_id,channel_id,bot_id,push_mode,status,send_phase,operation_no,created_at,priority)
			SELECT n,87,n,80,'bot','pending','',CASE WHEN n%10=0 THEN 'profile:'||n ELSE 'cycle_batch:'||n END,NOW()-INTERVAL '1 hour',CASE WHEN n%10=0 THEN 1 ELSE 90 END FROM generate_series(1,1000) n
			UNION ALL SELECT 2000+n,87,n,80,'bot','pending','',CASE WHEN n%10=0 THEN 'profile:later:'||n ELSE 'cycle_batch:later:'||n END,NOW(),CASE WHEN n%10=0 THEN 1 ELSE 90 END FROM generate_series(1,1000) n`); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgOutboxTable + ` (id,job_id,attempt_count,available_at,event_type,status,lease_expires_at)
			SELECT n,n,0,NOW()-INTERVAL '1 hour','publish','pending',NULL::timestamp FROM generate_series(1,1000) n
			UNION ALL SELECT 2000+n,2000+n,0,NOW(),'publish','pending',NULL::timestamp FROM generate_series(1,1000) n`); err != nil {
			return err
		}
		rows, err = tx.GetAll(telegramDueOutboxPostgresSQL(), "publish", time.Now(), "pending", "processing", time.Now(), time.Now(), "publish", time.Now(), "pending", "processing", time.Now())
		if err != nil {
			return err
		}
		if len(rows) != 1000 {
			t.Fatalf("1000 channels returned %d heads", len(rows))
		}
		for index, row := range rows {
			jobID := row["job_id"].Int64()
			if jobID > 1000 {
				t.Fatalf("channel returned a later job: index=%d job=%d", index, jobID)
			}
			if index < 100 && jobID%10 != 0 {
				t.Fatalf("manual channel head lost priority: index=%d job=%d", index, jobID)
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
		if err := removeTelegramAccountsWithDB(ctx, db, []int64{82}, 87, 628); err == nil || gerror.Current(err).Error() != "TG账号82仍被1个上架频道引用，请先更换绑定账号，再删除或解绑" {
			t.Fatalf("referenced account deletion should fail: %v", err)
		}
		// Server delete/unbind uses tenantID=0 and must preserve the same
		// actionable message consumed by the HTTP response middleware.
		if err := removeTelegramAccountsWithDB(ctx, db, []int64{82}, 0, 628); err == nil || gerror.Current(err).Error() != "TG账号82仍被1个上架频道引用，请先更换绑定账号，再删除或解绑" {
			t.Fatalf("server deletion lost reference message: %v", err)
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
