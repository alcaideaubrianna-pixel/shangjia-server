package sys

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestPartialAlbumReplacementLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_ACCOUNT_PHASE_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	if os.Getenv("YOUBAN_PARTIAL_ALBUM_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPartialAlbumReplacementLocalPostgres$", "-test.count=1")
		cmd.Env = append(os.Environ(), "YOUBAN_PARTIAL_ALBUM_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("partial album recovery: %v\n%s", err, out)
		}
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{accountLifecycleConfig("public")})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_tg_job(id bigint PRIMARY KEY,status text,send_phase text) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_attempt(id bigserial PRIMARY KEY,job_id bigint,tenant_id bigint,bot_id bigint,channel_id bigint,target_chat_id text,phase text,chunk_index int,attempt_no int,attempt_token text,expected_count int,confirmed_count int,status text,webhook_deadline timestamp,error_message text) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_message(job_id bigint,purpose text,status text) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_delivery_step(job_id bigint,step_key text,status text) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_tg_job VALUES(1,'sending','display_sending')`,
			`INSERT INTO hg_youban_publish_tg_attempt(job_id,tenant_id,bot_id,channel_id,target_chat_id,phase,attempt_no,attempt_token,expected_count,confirmed_count,status) VALUES(1,11,14,46,'-1001','display',1,'old',9,8,'retry_wait')`,
			`INSERT INTO hg_youban_publish_tg_message VALUES(1,'display','sent')`,
			`INSERT INTO hg_youban_publish_tg_delivery_step VALUES(1,'partial_album_cleanup:1','running')`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if _, err := replaceCleanedPartialAttemptTx(ctx, tx, 1, 1, 9); err == nil {
			t.Fatal("replacement admitted before deletion confirmation")
		}
		if _, err := tx.Exec(`UPDATE hg_youban_publish_tg_message SET status='deleted'`); err != nil {
			return err
		}
		// Simulate a failed local commit: rollback must preserve the cleanup
		// checkpoint and original Attempt so a restart can resume safely.
		rollback := tx.Transaction(ctx, func(ctx context.Context, nested gdb.TX) error {
			if _, err := replaceCleanedPartialAttemptTx(ctx, nested, 1, 1, 9); err != nil {
				return err
			}
			return context.Canceled
		})
		if rollback == nil {
			t.Fatal("injected transaction failure was ignored")
		}
		next, err := replaceCleanedPartialAttemptTx(ctx, tx, 1, 1, 9)
		if err != nil || next.AttemptNo != 2 || next.AttemptToken == "old" || next.ExpectedCount != 9 {
			t.Fatalf("replacement after cleanup: %+v err=%v", next, err)
		}
		if _, err := replaceCleanedPartialAttemptTx(ctx, tx, 1, 1, 9); err == nil {
			t.Fatal("duplicate recovery created a second replacement")
		}
		if _, err := tx.Exec(`UPDATE hg_youban_publish_tg_attempt SET attempt_no=3 WHERE id=?`, next.Id); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_tg_delivery_step VALUES(1,?,'running')`, telegramPartialCleanupCheckpoint+g.NewVar(next.Id).String()); err != nil {
			return err
		}
		if _, err := replaceCleanedPartialAttemptTx(ctx, tx, 1, next.Id, 9); err == nil {
			t.Fatal("fourth send attempt was admitted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
