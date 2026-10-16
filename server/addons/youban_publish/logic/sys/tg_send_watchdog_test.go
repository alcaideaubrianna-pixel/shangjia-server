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
			"CREATE TEMP TABLE " + publishTgAttemptTable + " (id bigint,job_id bigint,status text,webhook_deadline timestamp,response_received int DEFAULT 0,expected_count int DEFAULT 2)",
			"CREATE TEMP TABLE " + publishTgAttemptMessageTable + " (attempt_id bigint,message_id bigint)",
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
			"INSERT INTO " + publishTgAttemptTable + "(id,job_id,status,webhook_deadline) VALUES(1,100,'sending',LOCALTIMESTAMP)",
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
		if n, err := telegramTimeoutUnresolvedAttemptCount(tx, 100); err != nil || n != 1 {
			t.Fatalf("unknown send must hold replacement: %d %v", n, err)
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgAttemptMessageTable + " VALUES(1,11)"); err != nil {
			return err
		}
		if n, err := telegramTimeoutUnresolvedAttemptCount(tx, 100); err != nil || n != 1 {
			t.Fatalf("partial album must hold replacement: %d %v", n, err)
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgAttemptMessageTable + " VALUES(1,12)"); err != nil {
			return err
		}
		if n, err := telegramTimeoutUnresolvedAttemptCount(tx, 100); err != nil || n != 1 {
			t.Fatalf("message count alone cannot prove sender completion: %d %v", n, err)
		}
		if _, err := tx.Exec("UPDATE " + publishTgAttemptTable + " SET response_received=1 WHERE id=1"); err != nil {
			return err
		}
		if n, err := telegramTimeoutUnresolvedAttemptCount(tx, 100); err != nil || n != 0 {
			t.Fatalf("explicit response should release gate: %d %v", n, err)
		}
		// Three tail generations are allowed; the fourth expiry must stop.
		for retry := 1; retry <= telegramSendMaxRetries; retry++ {
			if tail.TimeoutRetryCount != retry {
				t.Fatalf("retry counter reset: %+v", tail)
			}
			if _, err := tx.Model(publishTgJobTable).Where("id", tail.Id).Data(map[string]interface{}{
				"status": telegramJobStatusSending, "send_phase": "display_sending",
				"send_deadline_at": gdb.Raw("LOCALTIMESTAMP - interval '1 minute'"),
			}).Update(); err != nil {
				return err
			}
			_, next, err := s.expireTelegramSendTx(ctx, tx, tail.Id)
			if err != nil {
				return err
			}
			if retry == telegramSendMaxRetries {
				if next != 0 {
					t.Fatal("created a fourth retry")
				}
			} else {
				if next <= tail.Id {
					t.Fatal("missing next retry")
				}
				if err := tx.Model(publishTgJobTable).Where("id", next).Scan(&tail); err != nil {
					return err
				}
			}
		}
		claimed, err := tx.GetAll(telegramTimeoutCleanupClaimSQL, time.Now().Add(time.Minute), time.Now().Add(2*time.Minute))
		if err != nil {
			return err
		}
		if len(claimed) != 4 {
			t.Fatalf("expected four expired generations, got %d", len(claimed))
		}
		againClaimed, err := tx.GetAll(telegramTimeoutCleanupClaimSQL, time.Now(), time.Now().Add(2*time.Minute))
		if err != nil {
			return err
		}
		if len(againClaimed) != 0 {
			t.Fatal("leased cleanup jobs were claimed again")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
