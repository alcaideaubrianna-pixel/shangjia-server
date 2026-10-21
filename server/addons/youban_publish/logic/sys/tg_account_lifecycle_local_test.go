package sys

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gotd/td/tg"
	"hotgo/internal/library/cache"
)

// Only Telegram RPC responses are simulated. Attempts, response conversion,
// confirmation transactions and finalization use the production implementations.
// Each stage runs in a fresh process against committed local PostgreSQL data.
func TestAccountFallbackLifecycleLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_ACCOUNT_PHASE_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	if stage := os.Getenv("YOUBAN_ACCOUNT_TEST_STAGE"); stage != "" {
		runAccountLifecycleStage(t, stage)
		return
	}
	ctx := context.Background()
	db, err := gdb.New(accountLifecycleConfig("public"))
	if err != nil {
		t.Fatal(err)
	}
	redis := miniredis.RunT(t)
	for _, scenario := range []string{"success", "restart_after_display", "restart_before_verify_response", "crash_after_verify_send", "display_commit_failure", "verify_commit_failure", "multi_media", "reject_premature_completion"} {
		t.Run(scenario, func(t *testing.T) {
			schema := fmt.Sprintf("account_lifecycle_%d", time.Now().UnixNano())
			if _, err := db.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := db.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
					t.Error(err)
				}
			})
			stages := []string{"setup", "display"}
			if scenario == "display_commit_failure" {
				stages = append(stages, "display_recover")
			}
			if scenario == "restart_before_verify_response" {
				stages = append(stages, "prepare_verify")
			}
			if scenario == "reject_premature_completion" {
				stages = append(stages, "premature_finish")
			}
			if scenario == "crash_after_verify_send" {
				stages = append(stages, "verify_crash", "verify_recover")
			} else {
				stages = append(stages, "verify")
			}
			if scenario == "verify_commit_failure" {
				stages = append(stages, "verify_recover")
			}
			stages = append(stages, "finish", "finish")
			for _, stage := range stages {
				commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				cmd := exec.CommandContext(commandCtx, os.Args[0], "-test.run=^TestAccountFallbackLifecycleLocalPostgres$", "-test.count=1")
				cmd.Env = append(os.Environ(), "YOUBAN_ACCOUNT_TEST_STAGE="+stage, "YOUBAN_ACCOUNT_TEST_SCHEMA="+schema, "YOUBAN_ACCOUNT_TEST_REDIS="+redis.Addr(), "YOUBAN_ACCOUNT_TEST_SCENARIO="+scenario)
				output, err := cmd.CombinedOutput()
				cancel()
				if err != nil {
					t.Fatalf("stage %s: %v\n%s", stage, err, output)
				}
				if strings.Contains(string(output), "[WARN]") || strings.Contains(string(output), "[ERRO]") {
					t.Logf("stage %s diagnostics:\n%s", stage, output)
				}
			}
		})
	}
}

func accountLifecycleConfig(schema string) gdb.ConfigNode {
	return gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres", Namespace: schema}
}

