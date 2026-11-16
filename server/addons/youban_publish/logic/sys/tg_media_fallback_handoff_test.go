package sys

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestTelegramFallbackTaskIdentityFencesOperationPurposeAndGeneration(t *testing.T) {
	job := telegramJobRecord{Id: 42, TenantId: 7, OperationNo: "publish:7:123", ReconcileCount: 3}
	task := &collectorin.AccountTask{
		ID: 19, TenantID: 7, TaskType: collectorin.AccountTaskTypeMessageMediaFallback,
		TaskKey: telegramFallbackTaskKey(job, "display"),
	}
	if !telegramFallbackTaskIdentity(job, "display", task) {
		t.Fatal("current task must match its persisted handoff")
	}
	if telegramFallbackTaskIdentity(job, "verify", task) {
		t.Fatal("display task must not claim verification")
	}
	next := job
	next.OperationNo = "publish:7:124"
	if telegramFallbackTaskIdentity(next, "display", task) {
		t.Fatal("old operation must not claim new operation")
	}
	next = job
	next.ReconcileCount++
	if telegramFallbackTaskIdentity(next, "display", task) {
		t.Fatal("old recovery task/callback must not own new generation")
	}
	task.TaskKey = "message-media-fallback:42:display"
	if telegramFallbackTaskIdentity(job, "display", task) {
		t.Fatal("legacy task has no operation identity and must not send")
	}
	task.TaskKey = telegramFallbackTaskKey(job, "display")
	task.TenantID++
	if telegramFallbackTaskIdentity(job, "display", task) {
		t.Fatal("another tenant must not claim the handoff")
	}
}

func TestTelegramFallbackTaskKeyParserAcceptsGenerationIdentity(t *testing.T) {
	job := telegramJobRecord{Id: 42, OperationNo: "publish:7:123", ReconcileCount: 4}
	key := telegramFallbackTaskKey(job, "verify")
	id, purpose, err := parseMessageMediaFallbackTaskKey(key)
	if err != nil || id != 42 || purpose != "verify" {
		t.Fatalf("current task key did not parse: id=%d purpose=%s err=%v", id, purpose, err)
	}
	if strings.Contains(key, job.OperationNo) || len(key) > 160 {
		t.Fatal("task key must use a bounded operation digest")
	}
}

func TestTelegramFallbackRequiresExactPurposeOwnership(t *testing.T) {
	for _, status := range []JobStatus{JobStatusRetryWait, JobStatusSending, JobStatusUnknown} {
		job := telegramJobRecord{Status: status, LastDispatchError: telegramMediaFallbackReason("verify")}
		if !telegramFallbackOwnsPhase(job, "verify") || telegramFallbackOwnsPhase(job, "display") {
			t.Fatalf("ownership must match exact purpose for status %s", status)
		}
		job.LastDispatchError += ":stale"
		if telegramFallbackOwnsPhase(job, "verify") {
			t.Fatal("prefix match must not authorize a different intent")
		}
	}
	for _, status := range []JobStatus{JobStatusPending, JobStatusFailed, JobStatusSent, JobStatusSuperseded} {
		if telegramFallbackOwnsPhase(telegramJobRecord{Status: status, LastDispatchError: telegramMediaFallbackReason("display")}, "display") {
			t.Fatalf("status %s cannot own protocol delivery", status)
		}
	}
}

