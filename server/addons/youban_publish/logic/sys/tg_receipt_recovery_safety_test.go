package sys

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/internal/library/cache"
)

func TestTelegramReceiptRecoverySafetyLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_HEAD_PG") != "1" {
		t.Skip("requires isolated loopback PostgreSQL")
	}
	_ = g.DB()
	before := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", before)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if !cache.Initialized() {
		cache.SetAdapter(ctx)
	}
	install, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("fixture rollback")
	for _, scenario := range []string{"display", "partial", "malformed", "verify", "stale", "terminal", "deleted"} {
		t.Run(scenario, func(t *testing.T) {
			err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				for _, table := range []string{publishTgJobTable, publishTgMessageTable, publishProfileStateTable, publishTgOutboxTable} {
					start := strings.Index(string(install), `CREATE TABLE IF NOT EXISTS "`+table+`"`)
					ddl := string(install)[start:]
					ddl = strings.Replace(ddl[:strings.Index(ddl, ");")+2], "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1)
					if _, err := tx.Exec(ddl); err != nil {
						return err
					}
				}
				for _, ddl := range []string{
					"ALTER TABLE " + publishTgJobTable + " ADD COLUMN send_deadline_at timestamp, ADD COLUMN timeout_parent_job_id bigint, ADD COLUMN timeout_cleanup_at timestamp, ADD COLUMN timeout_retry_count int DEFAULT 0",
					"CREATE UNIQUE INDEX receipt_test_message ON " + publishTgMessageTable + "(job_id,tg_message_id)",
					"CREATE TEMP TABLE " + publishTgAttemptTable + " (id bigint, job_id bigint, phase text, status text, expected_count int, confirmed_count int DEFAULT 0, response_received int DEFAULT 0, webhook_deadline timestamp, error_message text DEFAULT '', updated_at timestamp)",
					"CREATE TEMP TABLE " + publishTgAttemptMessageTable + " (id bigserial, attempt_id bigint, message_id bigint, source text, message_payload text)",
					"INSERT INTO " + publishTgJobTable + "(id,channel_id,status,send_phase,updated_at,send_deadline_at) VALUES(3291193,231,'sending','verify_sending',LOCALTIMESTAMP-interval '1 hour',LOCALTIMESTAMP-interval '1 minute')",
					"INSERT INTO " + publishTgAttemptTable + "(id,job_id,phase,status,expected_count) VALUES(3317915,3291193,'display','sending',5),(3317916,3291193,'verify','sending',1)",
				} {
					if _, err := tx.Exec(ddl); err != nil {
						return err
					}
				}
				if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
					return err
				}
				count := 5
				if scenario == "partial" {
					count = 4
				}
				for i := 0; i < count; i++ {
					payload := `{}`
					if scenario == "malformed" {
						payload = "{"
					}
					if _, err := tx.Exec("INSERT INTO "+publishTgAttemptMessageTable+"(attempt_id,message_id,source,message_payload) VALUES(3317915,?,'response',?)", 1158187+i, payload); err != nil {
						return err
					}
				}
				if scenario == "verify" {
					if _, err := tx.Exec("INSERT INTO " + publishTgAttemptMessageTable + "(attempt_id,message_id,source,message_payload) VALUES(3317916,1158192,'response','{}')"); err != nil {
						return err
					}
				}
				s := NewSysPublish()
				var job telegramJobRecord
				if err := tx.Model(publishTgJobTable).Where("id", 3291193).Scan(&job); err != nil {
					return err
				}
				if scenario == "stale" || scenario == "terminal" || scenario == "deleted" {
					switch scenario {
					case "stale":
						_, err = tx.Exec("UPDATE " + publishTgJobTable + " SET send_phase='verify_confirmed' WHERE id=3291193")
					case "terminal":
						_, err = tx.Exec("UPDATE " + publishTgJobTable + " SET status='superseded',send_phase='timeout_cancelled' WHERE id=3291193")
					case "deleted":
						_, err = tx.Exec("INSERT INTO " + publishTgMessageTable + "(job_id,tg_message_id,status,deleted_at) VALUES(3291193,1158187,'deleted',LOCALTIMESTAMP)")
					}
					if err != nil {
						return err
					}
					restored, err := s.restoreConfirmedTelegramResponse(ctx, job, "display", nil)
					if err != nil {
						return err
					}
					if restored {
						t.Fatal("stale/terminal/deleted snapshot restored")
					}
					attempt, err := tx.Model(publishTgAttemptTable).Where("id", 3317915).One()
					if err != nil {
						return err
					}
					if attempt["status"].String() != telegramAttemptStatusSending || attempt["response_received"].Int() != 0 || attempt["confirmed_count"].Int() != 0 {
						t.Fatal("rejected restore confirmed attempt")
					}
					rows, err := tx.Model(publishTgMessageTable).Unscoped().Where("job_id", job.Id).All()
					if err != nil {
						return err
					}
					if scenario == "deleted" {
						if len(rows) != 1 || rows[0]["status"].String() != "deleted" || rows[0]["deleted_at"].IsNil() {
							t.Fatal("deleted projection resurrected")
						}
					} else if len(rows) != 0 {
						t.Fatal("rejected restore inserted projections")
					}
					return rollback
				}
				restored, restoreErr := s.restoreTelegramResponseBeforeRecovery(ctx, job)
				if scenario == "partial" || scenario == "malformed" {
					if restoreErr == nil {
						t.Fatal("invalid audit unexpectedly restored")
					}
				} else {
					if restoreErr != nil {
						return restoreErr
					}
					if restored != (scenario == "verify") {
						t.Fatalf("restored=%v", restored)
					}
					n, err := tx.Model(publishTgMessageTable).Where("job_id", job.Id).Count()
					if err != nil {
						return err
					}
					expected := 5
					if scenario == "verify" {
						expected = 6
					}
					if n != expected {
						t.Fatalf("projection count=%d want=%d", n, expected)
					}
					first, err := tx.Model(publishTgJobTable).Where("id", job.Id).One()
					if err != nil {
						return err
					}
					if _, err := s.restoreTelegramResponseBeforeRecovery(ctx, job); err != nil {
						return err
					}
					again, err := tx.Model(publishTgJobTable).Where("id", job.Id).One()
					if err != nil {
						return err
					}
					if first["updated_at"].String() != again["updated_at"].String() {
						t.Fatal("repeated restore refreshed stale clock")
					}
					if scenario == "verify" && again["send_phase"].String() != telegramSendPhaseVerifyConfirmed {
						t.Fatal("verify did not confirm locally")
					}
					if scenario == "display" && again["send_phase"].String() != telegramSendPhaseVerifySending {
						t.Fatal("display restoration regressed verify")
					}
				}
				_, _, err := s.expireTelegramSendTx(ctx, tx, job.Id)
				if err != nil {
					return err
				}
				current, err := tx.Model(publishTgJobTable).Where("id", job.Id).One()
				if err != nil {
					return err
				}
				if scenario == "verify" {
					if current["status"].String() != telegramJobStatusSending {
						t.Fatal("fully confirmed verify was fenced")
					}
				} else {
					if current["status"].String() != telegramJobStatusSuperseded {
						t.Fatal("unknown verification retained channel barrier")
					}
					unresolved, err := telegramTimeoutUnresolvedAttemptCount(tx, job.Id)
					if err != nil {
						return err
					}
					if unresolved == 0 {
						t.Fatal("unknown verification allowed blind replacement")
					}
					missing, err := telegramTimeoutUnprojectedAuditCount(tx, job.Id)
					if err != nil {
						return err
					}
					if (missing > 0) != (scenario != "display") {
						t.Fatalf("audit projection safety gate=%d", missing)
					}
					if scenario == "display" {
						if _, err := tx.Exec("UPDATE "+publishTgMessageTable+" SET status='deleted',deleted_at=LOCALTIMESTAMP WHERE job_id=?", job.Id); err != nil {
							return err
						}
						missing, err = telegramTimeoutUnprojectedAuditCount(tx, job.Id)
						if err != nil {
							return err
						}
						if missing != 0 {
							t.Fatal("deletion receipt lost through soft delete filtering")
						}
					}
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatal(err)
			}
		})
	}
}
