package sys

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestFailedCycleRecoveryParsing(t *testing.T) {
	for _, operation := range []string{
		"cycle_batch:due:2970840:9:683806:300:extra",
		"cycle_batch:due:2970840:9:683807:300",
		"cycle_batch:due:2970840:0:683806:300",
		"cycle_batch:due:2970840x:9:683806:300",
		"cycle_batch:due:02970840:9:683806:300",
		"cycle_batch:due:9223372036854775808:9:683806:300",
		"cycle_batch:123:683806:300",
	} {
		if _, _, ok := failedCycleDueSource(operation, 683806); ok {
			t.Errorf("accepted malformed or unrelated due operation %q", operation)
		}
	}
	if source, channel, ok := failedCycleDueSource("cycle_batch:due:2970840:9:683806:300", 683806); !ok || source != 2970840 || channel != 300 {
		t.Fatal("canonical source operation was not parsed")
	}
	root := int64(2970840)
	other := root + 1
	zero := int64(0)
	for _, tc := range []struct {
		name      string
		operation string
		recovery  *int64
		timeout   *int64
		want      bool
	}{
		{"legacy account plan", "profile_cycle:account:632:683806:2970840:plan:5", nil, nil, true},
		{"legacy auto recovery", "auto-recover:2970840:683806", nil, nil, true},
		{"explicit recovery", "timeout-child", &root, nil, true},
		{"explicit timeout", "timeout-child", nil, &root, true},
		{"matching explicit fields", "timeout-child", &root, &root, true},
		{"conflicting explicit fields", "timeout-child", &root, &other, false},
		{"explicit zero disables legacy", "profile_cycle:account:632:683806:2970840:plan:5", &zero, nil, false},
		{"explicit contradicts operation", "profile_cycle:account:632:683806:2970840:plan:5", &other, nil, false},
		{"wrong account", "profile_cycle:account:633:683806:2970840:plan:5", nil, nil, false},
		{"wrong profile", "profile_cycle:account:632:683807:2970840:plan:5", nil, nil, false},
		{"trailing data", "profile_cycle:account:632:683806:2970840:plan:5:extra", nil, nil, false},
		{"manual", "profile:683806:later", nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := failedCycleRecoveryJob{ID: 3211050, AccountID: 632, ProfileID: 683806,
				OperationNo: tc.operation, RecoveryOfJobID: tc.recovery, TimeoutParentID: tc.timeout}
			_, ok := failedCycleRecoveryParent(job)
			if ok != tc.want {
				t.Fatalf("parent accepted=%v want=%v", ok, tc.want)
			}
		})
	}
}

