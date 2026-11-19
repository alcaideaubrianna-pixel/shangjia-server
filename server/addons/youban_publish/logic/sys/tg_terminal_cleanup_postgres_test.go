package sys

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
)

// Terminal failures share the deletion evidence gates, but must never resume a tail.
func TestTelegramTerminalCleanupEvidencePostgres(t *testing.T) {
	for _, status := range []string{"failed"} {
		for _, tc := range []struct {
			name, evidence               string
			reject, nilToken, staleToken bool
		}{
			{name: "clean terminal releases"},
			{name: "sent message blocks", evidence: "INSERT INTO " + publishTgMessageTable + " VALUES (100,'sent',42)", reject: true},
			{name: "undeletable message blocks", evidence: "INSERT INTO " + publishTgMessageTable + " VALUES (100,'undeletable',42)", reject: true},
			{name: "missing projection blocks", evidence: "INSERT INTO " + publishTgAttemptTable + " VALUES (1,100,'superseded',1,1,''); INSERT INTO " + publishTgAttemptMessageTable + " VALUES (1,42)", reject: true},
			{name: "unresolved attempt blocks", evidence: "INSERT INTO " + publishTgAttemptTable + " VALUES (1,100,'superseded',0,0,'')", reject: true},
			{name: "deleted projection releases", evidence: "INSERT INTO " + publishTgAttemptTable + " VALUES (1,100,'superseded',1,1,''); INSERT INTO " + publishTgAttemptMessageTable + " VALUES (1,42); INSERT INTO " + publishTgMessageTable + " VALUES (100,'deleted',42)"},
			{name: "nil claim rejected", nilToken: true, reject: true},
			{name: "stale claim does not release", staleToken: true},
		} {
			t.Run(status+"/"+tc.name, func(t *testing.T) {
				db := telegramTimeoutFinishLocalDB(t)
				ctx := context.Background()
				telegramTimeoutFinishSeed(t, db)
				if _, err := db.Exec(ctx, "UPDATE "+publishTgJobTable+" SET status=?,send_phase='timeout_cancelled' WHERE id=100", status); err != nil {
					t.Fatal(err)
				}
				if tc.nilToken {
					if _, err := db.Exec(ctx, "UPDATE "+publishTgJobTable+" SET timeout_cleanup_at=NULL WHERE id=100"); err != nil {
						t.Fatal(err)
					}
				}
				var snapshot telegramJobRecord
				if err := db.Model(publishTgJobTable).Where("id", 100).Scan(&snapshot); err != nil {
					t.Fatal(err)
				}
				if tc.staleToken {
					if _, err := db.Exec(ctx, "UPDATE "+publishTgJobTable+" SET timeout_cleanup_at='2026-11-18 02:00:00' WHERE id=100"); err != nil {
						t.Fatal(err)
					}
				}
				if tc.evidence != "" {
					if _, err := db.Exec(ctx, tc.evidence); err != nil {
						t.Fatal(err)
					}
				}
				before, err := db.GetOne(ctx, "SELECT status,send_phase,timeout_cleanup_at FROM "+publishTgJobTable+" WHERE id=100")
				if err != nil {
					t.Fatal(err)
				}
				err = new(sSysPublish).finishTelegramSendTimeoutCleanup(ctx, snapshot)
				if (err != nil) != tc.reject {
					t.Fatalf("error=%v reject=%v", err, tc.reject)
				}
				after, err := db.GetOne(ctx, "SELECT status,send_phase,timeout_cleanup_at FROM "+publishTgJobTable+" WHERE id=100")
				if err != nil {
					t.Fatal(err)
				}
				if after["status"].String() != status {
					t.Fatalf("terminal status changed: %v", after)
				}
				if tc.reject || tc.staleToken {
					if after["send_phase"].String() != before["send_phase"].String() || after["timeout_cleanup_at"].String() != before["timeout_cleanup_at"].String() {
						t.Fatalf("unsafe release: before=%v after=%v", before, after)
					}
				} else if !after["timeout_cleanup_at"].IsNil() || after["send_phase"].String() != telegramSendPhaseTimeoutCancelled {
					t.Fatalf("cleanup did not release: %v", after)
				}
				tail, err := db.GetOne(ctx, "SELECT status,send_phase FROM "+publishTgJobTable+" WHERE id=101")
				if err != nil {
					t.Fatal(err)
				}
				if tail["status"].String() != "failed" || tail["send_phase"].String() != "timeout_wait_cleanup" {
					t.Fatalf("terminal cleanup activated or changed timeout tail: %v", tail)
				}
				outbox, err := db.GetValue(ctx, "SELECT status FROM "+publishTgOutboxTable+" WHERE id=101")
				if err != nil || outbox.String() != "pending" {
					t.Fatalf("terminal cleanup changed tail outbox: %v %v", outbox, err)
				}
			})
		}
	}
}

