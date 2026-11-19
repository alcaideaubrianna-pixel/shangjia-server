package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

func TestTelegramBotMembershipExactError(t *testing.T) {
	for _, message := range []string{
		telegramBotNotMemberMessage,
		"Forbidden: bot is not a member of the channel chat",
		"FORBIDDEN: BOT IS NOT A MEMBER OF THE CHANNEL CHAT",
	} {
		err := gerror.Wrap(errors.New(message), "删除频道历史消息失败")
		if !isTelegramBotNotMemberError(err) || !isTelegramPermanentSendError(err) {
			t.Fatalf("explicit membership failure not terminal: %v", err)
		}
	}
	for _, message := range []string{
		"bot is not a member", "bot was kicked", "chat not found",
		"Forbidden: not enough rights to delete messages", "429 Too Many Requests",
		"context deadline exceeded", "CHAT_WRITE_FORBIDDEN", "unauthorized",
	} {
		if isTelegramBotNotMemberError(errors.New(message)) {
			t.Fatalf("unrelated error would disable cycle: %q", message)
		}
	}
	if isTelegramBotNotMemberError(nil) {
		t.Fatal("nil error accepted")
	}
	cause := gerror.Wrap(&telegramBotMembershipError{botID: 64, cause: errors.New(telegramBotNotMemberMessage)}, "历史清理")
	actor := telegramMembershipFailureActor(telegramJobRecord{Id: 100, BotId: 99}, cause)
	if actor.BotId != 64 || actor.Id != 100 {
		t.Fatalf("historical deletion attributed to new Bot: %+v", actor)
	}
}

func telegramMembershipLocalDB(t *testing.T) gdb.DB {
	t.Helper()
	db := telegramTimeoutFinishLocalDB(t)
	ctx := context.Background()
	for _, sql := range []string{
		`ALTER TABLE ` + publishTgJobTable + ` ADD COLUMN last_dispatch_error text DEFAULT '',
			ADD COLUMN created_at timestamp DEFAULT LOCALTIMESTAMP,
			ADD COLUMN task_id bigint, ADD COLUMN asynq_task_id text DEFAULT ''`,
		`ALTER TABLE ` + publishTgOutboxTable + ` ADD COLUMN lease_owner text DEFAULT '',
			ADD COLUMN delivered_at timestamp`,
		`ALTER TABLE ` + publishChannelTable + ` ADD COLUMN created_by bigint DEFAULT 1,
			ADD COLUMN cycle_publish_enabled int DEFAULT 1,
			ADD COLUMN cycle_next_run_at timestamp DEFAULT LOCALTIMESTAMP,
			ADD COLUMN cycle_active_run_id bigint DEFAULT 99,
			ADD COLUMN cycle_config_version bigint DEFAULT 1,
			ADD COLUMN cycle_last_error_message text DEFAULT '',
			ADD COLUMN bot_permission_status_json text DEFAULT ''`,
		`CREATE TABLE ` + channelPermissionCircuitTable + ` (
			tenant_id bigint,channel_id bigint,paused int DEFAULT 0,generation text DEFAULT '',
			account_id bigint,last_job_id bigint,notified int DEFAULT 0,resumed_at timestamp,
			PRIMARY KEY(tenant_id,channel_id))`,
		`INSERT INTO ` + publishChannelTable + ` (id,tenant_id,status) VALUES(1,1,1),(2,2,1)`,
		`INSERT INTO ` + publishTgJobTable + ` (id,tenant_id,account_id,profile_id,channel_id,operation_no,status,send_phase)
			VALUES (100,1,1,100,1,'profile:100','sending','cleanup_processing'),
			(101,1,1,101,1,'profile_cycle:101','pending',''),
			(102,1,1,102,1,'profile:102','unknown','display_confirmed'),
			(103,2,2,103,2,'profile_cycle:103','pending',''),
			(104,1,1,104,1,'down:104','pending',''),
			(105,1,1,105,1,'profile:105','pending','')`,
		`INSERT INTO ` + publishTgMessageTable + ` VALUES (105,'sent',500)`,
		`INSERT INTO ` + publishTgOutboxTable + ` (id,job_id,event_type,status,available_at)
			VALUES (101,101,'telegram_job.publish','pending',LOCALTIMESTAMP),
			(103,103,'telegram_job.publish','pending',LOCALTIMESTAMP)`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.GetCore().ClearTableFieldsAll(ctx) })
	return db
}