// No default database configuration is read or replaced. Every test owns a
// fresh schema on an explicitly opted-in loopback high port.
func failedCycleRecoveryTestDB(t *testing.T) gdb.DB {
	t.Helper()
	portText := os.Getenv("YOUBAN_PROFILE_STATE_TEST_PG_PORT")
	if portText == "" {
		t.Skip("set YOUBAN_PROFILE_STATE_TEST_PG_PORT=55490 for isolated loopback PostgreSQL")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 49152 || port > 65535 {
		t.Fatal("test PostgreSQL port must be an explicit loopback high port (49152..65535)")
	}
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: portText, User: "postgres", Name: "postgres"}
	root, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	schema := fmt.Sprintf("profile_state_recovery_%d", time.Now().UnixNano())
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = root.Close(ctx)
	})
	node.Namespace = schema
	db, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(ctx) })
	for _, query := range []string{
		`CREATE TABLE hg_content_profile(id bigint PRIMARY KEY,status int,visibility text,deleted_at timestamp)`,
		`CREATE TABLE ` + publishProfileStateTable + `(tenant_id bigint,account_id bigint,profile_id bigint,publish_operation_no text,publish_task_status text,publish_task_updated_at timestamp,updated_at timestamp,deleted_at timestamp)`,
		`CREATE INDEX idx_failed_active ON ` + publishProfileStateTable + `(publish_task_status,publish_task_updated_at,profile_id) WHERE deleted_at IS NULL AND publish_task_status<>''`,
		`CREATE TABLE ` + publishNoteIndexTable + `(tenant_id bigint,account_id bigint,profile_id bigint,task_status text,updated_at timestamp,deleted_at timestamp)`,
		`CREATE TABLE ` + publishTgJobTable + `(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,operation_no text,status text,send_phase text,sent_at timestamp,recovery_of_job_id bigint,timeout_parent_job_id bigint,task_id bigint)`,
		`CREATE INDEX idx_test_job_profile ON ` + publishTgJobTable + `(profile_id,channel_id,id DESC)`,
		`CREATE TABLE ` + publishChannelProfileTable + `(tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,status text)`,
		`CREATE TABLE ` + publishProfileChannelTable + `(tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,is_manual int,deleted_at timestamp)`,
		`CREATE TABLE ` + publishChannelTable + `(id bigint,tenant_id bigint,status int,publish_direction text,is_default_selected int,deleted_at timestamp)`,
		`CREATE TABLE ` + publishMediaTable + `(profile_id bigint,purpose text,deleted_at timestamp)`,
		`CREATE TABLE ` + publishTgMessageTable + `(job_id bigint,purpose text,status text,deleted_at timestamp)`,
		`CREATE TABLE ` + publishProfileCancelTable + `(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,uuid text,operation_no text,status text,error_message text)`,
		`INSERT INTO hg_content_profile VALUES(683806,1,'public',NULL)`,
		`INSERT INTO ` + publishProfileStateTable + ` VALUES(1,632,683806,'cycle_batch:due:2970840:9:683806:300','failed','2026-11-18 01:00:00','2026-11-18 01:00:00',NULL)`,
		`INSERT INTO ` + publishNoteIndexTable + ` VALUES(1,632,683806,'failed',NULL,NULL)`,
		`INSERT INTO ` + publishTgJobTable + `(id,tenant_id,account_id,profile_id,channel_id,operation_no,status,send_phase,sent_at)
		 VALUES(2970840,1,632,683806,300,'profile:old','superseded','verify_confirmed','2026-11-17 01:00:00'),
		 (3211050,1,632,683806,300,'profile_cycle:account:632:683806:2970840:plan:5','sent','verify_confirmed','2026-11-18 02:00:00'),
		 (3211051,1,632,683806,301,'profile:other','sent','verify_confirmed','2026-11-17 02:00:00'),
		 (3211052,1,632,683806,302,'profile:other','sent','verify_confirmed','2026-11-17 02:00:00'),
		 (3211053,1,632,683806,303,'profile:other','sent','verify_confirmed','2026-11-17 02:00:00')`,
		`INSERT INTO ` + publishChannelProfileTable + ` SELECT 1,632,683806,c,'active' FROM generate_series(300,303)c`,
		`INSERT INTO ` + publishChannelTable + ` SELECT c,1,1,'up',1,NULL::timestamp FROM generate_series(300,303)c`,
		`INSERT INTO ` + publishMediaTable + ` SELECT 683806,'display',NULL::timestamp FROM generate_series(1,4) UNION ALL SELECT 683806,'verify',NULL::timestamp`,
		`INSERT INTO ` + publishTgMessageTable + ` SELECT j.id,'display','sent',NULL::timestamp FROM ` + publishTgJobTable + ` j CROSS JOIN generate_series(1,4) WHERE j.id>2970840`,
		`INSERT INTO ` + publishTgMessageTable + ` SELECT id,'verify','sent',NULL::timestamp FROM ` + publishTgJobTable + ` WHERE id>2970840`,
	} {
		if _, err = db.Exec(ctx, query); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	return db
}

