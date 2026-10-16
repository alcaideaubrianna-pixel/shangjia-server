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

func TestTelegramAPITimeoutFallback(t *testing.T) {
	if !shouldFallbackTelegramMediaToAccount(context.DeadlineExceeded) {
		t.Fatal("API timeout must hand off to Account")
	}
	if isTelegramAPITimeout(context.Canceled) {
		t.Fatal("shutdown cancellation must not start a new send")
	}
	if isTelegramAPITimeout(errors.New("bad request")) {
		t.Fatal("rejection is not a timeout")
	}
	if telegramSendBudget != 10*time.Minute {
		t.Fatal("publication must have a ten-minute total budget")
	}
}

func TestTelegramSendWatchdogLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_HEAD_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL on port 35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	install, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../resource/sql/telegram_send_watchdog.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, table := range []string{publishTgJobTable, publishProfileStateTable} {
			start := strings.Index(string(install), `CREATE TABLE IF NOT EXISTS "`+table+`"`)
			if start < 0 {
				t.Fatalf("missing fixture table %s", table)
			}
			ddl := string(install)[start:]
			ddl = strings.Replace(ddl[:strings.Index(ddl, ");")+2], "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1)
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		for _, ddl := range []string{
			"ALTER TABLE " + publishTgJobTable + " ADD COLUMN push_mode text DEFAULT ''",
			"CREATE TEMP TABLE " + publishTgAttemptTable + " (id bigint,job_id bigint,status text,webhook_deadline timestamp)",
			"CREATE TEMP TABLE " + publishTgOutboxTable + " (job_id bigint,status text,lease_owner text,lease_expires_at timestamp,last_error text,delivered_at timestamp,updated_at timestamp)",
			"CREATE TEMP TABLE hg_content_profile(id bigint,status int,visibility text,deleted_at timestamp)",
			"CREATE TEMP TABLE " + publishChannelTable + " (id bigint,tenant_id bigint,status int,deleted_at timestamp)",
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		// Temporary fixture tables are transaction-local; PostgreSQL forbids
		// CONCURRENTLY in a transaction, unlike the explicit production runner.
		if _, err := tx.Exec(strings.ReplaceAll(string(migration), " CONCURRENTLY", "")); err != nil {
			return err
		}
		for _, sql := range []string{
			"INSERT INTO hg_content_profile VALUES(1,1,'public',NULL)",
			"INSERT INTO " + publishChannelTable + " VALUES(1,1,1,NULL)",
			"INSERT INTO " + publishProfileStateTable + "(tenant_id,account_id,profile_id,publish_operation_no) VALUES(1,1,1,'collect:1')",
			"INSERT INTO " + publishTgJobTable + "(id,tenant_id,account_id,profile_id,channel_id,operation_no,status,send_phase,send_deadline_at,collect_event_id) VALUES(100,1,1,1,1,'collect:1','sending','display_confirmed',LOCALTIMESTAMP - interval '1 minute',77)",
			"SELECT setval(pg_get_serial_sequence('" + publishTgJobTable + "','id'),100)",
			"INSERT INTO " + publishTgAttemptTable + " VALUES(1,100,'sending',LOCALTIMESTAMP)",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		s := NewSysPublish()
		old, tailID, err := s.expireTelegramSendTx(ctx, tx, 100)
		if err != nil {
			return err
		}
		if tailID <= 100 || old.CollectEventId != 77 {
			t.Fatal("timeout did not preserve source and move behind existing jobs")
		}
		var tail telegramJobRecord
		if err := tx.Model(publishTgJobTable).Where("id", tailID).Scan(&tail); err != nil {
			return err
		}
		if tail.Status != telegramJobStatusFailed || tail.SendPhase != "timeout_wait_cleanup" || tail.ProfileId != old.ProfileId || tail.CollectEventId != 77 || tail.OperationNo != old.OperationNo {
			t.Fatalf("unsafe replacement: %+v", tail)
		}
		_, again, err := s.expireTelegramSendTx(ctx, tx, 100)
		if err != nil {
			return err
		}
		if again != 0 {
			t.Fatal("repeated watchdog created another replacement")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
