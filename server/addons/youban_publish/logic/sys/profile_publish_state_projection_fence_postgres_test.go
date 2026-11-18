package sys

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"

	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
)

// Opt-in, disposable loopback PostgreSQL only; never use an application DSN.
// These tests exercise the transaction body without starting Redis or workers.
func profileProjectionFenceLocalDB(t *testing.T) gdb.DB {
	t.Helper()
	if os.Getenv("YOUBAN_PROFILE_PROJECTION_FENCE_TEST") != "1" {
		t.Skip("requires disposable PostgreSQL on 127.0.0.1:35432")
	}
	t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "true")
	if cache.Initialized() {
		previous := cache.Instance().GetAdapter()
		cache.Instance().SetAdapter(gcache.NewAdapterMemory())
		t.Cleanup(func() { cache.Instance().SetAdapter(previous) })
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}
	root, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	schema := fmt.Sprintf("profile_projection_fence_%d", time.Now().UnixNano())
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	db := g.DB()
	for _, query := range []string{
		`CREATE TABLE ` + publishProfileStateTable + ` (
			tenant_id bigint,account_id bigint,profile_id bigint,
			publish_operation_no text,publish_task_status text,
			publish_task_updated_at timestamp,updated_at timestamp,deleted_at timestamp,
			PRIMARY KEY (tenant_id,account_id,profile_id))`,
		`CREATE TABLE ` + publishProfileCancelTable + ` (
			tenant_id bigint,account_id bigint,profile_id bigint,operation_no text)`,
		`CREATE TABLE ` + dao.ContentProfile.Table() + ` (
			id bigint PRIMARY KEY,status integer,visibility text,published_at timestamp,
			updated_at timestamp,deleted_at timestamp)`,
		`CREATE TABLE ` + publishNoteIndexTable + ` (
			tenant_id bigint,account_id bigint,profile_id bigint,task_status text,
			updated_at timestamp,deleted_at timestamp)`,
		`INSERT INTO ` + publishProfileStateTable + `
			VALUES (1,1,10,'profile:old','publishing',LOCALTIMESTAMP,LOCALTIMESTAMP,NULL)`,
		`INSERT INTO ` + dao.ContentProfile.Table() + ` (id,status,visibility) VALUES (10,2,'private')`,
		`INSERT INTO ` + publishNoteIndexTable + ` (tenant_id,account_id,profile_id,task_status)
			VALUES (1,1,10,'publishing')`,
	} {
		if _, err := db.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func profileProjectionFenceJob() telegramJobRecord {
	return telegramJobRecord{TenantId: 1, AccountId: 1, ProfileId: 10, OperationNo: "profile:old"}
}

func TestProfileProjectionRecoveryFencePostgres(t *testing.T) {
	for _, tc := range []struct {
		name       string
		change     string
		operation  string
		wantOp     string
		wantStatus string
		completed  bool
		public     bool
		wantError  bool
		cancelOff  bool
	}{
		{name: "current operation", wantOp: "profile:old", completed: true, public: true},
		{name: "current pending operation", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='pending'",
			wantOp: "profile:old", completed: true, public: true},
		{name: "empty operation state", change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='',publish_task_status=''"},
		{name: "new takedown cleared same operation", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status=''",
			wantOp: "profile:old"},
		{name: "new operation", change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:new'",
			wantOp: "profile:new", wantStatus: "publishing"},
		{name: "canceling", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='canceling'",
			wantOp: "profile:old", wantStatus: "canceling"},
		{name: "canceled", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='canceled'",
			wantOp: "profile:old", wantStatus: "canceled"},
		{name: "failed", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='failed'",
			wantOp: "profile:old", wantStatus: "failed"},
		{name: "tombstone with stale active projection", change: "INSERT INTO " + publishProfileCancelTable + " VALUES (1,1,10,'profile:old')",
			wantOp: "profile:old", wantStatus: "publishing"},
		{name: "deleted projection", change: "UPDATE " + publishProfileStateTable + " SET deleted_at=LOCALTIMESTAMP",
			wantOp: "profile:old", wantStatus: "publishing"},
		{name: "fence remains when cancel disabled", change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:new'",
			wantOp: "profile:new", wantStatus: "publishing", cancelOff: true},
		{name: "cleared state when cancel disabled", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status=''",
			wantOp: "profile:old", cancelOff: true},
		{name: "cycle preserves business visibility", change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile_cycle:old'",
			operation: "profile_cycle:old", wantOp: "profile_cycle:old", completed: true},
		{name: "clear failure rolls back public write", change: "ALTER TABLE " + publishNoteIndexTable + " ADD CONSTRAINT reject_clear CHECK (task_status <> '')",
			wantOp: "profile:old", wantStatus: "publishing", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := profileProjectionFenceLocalDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if tc.cancelOff {
				t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "false")
			}
			if tc.change != "" {
				if _, err := db.Exec(ctx, tc.change); err != nil {
					t.Fatal(err)
				}
			}
			job := profileProjectionFenceJob()
			if tc.operation != "" {
				job.OperationNo = tc.operation
			}
			var completed bool
			err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				var err error
				completed, err = (&sSysPublish{}).recoverCompletedProfilePublishOperationTx(ctx, tx, job)
				return err
			})
			if (err != nil) != tc.wantError || completed != tc.completed {
				t.Fatalf("completed=%v err=%v; want completed=%v error=%v", completed, err, tc.completed, tc.wantError)
			}
			row, err := db.GetOne(ctx, `SELECT p.status,p.visibility,p.published_at,
				ps.publish_operation_no,ps.publish_task_status,n.task_status
				FROM `+dao.ContentProfile.Table()+` p
				JOIN `+publishProfileStateTable+` ps ON ps.profile_id=p.id
				JOIN `+publishNoteIndexTable+` n ON n.profile_id=p.id WHERE p.id=10`)
			if err != nil {
				t.Fatal(err)
			}
			if row["publish_operation_no"].String() != tc.wantOp || row["publish_task_status"].String() != tc.wantStatus {
				t.Fatalf("operation state changed incorrectly: %v", row)
			}
			if tc.public {
				if row["status"].Int() != 1 || row["visibility"].String() != "public" || row["published_at"].IsNil() {
					t.Fatalf("current completed operation was not published: %v", row)
				}
			} else if row["status"].Int() != 2 || row["visibility"].String() != "private" || !row["published_at"].IsNil() {
				t.Fatalf("stale/canceled/cycle/rolled-back operation changed business visibility: %v", row)
			}
			wantIndex := "publishing"
			if tc.completed {
				wantIndex = ""
			}
			if row["task_status"].String() != wantIndex {
				t.Fatalf("unexpected note-index task status: %v", row)
			}
		})
	}
}