func failedCycleTestSnapshot() profilePublishOperationState {
	return profilePublishOperationState{TenantId: 1, AccountId: 632, ProfileId: 683806,
		PublishOperationNo: "cycle_batch:due:2970840:9:683806:300",
		PublishTaskStatus:  sysin.PublishTaskStatusFailed, PublishTaskUpdatedAt: gtime.New("2026-11-18 01:00:00")}
}

func TestFailedCycleRecoveryPostgres(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change string
		want   bool
	}{
		{"empty old operation with proven source", "", true},
		{"wrong source tenant", "UPDATE " + publishTgJobTable + " SET tenant_id=2 WHERE id=2970840", false},
		{"wrong source account", "UPDATE " + publishTgJobTable + " SET account_id=633 WHERE id=2970840", false},
		{"wrong source profile", "UPDATE " + publishTgJobTable + " SET profile_id=683807 WHERE id=2970840", false},
		{"wrong source channel", "UPDATE " + publishTgJobTable + " SET channel_id=301 WHERE id=2970840", false},
		{"source missing", "DELETE FROM " + publishTgJobTable + " WHERE id=2970840", false},
		{"unrelated later success", "UPDATE " + publishTgJobTable + " SET operation_no='profile:manual' WHERE id=3211050", false},
		{"explicit conflicting source", "UPDATE " + publishTgJobTable + " SET recovery_of_job_id=2970841 WHERE id=3211050", false},
		{"explicit timeout source", "UPDATE " + publishTgJobTable + " SET operation_no='timeout-tail',timeout_parent_job_id=2970840 WHERE id=3211050", true},
		{"explicit recovery source", "UPDATE " + publishTgJobTable + " SET operation_no='auto-recover:2970840:683806',recovery_of_job_id=2970840 WHERE id=3211050", true},
		{"explicit zero is authoritative", "UPDATE " + publishTgJobTable + " SET recovery_of_job_id=0 WHERE id=3211050", false},
		{"newer projection time", "UPDATE " + publishProfileStateTable + " SET publish_task_updated_at='2026-11-18 01:01:00'", false},
		{"different current operation", "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:new'", false},
		{"unpublishing", "UPDATE " + publishProfileStateTable + " SET publish_task_status='unpublishing'", false},
		{"canceling", "UPDATE " + publishProfileStateTable + " SET publish_task_status='canceling'", false},
		{"canceled", "UPDATE " + publishProfileStateTable + " SET publish_task_status='canceled'", false},
		{"offline", "UPDATE hg_content_profile SET status=2", false},
		{"private", "UPDATE hg_content_profile SET visibility='private'", false},
		{"profile deleted", "UPDATE hg_content_profile SET deleted_at=NOW()", false},
		{"old success", "UPDATE " + publishTgJobTable + " SET sent_at='2026-11-18 00:59:00' WHERE id=3211050", false},
		{"same time success", "UPDATE " + publishTgJobTable + " SET sent_at='2026-11-18 01:00:00' WHERE id=3211050", false},
		{"success timestamp absent", "UPDATE " + publishTgJobTable + " SET sent_at=NULL WHERE id=3211050", false},
		{"missing target job", "DELETE FROM " + publishTgJobTable + " WHERE id=3211053", false},
		{"target active", "UPDATE " + publishTgJobTable + " SET status='failed_retry' WHERE id=3211053", false},
		{"target reconciling", "UPDATE " + publishTgJobTable + " SET status='reconciling' WHERE id=3211053", false},
		{"target partial failure", "UPDATE " + publishTgJobTable + " SET status='failed' WHERE id=3211053", false},
		{"sent without phase", "UPDATE " + publishTgJobTable + " SET send_phase='' WHERE id=3211050", false},
		{"verify still sending", "UPDATE " + publishTgJobTable + " SET send_phase='verify_sending' WHERE id=3211050", false},
		{"wrong no verify terminal phase", "UPDATE " + publishTgJobTable + " SET send_phase='completed_without_verify' WHERE id=3211050", false},
		{"verify message missing", "DELETE FROM " + publishTgMessageTable + " WHERE job_id=3211050 AND purpose='verify'", false},
		{"verify message deleted", "UPDATE " + publishTgMessageTable + " SET deleted_at=NOW() WHERE job_id=3211050 AND purpose='verify'", false},
		{"display messages no longer sent", "UPDATE " + publishTgMessageTable + " SET status='deleted' WHERE job_id=3211050 AND purpose='display'", false},
		{"new channel configured without job", "INSERT INTO " + publishChannelTable + " VALUES(304,1,1,'up',1,NULL)", false},
		{"later same channel real failure", "INSERT INTO " + publishTgJobTable + "(id,tenant_id,account_id,profile_id,channel_id,operation_no,status) VALUES(4000000,1,632,683806,300,'profile:new','failed')", false},
		{"old operation contains unproven failure", "INSERT INTO " + publishTgJobTable + "(id,tenant_id,account_id,profile_id,channel_id,operation_no,status) VALUES(3000000,1,632,683806,301,'cycle_batch:due:2970840:9:683806:300','failed')", false},
		{"active on unselected channel", "INSERT INTO " + publishTgJobTable + "(id,tenant_id,account_id,profile_id,channel_id,operation_no,status) VALUES(4000000,1,632,683806,399,'profile:active','unknown')", false},
		{"timeout cleanup on unselected channel", "INSERT INTO " + publishTgJobTable + "(id,tenant_id,account_id,profile_id,channel_id,operation_no,status,send_phase) VALUES(4000000,1,632,683806,399,'profile:cleanup','failed','timeout_wait_cleanup')", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "false")
			db := failedCycleRecoveryTestDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if tc.change != "" {
				if _, err := db.Exec(ctx, tc.change); err != nil {
					t.Fatal(err)
				}
			}
			before, err := db.GetOne(ctx, "SELECT publish_task_status,publish_operation_no,publish_task_updated_at FROM "+publishProfileStateTable)
			if err != nil {
				t.Fatal(err)
			}
			got, err := (&sSysPublish{}).recoverFailedCyclePublishState(ctx, db, failedCycleTestSnapshot())
			if err != nil || got != tc.want {
				t.Fatalf("repaired=%v want=%v err=%v", got, tc.want, err)
			}
			state, err := db.GetOne(ctx, "SELECT publish_task_status,publish_operation_no,publish_task_updated_at FROM "+publishProfileStateTable)
			if err != nil {
				t.Fatal(err)
			}
			index, err := db.GetOne(ctx, "SELECT task_status FROM "+publishNoteIndexTable)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want {
				if state["publish_task_status"].String() != "" || index["task_status"].String() != "" {
					t.Fatalf("projection diverged state=%v index=%v", state, index)
				}
			} else if state["publish_task_status"].String() != before["publish_task_status"].String() ||
				state["publish_task_updated_at"].String() != before["publish_task_updated_at"].String() ||
				index["task_status"].String() != "failed" {
				t.Fatalf("rejected reconciliation mutated state/index: %v / %v", state, index)
			}
		})
	}
}

