package sys

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestTelegramPermissionCircuitClassificationAndStreak(t *testing.T) {
	for _, message := range []string{"Bad Request: chat not found", "Forbidden: bot is not a member of the channel chat", "Forbidden: bot was kicked from the channel chat", "CHAT_WRITE_FORBIDDEN", "not enough rights to send text messages"} {
		if !isTelegramPermissionCircuitError(errors.New(message)) {
			t.Fatalf("permission error missed: %s", message)
		}
	}
	for _, err := range []error{nil, context.DeadlineExceeded, errors.New("HTTP 404"), errors.New("WEBPAGE_CURL_FAILED"), errors.New("Too Many Requests: retry after 30"), errors.New("wrong file identifier")} {
		if isTelegramPermissionCircuitError(err) {
			t.Fatalf("nonpermission error counted: %v", err)
		}
	}
	jobs := make([]telegramJobRecord, 10)
	for i := range jobs {
		jobs[i] = telegramJobRecord{Id: int64(i + 1), Status: "failed", ErrorMessage: "Bad Request: chat not found"}
	}
	if telegramChannelPermissionStreak(jobs[:9]) || !telegramChannelPermissionStreak(jobs) {
		t.Fatal("threshold must be exactly 10 distinct terminal jobs")
	}
	for _, status := range []string{"sent", "unknown", "failed_retry"} {
		jobs[5].Status = status
		if telegramChannelPermissionStreak(jobs) {
			t.Fatalf("%s must break streak", status)
		}
	}
	jobs[5].Status, jobs[5].ErrorMessage = "failed", "HTTP 404"
	if telegramChannelPermissionStreak(jobs) {
		t.Fatal("media failure must break streak")
	}
}

func TestTelegramPermissionCircuitLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_PERMISSION_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		migration, err := os.ReadFile("../../resource/sql/channel_permission_circuit.pgsql.sql")
		if err != nil {
			return err
		}
		if _, err = tx.Exec(strings.Replace(string(migration), "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1)); err != nil {
			return err
		}
		for _, sql := range []string{
			"CREATE TEMP TABLE " + publishTgJobTable + " (id bigint PRIMARY KEY,tenant_id bigint,channel_id bigint,status text,dispatch_status text,last_dispatch_error text,next_retry_at timestamp,updated_at timestamp,created_at timestamp DEFAULT NOW(),dispatch_count int DEFAULT 1,operation_no text DEFAULT '',error_message text DEFAULT 'Bad Request: chat not found',send_phase text DEFAULT '')",
			"CREATE TEMP TABLE " + publishTgAttemptTable + " (id bigint,job_id bigint)",
			"CREATE TEMP TABLE " + publishTgMessageTable + " (id bigint,job_id bigint)",
			"INSERT INTO " + publishTgAttemptTable + " VALUES (1,1)",
			"CREATE TEMP TABLE " + publishTgOutboxTable + " (id bigserial PRIMARY KEY,job_id bigint,status text,event_type text DEFAULT 'telegram_job.publish',dedupe_key text UNIQUE,available_at timestamp,updated_at timestamp,created_at timestamp,lease_owner text,lease_expires_at timestamp,delivered_at timestamp)",
			"INSERT INTO " + publishTgJobTable + " (id,tenant_id,channel_id,status,dispatch_status,last_dispatch_error,next_retry_at,updated_at) VALUES (1,1,20,'pending','idle','',NULL,NULL),(2,1,20,'unknown','idle','',NULL,NULL),(3,2,20,'pending','idle','',NULL,NULL),(4,1,21,'pending','idle','',NULL,NULL),(5,1,20,'failed','done','',NULL,NULL)",
			"INSERT INTO " + publishTgOutboxTable + " (id,job_id,status,available_at,dedupe_key) VALUES (1,1,'pending',NOW()+interval '1 hour','telegram-job:1:initial'),(2,3,'pending',NOW()+interval '1 hour','telegram-job:3:initial')",
		} {
			if _, err = tx.Exec(sql); err != nil {
				return err
			}
		}
		job := telegramJobRecord{Id: 10, TenantId: 1, ChannelId: 20}
		historyJob := telegramJobRecord{TenantId: 1, ChannelId: 30}
		if _, err = tx.Exec("INSERT INTO " + publishTgJobTable + " (id,tenant_id,channel_id,status,updated_at) SELECT 10+n,1,30,'failed',NOW() FROM generate_series(1,10) n"); err != nil {
			return err
		}
		history, err := channelPermissionRecentJobs(ctx, tx.Model(publishTgJobTable), historyJob, nil)
		if err != nil {
			return err
		}
		if !telegramChannelPermissionStreak(history) {
			t.Fatal("real PostgreSQL last-10 query missed failures")
		}
		if _, err = tx.Exec("UPDATE " + publishTgJobTable + " SET status='sent' WHERE id=20"); err != nil {
			return err
		}
		history, err = channelPermissionRecentJobs(ctx, tx.Model(publishTgJobTable), historyJob, nil)
		if err != nil {
			return err
		}
		if telegramChannelPermissionStreak(history) {
			t.Fatal("success did not break SQL streak")
		}
		history, err = channelPermissionRecentJobs(ctx, tx.Model(publishTgJobTable), historyJob, &channelPermissionCircuit{ResumedAt: gtime.New(time.Now().Add(time.Hour))})
		if err != nil {
			return err
		}
		if len(history) != 0 {
			t.Fatal("pre-resume failures included")
		}
		if err = pauseChannelPermissionTx(ctx, tx, job, 77, "generation-one", nil); err != nil {
			return err
		}
		if err = pauseChannelPermissionTx(ctx, tx, job, 77, "duplicate", nil); err != nil {
			return err
		}
		row, err := tx.GetOne("SELECT generation FROM " + channelPermissionCircuitTable + " WHERE tenant_id=1 AND channel_id=20")
		if err != nil {
			return err
		}
		if row["generation"].String() != "generation-one" {
			t.Fatal("duplicate pause replaced active generation")
		}
		parked, err := parkPermissionPausedJobTx(ctx, tx, telegramJobRecord{Id: 1, TenantId: 1, ChannelId: 20}, "generation-one")
		if err != nil {
			return err
		}
		if !parked {
			t.Fatal("paused worker was not parked")
		}
		rows, err := tx.GetAll("SELECT id,last_dispatch_error,next_retry_at FROM " + publishTgJobTable + " ORDER BY id")
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row["id"].Int() == 1 {
				if row["last_dispatch_error"].String() != channelPermissionPauseMessage || row["next_retry_at"].IsNil() {
					t.Fatal("pending job not parked")
				}
			} else if row["last_dispatch_error"].String() != "" {
				t.Fatal("pause modified unknown, failed or another channel/tenant")
			}
		}
		in := &sysin.BotChannelActionInp{TenantId: 1, AccountId: 77, ChannelId: 20}
		if err = resumeChannelPermissionTx(ctx, tx, in, "stale"); err == nil {
			t.Fatal("stale callback resumed channel")
		}
		wrong := *in
		wrong.AccountId = 78
		if err = resumeChannelPermissionTx(ctx, tx, &wrong, "generation-one"); err == nil {
			t.Fatal("another owner resumed channel")
		}
		if err = resumeChannelPermissionTx(ctx, tx, in, "generation-one"); err != nil {
			return err
		}
		row, err = tx.GetOne("SELECT next_retry_at,last_dispatch_error FROM " + publishTgJobTable + " WHERE id=1")
		if err != nil {
			return err
		}
		if !row["next_retry_at"].IsNil() || row["last_dispatch_error"].String() != "" {
			t.Fatal("resume left job asleep")
		}
		row, err = tx.GetOne("SELECT available_at<=clock_timestamp() AS due FROM " + publishTgOutboxTable + " WHERE id=1")
		if err != nil {
			return err
		}
		if !row["due"].Bool() {
			t.Fatal("resume left Outbox delayed")
		}
		if err = pauseChannelPermissionTx(ctx, tx, job, 77, "stale-pause", nil); err != nil {
			return err
		}
		row, err = tx.GetOne("SELECT paused FROM " + channelPermissionCircuitTable + " WHERE tenant_id=1 AND channel_id=20")
		if err != nil {
			return err
		}
		if row["paused"].Int() != 0 {
			t.Fatal("stale failure reopened circuit after resume")
		}
		parked, err = parkPermissionPausedJobTx(ctx, tx, telegramJobRecord{Id: 1, TenantId: 1, ChannelId: 20}, "generation-one")
		if err != nil {
			return err
		}
		if parked {
			t.Fatal("late worker parked job after resume")
		}
		// Long-term pause: no new job, no hourly transport retry, and a
		// bounded batch of only untouched publishing jobs is cancelled.
		for _, statement := range []string{
			"CREATE TEMP TABLE " + publishSuccessRecordTable + " (job_id bigint)",
			"INSERT INTO " + publishTgJobTable + " (id,tenant_id,channel_id,status,created_at) SELECT 1000+n,1,40,'pending',NOW()-interval '2 months' FROM generate_series(1,250) n",
			"INSERT INTO " + publishTgJobTable + " (id,tenant_id,channel_id,status,operation_no) VALUES (2001,1,40,'pending','down:1:1'),(2002,2,40,'pending','profile:other'),(2003,1,40,'unknown','profile:unknown'),(2004,1,40,'pending','profile:evidence')",
			"INSERT INTO " + publishTgAttemptTable + " VALUES (2,2004)",
			"INSERT INTO " + publishTgOutboxTable + " (id,job_id,status,dedupe_key) VALUES (1001,1001,'pending','old-job-1001')",
			"INSERT INTO " + publishSuccessRecordTable + " VALUES (1001)",
		} {
			if _, err = tx.Exec(statement); err != nil {
				return err
			}
		}
		longJob := telegramJobRecord{Id: 3000, TenantId: 1, ChannelId: 40}
		if err = pauseChannelPermissionTx(ctx, tx, longJob, 77, "long-pause", nil); err != nil {
			return err
		}
		newJob, err := (&sSysPublish{}).ensureTelegramProfileJobWithMetaTx(ctx, tx, gdb.Record{"tenant_id": gvar.New(1)}, telegramJobChannel{Id: 40}, "new-profile", telegramProfilePublishMeta{})
		if err != nil || newJob != 0 {
			t.Fatalf("paused creation must not create a job: id=%d err=%v", newJob, err)
		}
		if !errors.Is(profilePublishPreparationError(1, nil), errTelegramPublishPermissionPaused) {
			t.Fatal("zero jobs on a permission-paused channel must not report successful submission")
		}
		for _, statement := range []string{
			"CREATE TEMP TABLE " + publishCycleRunTable + " (id bigint PRIMARY KEY,status text,stage text,cursor_id bigint,queued_count int,error_message text,finished_at timestamp,updated_at timestamp)",
			"CREATE TEMP TABLE " + publishChannelTable + " (id bigint PRIMARY KEY,cycle_active_run_id bigint,cycle_last_error_message text,cycle_last_run_at timestamp,updated_at timestamp)",
			"INSERT INTO " + publishCycleRunTable + " (id,status,cursor_id,queued_count) VALUES (900,'running',10,3)",
			"INSERT INTO " + publishChannelTable + " (id,cycle_active_run_id) VALUES (40,900)",
		} {
			if _, err = tx.Exec(statement); err != nil {
				return err
			}
		}
		run := cycleRunRecord{Id: 900, ChannelId: 40, QueuedCount: 3}
		if err = finishPermissionPausedCycleRunTx(ctx, tx, run, 11, 1); err != nil {
			return err
		}
		// Redelivery must not overwrite the successful prefix or count the
		// blocked profile as queued. The channel ownership clears atomically.
		if err = finishPermissionPausedCycleRunTx(ctx, tx, run, 99, 9); err != nil {
			return err
		}
		row, err = tx.GetOne("SELECT status,cursor_id,queued_count FROM " + publishCycleRunTable + " WHERE id=900")
		if err != nil || row["status"].String() != cycleRunStatusSkipped || row["cursor_id"].Int() != 11 || row["queued_count"].Int() != 4 {
			t.Fatalf("paused cycle lost its real submitted prefix: %v %v", row, err)
		}
		row, err = tx.GetOne("SELECT cycle_active_run_id FROM " + publishChannelTable + " WHERE id=40")
		if err != nil || row["cycle_active_run_id"].Int64() != 0 {
			t.Fatalf("paused cycle still occupies the channel: %v %v", row, err)
		}
		count, err := tx.Model(publishTgJobTable).Where("channel_id", 40).Where("status", "superseded").Count()
		if err != nil || count != 200 {
			t.Fatalf("cancellation must be bounded to 200: count=%d err=%v", count, err)
		}
		row, err = tx.GetOne("SELECT status FROM " + publishTgOutboxTable + " WHERE job_id=1001")
		if err != nil || row["status"].String() != telegramOutboxDelivered {
			t.Fatalf("cancelled outbox is still retrying: %v %v", row, err)
		}
		for _, id := range []int64{2001, 2002, 2003, 2004} {
			row, err = tx.GetOne("SELECT status FROM "+publishTgJobTable+" WHERE id=?", id)
			if err != nil || row["status"].String() == "superseded" {
				t.Fatalf("deletion, other tenant or evidence was cancelled: id=%d err=%v", id, err)
			}
		}
		if err = resumeChannelPermissionTx(ctx, tx, &sysin.BotChannelActionInp{TenantId: 1, AccountId: 77, ChannelId: 40}, "long-pause"); err != nil {
			return err
		}
		parked, err = parkPermissionPausedJobTx(ctx, tx, telegramJobRecord{Id: 1250, TenantId: 1, ChannelId: 40}, "long-pause")
		if err != nil || !parked {
			t.Fatalf("resume must not send untouched historical backlog: parked=%v err=%v", parked, err)
		}
		if _, err = tx.Exec("INSERT INTO " + publishTgJobTable + " (id,tenant_id,channel_id,status,created_at) VALUES (3001,1,40,'pending',NOW()+interval '1 hour')"); err != nil {
			return err
		}
		parked, err = parkPermissionPausedJobTx(ctx, tx, telegramJobRecord{Id: 3001, TenantId: 1, ChannelId: 40}, "long-pause")
		if err != nil || parked {
			t.Fatalf("new post-resume jobs must proceed: parked=%v err=%v", parked, err)
		}
		if _, err = tx.Exec("UPDATE " + publishTgJobTable + " SET updated_at=NOW()-interval '33 days' WHERE id=1001"); err != nil {
			return err
		}
		if err = prunePermissionCancelledJobsTx(ctx, tx, gtime.New(time.Now().AddDate(0, -1, 0))); err != nil {
			return err
		}
		for _, table := range []string{publishTgJobTable, publishTgOutboxTable, publishSuccessRecordTable} {
			key := "job_id"
			if table == publishTgJobTable {
				key = "id"
			}
			count, err = tx.Model(table).Where(key, 1001).Count()
			if err != nil || count != 0 {
				t.Fatalf("expired schedule survived: table=%s count=%d err=%v", table, count, err)
			}
		}
		count, err = tx.Model(publishTgJobTable).Where("id", 1002).Count()
		if err != nil || count != 1 {
			t.Fatalf("recent cancellation must retain its month: %d %v", count, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