func TestProfileProjectionRecoveryWaitsForTakedownCommitPostgres(t *testing.T) {
	db := profileProjectionFenceLocalDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	job := profileProjectionFenceJob() // snapshot captured before the takedown
	if _, err := db.Exec(ctx, "UPDATE "+dao.ContentProfile.Table()+" SET status=1,visibility='public' WHERE id=10"); err != nil {
		t.Fatal(err)
	}
	down, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = down.Rollback() }()
	if _, err := down.Model(publishProfileStateTable).Where("profile_id", job.ProfileId).
		LockUpdate().One(); err != nil {
		t.Fatal(err)
	}
	if _, err := down.Exec(`UPDATE ` + publishProfileStateTable + ` SET publish_task_status='' WHERE profile_id=10`); err != nil {
		t.Fatal(err)
	}
	if _, err := down.Exec("UPDATE " + dao.ContentProfile.Table() + " SET status=2,visibility='private' WHERE id=10"); err != nil {
		t.Fatal(err)
	}
	type recoveryResult struct {
		completed bool
		err       error
	}
	started := make(chan int, 1)
	finished := make(chan recoveryResult, 1)
	go func() {
		var completed bool
		err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			row, err := tx.GetOne("SELECT pg_backend_pid() AS pid")
			if err != nil {
				return err
			}
			started <- row["pid"].Int()
			completed, err = (&sSysPublish{}).recoverCompletedProfilePublishOperationTx(ctx, tx, job)
			return err
		})
		finished <- recoveryResult{completed, err}
	}()
	defer func() {
		cancel()
		_ = down.Rollback()
		// Join the recovery before the disposable schema/config is removed.
		<-finished
	}()
	var pid int
	select {
	case pid = <-started:
	case result := <-finished:
		finished <- result
		t.Fatalf("recovery did not start: %v", result.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Observe a real PostgreSQL lock wait, rather than relying on a sleep to
	// guess whether the recovery crossed its current-operation check.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
waiting:
	for {
		select {
		case result := <-finished:
			finished <- result
			t.Fatalf("recovery bypassed the held operation fence: %+v", result)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
			row, err := db.GetOne(ctx, "SELECT wait_event_type FROM pg_stat_activity WHERE pid=?", pid)
			if err != nil {
				t.Fatal(err)
			}
			if row["wait_event_type"].String() == "Lock" {
				break waiting
			}
		}
	}
	if err := down.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-finished:
		finished <- result
		if result.err != nil || result.completed {
			t.Fatalf("old completion survived the newer takedown: %+v", result)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	row, err := db.GetOne(ctx, "SELECT status,visibility FROM "+dao.ContentProfile.Table()+" WHERE id=10")
	if err != nil || row["status"].Int() != 2 || row["visibility"].String() != "private" {
		t.Fatalf("recovery republished after takedown: row=%v err=%v", row, err)
	}
}

func TestProfileProjectionRecoveryMissingStatePostgres(t *testing.T) {
	db := profileProjectionFenceLocalDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.Exec(ctx, "DELETE FROM "+publishProfileStateTable); err != nil {
		t.Fatal(err)
	}
	var completed bool
	err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var err error
		completed, err = (&sSysPublish{}).recoverCompletedProfilePublishOperationTx(ctx, tx, profileProjectionFenceJob())
		return err
	})
	if err != nil || completed {
		t.Fatalf("missing operation must not complete: completed=%v err=%v", completed, err)
	}
	row, err := db.GetOne(ctx, "SELECT status,visibility,published_at FROM "+dao.ContentProfile.Table()+" WHERE id=10")
	if err != nil || row["status"].Int() != 2 || row["visibility"].String() != "private" || !row["published_at"].IsNil() {
		t.Fatalf("missing operation changed business visibility: row=%v err=%v", row, err)
	}
}
