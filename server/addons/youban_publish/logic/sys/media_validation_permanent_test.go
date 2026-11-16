package sys

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"

	"hotgo/internal/library/cache"
	"hotgo/utility/dbtime"
)

func TestProfileMediaPermanentValidation(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		counts                    profileMediaReadinessCounts
		expectedVerify            int
		writing, ready, permanent bool
	}{
		{"ready single display", profileMediaReadinessCounts{Display: 1, Verify: 1}, 1, false, false, true},
		{"zero display", profileMediaReadinessCounts{}, 0, false, false, true},
		{"single pending metadata cannot grow display", profileMediaReadinessCounts{Display: 1, Pending: 1}, 0, false, false, true},
		{"declared verification missing", profileMediaReadinessCounts{Display: 2}, 1, false, false, true},
		{"failed source unavailable", profileMediaReadinessCounts{Display: 2, Unavailable: 1}, 0, false, false, true},
		{"failed original source usable", profileMediaReadinessCounts{Display: 2}, 0, false, true, false},
		{"all ready", profileMediaReadinessCounts{Display: 2, Verify: 1}, 1, false, true, false},
		{"no verification declared", profileMediaReadinessCounts{Display: 2}, 0, false, true, false},
		{"pending metadata", profileMediaReadinessCounts{Display: 2, Pending: 1}, 0, false, false, false},
		{"bot write not yet persisted", profileMediaReadinessCounts{}, 1, true, false, false},
		{"partial bot write", profileMediaReadinessCounts{Display: 1}, 1, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ready, err := validateProfileMediaReadiness(tc.counts, tc.expectedVerify, tc.writing)
			if ready != tc.ready || errors.Is(err, errProfileMediaValidation) != tc.permanent {
				t.Fatalf("ready=%v err=%v", ready, err)
			}
			if tc.permanent {
				if !isProfileMediaValidationError(gerror.Wrap(err, "wrapped")) || !isProfileMediaValidationError(errors.New(err.Error())) {
					t.Fatal("permanent marker lost after wrapping or persistence")
				}
				job := telegramJobRecord{Id: 1, ProfileId: 1, ChannelId: 1, OperationNo: "profile:1", ErrorMessage: err.Error()}
				if terminalPublishRecoveryEligible(job, err) || terminalPublishRecoveryEligible(job, errors.New("old retry")) {
					t.Fatal("permanent media failure must not recover, including previously queued recovery")
				}
			}
		})
	}
}

func TestProfileMediaWaitBounds(t *testing.T) {
	now := time.Now().In(dbtime.Location())
	for _, tc := range []struct {
		attempts int
		age      time.Duration
		want     bool
	}{
		{0, 0, false}, {2, 9 * time.Minute, false}, {3, time.Minute, true},
		{0, 10 * time.Minute, true}, {0, 11 * time.Minute, true},
	} {
		if got := mediaWaitExhausted(tc.attempts, gtime.NewFromTime(now.Add(-tc.age)), now); got != tc.want {
			t.Fatalf("attempts=%d age=%s exhausted=%v", tc.attempts, tc.age, got)
		}
	}
}

