package sys

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
)

// Reproduce job2249614 without Telegram: display is confirmed, but no Bot
// verification Attempt exists because the entire album used account fallback.
func TestAccountFallbackStartsVerificationLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_ACCOUNT_PHASE_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	// GoFrame caches default clients. Keep local configuration isolated from
	// other tests even when the whole regression selection runs in one command.
	if os.Getenv("YOUBAN_ACCOUNT_PHASE_TEST_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAccountFallbackStartsVerificationLocalPostgres$", "-test.count=1")
		cmd.Env = append(os.Environ(), "YOUBAN_ACCOUNT_PHASE_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated phase regression: %v\n%s", err, output)
		}
		return
	}
	before := gdb.GetConfig("default")
	redisBefore, _ := gredis.GetConfig()
	t.Cleanup(func() { gdb.SetConfigGroup("default", before); gredis.SetConfig(redisBefore) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	redis := miniredis.RunT(t)
	gredis.SetConfig(&gredis.Config{Address: redis.Addr()})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, ddl := range []string{
			`CREATE TEMP TABLE hg_youban_publish_tg_job(id bigint PRIMARY KEY,status text,send_phase text) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_attempt(id bigserial PRIMARY KEY,job_id bigint,tenant_id bigint,bot_id bigint,channel_id bigint,target_chat_id text,phase text,chunk_index int,attempt_no int,attempt_token text,expected_count int,confirmed_count int,response_received int DEFAULT 0,status text,error_message text,webhook_deadline timestamp,created_at timestamp,updated_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_attempt_message(attempt_id bigint,message_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_delivery_step(job_id bigint,step_key text,status text,attempt_id bigint,error_message text,started_at timestamp,finished_at timestamp,created_at timestamp,updated_at timestamp,UNIQUE(job_id,step_key)) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job_log(job_id bigint,tenant_id bigint,account_id bigint,profile_id bigint,action text,status text,message text,created_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_tg_job VALUES(2249614,'sending','display_confirmed')`,
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		s := new(sSysPublish)
		job := telegramJobRecord{Id: 2249614, TenantId: 19, ChannelId: 75, ProfileId: 468643, TargetChatId: "-1002563223375", SendPhase: "display_sending"}
		attempt, err := s.accountFallbackDeliveryAttempt(ctx, job, "verify", 1)
		if err != nil {
			return err
		}
		phase, err := tx.GetValue("SELECT send_phase FROM hg_youban_publish_tg_job WHERE id=2249614")
		if err != nil || phase.String() != "verify_sending" || attempt.Phase != "verify" {
			t.Fatalf("verification not advanced atomically: phase=%v attempt=%+v err=%v", phase, attempt, err)
		}
		// Retrying an active/unknown Attempt must reuse it, never send twice.
		for _, status := range []string{"sending", "awaiting_webhook", "retry_wait"} {
			if _, err := tx.Exec("UPDATE hg_youban_publish_tg_attempt SET status=? WHERE id=?", status, attempt.Id); err != nil {
				return err
			}
			again, err := s.accountFallbackDeliveryAttempt(ctx, job, "verify", 1)
			if err != nil || again.Id != attempt.Id {
				t.Fatalf("active attempt not reused status=%s id=%d err=%v", status, again.Id, err)
			}
		}
		// Without an active Attempt, an unconfirmed prior phase is rejected.
		if _, err := tx.Exec("UPDATE hg_youban_publish_tg_attempt SET status='superseded'; UPDATE hg_youban_publish_tg_job SET send_phase='cleanup_confirmed'"); err != nil {
			return err
		}
		if _, err := s.accountFallbackDeliveryAttempt(ctx, job, "verify", 1); err == nil {
			t.Fatal("verification started before display confirmation")
		}
		for _, phase := range []string{"verify_confirmed", "timeout_cancelled", "verify_sending"} {
			if _, err := tx.Exec("UPDATE hg_youban_publish_tg_job SET send_phase=?", phase); err != nil {
				return err
			}
			if _, err := s.accountFallbackDeliveryAttempt(ctx, job, "verify", 1); err == nil {
				t.Fatalf("unsafe new verification accepted at phase=%s", phase)
			}
		}
		count, err := tx.GetValue("SELECT COUNT(*) FROM hg_youban_publish_tg_attempt")
		if err != nil || count.Int() != 1 {
			t.Fatalf("retries/rejected transitions created extra attempts count=%v err=%v", count, err)
		}
		// Keep fixtures private even when nested operations use transactions.
		return errors.New("rollback local fixtures")
	})
	if err != nil && err.Error() != "rollback local fixtures" {
		t.Fatal(err)
	}
}
