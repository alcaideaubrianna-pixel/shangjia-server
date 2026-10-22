package sys

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
)

// A transaction owns the only connection. Message persistence must not borrow
// another connection for ORM schema discovery while holding the channel lease.
func TestTelegramMessageCommitSingleConnectionLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COMMIT_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres", MaxOpenConnCount: 1, MaxIdleConnCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		_, err := tx.Exec("CREATE TEMP TABLE " + publishTgMessageTable + ` (
		job_id bigint, tenant_id bigint, account_id bigint, profile_id bigint, bot_id bigint,
		target_chat_id text, tg_message_id bigint, media_group_id text, media_id bigint, purpose text,
		tg_file_id text, tg_file_unique_id text, status text, sent_at timestamp, deleted_at timestamp,
		UNIQUE(job_id,tg_message_id)) ON COMMIT DROP`)
		if err != nil {
			return err
		}
		job := telegramJobRecord{Id: 2236331, TenantId: 74, AccountId: 609, ProfileId: 672924, BotId: 59, TargetChatId: "-1002654549416"}
		messages := []*telegramSentMessage{{MessageId: 16495, MediaGroupId: "album", MediaId: 5120694, Purpose: "display", TgFileId: "file", TgFileUniqueId: "unique"}}
		if err := saveTelegramSentMessagesTx(ctx, tx, job, messages); err != nil {
			return err
		}
		if err := saveTelegramSentMessagesTx(ctx, tx, job, messages); err != nil {
			return err
		}
		row, err := tx.GetOne("SELECT count(*) AS n,min(media_id) AS media_id,min(tg_file_unique_id) AS file_id FROM " + publishTgMessageTable)
		if err != nil {
			return err
		}
		if row["n"].Int() != 1 || row["media_id"].Int64() != 5120694 || row["file_id"].String() != "unique" {
			t.Fatalf("projection lost identity: %v", row)
		}
		if err := saveTelegramSentMessagesTx(ctx, tx, job, []*telegramSentMessage{{MessageId: 16512, MediaGroupId: "album", Purpose: "verify"}}); err == nil {
			t.Fatal("mixed display/verification album accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Statement triggers make the round-trip regression observable independently of
// local database speed. Nine media must use one INSERT per table, even on replay.
func TestTelegramAlbumBatchCommitLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COMMIT_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres", MaxOpenConnCount: 1, MaxIdleConnCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		statements := []string{
			"CREATE TEMP TABLE " + publishTgMessageTable + ` (
			job_id bigint, tenant_id bigint, account_id bigint, profile_id bigint, bot_id bigint,
			target_chat_id text, tg_message_id bigint, media_group_id text, media_id bigint, purpose text,
			tg_file_id text, tg_file_unique_id text, status text, sent_at timestamp, deleted_at timestamp,
			UNIQUE(job_id,tg_message_id)) ON COMMIT DROP`,
			"CREATE TEMP TABLE " + publishTgAttemptMessageTable + ` (
			attempt_id bigint, job_id bigint, target_chat_id text, message_id bigint,
			message_payload text, source text, created_at timestamp,
			UNIQUE(attempt_id,message_id)) ON COMMIT DROP`,
			"CREATE TEMP TABLE batch_insert_counts (table_name text) ON COMMIT DROP",
			`CREATE OR REPLACE FUNCTION pg_temp.count_batch_insert() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN INSERT INTO batch_insert_counts VALUES (TG_TABLE_NAME); RETURN NULL; END $$`,
			"CREATE TRIGGER count_insert AFTER INSERT ON " + publishTgMessageTable + " FOR EACH STATEMENT EXECUTE FUNCTION pg_temp.count_batch_insert()",
			"CREATE TRIGGER count_insert AFTER INSERT ON " + publishTgAttemptMessageTable + " FOR EACH STATEMENT EXECUTE FUNCTION pg_temp.count_batch_insert()",
		}
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		job := telegramJobRecord{Id: 1, TenantId: 80, TargetChatId: "-1003767136074"}
		attempt := telegramDeliveryAttempt{Id: 2, JobId: 1, TargetChatId: job.TargetChatId}
		messages := []*telegramSentMessage{nil, {MessageId: 0}}
		for i := int64(1); i <= 9; i++ {
			messages = append(messages, &telegramSentMessage{MessageId: i, MediaId: i, MediaGroupId: "album", Purpose: "display", TgFileId: "original"})
		}
		// Duplicate key must preserve original identity but update file metadata.
		messages = append(messages, &telegramSentMessage{MessageId: 1, MediaId: 999, MediaGroupId: "album", Purpose: "display", TgFileId: "latest"})
		for i := 0; i < 2; i++ {
			if err := recordTelegramAttemptMessagesTx(ctx, tx, attempt, messages, "response"); err != nil {
				return err
			}
			if err := saveTelegramSentMessagesTx(ctx, tx, job, messages); err != nil {
				return err
			}
		}
		row, err := tx.GetOne("SELECT count(*) n FROM batch_insert_counts")
		if err != nil {
			return err
		}
		if row["n"].Int() != 4 {
			t.Fatalf("expected four batch INSERTs, got %v", row)
		}
		row, err = tx.GetOne("SELECT count(*) n FROM " + publishTgMessageTable)
		if err != nil {
			return err
		}
		if row["n"].Int() != 9 {
			t.Fatalf("unexpected message count: %v", row)
		}
		row, err = tx.GetOne("SELECT media_id,tg_file_id FROM " + publishTgMessageTable + " WHERE tg_message_id=1")
		if err != nil {
			return err
		}
		if row["media_id"].Int64() != 1 || row["tg_file_id"].String() != "latest" {
			t.Fatalf("identity/metadata changed: %v", row)
		}
		if err := recordTelegramAttemptMessagesTx(ctx, tx, attempt, []*telegramSentMessage{{MessageId: 1, Purpose: "verify"}}, "partial_response"); err != nil {
			return err
		}
		row, err = tx.GetOne("SELECT source,message_payload FROM " + publishTgAttemptMessageTable + " WHERE message_id=1")
		if err != nil {
			return err
		}
		if row["source"].String() != "response" || !strings.Contains(row["message_payload"].String(), "display") {
			t.Fatalf("partial response overwrote authoritative evidence: %v", row)
		}
		if err := saveTelegramSentMessagesTx(ctx, tx, job, []*telegramSentMessage{{MessageId: 10, MediaGroupId: "new", Purpose: "display"}, {MessageId: 11, MediaGroupId: "new", Purpose: "verify"}}); err == nil {
			t.Fatal("mixed incoming album accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTelegramConfirmationLockBudgetLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COMMIT_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres", MaxOpenConnCount: 2, MaxIdleConnCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	holder, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()
	if _, err := holder.Exec("SELECT pg_advisory_xact_lock(2236331)"); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := boundTelegramConfirmationTransaction(ctx, tx); err != nil {
			return err
		}
		_, err := tx.Exec("SELECT pg_advisory_xact_lock(2236331)")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "lock timeout") {
		t.Fatalf("expected bounded lock failure, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("lock wait exceeded budget: %s", elapsed)
	}
	if err := holder.Rollback(); err != nil {
		t.Fatal(err)
	}
	// A pooled connection must remain usable after the failed confirmation.
	if err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := boundTelegramConfirmationTransaction(ctx, tx); err != nil {
			return err
		}
		_, err := tx.Exec("SELECT pg_advisory_xact_lock(2236331)")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Do not weaken an operator's stricter timeout. Exercise server-side
	// statement cancellation independently of the driver's context deadline.
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("SET LOCAL statement_timeout = '100ms'"); err != nil {
			return err
		}
		if err := boundTelegramConfirmationTransaction(ctx, tx); err != nil {
			return err
		}
		_, err := tx.Exec("SELECT pg_sleep(0.3)")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "statement timeout") {
		t.Fatalf("stricter statement timeout was lost: %v", err)
	}
}