// Real SQL validates aggregation, uploaded compatibility and read-only gates.
// Only temporary tables in a disposable local database are used.
func TestProfileMediaValidationLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MEDIA_VALIDATION_TEST_PG_PORT")
	if port == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	beforeDB, beforeCfg := gdb.GetConfig("default"), g.Cfg().GetAdapter()
	t.Cleanup(func() { gdb.SetConfigGroup("default", beforeDB); g.Cfg().SetAdapter(beforeCfg) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	adapter, err := gcfg.NewAdapterContent(`{"cache":{"adapter":"memory"}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cache.SetAdapter(ctx)
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, ddl := range []string{
			`CREATE TEMP TABLE hg_content_profile(id bigint,profile_no text,deleted_at timestamp)`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,expected_display_media_count int DEFAULT 0,expected_verify_media_count int DEFAULT 0,media_intent_status text DEFAULT '',publish_task_status text DEFAULT '',publish_operation_no text DEFAULT '',publish_task_updated_at timestamp,created_at timestamp,updated_at timestamp,deleted_at timestamp)`,
			`CREATE TEMP TABLE hg_youban_publish_media(id bigint,profile_id bigint,purpose text,processing_status text,file_url text,storage_path text,tg_file_id text,processed_tg_file_id text,deleted_at timestamp)`,
			`CREATE TEMP TABLE hg_youban_publish_profile_media_operation(profile_no text,tenant_id bigint,account_id bigint,status text,retry_count int,created_at timestamp)`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job(id bigint,profile_id bigint,tenant_id bigint,account_id bigint,status text,send_phase text,dispatch_status text,error_message text,last_dispatch_error text,next_retry_at timestamp,retry_count int,updated_at timestamp)`,
			`CREATE TEMP TABLE hg_youban_publish_note_index(profile_id bigint,tenant_id bigint,account_id bigint,task_status text,updated_at timestamp,deleted_at timestamp)`,
			`INSERT INTO hg_content_profile VALUES(1,'T00001',NULL)`,
			`INSERT INTO hg_youban_publish_profile_state(profile_id,tenant_id,account_id,created_at) VALUES(1,7,8,'2026-01-01')`,
			`INSERT INTO hg_youban_publish_note_index VALUES(1,7,8,'waiting_media',NULL,NULL)`,
			`INSERT INTO hg_youban_publish_media(id,profile_id,purpose,processing_status,tg_file_id) VALUES(1,1,'display','uploaded','cached'),(2,1,'display','uploaded','cached')`,
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		s := &sSysPublish{}
		ready, err := s.profileMediaReady(ctx, 1)
		if err != nil || !ready {
			t.Fatalf("uploaded media blocked: ready=%v err=%v", ready, err)
		}
		if _, err = tx.Exec(`DELETE FROM hg_youban_publish_media WHERE id=2`); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE hg_youban_publish_profile_state SET publish_task_status='waiting_media',publish_operation_no='profile:1',publish_task_updated_at=?`, time.Now().In(dbtime.Location())); err != nil {
			return err
		}
		ready, err = s.profileMediaReady(ctx, 1)
		if ready || !errors.Is(err, errProfileMediaValidation) || (!strings.Contains(err.Error(), "实际 1 个") || !strings.Contains(err.Error(), "要求大于 1 个")) {
			t.Fatalf("single display: %v %v", ready, err)
		}
		v, err := tx.GetValue(`SELECT publish_task_status FROM hg_youban_publish_profile_state`)
		if err != nil {
			return err
		}
		if v.String() != "waiting_media" {
			t.Fatal("readiness gate wrote state")
		}
		body, _ := json.Marshal(profileMaintenancePayload{ProfileId: 1, TenantId: 7, AccountId: 8, WakePublish: true})
		if err = s.handleProfileMaintenanceTask(ctx, asynq.NewTask(tgTaskTypeProfileMaintenance, body)); err != nil {
			return err
		}
		v, err = tx.GetValue(`SELECT publish_task_status FROM hg_youban_publish_profile_state`)
		if err != nil {
			return err
		}
		if v.String() != "failed" {
			t.Fatalf("maintenance did not settle: %s", v.String())
		}
		v, err = tx.GetValue(`SELECT task_status FROM hg_youban_publish_note_index`)
		if err != nil {
			return err
		}
		if v.String() != "failed" {
			t.Fatalf("index not settled: %s", v.String())
		}
		// A fresh Bot operation protects an empty/partial write; its age and retry count are bounded.
		if _, err = tx.Exec(`INSERT INTO hg_youban_publish_profile_media_operation VALUES('T00001',7,8,'processing',1,?)`, time.Now().In(dbtime.Location())); err != nil {
			return err
		}
		ready, err = s.profileMediaReady(ctx, 1)
		if ready || err != nil {
			t.Fatalf("in-progress Bot write killed: %v %v", ready, err)
		}
		if _, err = tx.Exec(`UPDATE hg_youban_publish_profile_media_operation SET retry_count=3`); err != nil {
			return err
		}
		_, err = s.profileMediaReady(ctx, 1)
		if !errors.Is(err, errProfileMediaValidation) {
			t.Fatalf("unbounded operation retries: %v", err)
		}
		if _, err = tx.Exec(`UPDATE hg_youban_publish_profile_media_operation SET retry_count=0,created_at='2026-01-01'`); err != nil {
			return err
		}
		_, err = s.profileMediaReady(ctx, 1)
		if !errors.Is(err, errProfileMediaValidation) {
			t.Fatalf("unbounded operation age: %v", err)
		}
		// Exact production CAS must not touch in-flight or unknown results.
		for i, status := range []string{telegramJobStatusPending, telegramJobStatusRetryWait, telegramJobStatusSending, telegramJobStatusConfirming} {
			if _, err = tx.Exec(`INSERT INTO hg_youban_publish_tg_job(id,status,send_phase) VALUES(?,?,'')`, i+1, status); err != nil {
				return err
			}
			changed, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{JobId: int64(i + 1), From: telegramJobRetryableStatuses(), To: telegramJobStatusFailed, SendPhases: []string{""}, ErrorMessage: "invalid"})
			if err != nil {
				return err
			}
			if changed != (i < 2) {
				t.Fatalf("CAS changed %s=%v", status, changed)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
