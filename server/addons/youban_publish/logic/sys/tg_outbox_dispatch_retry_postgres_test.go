package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// This fixture is opt-in and only connects to a disposable loopback database.
func telegramOutboxRetryLocalDB(t *testing.T) gdb.DB {
	t.Helper()
	if os.Getenv("YOUBAN_LOCAL_TG_OUTBOX") != "1" {
		t.Skip("requires disposable PostgreSQL on 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}
	root, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	schema := fmt.Sprintf("tg_outbox_retry_%d", time.Now().UnixNano())
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	db := g.DB()
	if _, err = db.Exec(ctx, `CREATE TABLE `+publishTgOutboxTable+` (
		id bigint PRIMARY KEY, job_id bigint NOT NULL, event_type text NOT NULL,
		status text NOT NULL, attempt_count integer NOT NULL,
		available_at timestamp NOT NULL, lease_expires_at timestamp, last_error text DEFAULT '',
		payload text DEFAULT 'immutable', updated_at timestamp DEFAULT LOCALTIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTelegramOutboxDispatchRetryLocalPostgres(t *testing.T) {
	db := telegramOutboxRetryLocalDB(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		status     string
		attempts   int
		future     bool
		jobID      int64
		event      string
		wantChange bool
		rearmed    bool
	}{
		{"pending", "pending", 0, false, 20, telegramOutboxEventPublish, true, false},
		{"delivered concurrently", "delivered", 0, false, 20, telegramOutboxEventPublish, false, false},
		{"leased concurrently", "processing", 0, false, 20, telegramOutboxEventPublish, false, false},
		{"retried concurrently", "pending", 1, false, 20, telegramOutboxEventPublish, false, false},
		{"future schedule", "pending", 0, true, 20, telegramOutboxEventPublish, false, false},
		{"different job", "pending", 0, false, 21, telegramOutboxEventPublish, false, false},
		{"different event", "pending", 0, false, 20, "telegram_job.cleanup", false, false},
		{"rearmed concurrently without delay", "pending", 0, false, 20, telegramOutboxEventPublish, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.Exec(ctx, "DELETE FROM "+publishTgOutboxTable); err != nil {
				t.Fatal(err)
			}
			offset := "-1 hour"
			if tc.future {
				offset = "1 hour"
			}
			if _, err := db.Exec(ctx, `INSERT INTO `+publishTgOutboxTable+`
				(id,job_id,event_type,status,attempt_count,available_at)
				VALUES (10,?,?,?,?,LOCALTIMESTAMP + CAST(? AS interval))`,
				tc.jobID, tc.event, tc.status, tc.attempts, offset); err != nil {
				t.Fatal(err)
			}
			input := telegramJobOutboxRecord{Id: 10, JobId: 20, AttemptCount: 0}
			snapshot, err := db.GetOne(ctx, "SELECT available_at FROM "+publishTgOutboxTable+" WHERE id=10")
			if err != nil {
				t.Fatal(err)
			}
			input.AvailableAt = snapshot["available_at"].GTime()
			if tc.rearmed {
				if _, err := db.Exec(ctx, "UPDATE "+publishTgOutboxTable+" SET available_at=LOCALTIMESTAMP WHERE id=10"); err != nil {
					t.Fatal(err)
				}
			}
			s := &sSysPublish{}
			if err := s.deferPendingTelegramOutbox(ctx, input, errors.New("enqueue failed")); err != nil {
				t.Fatal(err)
			}
			row, err := db.GetOne(ctx, `SELECT status,attempt_count,last_error,payload,
				available_at>LOCALTIMESTAMP AS future FROM `+publishTgOutboxTable+` WHERE id=10`)
			if err != nil {
				t.Fatal(err)
			}
			wantAttempts := tc.attempts
			if tc.wantChange {
				wantAttempts++
			}
			if row["attempt_count"].Int() != wantAttempts || row["status"].String() != tc.status ||
				row["payload"].String() != "immutable" {
				t.Fatalf("retry overwrote concurrent state or payload: %v", row)
			}
			if tc.wantChange {
				if !row["future"].Bool() || row["last_error"].String() != "enqueue failed" {
					t.Fatalf("failed intent was not deferred: %v", row)
				}
				// A stale delivery result must not increment the same generation twice.
				if err := s.deferPendingTelegramOutbox(ctx, input, errors.New("late failure")); err != nil {
					t.Fatal(err)
				}
				attempts, err := db.GetValue(ctx, "SELECT attempt_count FROM "+publishTgOutboxTable+" WHERE id=10")
				if err != nil || attempts.Int() != wantAttempts {
					t.Fatalf("stale failure changed retry generation: %v %v", attempts, err)
				}
			} else if row["last_error"].String() != "" {
				t.Fatalf("unrelated or concurrent intent was changed: %v", row)
			}
		})
	}
}
