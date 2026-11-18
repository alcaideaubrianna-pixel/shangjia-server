package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestProfileFullPushScopeIdentity(t *testing.T) {
	scope, ok := parseProfileFullPushScope("full_push:7:100:3")
	if !ok || scope.BatchNo != "full_push:7:100" || scope.ChannelID != 7 || scope.ProfileID != 3 {
		t.Fatalf("unexpected scope: %+v valid:%t", scope, ok)
	}
	for _, operation := range []string{
		"full_push:7:100", "full_push:7:100:3:4", "full_push:8:batch:3",
		"full_push:07:100:3", "full_push:7:0:3", "full_push:7:100:-3",
		"full_push:7:100:3 ", "full_push:+7:100:3", "profile:7:100:3",
	} {
		if _, valid := parseProfileFullPushScope(operation); valid {
			t.Fatalf("accepted invalid full-push identity %q", operation)
		}
	}
}

func TestProfileFullPushAdmissionTerminalStatuses(t *testing.T) {
	for _, status := range []string{"", "failed", "canceled"} {
		if !profileFullPushAdmissionStatusIsTerminal(status) {
			t.Fatalf("new admission must allow terminal status %q", status)
		}
	}
	for _, status := range []string{"pending", "publishing", "editing", "waiting_media", "canceling", "unpublishing", "unknown"} {
		if profileFullPushAdmissionStatusIsTerminal(status) {
			t.Fatalf("new admission must not take over status %q", status)
		}
	}
}

func TestProfileFullPushFenceDisabledCompatibility(t *testing.T) {
	t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "false")
	job := telegramJobRecord{ProfileId: 3, OperationNo: "full_push:legacy"}
	// nil TX proves that the legacy disabled path performs no new reads.
	if err := lockProfilePublishFenceTx(context.Background(), nil, job); err != nil {
		t.Fatal(err)
	}
	if err := lockProfileFullPushFenceTx(context.Background(), nil, job); err != nil {
		t.Fatal(err)
	}
}

func TestProfileSupersededSendEvidence(t *testing.T) {
	job := telegramJobRecord{SendPhase: telegramSendPhaseCleanupConfirmed}
	if !profileSupersededJobHasNoSendEvidence(job, 0, 0) {
		t.Fatal("pre-Attempt cleanup phase should be safe to retire")
	}
	if profileSupersededJobHasNoSendEvidence(job, 1, 0) ||
		profileSupersededJobHasNoSendEvidence(job, 0, 1) {
		t.Fatal("an Attempt or message must retain receipt rights")
	}
	for _, phase := range []SendPhase{
		telegramSendPhaseDisplaySending, telegramSendPhaseDisplayConfirmed,
		telegramSendPhaseVerifySending, telegramSendPhaseVerifyConfirmed,
		telegramSendPhaseCompletedNoVerify, telegramSendPhaseTimeoutCancelled,
	} {
		job.SendPhase = phase
		if profileSupersededJobHasNoSendEvidence(job, 0, 0) {
			t.Fatalf("phase alone retains remote uncertainty: %s", phase)
		}
	}
	job.SendPhase = telegramSendPhaseCleanupConfirmed
	job.TaskId = 10
	if profileSupersededJobHasNoSendEvidence(job, 0, 0) {
		t.Fatal("protocol task cannot be declared absent")
	}
}

func TestProfilePublishJobGenerationIdentity(t *testing.T) {
	job := telegramJobRecord{Id: 10, TenantId: 1, AccountId: 0, ProfileId: 3,
		ChannelId: 7, OperationNo: "full_push:7:100:3", BotId: 8,
		AsynqTaskId: "generation-1", ReconcileCount: 1}
	if !profilePublishJobGenerationMatches(job, job) {
		t.Fatal("bot account zero is a valid owner")
	}
	for _, mutate := range []func(*telegramJobRecord){
		func(j *telegramJobRecord) { j.Id++ },
		func(j *telegramJobRecord) { j.TenantId++ },
		func(j *telegramJobRecord) { j.AccountId++ },
		func(j *telegramJobRecord) { j.ProfileId++ },
		func(j *telegramJobRecord) { j.ChannelId++ },
		func(j *telegramJobRecord) { j.OperationNo = "full_push:7:101:3" },
		func(j *telegramJobRecord) { j.ReconcileCount++ },
	} {
		current := job
		mutate(&current)
		if profilePublishJobGenerationMatches(job, current) {
			t.Fatalf("stale generation accepted: %+v", current)
		}
	}
	for _, mutate := range []func(*telegramJobRecord){
		func(j *telegramJobRecord) { j.BotId++ },
		func(j *telegramJobRecord) { j.AsynqTaskId = "redispatched-task" },
	} {
		current := job
		mutate(&current)
		if !profilePublishJobOperationMatches(job, current) || !profilePublishJobGenerationMatches(job, current) {
			t.Fatalf("transport/queue metadata replaced operation identity: %+v", current)
		}
	}
	current := job
	current.ReconcileCount++
	if !profilePublishJobOperationMatches(job, current) || profilePublishJobGenerationMatches(job, current) {
		t.Fatal("handoff must retain operation authority but fence the old error handler")
	}
}