func TestFailedCycleHistoryCannotMaskFailurePostgres(t *testing.T) {
	t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "false")
	db := failedCycleRecoveryTestDB(t)
	ctx := context.Background()
	s := &sSysPublish{}
	if err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return s.writeProfilePublishOperationStateTx(ctx, tx, 1, 632, 683806,
			"cycle_batch:due:2970840:9:683806:300", sysin.PublishTaskStatusFailed, true)
	}); err != nil {
		t.Fatal(err)
	}
	row, err := db.GetOne(ctx, "SELECT publish_task_status FROM "+publishProfileStateTable)
	if err != nil || row["publish_task_status"].String() != "failed" {
		t.Fatalf("historical sent masked true failure: %v err=%v", row, err)
	}
}

func TestFailedCycleSameOperationRecoveryPostgres(t *testing.T) {
	for _, partialFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("partialFailure_%t", partialFailure), func(t *testing.T) {
			t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "false")
			db := failedCycleRecoveryTestDB(t)
			ctx := context.Background()
			for _, query := range []string{
				"UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:manual'",
				"UPDATE " + publishTgJobTable + " SET operation_no='profile:manual' WHERE id>2970840",
			} {
				if _, err := db.Exec(ctx, query); err != nil {
					t.Fatal(err)
				}
			}
			if partialFailure {
				if _, err := db.Exec(ctx, "UPDATE "+publishTgJobTable+" SET status='failed' WHERE id=3211053"); err != nil {
					t.Fatal(err)
				}
			}
			snapshot := failedCycleTestSnapshot()
			snapshot.PublishOperationNo = "profile:manual"
			repaired, err := (&sSysPublish{}).recoverFailedCyclePublishState(ctx, db, snapshot)
			if err != nil || repaired == partialFailure {
				t.Fatalf("same operation repaired=%v partialFailure=%v err=%v", repaired, partialFailure, err)
			}
		})
	}
}

