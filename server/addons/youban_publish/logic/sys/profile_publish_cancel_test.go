package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestProfileCancelContract(t *testing.T) {
	for _, status := range []string{"pending", "publishing", "cycling", "canceling", "canceled", "waiting_media"} {
		if err := profileCancelRequestAllowed("opaque:a:b", status, "opaque:a:b"); err != nil {
			t.Fatalf("%s: %v", status, err)
		}
	}
	for _, status := range []string{"", "published", "failed", "unpublishing", "editing"} {
		if err := profileCancelRequestAllowed("opaque", status, "opaque"); err == nil {
			t.Fatalf("completed/non-publish state accepted: %q", status)
		}
	}
	if err := profileCancelRequestAllowed("new", "canceling", "old"); err == nil {
		t.Fatal("stale cancellation was allowed")
	}
	for _, in := range []sysin.ProfilePublishCancelInp{
		{Uuid: "u"}, {OperationNo: "x"}, {Uuid: "u", OperationNo: " x"},
		{Uuid: "u", OperationNo: strings.Repeat("x", 129)},
	} {
		if in.Filter(context.Background()) == nil {
			t.Fatalf("invalid selector accepted: %+v", in)
		}
	}
	for _, value := range []string{"", "true", "TRUE"} {
		t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", value)
		if !profilePublishCancelEnabled() {
			t.Fatalf("cancellation must be enabled for %q", value)
		}
	}
	t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "false")
	if profilePublishCancelEnabled() {
		t.Fatal("explicit false must disable cancellation")
	}
	if canceled, err := profileOperationCanceledTx(context.Background(), nil, cancelFixtureOperation()); err != nil || canceled {
		t.Fatalf("disabled feature must not access cancellation schema: %v %v", canceled, err)
	}
	if err := lockProfilePublishFenceTx(context.Background(), nil, cancelFixtureOperation()); err != nil {
		t.Fatalf("ordinary sends gained a pre-migration schema dependency: %v", err)
	}
}