func TestTelegramBotMembershipStopAtomicPostgres(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail bool
	}{
		{"commit", false}, {"rollback permission write", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := telegramMembershipLocalDB(t)
			ctx := context.Background()
			job := telegramJobRecord{Id: 100, TenantId: 1, AccountId: 1, ChannelId: 1, BotId: 99}
			cause := gerror.Wrap(&telegramBotMembershipError{botID: 64, cause: errors.New(telegramBotNotMemberMessage)}, "历史清理")
			if tc.fail {
				if _, err := db.Exec(ctx, "ALTER TABLE "+publishChannelTable+" ADD CHECK (cycle_publish_enabled=1)"); err != nil {
					t.Fatal(err)
				}
			}
			err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				if _, err := lockChannelPermissionCreationTx(ctx, tx, 1, 1); err != nil {
					return err
				}
				changed, err := new(sSysPublish).transitionTelegramJobTx(ctx, tx, telegramJobTransition{
					JobId: 100, From: []JobStatus{JobStatusSending}, To: JobStatusFailed, CleanupStartedSend: true,
				})
				if err != nil {
					return err
				}
				if !changed {
					return errors.New("CAS did not claim sending job")
				}
				_, err = new(sSysPublish).stopTelegramBotNotMemberChannelTx(ctx, tx, job, cause)
				return err
			})
			if (err != nil) != tc.fail {
				t.Fatalf("transaction error=%v expected rollback=%v", err, tc.fail)
			}
			current, err := db.GetOne(ctx, "SELECT status,send_phase,timeout_cleanup_at FROM "+publishTgJobTable+" WHERE id=100")
			if err != nil {
				t.Fatal(err)
			}
			channel, err := db.GetOne(ctx, "SELECT * FROM "+publishChannelTable+" WHERE id=1")
			if err != nil {
				t.Fatal(err)
			}
			if tc.fail {
				if current["status"].String() != "sending" || current["send_phase"].String() != "cleanup_processing" || channel["cycle_publish_enabled"].Int() != 1 {
					t.Fatalf("partial failure commit: job=%v channel=%v", current, channel)
				}
				return
			}
			if current["status"].String() != "failed" || current["send_phase"].String() != "" || !current["timeout_cleanup_at"].IsNil() {
				t.Fatalf("pre-send failure retains channel: %v", current)
			}
			if channel["cycle_publish_enabled"].Int() != 0 || !channel["cycle_next_run_at"].IsNil() || channel["cycle_active_run_id"].Int64() != 0 || channel["cycle_config_version"].Int64() != 2 {
				t.Fatalf("cycle not invalidated: %v", channel)
			}
			permission := channelBotPermissionStateForBot(channel["bot_permission_status_json"].String(), 64)
			if permission == nil || permission.InChannel != 0 || permission.CanSendMessages != 0 || permission.CanDeleteMessages != 0 || permission.Status != "error" {
				t.Fatalf("actual Bot permission not projected: %v", permission)
			}
			if channelBotPermissionStateForBot(channel["bot_permission_status_json"].String(), 99) != nil {
				t.Fatal("marked new job Bot instead of actual failing Bot")
			}
			for id, want := range map[int]string{101: "superseded", 102: "unknown", 103: "pending", 104: "pending", 105: "pending"} {
				status, err := db.GetValue(ctx, "SELECT status FROM "+publishTgJobTable+" WHERE id=?", id)
				if err != nil || status.String() != want {
					t.Fatalf("wrong cancellation id=%d got=%v want=%s err=%v", id, status, want, err)
				}
			}
			outbox, err := db.GetValue(ctx, "SELECT status FROM "+publishTgOutboxTable+" WHERE id=101")
			if err != nil || outbox.String() != "delivered" {
				t.Fatalf("cancelled outbox remains pending: %v %v", outbox, err)
			}
			other, err := db.GetValue(ctx, "SELECT cycle_publish_enabled FROM "+publishChannelTable+" WHERE id=2")
			if err != nil || other.Int() != 1 {
				t.Fatalf("other channel affected: %v %v", other, err)
			}
		})
	}
}

func TestTelegramBotMembershipRejectOtherErrorsPostgres(t *testing.T) {
	db := telegramMembershipLocalDB(t)
	ctx := context.Background()
	for _, message := range []string{"chat not found", "bot was kicked", "Forbidden: not enough rights", "429 Too Many Requests"} {
		err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			_, err := new(sSysPublish).stopTelegramBotNotMemberChannelTx(ctx, tx,
				telegramJobRecord{TenantId: 1, ChannelId: 1}, errors.New(message))
			return err
		})
		if err == nil {
			t.Fatalf("accepted unrelated error: %s", message)
		}
	}
	value, err := db.GetValue(ctx, "SELECT cycle_publish_enabled FROM "+publishChannelTable+" WHERE id=1")
	if err != nil || value.Int() != 1 {
		t.Fatalf("unrelated error disabled cycle: %v %v", value, err)
	}
}

func TestTelegramTerminalCleanupOrderAgreementPostgres(t *testing.T) {
	db := telegramMembershipLocalDB(t)
	ctx := context.Background()
	for _, status := range []JobStatus{JobStatusPending, JobStatusSending, JobStatusFailed, JobStatusSent, JobStatusSuperseded} {
		for _, marked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/marked=%v", status, marked), func(t *testing.T) {
				marker := "NULL"
				if marked {
					marker = "LOCALTIMESTAMP"
				}
				if _, err := db.Exec(ctx, "UPDATE "+publishTgJobTable+" SET status=?,send_phase='cleanup_processing',timeout_cleanup_at="+marker+" WHERE id=100", status); err != nil {
					t.Fatal(err)
				}
				var job telegramJobRecord
				if err := db.Model(publishTgJobTable).Where("id", 100).Scan(&job); err != nil {
					t.Fatal(err)
				}
				sqlResult, err := db.GetValue(ctx, "SELECT "+telegramChannelPendingCleanupSQL("j")+" FROM "+publishTgJobTable+" j WHERE id=100")
				if err != nil {
					t.Fatal(err)
				}
				want := marked || !status.IsTerminal()
				if sqlResult.Bool() != want || telegramChannelJobPendingCleanup(job) != want {
					t.Fatalf("SQL/Go fence differs status=%s marked=%v sql=%v go=%v", status, marked, sqlResult, telegramChannelJobPendingCleanup(job))
				}
			})
		}
	}
}