// Exercise the actual SQL CASE expressions in a single UPDATE, so status and
// cleanup claim cannot expose an intermediate unblocked terminal row.
func TestTelegramTerminalFailureAtomicFieldsPostgres(t *testing.T) {
	for _, phase := range []string{"", "pending", "cleanup_processing", "display_sending", "display_confirmed", "verify_sending"} {
		t.Run("phase="+phase, func(t *testing.T) {
			db := telegramTimeoutFinishLocalDB(t)
			ctx := context.Background()
			telegramTimeoutFinishSeed(t, db)
			if _, err := db.Exec(ctx, "UPDATE "+publishTgJobTable+" SET status='sending',send_phase=?,timeout_cleanup_at=NULL WHERE id=100", phase); err != nil {
				t.Fatal(err)
			}
			if err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				changed, err := new(sSysPublish).transitionTelegramJobTx(ctx, tx, telegramJobTransition{
					JobId: 100, From: []JobStatus{JobStatusSending}, To: JobStatusFailed, CleanupStartedSend: true,
				})
				if err == nil && !changed {
					t.Fatal("actual failure transition did not update the sending row")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			row, err := db.GetOne(ctx, "SELECT status,send_phase,timeout_cleanup_at FROM "+publishTgJobTable+" WHERE id=100")
			if err != nil {
				t.Fatal(err)
			}
			started := phase == "display_sending" || phase == "display_confirmed" || phase == "verify_sending"
			if row["status"].String() != "failed" {
				t.Fatalf("failure not atomic: %v", row)
			}
			if started {
				if row["send_phase"].String() != telegramSendPhaseTimeoutCancelled || row["timeout_cleanup_at"].IsNil() {
					t.Fatalf("started failure lost cleanup claim: %v", row)
				}
			} else {
				wantPhase := phase
				if phase == "cleanup_processing" {
					wantPhase = ""
				}
				if row["send_phase"].String() != wantPhase || !row["timeout_cleanup_at"].IsNil() {
					t.Fatalf("unstarted failure retained active cleanup or created a claim: %v", row)
				}
			}
		})
	}
}

func TestTelegramTerminalCleanupFlagRejectsNonFailedPostgres(t *testing.T) {
	for _, target := range []JobStatus{JobStatusSent, JobStatusSuperseded, telegramJobStatusRetryWait} {
		t.Run(target.String(), func(t *testing.T) {
			db := telegramTimeoutFinishLocalDB(t)
			ctx := context.Background()
			telegramTimeoutFinishSeed(t, db)
			if _, err := db.Exec(ctx, "UPDATE "+publishTgJobTable+" SET status='sending',send_phase='display_confirmed',timeout_cleanup_at=NULL WHERE id=100"); err != nil {
				t.Fatal(err)
			}
			err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				changed, err := new(sSysPublish).transitionTelegramJobTx(ctx, tx, telegramJobTransition{
					JobId: 100, From: []JobStatus{JobStatusSending}, To: target, CleanupStartedSend: true,
				})
				if changed {
					t.Error("invalid cleanup flag changed the row")
				}
				return err
			})
			if err == nil {
				t.Fatal("non-failed transition accepted cleanup flag")
			}
			row, err := db.GetOne(ctx, "SELECT status,send_phase,timeout_cleanup_at FROM "+publishTgJobTable+" WHERE id=100")
			if err != nil {
				t.Fatal(err)
			}
			if row["status"].String() != "sending" || row["send_phase"].String() != "display_confirmed" || !row["timeout_cleanup_at"].IsNil() {
				t.Fatalf("rejected transition mutated row: %v", row)
			}
		})
	}
}