func runAccountLifecycleStage(t *testing.T, stage string) {
	t.Helper()
	schema := os.Getenv("YOUBAN_ACCOUNT_TEST_SCHEMA")
	if !regexp.MustCompile(`^account_lifecycle_[0-9]+$`).MatchString(schema) {
		t.Fatal("invalid private test schema")
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{accountLifecycleConfig(schema)})
	gredis.SetConfig(&gredis.Config{Address: os.Getenv("YOUBAN_ACCOUNT_TEST_REDIS")})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	config := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
	for key, value := range map[string]interface{}{"cache.adapter": "memory", "redis.default.address": os.Getenv("YOUBAN_ACCOUNT_TEST_REDIS"), "redis.default.pass": "", "redis.default.db": 0} {
		if err := config.Set(key, value); err != nil {
			t.Fatal(err)
		}
	}
	cache.SetAdapter(ctx)
	db := g.DB()
	s := new(sSysPublish)
	mustExec := func(sql string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	if stage == "setup" {
		data, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
		if err != nil {
			t.Fatal(err)
		}
		// Reuse installation table definitions, without seed data, global settings
		// or heavyweight indexes. Everything is confined to this private schema.
		for _, ddl := range regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS "[^"]+" \(.*?\n\);`).FindAllString(string(data), -1) {
			mustExec(ddl)
		}
		for _, ddl := range regexp.MustCompile(`(?m)^ALTER TABLE "[^"]+" ADD COLUMN IF NOT EXISTS [^;]+;`).FindAllString(string(data), -1) {
			mustExec(ddl)
		}
		for _, ddl := range regexp.MustCompile(`(?m)^CREATE UNIQUE INDEX[^;]+;`).FindAllString(string(data), -1) {
			mustExec(ddl)
		}
		mustExec(`CREATE TABLE hg_content_profile(id bigint PRIMARY KEY,profile_no text,title text,summary text,plain_text text,province text,city text,tag text,tag_params text,cup_size text,source_type text,source_note_uuid text,review_status int,status int,visibility text,published_at timestamp,created_at timestamp,updated_at timestamp,deleted_at timestamp)`)
		mustExec(`CREATE TABLE test_remote_response(phase text PRIMARY KEY,send_count int NOT NULL DEFAULT 1,attempt_id bigint NOT NULL,attempt_token text NOT NULL)`)
		mustExec(`INSERT INTO hg_content_profile(id,profile_no,status,visibility,source_type) VALUES(468643,'D87396',1,'public','collect')`)
		mustExec(`INSERT INTO hg_youban_publish_profile_state(id,tenant_id,account_id,profile_id,publish_operation_no,publish_task_status) VALUES(1,19,464,468643,'cycle_batch:local-test','publishing')`)
		mustExec(`INSERT INTO hg_youban_publish_account(id,tenant_id,nickname) VALUES(464,19,'local account')`)
		mustExec(`INSERT INTO hg_youban_publish_channel(id,tenant_id,status,publish_direction) VALUES(75,19,1,'up')`)
		mustExec(`INSERT INTO hg_youban_publish_media(id,profile_id,media_type,purpose) VALUES(5151488,468643,'video','verify')`)
		mustExec(`INSERT INTO hg_youban_publish_tg_job(id,tenant_id,account_id,profile_id,channel_id,target_chat_id,status,dispatch_status,send_phase,operation_no) VALUES(2249614,19,464,468643,75,'-1002563223375','sending','processing','cleanup_confirmed','cycle_batch:local-test')`)
		return
	}
	job, err := s.telegramJobById(ctx, 2249614)
	if err != nil {
		t.Fatal(err)
	}
	scenario := os.Getenv("YOUBAN_ACCOUNT_TEST_SCENARIO")
	if stage == "premature_finish" {
		if err := s.finalizeTelegramMediaDelivery(ctx, job, true, true); err == nil {
			t.Fatal("completed before verification")
		}
		status, err := s.telegramJobCurrentStatus(ctx, job.Id)
		if err != nil || status != "sending" {
			t.Fatalf("premature finalization changed status=%s err=%v", status, err)
		}
		return
	}
	if stage == "finish" {
		if err := s.finalizeTelegramMediaDelivery(ctx, job, true, true); err != nil {
			t.Fatal(err)
		}
		row, err := db.GetOne(ctx, `SELECT status,dispatch_status,send_phase FROM hg_youban_publish_tg_job WHERE id=2249614`)
		if err != nil || row["status"].String() != "sent" || row["dispatch_status"].String() != "done" || row["send_phase"].String() != "verify_confirmed" {
			t.Fatalf("not finalized: %v err=%v", row, err)
		}
		messageCount := 2
		if scenario == "multi_media" {
			messageCount = 9
		}
		for sql, want := range map[string]int{
			`SELECT count(*) FROM hg_youban_publish_tg_attempt WHERE job_id=2249614 AND status='confirmed' AND response_received=1`:       2,
			`SELECT count(*) FROM hg_youban_publish_tg_attempt WHERE job_id=2249614`:                                                      2,
			`SELECT count(*) FROM test_remote_response WHERE send_count=1`:                                                                2,
			`SELECT count(*) FROM hg_youban_publish_tg_message WHERE job_id=2249614 AND status='sent'`:                                    messageCount,
			`SELECT count(*) FROM hg_youban_publish_tg_attempt_message WHERE job_id=2249614`:                                              messageCount,
			`SELECT count(*) FROM hg_youban_publish_success_record WHERE job_id=2249614 AND status='success'`:                             1,
			`SELECT count(*) FROM hg_youban_publish_profile_state WHERE profile_id=468643 AND publish_task_status=''`:                     1,
			`SELECT count(*) FROM hg_youban_publish_tg_delivery_step WHERE job_id=2249614 AND step_key='finalize' AND status='succeeded'`: 1,
		} {
			v, err := db.GetValue(ctx, sql)
			if err != nil || v.Int() != want {
				t.Fatalf("unexpected final projection: %s value=%v want=%d err=%v", sql, v, want, err)
			}
		}
		return
	}
	phase := strings.TrimSuffix(strings.TrimSuffix(stage, "_recover"), "_crash")
	if stage == "prepare_verify" {
		phase = "verify"
	}
	expected := 1
	if scenario == "multi_media" {
		expected = 6
		if phase == "verify" {
			expected = 3
		}
	}
	attempt, err := s.accountFallbackDeliveryAttempt(ctx, job, phase, expected)
	if err != nil {
		t.Fatal(err)
	}
	if stage == "prepare_verify" {
		return
	}
	// Persist a fake Telegram receipt separately from local confirmation, so a
	// process exit cannot erase evidence of a successful external send.
	v, err := db.GetValue(ctx, `SELECT count(*) FROM test_remote_response WHERE phase=?`, phase)
	if err != nil {
		t.Fatal(err)
	}
	if v.Int() == 0 {
		mustExec(`INSERT INTO test_remote_response(phase,attempt_id,attempt_token) VALUES(?,?,?)`, phase, attempt.Id, attempt.AttemptToken)
	} else {
		receipt, err := db.GetOne(ctx, `SELECT attempt_id,attempt_token FROM test_remote_response WHERE phase=?`, phase)
		if err != nil || receipt["attempt_id"].Int64() != attempt.Id || receipt["attempt_token"].String() != attempt.AttemptToken {
			t.Fatalf("restart replaced original Attempt: receipt=%v attempt=%+v err=%v", receipt, attempt, err)
		}
	}
	mediaID, messageID := int64(4928516), 41128
	if phase == "verify" {
		mediaID, messageID = 5151488, 41134
	}
	media := make([]*telegramMediaItem, expected)
	history := make([]*tg.Message, expected)
	updates := &tg.Updates{}
	for i := range media {
		media[i] = &telegramMediaItem{Id: mediaID + int64(i), Purpose: phase}
		history[i] = &tg.Message{ID: messageID + i}
		if expected > 1 {
			history[i].SetGroupedID(int64(messageID))
		}
		updates.Updates = append(updates.Updates, &tg.UpdateNewChannelMessage{Message: history[i]})
	}
	history[0].Message = telegramAttemptMarker(attempt.AttemptToken)
	messages := gotdSentMessagesFromUpdates(updates, media)
	if v.Int() > 0 {
		// The actual history matching used by account fallback after restart.
		messages, err = selectTelegramAttemptMessages(history, attempt)
		if err != nil {
			t.Fatal(err)
		}
		bindAccountFallbackMessagesToMedia(messages, media)
	}
	if err := validateTelegramMediaSendResult(phase, media, messages); err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(stage, "_crash") {
		return
	}
	if scenario == phase+"_commit_failure" && !strings.HasSuffix(stage, "_recover") {
		mustExec(`CREATE FUNCTION test_fail_confirmation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected confirmation failure'; END $$`)
		mustExec(`CREATE TRIGGER fail_confirmation BEFORE UPDATE ON hg_youban_publish_tg_job FOR EACH ROW WHEN (NEW.send_phase='` + phase + `_confirmed') EXECUTE FUNCTION test_fail_confirmation()`)
		if err := s.confirmTelegramAPISuccess(ctx, job, attempt, messages); err == nil || !strings.Contains(err.Error(), "injected confirmation failure") {
			t.Fatalf("expected injected transaction failure: %v", err)
		}
		for _, table := range []string{publishTgAttemptMessageTable, publishTgMessageTable} {
			v, err := db.GetValue(ctx, "SELECT count(*) FROM "+table+" WHERE job_id=2249614"+map[bool]string{true: " AND purpose='" + phase + "'", false: " AND attempt_id=" + fmt.Sprint(attempt.Id)}[table == publishTgMessageTable])
			if err != nil || v.Int() != 0 {
				t.Fatalf("failed confirmation leaked rows: %s count=%v err=%v", table, v, err)
			}
		}
		row, err := db.GetOne(ctx, `SELECT status,response_received,confirmed_count FROM hg_youban_publish_tg_attempt WHERE id=?`, attempt.Id)
		if err != nil || row["status"].String() != "sending" || row["response_received"].Int() != 0 || row["confirmed_count"].Int() != 0 {
			t.Fatalf("failed commit leaked Attempt confirmation: %v err=%v", row, err)
		}
		mustExec(`DROP TRIGGER fail_confirmation ON hg_youban_publish_tg_job`)
		// Recovery is a new OS process using the same committed Attempt/receipt.
		return
	}
	if err := s.confirmTelegramAPISuccess(ctx, job, attempt, messages); err != nil {
		t.Fatal(err)
	}
	if err := s.confirmTelegramAPISuccess(ctx, job, attempt, messages); err != nil {
		t.Fatalf("confirmation replay: %v", err)
	}
}