func TestFailedCycleConcurrentCASPostgres(t *testing.T) {
	t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "false")
	db := failedCycleRecoveryTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	start := make(chan struct{})
	type result struct {
		repaired bool
		err      error
	}
	results := make(chan result, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			ok, err := (&sSysPublish{}).recoverFailedCyclePublishState(ctx, db, failedCycleTestSnapshot())
			results <- result{ok, err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	successes := 0
	for r := range results {
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.repaired {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent CAS winners=%d want=1", successes)
	}
}

func TestFailedCycleIndexFailureRollsBackPostgres(t *testing.T) {
	t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "false")
	db := failedCycleRecoveryTestDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, "ALTER TABLE "+publishNoteIndexTable+" ADD CONSTRAINT reject_clear CHECK(task_status<>'')"); err != nil {
		t.Fatal(err)
	}
	repaired, err := (&sSysPublish{}).recoverFailedCyclePublishState(ctx, db, failedCycleTestSnapshot())
	if err == nil || repaired {
		t.Fatalf("index rejection must roll back: repaired=%v err=%v", repaired, err)
	}
	state, readErr := db.GetOne(ctx, "SELECT publish_task_status FROM "+publishProfileStateTable)
	if readErr != nil || state["publish_task_status"].String() != "failed" {
		t.Fatalf("state escaped rollback: %v err=%v", state, readErr)
	}
}

func TestFailedCycleMediaPreflightDoesNotWrite(t *testing.T) {
	// This deliberately has no database setup: preflight cannot claim or clear
	// any operation merely because a scheduler observes unavailable media.
	if err := deferCycleMediaPreflight(context.Background(), 1, 632, 683806,
		"cycle_batch:due:2970840:9:683806:300", "failed_without_source"); err != errProfileCycleMediaPending {
		t.Fatalf("want retryable media sentinel, got %v", err)
	}
}

func TestFailedCycleCancellationFencePostgres(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceled_%t", canceled), func(t *testing.T) {
			t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "true")
			db := failedCycleRecoveryTestDB(t)
			ctx := context.Background()
			if canceled {
				_, err := db.Exec(ctx, "INSERT INTO "+publishProfileCancelTable+
					" (tenant_id,account_id,profile_id,uuid,operation_no,status,error_message) VALUES(1,632,683806,'fixture','cycle_batch:due:2970840:9:683806:300','canceled','')")
				if err != nil {
					t.Fatal(err)
				}
			}
			repaired, err := (&sSysPublish{}).recoverFailedCyclePublishState(ctx, db, failedCycleTestSnapshot())
			if err != nil || repaired == canceled {
				t.Fatalf("default fence repaired=%v canceled=%v err=%v", repaired, canceled, err)
			}
		})
	}
}