func TestTelegramBotMembershipConcurrentStopPostgres(t *testing.T) {
	db := telegramMembershipLocalDB(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				if _, err := lockChannelPermissionCreationTx(ctx, tx, 1, 1); err != nil {
					return err
				}
				_, err := new(sSysPublish).stopTelegramBotNotMemberChannelTx(ctx, tx,
					telegramJobRecord{Id: 100, TenantId: 1, AccountId: 1, ChannelId: 1, BotId: 64},
					errors.New(telegramBotNotMemberMessage))
				return err
			})
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	value, err := db.GetValue(ctx, "SELECT count(*) FROM "+channelPermissionCircuitTable+" WHERE tenant_id=1 AND channel_id=1 AND paused=1 AND generation<>''")
	if err != nil || value.Int() != 1 {
		t.Fatalf("concurrent stop lost/doubled circuit: %v %v", value, err)
	}
	permission, err := db.GetValue(ctx, "SELECT bot_permission_status_json FROM "+publishChannelTable+" WHERE id=1")
	if err != nil || !strings.Contains(permission.String(), telegramBotNotMemberMessage) {
		t.Fatalf("permission projection missing: %v %v", permission, err)
	}
}

func TestTelegramCanceledOrderFinalizePostgres(t *testing.T) {
	db := telegramMembershipLocalDB(t)
	original, cancel := context.WithCancel(context.Background())
	cancel()
	finalize, stop := telegramFinalizeContext(original, "job.postpone_channel_order")
	defer stop()
	if finalize.Err() != nil {
		t.Fatalf("finalization inherited execution cancellation: %v", finalize.Err())
	}
	if deadline, ok := finalize.Deadline(); !ok || !deadline.After(time.Now()) {
		t.Fatal("finalization must have a future bounded deadline")
	}
	s := new(sSysPublish)
	if err := s.postponeTelegramJobForChannelOrder(finalize, telegramJobRecord{Id: 101}); err != nil {
		t.Fatalf("cancelled execution prevented actual postpone/rearm: %+v", err)
	}
	job, err := db.GetOne(context.Background(), "SELECT status,dispatch_status,next_retry_at,last_dispatch_error FROM "+publishTgJobTable+" WHERE id=101")
	if err != nil || job["status"].String() != "pending" || job["dispatch_status"].String() != "idle" || job["next_retry_at"].IsNil() {
		t.Fatalf("postpone not persisted: %v %v", job, err)
	}
	outbox, err := db.GetOne(context.Background(), "SELECT status,available_at,lease_owner FROM "+publishTgOutboxTable+" WHERE id=101")
	if err != nil || outbox["status"].String() != "pending" || outbox["available_at"].IsNil() || outbox["lease_owner"].String() != "" {
		t.Fatalf("outbox not rearmed: %v %v", outbox, err)
	}
	changed, err := s.transitionTelegramJob(finalize, telegramJobTransition{
		JobId: 101, From: []JobStatus{JobStatusPending}, To: JobStatusSuperseded,
	})
	if err != nil || !changed {
		t.Fatalf("cancelled execution prevented terminal state write: changed=%v err=%+v", changed, err)
	}
	status, err := db.GetValue(context.Background(), "SELECT status FROM "+publishTgJobTable+" WHERE id=101")
	if err != nil || status.String() != "superseded" {
		t.Fatalf("terminal state not persisted: %v %v", status, err)
	}
}

func TestTelegramMembershipFailureCASPostgres(t *testing.T) {
	db := telegramMembershipLocalDB(t)
	ctx := context.Background()
	transition := telegramJobTransition{JobId: 101, From: []JobStatus{JobStatusSending}, To: JobStatusFailed, CleanupStartedSend: true}
	changed, err := new(sSysPublish).failTelegramBotNotMember(ctx,
		telegramJobRecord{Id: 101, TenantId: 1, AccountId: 1, ChannelId: 1, BotId: 64},
		transition, errors.New(telegramBotNotMemberMessage))
	if err != nil || changed {
		t.Fatalf("stale failure should not change a pending job: %v %v", changed, err)
	}
	enabled, err := db.GetValue(ctx, "SELECT cycle_publish_enabled FROM "+publishChannelTable+" WHERE id=1")
	if err != nil || enabled.Int() != 1 {
		t.Fatalf("failed CAS disabled channel: %v %v", enabled, err)
	}
}