func TestTelegramFallbackRetiresOnlyProvenRejection(t *testing.T) {
	rejected := telegramDeliveryAttempt{Status: telegramAttemptStatusRetryWait, ErrorMessage: "Bad Request: wrong file identifier"}
	if !telegramFallbackAttemptCanRetire(rejected, 0) {
		t.Fatal("definitely rejected Bot token must be retired before Account creates another")
	}
	cases := []telegramDeliveryAttempt{
		{Status: telegramAttemptStatusSending, ErrorMessage: "Bad Request"},
		{Status: telegramAttemptStatusWaiting, ErrorMessage: "timeout"},
		{Status: telegramAttemptStatusRetryWait},
		{Status: telegramAttemptStatusRetryWait, ErrorMessage: context.DeadlineExceeded.Error()},
		{Status: telegramAttemptStatusRetryWait, ErrorMessage: "Bad Request", ResponseReceived: 1},
		{Status: telegramAttemptStatusRetryWait, ErrorMessage: "Bad Request", ConfirmedCount: 1},
		{Status: telegramAttemptStatusRetryWait, ErrorMessage: "Bad Request", WebhookDeadline: gtime.Now().Add(-time.Hour)},
	}
	for _, attempt := range cases {
		if telegramFallbackAttemptCanRetire(attempt, 0) {
			t.Fatalf("unresolved/confirmed evidence must block resend: %+v", attempt)
		}
	}
	if telegramFallbackAttemptCanRetire(rejected, 1) {
		t.Fatal("even one audited message prevents treating an attempt as unsent")
	}
}

func TestTelegramFallbackBotFailureKeepsPreparationProvenance(t *testing.T) {
	for _, err := range []error{context.DeadlineExceeded, context.Canceled, errTelegramDeliveryUncertain, errTelegramAttemptNeedsReconcile} {
		if telegramFallbackBotFailureKnown(err) {
			t.Fatalf("unknown send cannot authorize protocol delivery: %v", err)
		}
	}
	if !telegramFallbackBotFailureKnown(telegramPreparationError(context.DeadlineExceeded)) {
		t.Fatal("pre-send preparation deadline proves Telegram send was never started")
	}
	if telegramFallbackBotFailureKnown(nil) {
		t.Fatal("missing failure is not a rejection proof")
	}
}

func TestTelegramFallbackFailureRetainsCauseAndRedactsSecrets(t *testing.T) {
	cause := errors.New("下载远程媒体失败：http 404 https://example.invalid/video.mp4?signature=secret")
	message := telegramFallbackFailureMessage(cause)
	if !strings.Contains(message, "下载远程媒体失败：http 404") {
		t.Fatal("terminal failure must retain the original diagnostic cause")
	}
	if strings.Contains(message, "https://") || strings.Contains(message, "signature=secret") {
		t.Fatal("Job error and durable logs must redact signed media addresses")
	}
	if !isTelegramMediaSourceUnavailableError(cause) {
		t.Fatal("original cause must remain available to select media repair")
	}
}

func TestTelegramFallbackTransitionPreservesPrerequisite(t *testing.T) {
	job := telegramJobRecord{Id: 42, ReconcileCount: 7}
	for _, tc := range []struct {
		purpose string
		from    SendPhase
		to      SendPhase
	}{
		{"display", SendPhaseDisplaySending, SendPhaseCleanupConfirmed},
		{"verify", SendPhaseVerifySending, SendPhaseDisplayConfirmed},
	} {
		in := telegramMediaFallbackTransition(job, tc.purpose)
		_, data, err := telegramJobTransitionUpdate(in)
		if err != nil {
			t.Fatal(err)
		}
		if in.SendPhase == nil || *in.SendPhase != tc.to || len(in.SendPhases) != 1 || in.SendPhases[0] != tc.from {
			t.Fatalf("handoff lost phase prerequisite: %+v", in)
		}
		if data["status"] != "failed_retry" || data["dispatch_status"] != "idle" || data["reconcile_count"] != 8 {
			t.Fatalf("handoff must relinquish Bot ownership with a new generation: %+v", data)
		}
	}
}