// Explicit opt-in, loopback-only disposable PostgreSQL. Never reads application
// DSNs and never starts consumers, Redis, Telegram or production services.
func profileCancelPostgresFixture(t *testing.T) (context.Context, func(func(context.Context, gdb.TX) error) error) {
	t.Helper()
	if os.Getenv("YOUBAN_PUBLISH_CANCEL_TEST_PG") != "1" {
		t.Skip("set YOUBAN_PUBLISH_CANCEL_TEST_PG=1 for disposable loopback PostgreSQL :55487")
	}
	t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "true")
	before := gdb.GetConfig("default")
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55487", User: "cancel_test", Name: "postgres"}})
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	schema := fmt.Sprintf("cancel_test_%d", time.Now().UnixNano())
	if _, err := g.DB().Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := g.DB().Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		if err != nil {
			t.Error(err)
		}
	})
	run := func(fn func(context.Context, gdb.TX) error) error {
		return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			if _, err := tx.Exec("SET LOCAL search_path TO " + schema); err != nil {
				return err
			}
			return fn(ctx, tx)
		})
	}
	install, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../resource/sql/profile_publish_cancel.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	err = run(func(ctx context.Context, tx gdb.TX) error {
		for _, table := range []string{publishProfileStateTable, publishNoteIndexTable, publishTgJobTable, publishTgMessageTable, publishTgOutboxTable} {
			start := strings.Index(string(install), `CREATE TABLE IF NOT EXISTS "`+table+`"`)
			if start < 0 {
				return fmt.Errorf("missing fixture DDL %s", table)
			}
			ddl := string(install)[start:]
			ddl = ddl[:strings.Index(ddl, ");")+2]
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		for _, ddl := range []string{
			string(migration),
			"ALTER TABLE " + publishTgOutboxTable + " ADD COLUMN IF NOT EXISTS operation_no varchar(128) DEFAULT '', ADD COLUMN IF NOT EXISTS payload text DEFAULT ''",
			"CREATE UNIQUE INDEX test_cancel_outbox_key ON " + publishTgOutboxTable + "(dedupe_key)",
			"CREATE UNIQUE INDEX test_cancel_message ON " + publishTgMessageTable + "(job_id,tg_message_id)",
			"CREATE TABLE " + publishTgAttemptTable + " (id bigint PRIMARY KEY,job_id bigint,phase text,status text,expected_count int,response_received int DEFAULT 0,confirmed_count int DEFAULT 0,error_message text DEFAULT '',webhook_deadline timestamp,updated_at timestamp)",
			"CREATE TABLE " + publishTgAttemptMessageTable + " (id bigserial PRIMARY KEY,attempt_id bigint,job_id bigint,target_chat_id text,message_id bigint,message_payload text,source text,created_at timestamp,UNIQUE(attempt_id,message_id))",
			"CREATE TABLE " + telegramFallbackAccountTaskTable + " (id bigint,tenant_id bigint,task_key text,status text)",
			"CREATE TABLE hg_content_profile (id bigint PRIMARY KEY,status int,visibility text,source_note_uuid text,profile_no text,deleted_at timestamp)",
			"INSERT INTO hg_content_profile VALUES(3,1,'public','test-uuid','TEST1',NULL)",
			"INSERT INTO " + publishProfileStateTable + " (tenant_id,account_id,profile_id,publish_operation_no,publish_task_status) VALUES(1,2,3,'opaque:a:b','publishing')",
			"INSERT INTO " + publishNoteIndexTable + " (tenant_id,account_id,profile_id,task_status) VALUES(1,2,3,'publishing')",
			"INSERT INTO " + publishTgJobTable + " (id,tenant_id,account_id,profile_id,channel_id,operation_no,status,send_phase) VALUES(10,1,2,3,7,'opaque:a:b','sending','display_sending'),(11,1,2,3,8,'old-round','sent','display_confirmed'),(12,9,2,3,7,'opaque:a:b','pending','')",
			"INSERT INTO " + publishTgOutboxTable + " (job_id,event_type,dedupe_key,status,available_at) VALUES(10,'telegram_job.publish','send10','pending',LOCALTIMESTAMP)",
			"INSERT INTO " + publishTgAttemptTable + " (id,job_id,phase,status,expected_count) VALUES(20,10,'display','sending',1)",
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		return g.DB().GetCore().ClearTableFieldsAll(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, run
}

func cancelFixtureOperation() telegramJobRecord {
	return telegramJobRecord{Id: 10, TenantId: 1, AccountId: 2, ProfileId: 3, OperationNo: "opaque:a:b", TargetChatId: "-100"}
}

func TestProfileCancelDurabilityAndExactScopePostgres(t *testing.T) {
	_, run := profileCancelPostgresFixture(t)
	s := &sSysPublish{}
	job := cancelFixtureOperation()
	rollback := errors.New("simulate transaction abort")
	err := run(func(ctx context.Context, tx gdb.TX) error {
		if err := s.acceptProfilePublishCancelTx(ctx, tx, job); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	err = run(func(ctx context.Context, tx gdb.TX) error {
		for _, table := range []string{publishProfileCancelTable, publishTgOutboxTable} {
			mod := tx.Model(table)
			if table == publishTgOutboxTable {
				mod = mod.WhereLike("event_type", telegramOutboxCleanupPrefix+"%")
			}
			n, err := mod.Count()
			if err != nil || n != 0 {
				return fmt.Errorf("intent escaped rollback %s: %d %v", table, n, err)
			}
		}
		for i := 0; i < 2; i++ {
			if err := s.acceptProfilePublishCancelTx(ctx, tx, job); err != nil {
				return err
			}
		}
		n, err := tx.Model(publishProfileCancelTable).Count()
		if err != nil || n != 1 {
			return fmt.Errorf("not idempotent %d %v", n, err)
		}
		if err := s.stopCanceledProfileJobsTx(ctx, tx, job); err != nil {
			return err
		}
		for id, want := range map[int64]string{10: "superseded", 11: "sent", 12: "pending"} {
			row, err := tx.Model(publishTgJobTable).Where("id", id).One()
			if err != nil || row["status"].String() != want {
				return fmt.Errorf("scope leak job %d: %v %v", id, row, err)
			}
		}
		changed, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{JobId: 10, From: []JobStatus{JobStatusSuperseded}, To: JobStatusPending})
		if err == nil && changed {
			return errors.New("canceled Job was restarted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProfileCancelLateReceiptAndPollingPostgres(t *testing.T) {
	_, run := profileCancelPostgresFixture(t)
	s := &sSysPublish{}
	job := cancelFixtureOperation()
	attempt := telegramDeliveryAttempt{Id: 20, JobId: 10, Phase: "display", ExpectedCount: 1, TargetChatId: "-100"}
	intent := profilePublishCancel{TenantID: 1, AccountID: 2, ProfileID: 3, OperationNo: job.OperationNo}
	err := run(func(ctx context.Context, tx gdb.TX) error {
		if err := s.acceptProfilePublishCancelTx(ctx, tx, job); err != nil {
			return err
		}
		if err := s.stopCanceledProfileJobsTx(ctx, tx, job); err != nil {
			return err
		}
		if err := s.settleProfilePublishCancelTx(ctx, tx, intent); err != nil {
			return err
		}
		state, _ := tx.Model(publishProfileStateTable).One()
		if state["publish_task_status"].String() != "canceling" {
			return errors.New("in-flight send was declared canceled")
		}
		// First cleanup has already run when the Bot/protocol reply arrives.
		if _, err := tx.Model(publishTgOutboxTable).WhereLike("event_type", telegramOutboxCleanupPrefix+"%").
			Data(g.Map{"status": telegramOutboxDelivered}).Update(); err != nil {
			return err
		}
		messages := []*telegramSentMessage{{MessageId: 99, Purpose: "display"}}
		if err := recordTelegramAttemptMessagesTx(ctx, tx, attempt, messages, "response"); err != nil {
			return err
		}
		n, err := tx.Model(publishTgOutboxTable).WhereLike("event_type", telegramOutboxCleanupPrefix+"%").Where("status", telegramOutboxPending).Count()
		if err != nil || n != 1 {
			return fmt.Errorf("late receipt missing new durable cleanup: %d %v", n, err)
		}
		if err := s.writeProfilePublishOperationStateTx(ctx, tx, 1, 2, 3, job.OperationNo, sysin.PublishTaskStatusPublished, true); err != nil {
			return err
		}
		state, _ = tx.Model(publishProfileStateTable).One()
		if state["publish_task_status"].String() != "canceling" {
			return errors.New("late success overwrote cancel")
		}
		if _, err := tx.Model(publishTgMessageTable).Where("job_id", 10).Data(g.Map{"status": "deleted"}).Update(); err != nil {
			return err
		}
		if _, err := tx.Model(publishTgOutboxTable).WhereLike("event_type", telegramOutboxCleanupPrefix+"%").
			Data(g.Map{"status": telegramOutboxDelivered}).Update(); err != nil {
			return err
		}
		if err := s.settleProfilePublishCancelTx(ctx, tx, intent); err != nil {
			return err
		}
		if err := recordTelegramAttemptMessagesTx(ctx, tx, attempt, messages, "response"); err != nil {
			return err
		}
		state, _ = tx.Model(publishProfileStateTable).One()
		if state["publish_task_status"].String() != "canceled" || state["publish_operation_no"].String() != job.OperationNo {
			return fmt.Errorf("terminal polling identity lost: %v", state)
		}
		if err := s.acceptProfilePublishCancelTx(ctx, tx, job); err != nil {
			return fmt.Errorf("completed cancellation is not idempotent: %w", err)
		}
		n, _ = tx.Model(publishTgMessageTable).Where("status", "sent").Count()
		if n != 0 {
			return errors.New("duplicate receipt resurrected deleted message")
		}
		original, _ := tx.Model("hg_content_profile").Value("status")
		if original.Int() != 1 {
			return errors.New("business status changed")
		}
		polled, err := s.profileTaskStatusBySelector(ctx, &sysin.ProfileViewInp{Id: 3})
		if err != nil || polled.TaskStatus != "canceled" || polled.OperationNo != job.OperationNo {
			return fmt.Errorf("read-only status selector lost cancellation identity: %+v %v", polled, err)
		}
		// Starting a new operation retires the displayed terminal, not the old
		// cancellation tombstone. Old UI must not cancel this newer round.
		if err := s.beginProfilePublishOperationTx(ctx, tx, 1, 2, 3, "next"); err != nil {
			return err
		}
		if err := s.acceptProfilePublishCancelTx(ctx, tx, job); err == nil {
			return errors.New("old page canceled a new round")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProfileCancelCleanupFailureRollsBackPostgres(t *testing.T) {
	_, run := profileCancelPostgresFixture(t)
	s := &sSysPublish{}
	job := cancelFixtureOperation()
	if err := run(func(ctx context.Context, tx gdb.TX) error {
		_, err := tx.Exec("ALTER TABLE " + publishTgOutboxTable + " ADD CONSTRAINT reject_cleanup CHECK(event_type='telegram_job.publish')")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := run(func(ctx context.Context, tx gdb.TX) error {
		return s.acceptProfilePublishCancelTx(ctx, tx, job)
	}); err == nil {
		t.Fatal("cancellation was accepted without durable cleanup")
	}
	if err := run(func(ctx context.Context, tx gdb.TX) error {
		count, err := tx.Model(publishProfileCancelTable).Count()
		if err != nil || count != 0 {
			return fmt.Errorf("cancellation intent escaped failed cleanup transaction: %d %v", count, err)
		}
		state, err := tx.Model(publishProfileStateTable).One()
		if err != nil || state["publish_task_status"].String() != "publishing" {
			return fmt.Errorf("failed cleanup changed task status: %v %v", state, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProfileCancelConcurrentGenerationPostgres(t *testing.T) {
	_, run := profileCancelPostgresFixture(t)
	s := &sSysPublish{}
	job := cancelFixtureOperation()
	locked := make(chan struct{})
	release := make(chan struct{})
	results := make(chan error, 2)
	var once sync.Once
	go func() {
		results <- run(func(ctx context.Context, tx gdb.TX) error {
			if err := s.acceptProfilePublishCancelTx(ctx, tx, job); err != nil {
				return err
			}
			once.Do(func() { close(locked) })
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("cancel transaction did not lock operation")
	}
	go func() {
		results <- run(func(ctx context.Context, tx gdb.TX) error {
			err := lockProfilePublishFenceTx(ctx, tx, job)
			if !errors.Is(err, errTelegramJobSuperseded) {
				return fmt.Errorf("generation escaped cancel fence: %v", err)
			}
			return nil
		})
	}()
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestProfileCancelPartialProtocolReceiptPostgres(t *testing.T) {
	_, run := profileCancelPostgresFixture(t)
	s := &sSysPublish{}
	job := cancelFixtureOperation()
	attempt := telegramDeliveryAttempt{Id: 20, JobId: 10, Phase: "display", ExpectedCount: 2, TargetChatId: "-100"}
	intent := profilePublishCancel{TenantID: 1, AccountID: 2, ProfileID: 3, OperationNo: job.OperationNo}
	err := run(func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model(publishTgAttemptTable).Where("id", 20).Data(g.Map{"expected_count": 2}).Update(); err != nil {
			return err
		}
		if err := s.acceptProfilePublishCancelTx(ctx, tx, job); err != nil {
			return err
		}
		if err := s.stopCanceledProfileJobsTx(ctx, tx, job); err != nil {
			return err
		}
		if _, err := tx.Model(telegramFallbackAccountTaskTable).Insert(g.Map{
			"id": 1, "tenant_id": 1, "task_key": telegramFallbackTaskKey(job, "display"), "status": "processing",
		}); err != nil {
			return err
		}
		if err := recordTelegramAttemptMessagesTx(ctx, tx, attempt, []*telegramSentMessage{{MessageId: 101, Purpose: "display"}}, "response"); err != nil {
			return err
		}
		if err := s.settleProfilePublishCancelTx(ctx, tx, intent); err != nil {
			return err
		}
		checkPending := func() error {
			state, err := tx.Model(publishProfileStateTable).One()
			if err != nil || state["publish_task_status"].String() != "canceling" {
				return fmt.Errorf("partial/in-flight protocol send prematurely canceled: %v %v", state, err)
			}
			return nil
		}
		if err := checkPending(); err != nil {
			return err
		}
		if _, err := tx.Model(telegramFallbackAccountTaskTable).Where("id", 1).Data(g.Map{"status": "succeeded"}).Update(); err != nil {
			return err
		}
		// Even after the task owner finishes, incomplete remote evidence is
		// not proof of absence and must remain recoverable.
		if err := s.settleProfilePublishCancelTx(ctx, tx, intent); err != nil {
			return err
		}
		if err := checkPending(); err != nil {
			return err
		}
		if err := recordTelegramAttemptMessagesTx(ctx, tx, attempt, []*telegramSentMessage{{MessageId: 102, Purpose: "display"}}, "webhook"); err != nil {
			return err
		}
		if _, err := tx.Model(publishTgMessageTable).Where("job_id", 10).Data(g.Map{"status": "deleted"}).Update(); err != nil {
			return err
		}
		if _, err := tx.Model(publishTgOutboxTable).Where("job_id", 10).Data(g.Map{"status": telegramOutboxDelivered}).Update(); err != nil {
			return err
		}
		if err := s.settleProfilePublishCancelTx(ctx, tx, intent); err != nil {
			return err
		}
		state, err := tx.Model(publishProfileStateTable).One()
		if err != nil || state["publish_task_status"].String() != "canceled" {
			return fmt.Errorf("resolved operation did not settle: %v %v", state, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