// Uses the existing explicit-opt-in disposable PostgreSQL fixture. No test
// reads production DSNs, starts a consumer, or requires Telegram/Redis.
func TestProfileFullPushIndependentChannelFencePostgres(t *testing.T) {
	cases := []struct {
		name       string
		change     string
		minimal    bool
		admission  bool
		wantClosed bool
	}{
		{name: "A survives B UI operation"},
		{name: "admission with explicit channel", admission: true},
		{name: "admission after completed ordinary publish", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:3:90',publish_task_status=''"},
		{name: "admission after previous cancellation tombstone", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:3:90',publish_task_status=''"},
		{name: "existing Job cannot cross completed ordinary publish", change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:3:90',publish_task_status=''", wantClosed: true},
		{name: "admission blocks ordinary pending", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:3:90',publish_task_status='pending'", wantClosed: true},
		{name: "admission blocks ordinary publishing", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:3:90',publish_task_status='publishing'", wantClosed: true},
		{name: "admission after failed legacy empty operation", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='',publish_task_status='failed'"},
		{name: "admission blocks canceled legacy empty operation", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='',publish_task_status='canceled'", wantClosed: true},
		{name: "admission after failed ordinary publish", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:3:90',publish_task_status='failed'"},
		{name: "admission blocks ordinary editing", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:3:90',publish_task_status='editing'", wantClosed: true},
		{name: "admission blocks ordinary waiting media", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:3:90',publish_task_status='waiting_media'", wantClosed: true},
		{name: "admission blocks canceling", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='canceling'", wantClosed: true},
		{name: "admission after canceled fullpush", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='canceled'"},
		{name: "admission after canceled ordinary publish", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:3:90',publish_task_status='canceled'"},
		{name: "admission blocks incoming cancellation tombstone", admission: true, change: "INSERT INTO " + publishProfileCancelTable + " (tenant_id,account_id,profile_id,operation_no,status) VALUES(1,2,3,'full_push:7:100:3','canceled')", wantClosed: true},
		{name: "admission blocks same canceled operation", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='full_push:7:100:3',publish_task_status='canceled'", wantClosed: true},
		{name: "admission blocks unpublishing", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='unpublishing'", wantClosed: true},
		{name: "admission blocks completed takedown", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='unpublish:1:90',publish_task_status=''", wantClosed: true},
		{name: "admission blocks completed bot deletion", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='bot_delete:3:90',publish_task_status=''", wantClosed: true},
		{name: "admission blocks malformed fullpush slot", admission: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='full_push:invalid',publish_task_status=''", wantClosed: true},
		{name: "admission requires active batch", admission: true, change: "UPDATE " + publishFullPushBatchTable + " SET active_key=NULL WHERE id=1", wantClosed: true},
		{name: "admission blocks offline profile", admission: true, change: "UPDATE hg_content_profile SET status=2,visibility='private'", wantClosed: true},
		{name: "admission blocks deleted state", admission: true, change: "UPDATE " + publishProfileStateTable + " SET deleted_at=LOCALTIMESTAMP", wantClosed: true},
		{name: "B completion retains operation", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status=''"},
		{name: "completed batch remains valid", change: "UPDATE " + publishFullPushBatchTable + " SET status='completed'"},
		{name: "partial batch remains valid", change: "UPDATE " + publishFullPushBatchTable + " SET status='partial_failed'"},
		{name: "minimal current recovery", minimal: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='full_push:7:100:3'"},
		{name: "minimal noncurrent recovery", minimal: true, wantClosed: true},
		{name: "bot account zero", change: "UPDATE " + publishProfileStateTable + " SET account_id=0"},
		{name: "canceling state", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='canceling'", wantClosed: true},
		{name: "down state", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='unpublishing'", wantClosed: true},
		{name: "deleted state", change: "UPDATE " + publishProfileStateTable + " SET deleted_at=LOCALTIMESTAMP", wantClosed: true},
		{name: "offline profile", change: "UPDATE hg_content_profile SET status=2,visibility='private'", wantClosed: true},
		{name: "deleted profile", change: "UPDATE hg_content_profile SET deleted_at=LOCALTIMESTAMP", wantClosed: true},
		{name: "forged UI batch", change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='full_push:8:999:3'", wantClosed: true},
		{name: "new manual operation", change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:new'", wantClosed: true},
		{name: "same channel new batch", change: "INSERT INTO " + publishFullPushBatchTable + " VALUES(3,1,7,'full_push:7:102',3,'pending','1:7')", wantClosed: true},
		{name: "same channel new Job", change: "INSERT INTO " + publishTgJobTable + " (id,tenant_id,account_id,profile_id,channel_id,operation_no,operation_type,status) VALUES(15,1,2,3,7,'profile:new','publish','pending')", wantClosed: true},
		{name: "protocol handoff retains operation authority", change: "UPDATE " + publishTgJobTable + " SET reconcile_count=1 WHERE id=10"},
		{name: "Bot switch retains operation authority", change: "UPDATE " + publishTgJobTable + " SET bot_id=99 WHERE id=10"},
		{name: "redispatch retains operation authority", change: "UPDATE " + publishTgJobTable + " SET asynq_task_id='redispatched-task' WHERE id=10"},
		{name: "superseded persistent Job", change: "UPDATE " + publishTgJobTable + " SET status='superseded' WHERE id=10", wantClosed: true},
		{name: "exact canceled operation", change: "INSERT INTO " + publishProfileCancelTable + " (tenant_id,account_id,profile_id,operation_no,status) VALUES(1,2,3,'full_push:7:100:3','canceled')", wantClosed: true},
		{name: "other profile cancel isolated", change: "INSERT INTO " + publishProfileCancelTable + " (tenant_id,account_id,profile_id,operation_no,status) VALUES(1,2,4,'full_push:7:100:3','canceled')"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, run := profileCancelPostgresFixture(t)
			err := run(func(ctx context.Context, tx gdb.TX) error {
				for _, sql := range []string{
					"CREATE TABLE " + publishFullPushBatchTable + " (id bigint PRIMARY KEY,tenant_id bigint,channel_id bigint,batch_no text UNIQUE,snapshot_max_profile_id bigint,status text,active_key text UNIQUE)",
					"INSERT INTO " + publishFullPushBatchTable + " VALUES(1,1,7,'full_push:7:100',3,'dispatching',NULL),(2,1,8,'full_push:8:101',3,'dispatching',NULL)",
					"UPDATE " + publishProfileStateTable + " SET publish_operation_no='full_push:8:101:3',publish_task_status='publishing'",
					"UPDATE " + publishTgJobTable + " SET operation_no='full_push:7:100:3' WHERE id=10",
				} {
					if _, err := tx.Exec(sql); err != nil {
						return err
					}
				}
				if tc.admission {
					if _, err := tx.Exec("UPDATE " + publishFullPushBatchTable + " SET active_key='1:7' WHERE id=1"); err != nil {
						return err
					}
				}
				if tc.change != "" {
					if _, err := tx.Exec(tc.change); err != nil {
						return err
					}
				}
				if tc.name == "admission after previous cancellation tombstone" || tc.name == "admission after canceled ordinary publish" {
					if _, err := tx.Exec("INSERT INTO " + publishProfileCancelTable + " (tenant_id,account_id,profile_id,operation_no,status) VALUES(1,2,3,'profile:3:90','canceled')"); err != nil {
						return err
					}
				}
				job := telegramJobRecord{Id: 10, TenantId: 1, AccountId: 2, ProfileId: 3,
					ChannelId: 7, OperationNo: "full_push:7:100:3"}
				if tc.name == "bot account zero" {
					job.AccountId = 0
					if _, err := tx.Exec("UPDATE " + publishTgJobTable + " SET account_id=0 WHERE id=10"); err != nil {
						return err
					}
				}
				if tc.minimal {
					job.Id = 0
					job.ChannelId = 0
				}
				if tc.admission {
					job.Id = 0
				}
				err := lockProfilePublishFenceTx(ctx, tx, job)
				if tc.wantClosed {
					if !errors.Is(err, errTelegramJobSuperseded) {
						return fmt.Errorf("expected closed fence, got %v", err)
					}
					return nil
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProfileSupersededSendSettlementPostgres(t *testing.T) {
	cases := []struct {
		name       string
		change     string
		retired    bool
		awaiting   bool
		wantError  bool
		wantStatus string
	}{
		{name: "no Attempt or remote evidence", retired: true, wantStatus: "superseded"},
		{name: "still valid owner", change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='opaque:a:b'", wantError: true, wantStatus: "sending"},
		{name: "inflight Attempt", change: "INSERT INTO " + publishTgAttemptTable + " (id,job_id,phase,status,expected_count) VALUES(21,10,'display','sending',1)", awaiting: true, wantStatus: "unknown"},
		{name: "superseded Attempt retains receipt rights", change: "INSERT INTO " + publishTgAttemptTable + " (id,job_id,phase,status,expected_count) VALUES(21,10,'display','superseded',1)", awaiting: true, wantStatus: "unknown"},
		{name: "orphan message audit", change: "INSERT INTO " + publishTgAttemptMessageTable + " (attempt_id,job_id,message_id) VALUES(99,10,123)", awaiting: true, wantStatus: "unknown"},
		{name: "remote phase without Attempt", change: "UPDATE " + publishTgJobTable + " SET send_phase='display_sending' WHERE id=10", awaiting: true, wantStatus: "unknown"},
		{name: "replacement generation", change: "UPDATE " + publishTgJobTable + " SET reconcile_count=1 WHERE id=10", wantStatus: "sending"},
		{name: "already canceled Job", change: "UPDATE " + publishTgJobTable + " SET status='superseded' WHERE id=10", retired: true, wantStatus: "superseded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, run := profileCancelPostgresFixture(t)
			err := run(func(ctx context.Context, tx gdb.TX) error {
				for _, sql := range []string{
					"DELETE FROM " + publishTgAttemptTable,
					"UPDATE " + publishTgJobTable + " SET send_phase='cleanup_confirmed' WHERE id=10",
					"UPDATE " + publishProfileStateTable + " SET publish_operation_no='new-operation'",
				} {
					if _, err := tx.Exec(sql); err != nil {
						return err
					}
				}
				var observed telegramJobRecord
				if err := tx.Model(publishTgJobTable).Where("id", 10).Scan(&observed); err != nil {
					return err
				}
				if tc.change != "" {
					if _, err := tx.Exec(tc.change); err != nil {
						return err
					}
				}
				retired, awaiting, transitioned, err := (&sSysPublish{}).settleSupersededTelegramSendTx(ctx, tx, observed)
				wantTransition := tc.awaiting || tc.name == "no Attempt or remote evidence"
				if (err != nil) != tc.wantError || retired != tc.retired || awaiting != tc.awaiting || transitioned != wantTransition {
					return fmt.Errorf("retired=%t awaiting=%t transitioned=%t err=%v", retired, awaiting, transitioned, err)
				}
				if tc.name == "no Attempt or remote evidence" {
					repaired, waiting, changed, repairErr := (&sSysPublish{}).settleSupersededTelegramSendTx(ctx, tx, observed)
					if repairErr != nil || !repaired || waiting || changed {
						return fmt.Errorf("idempotent outbox repair must not report another transition: retired=%t awaiting=%t transitioned=%t err=%v", repaired, waiting, changed, repairErr)
					}
				}
				row, err := tx.Model(publishTgJobTable).Where("id", 10).One()
				if err != nil || row["status"].String() != tc.wantStatus {
					return fmt.Errorf("unexpected Job status %v err:%v", row["status"], err)
				}
				if tc.awaiting && row["dispatch_status"].String() != DispatchStatusProcessing.String() {
					return fmt.Errorf("receipt-bearing Job lost in-flight dispatch: %v", row["dispatch_status"])
				}
				if tc.retired && tc.name != "already canceled Job" && row["dispatch_status"].String() != DispatchStatusDone.String() {
					return fmt.Errorf("terminal Job did not complete dispatch: %v", row["dispatch_status"])
				}
				outbox, err := tx.Model(publishTgOutboxTable).Where("job_id", 10).One()
				if err != nil || outbox["status"].String() != telegramOutboxPending {
					return fmt.Errorf("transaction must not ACK outbox before commit: %v err:%v", outbox["status"], err)
				}
				if tc.name == "inflight Attempt" || tc.name == "superseded Attempt retains receipt rights" {
					attempt, err := tx.Model(publishTgAttemptTable).Where("id", 21).One()
					expected := "sending"
					if tc.name == "superseded Attempt retains receipt rights" {
						expected = "superseded"
					}
					if err != nil || attempt["status"].String() != expected {
						return fmt.Errorf("Attempt receipt rights changed: %v err:%v", attempt["status"], err)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