// Exercises the actual lease and evidence SQL against transaction-local tables.
// Explicit opt-in only; never accepts a configurable production DSN.
func TestTelegramFallbackFencingLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_HEAD_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL on port 35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, ddl := range []string{
			"CREATE TEMP TABLE " + telegramFallbackAccountTaskTable + "(id bigint,tenant_id bigint,account_id bigint,task_type text,task_key text,status text,attempt_count int,lease_owner text,lease_epoch bigint,lease_until timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgAttemptTable + "(id bigint,job_id bigint,phase text,status text,error_message text,response_received int,confirmed_count int,webhook_deadline timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgAttemptMessageTable + "(attempt_id bigint,message_id bigint) ON COMMIT DROP",
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		job := telegramJobRecord{Id: 42, TenantId: 7, OperationNo: "publish:7:123", ReconcileCount: 1}
		task := &collectorin.AccountTask{
			ID: 19, TenantID: 7, AccountID: 8, TaskType: collectorin.AccountTaskTypeMessageMediaFallback,
			TaskKey: telegramFallbackTaskKey(job, "display"), Status: collectorin.AccountTaskStatusProcessing,
			AttemptCount: 2, LeaseOwner: "worker-current", LeaseEpoch: 3,
		}
		if _, err := tx.Exec("INSERT INTO "+telegramFallbackAccountTaskTable+" VALUES(19,7,8,?,?,'processing',2,'worker-current',3,(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai')+INTERVAL '1 minute')", task.TaskType, task.TaskKey); err != nil {
			return err
		}
		if err := lockTelegramFallbackTaskTx(ctx, tx, task, false); err != nil {
			return err
		}
		old := *task
		old.LeaseEpoch--
		if err := lockTelegramFallbackTaskTx(ctx, tx, &old, false); !errors.Is(err, errTelegramJobSuperseded) {
			t.Fatalf("old lease unexpectedly accepted: %v", err)
		}
		if _, err := tx.Exec("UPDATE " + telegramFallbackAccountTaskTable + " SET status='finalizing',lease_owner='',lease_epoch=0"); err != nil {
			return err
		}
		if err := lockTelegramFallbackTaskTx(ctx, tx, task, true); err != nil {
			return err
		}
		old = *task
		old.AttemptCount--
		if err := lockTelegramFallbackTaskTx(ctx, tx, &old, true); !errors.Is(err, errTelegramJobSuperseded) {
			t.Fatalf("old callback unexpectedly accepted: %v", err)
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgAttemptTable + " VALUES(1,42,'display','retry_wait','Bad Request',0,0,NULL)"); err != nil {
			return err
		}
		if err := retireTelegramFallbackRejectedAttemptTx(ctx, tx, job, "display"); err != nil {
			return err
		}
		value, err := tx.GetValue("SELECT status FROM " + publishTgAttemptTable + " WHERE id=1")
		if err != nil {
			return err
		}
		if value.String() != telegramAttemptStatusSuperseded {
			t.Fatal("Bot rejection token was not retired")
		}
		if _, err := tx.Exec("UPDATE " + publishTgAttemptTable + " SET status='sending',error_message='' WHERE id=1"); err != nil {
			return err
		}
		if err := retireTelegramFallbackRejectedAttemptTx(ctx, tx, job, "display"); !errors.Is(err, errTelegramAttemptNeedsReconcile) {
			t.Fatalf("unresolved send unexpectedly retired: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTelegramFallbackVerificationOutboxAtomicLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_HEAD_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL on port 35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, ddl := range []string{
			"CREATE TEMP TABLE " + telegramFallbackAccountTaskTable + "(id bigint,tenant_id bigint,account_id bigint,task_type text,task_key text,status text,attempt_count int,lease_owner text,lease_epoch bigint,lease_until timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgJobTable + "(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,operation_no text,status text,send_phase text,dispatch_status text,last_dispatch_error text,reconcile_count int,next_retry_at timestamp,error_message text,updated_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgOutboxTable + "(job_id bigint,event_type text,dedupe_key text UNIQUE,status text,available_at timestamp,created_at timestamp,updated_at timestamp) ON COMMIT DROP",
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		job := telegramJobRecord{Id: 42, TenantId: 7, AccountId: 8, OperationNo: "publish:7:123",
			Status: JobStatusSending, SendPhase: SendPhaseDisplayConfirmed, ReconcileCount: 1,
			LastDispatchError: telegramMediaFallbackReason("display")}
		task := &collectorin.AccountTask{
			ID: 19, TenantID: 7, AccountID: 8, TaskType: collectorin.AccountTaskTypeMessageMediaFallback,
			TaskKey: telegramFallbackTaskKey(job, "display"), Status: collectorin.AccountTaskStatusProcessing,
			AttemptCount: 2, LeaseOwner: "worker-current", LeaseEpoch: 3,
		}
		if _, err := tx.Exec("INSERT INTO "+telegramFallbackAccountTaskTable+" VALUES(19,7,8,?,?,'processing',2,'worker-current',3,(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai')+INTERVAL '1 minute')", task.TaskType, task.TaskKey); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO "+publishTgJobTable+" VALUES(42,7,8,0,?,'sending','display_confirmed','processing',?,1,NULL,'',NULL)", job.OperationNo, job.LastDispatchError); err != nil {
			return err
		}
		s := &sSysPublish{}
		// Same production transaction body; a maintenance rollback must revert
		// both queue eligibility and Outbox intent, never only one of them.
		if _, err := tx.Exec("SAVEPOINT before_handoff"); err != nil {
			return err
		}
		if err := s.resumeTelegramVerificationAfterFallbackTx(ctx, tx, job, task); err != nil {
			return err
		}
		row, err := tx.GetOne("SELECT status,send_phase,dispatch_status,last_dispatch_error FROM " + publishTgJobTable + " WHERE id=42")
		if err != nil {
			return err
		}
		if row["status"].String() != "failed_retry" || row["send_phase"].String() != "display_confirmed" ||
			row["dispatch_status"].String() != "idle" || row["last_dispatch_error"].String() != "" {
			t.Fatalf("display handoff did not produce claimable verification: %v", row)
		}
		count, err := tx.GetValue("SELECT COUNT(*) FROM " + publishTgOutboxTable + " WHERE job_id=42 AND status='pending'")
		if err != nil {
			return err
		}
		if count.Int() != 1 {
			t.Fatal("verification was made claimable without its durable intent")
		}
		if _, err := tx.Exec("ROLLBACK TO SAVEPOINT before_handoff"); err != nil {
			return err
		}
		state, err := tx.GetValue("SELECT status FROM " + publishTgJobTable + " WHERE id=42")
		if err != nil {
			return err
		}
		count, err = tx.GetValue("SELECT COUNT(*) FROM " + publishTgOutboxTable)
		if err != nil {
			return err
		}
		if state.String() != "sending" || count.Int() != 0 {
			t.Fatal("rollback must retain Account ownership and remove verification intent together")
		}
		// Late callbacks from a previous operation/generation must not release
		// the new owner's job into the ordinary Bot queue.
		old := *task
		previous := job
		previous.ReconcileCount--
		old.TaskKey = telegramFallbackTaskKey(previous, "display")
		if err := s.resumeTelegramVerificationAfterFallbackTx(ctx, tx, job, &old); !errors.Is(err, errTelegramJobSuperseded) {
			t.Fatalf("stale callback unexpectedly released ownership: %v", err)
		}
		for _, status := range []JobStatus{JobStatusRetryWait, JobStatusUnknown} {
			if _, err := tx.Exec("SAVEPOINT confirmed_callback"); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE "+publishTgJobTable+" SET status=? WHERE id=42", status.String()); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE " + telegramFallbackAccountTaskTable + " SET status='finalizing',lease_owner='',lease_epoch=0 WHERE id=19"); err != nil {
				return err
			}
			finalizing := *task
			finalizing.Status = collectorin.AccountTaskStatusFinalizing
			if err := s.resumeTelegramVerificationAfterFallbackTx(ctx, tx, job, &finalizing); err != nil {
				return err
			}
			if _, err := tx.Exec("ROLLBACK TO SAVEPOINT confirmed_callback"); err != nil {
				return err
			}
		}
		// Unknown without confirmed display is never permission to resume.
		if _, err := tx.Exec("UPDATE " + publishTgJobTable + " SET status='unknown',send_phase='display_sending' WHERE id=42"); err != nil {
			return err
		}
		if err := s.resumeTelegramVerificationAfterFallbackTx(ctx, tx, job, task); !errors.Is(err, errTelegramJobSuperseded) {
			t.Fatalf("unconfirmed unknown unexpectedly resumed: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTelegramNonAccountReconcileFailureLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_HEAD_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL on port 35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, ddl := range []string{
			"CREATE TEMP TABLE " + publishTgJobTable + "(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,operation_no text,status text,send_phase text,dispatch_status text,last_dispatch_error text,next_retry_at timestamp,error_message text,updated_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishProfileStateTable + "(tenant_id bigint,account_id bigint,profile_id bigint,publish_operation_no text,publish_task_status text,publish_task_updated_at timestamp,updated_at timestamp) ON COMMIT DROP",
			"INSERT INTO " + publishTgJobTable + " VALUES(42,7,8,99,'publish:7:123','unknown','display_sending','processing','',NULL,'',NULL)",
			"INSERT INTO " + publishProfileStateTable + " VALUES(7,8,99,'publish:7:123','publishing',NULL,NULL)",
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		s := &sSysPublish{}
		job := telegramJobRecord{Id: 42, TenantId: 7, AccountId: 8, ProfileId: 99,
			OperationNo: "publish:7:123", Status: JobStatusUnknown, SendPhase: SendPhaseDisplaySending}
		message := telegramFallbackFailureMessage(errors.New("TG响应已确认但审计不完整，无法等待Webhook"))
		for _, status := range []JobStatus{JobStatusUnknown, JobStatusSending} {
			if _, err := tx.Exec("SAVEPOINT ordinary_failure"); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE "+publishTgJobTable+" SET status=? WHERE id=42", status.String()); err != nil {
				return err
			}
			snapshot := job
			snapshot.Status = status
			changed, err := s.failNonAccountTelegramJobTx(ctx, tx, snapshot, message)
			if err != nil {
				return err
			}
			if !changed {
				t.Fatalf("ordinary %s reconcile failure silently skipped", status)
			}
			row, err := tx.GetOne("SELECT j.status,j.error_message,s.publish_task_status FROM " + publishTgJobTable + " j JOIN " + publishProfileStateTable + " s ON s.profile_id=j.profile_id WHERE j.id=42")
			if err != nil {
				return err
			}
			if row["status"].String() != "failed" || row["publish_task_status"].String() != "failed" || row["error_message"].String() != message {
				t.Fatalf("Job and profile failure must converge together: %v", row)
			}
			changed, err = s.failNonAccountTelegramJobTx(ctx, tx, snapshot, message)
			if err != nil || changed {
				t.Fatalf("duplicate callback must not repeat compensation: changed=%v err=%v", changed, err)
			}
			if _, err := tx.Exec("ROLLBACK TO SAVEPOINT ordinary_failure"); err != nil {
				return err
			}
		}
		for _, mutation := range []string{
			"UPDATE " + publishTgJobTable + " SET last_dispatch_error='awaiting_account_fallback:display' WHERE id=42",
			"UPDATE " + publishTgJobTable + " SET operation_no='publish:7:124' WHERE id=42",
			"UPDATE " + publishTgJobTable + " SET status='sent' WHERE id=42",
			"UPDATE " + publishTgJobTable + " SET send_phase='display_confirmed' WHERE id=42",
			"UPDATE " + publishProfileStateTable + " SET publish_operation_no='publish:7:124'",
		} {
			if _, err := tx.Exec("SAVEPOINT owner_changed"); err != nil {
				return err
			}
			if _, err := tx.Exec(mutation); err != nil {
				return err
			}
			changed, err := s.failNonAccountTelegramJobTx(ctx, tx, job, message)
			if err != nil || changed {
				t.Fatalf("stale ordinary snapshot must not fail another owner: changed=%v err=%v mutation=%s", changed, err, mutation)
			}
			projection, err := tx.GetValue("SELECT publish_task_status FROM " + publishProfileStateTable + " WHERE profile_id=99")
			if err != nil {
				return err
			}
			if projection.String() != "publishing" {
				t.Fatal("rejected failure CAS modified profile state")
			}
			if _, err := tx.Exec("ROLLBACK TO SAVEPOINT owner_changed"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
