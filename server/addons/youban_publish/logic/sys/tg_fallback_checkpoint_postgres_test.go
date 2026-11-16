package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestTelegramFallbackTransportCheckpointLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_HEAD_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL on port 35432")
	}
	db, err := gdb.New(gdb.ConfigNode{
		Type: "pgsql", Host: "127.0.0.1", Port: "35432",
		User: os.Getenv("USER"), Name: "postgres",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("CREATE TEMP TABLE " + publishTgDeliveryStepTable + ` (
			job_id bigint NOT NULL,
			step_key text NOT NULL,
			status text NOT NULL,
			attempt_id bigint NOT NULL,
			error_message text NOT NULL,
			created_at timestamp,
			updated_at timestamp,
			UNIQUE(job_id,step_key)
		) ON COMMIT DROP`); err != nil {
			return err
		}
		// Reproduce both the first INSERT and the real conflict-update path.
		// Colons and quotes in task keys must remain bound data.
		for _, purpose := range []string{"display", "verify"} {
			key := "message-media-fallback:42:" + purpose + ":operation-digest:1"
			model := func() *gdb.Model { return tx.Model(publishTgDeliveryStepTable).Ctx(ctx) }
			if err := saveTelegramFallbackTransportCheckpoint(model(), 42, purpose, 101, key); err != nil {
				return err
			}
			created, err := tx.GetValue("SELECT created_at FROM "+publishTgDeliveryStepTable+" WHERE job_id=42 AND step_key=?", telegramFallbackTransportStep(purpose))
			if err != nil {
				return err
			}
			if created.IsNil() {
				t.Fatal("ORM must maintain checkpoint creation time")
			}
			newKey := "message-media-fallback:42:" + purpose + ":digest'quoted:2"
			if err := saveTelegramFallbackTransportCheckpoint(model(), 42, purpose, 202, newKey); err != nil {
				return err
			}
			row, err := tx.GetOne("SELECT attempt_id,status,error_message,created_at,updated_at FROM "+publishTgDeliveryStepTable+" WHERE job_id=42 AND step_key=?", telegramFallbackTransportStep(purpose))
			if err != nil {
				return err
			}
			if row["attempt_id"].Int64() != 202 || row["status"].String() != telegramDeliveryStepRunning ||
				row["error_message"].String() != newKey || row["created_at"].String() != created.String() ||
				row["updated_at"].IsNil() {
				t.Fatalf("checkpoint conflict update lost bound data or timestamps: %v", row)
			}
			// Duplicate delivery of the same checkpoint remains idempotent.
			if err := saveTelegramFallbackTransportCheckpoint(model(), 42, purpose, 202, newKey); err != nil {
				return err
			}
		}
		count, err := tx.GetValue("SELECT COUNT(*) FROM " + publishTgDeliveryStepTable)
		if err == nil && count.Int() != 2 {
			t.Fatalf("want one checkpoint per purpose, got %d", count.Int())
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