func TestFailedCycleManualOverridesDefaultPostgres(t *testing.T) {
	t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "true")
	db := failedCycleRecoveryTestDB(t)
	ctx := context.Background()
	for _, query := range []string{
		"INSERT INTO " + publishProfileChannelTable + " VALUES(1,632,683806,300,1,NULL)",
		"DELETE FROM " + publishChannelProfileTable + " WHERE channel_id<>300",
		"DELETE FROM " + publishTgJobTable + " WHERE channel_id<>300",
	} {
		if _, err := db.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	repaired, err := (&sSysPublish{}).recoverFailedCyclePublishState(ctx, db, failedCycleTestSnapshot())
	if err != nil || !repaired {
		t.Fatalf("manual A must override defaults B/C/D: repaired=%v err=%v", repaired, err)
	}
}

func TestFailedCycleOtherAccountManualKeepsDefaultsPostgres(t *testing.T) {
	for _, missingDefault := range []bool{false, true} {
		t.Run(fmt.Sprintf("missingDefault_%t", missingDefault), func(t *testing.T) {
			t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "true")
			db := failedCycleRecoveryTestDB(t)
			ctx := context.Background()
			for _, query := range []string{
				"INSERT INTO " + publishProfileChannelTable + " VALUES(1,633,683806,300,1,NULL)",
				// Leave only the source channel in the plan, so the other
				// targets can only be discovered through current defaults.
				"DELETE FROM " + publishChannelProfileTable + " WHERE channel_id<>300",
			} {
				if _, err := db.Exec(ctx, query); err != nil {
					t.Fatal(err)
				}
			}
			if missingDefault {
				if _, err := db.Exec(ctx, "DELETE FROM "+publishTgJobTable+" WHERE channel_id=301"); err != nil {
					t.Fatal(err)
				}
			}
			repaired, err := (&sSysPublish{}).recoverFailedCyclePublishState(ctx, db, failedCycleTestSnapshot())
			if err != nil || repaired == missingDefault {
				t.Fatalf("another account's manual must not hide defaults: repaired=%v missingDefault=%v err=%v", repaired, missingDefault, err)
			}
			if missingDefault {
				state, readErr := db.GetOne(ctx, "SELECT publish_task_status FROM "+publishProfileStateTable)
				if readErr != nil || state["publish_task_status"].String() != "failed" {
					t.Fatalf("missing default target must preserve failure: %v err=%v", state, readErr)
				}
				index, readErr := db.GetOne(ctx, "SELECT task_status FROM "+publishNoteIndexTable)
				if readErr != nil || index["task_status"].String() != "failed" {
					t.Fatalf("missing default target must preserve index: %v err=%v", index, readErr)
				}
			}
		})
	}
}

func TestFailedCycleCompletionLockOrderPostgres(t *testing.T) {
	t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "true")
	db := failedCycleRecoveryTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	// Model the normal completion order: hold state before acquiring profile.
	if _, err = tx.Model(publishProfileStateTable).Ctx(ctx).Where("profile_id", 683806).LockUpdate().One(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := (&sSysPublish{}).recoverFailedCyclePublishState(ctx, db, failedCycleTestSnapshot())
		done <- err
	}()
	// Wait until recovery is actually waiting on the state row. If it holds
	// profile first, the next acquisition deterministically forms a deadlock.
	waiting := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		row, readErr := db.GetOne(ctx, "SELECT 1 AS waiting FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND query LIKE ? LIMIT 1",
			"%"+publishProfileStateTable+"%")
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !row.IsEmpty() {
			waiting = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("recovery never reached the state row lock")
	}
	if _, err = tx.Model("hg_content_profile").Ctx(ctx).Where("id", 683806).LockUpdate().One(); err != nil {
		t.Fatalf("normal completion state->profile lock acquisition: %v", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatalf("concurrent recovery: %v", err)
	}
}
