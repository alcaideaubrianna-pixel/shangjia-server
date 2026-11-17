package sys

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

// Explicit opt-in, disposable loopback PostgreSQL only. All fixture tables are
// connection-local TEMP tables dropped at commit; no production DSN is read.
func TestCollectDispatchHandoffFailureOrderingPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port,
		User: "test", Name: "postgres",
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_event (
				id bigint PRIMARY KEY, status text, error_message text,
				processed_at timestamp, updated_at timestamp, deleted_at timestamp, source_id bigint DEFAULT 0
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch (
				id bigint PRIMARY KEY, event_id bigint, status text,
				updated_at timestamp, deleted_at timestamp, error_message text DEFAULT ''
			) ON COMMIT DROP`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		service := &sSysPublish{}
		reset := func(t *testing.T) {
			t.Helper()
			for _, sql := range []string{
				`DELETE FROM hg_youban_publish_collect_dispatch`,
				`DELETE FROM hg_youban_publish_collect_event`,
				`INSERT INTO hg_youban_publish_collect_event(id,status,error_message,processed_at,updated_at,deleted_at) VALUES (1,'media_ready','',NULL,now(),NULL)`,
				`INSERT INTO hg_youban_publish_collect_dispatch(id,event_id,status,updated_at,deleted_at) VALUES (10,1,'pending',now(),NULL)`,
			} {
				if _, err := tx.Exec(sql); err != nil {
					t.Fatal(err)
				}
			}
		}
		assertParent := func(t *testing.T, status, message string, processed bool) {
			t.Helper()
			row, err := tx.Model("hg_youban_publish_collect_event").Where("id", 1).One()
			if err != nil {
				t.Fatal(err)
			}
			if row["status"].String() != status || row["error_message"].String() != message ||
				(row["processed_at"].GTime() != nil) != processed {
				t.Fatalf("parent status/message/processed mismatch: %v", row)
			}
		}
		callback := func(t *testing.T) {
			t.Helper()
			// Match production ordering: persist dispatch failure, then project
			// it through the exact parent-update helper used by the callback.
			if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_dispatch SET status='failed',error_message='未找到原消息' WHERE id=10`); err != nil {
				t.Fatal(err)
			}
			if err := service.reopenCollectEventAfterDispatchFailure(ctx, 1, "未找到原消息"); err != nil {
				t.Fatal(err)
			}
		}
		assertRetry := func(t *testing.T) {
			t.Helper()
			err := service.finishCollectEventDispatchHandoff(ctx, 1)
			var retry *collectProcessRetryError
			if !errors.As(err, &retry) {
				t.Fatalf("unfinished branch must return retry, got %v", err)
			}
		}
		assertFailure := func(t *testing.T) {
			t.Helper()
			err := service.finishCollectEventDispatchHandoff(ctx, 1)
			var retry *collectProcessRetryError
			if err == nil || errors.As(err, &retry) {
				t.Fatalf("hard dispatch failure must return a normal error, got %v", err)
			}
			if err.Error() != "未找到原消息" {
				t.Fatalf("original dispatch failure was lost: %v", err)
			}
			row, readErr := tx.Model("hg_youban_publish_collect_event").Where("id", 1).One()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if shouldRecoverCollectEvent(row) {
				t.Fatal("hard TG failure bypassed the existing recovery policy")
			}
		}
		t.Run("callback_before_finalize", func(t *testing.T) {
			reset(t)
			callback(t)
			assertFailure(t)
			assertParent(t, sysin.CollectEventStatusFailed, "未找到原消息", true)
		})
		t.Run("callback_after_finalize", func(t *testing.T) {
			reset(t)
			if err := service.finishCollectEventDispatchHandoff(ctx, 1); err != nil {
				t.Fatal(err)
			}
			assertParent(t, sysin.CollectEventStatusDispatched, "", false)
			callback(t)
			assertParent(t, sysin.CollectEventStatusFailed, "未找到原消息", true)
			assertFailure(t)
			assertParent(t, sysin.CollectEventStatusFailed, "未找到原消息", true)
		})
		t.Run("failed_dispatch_visible_before_parent_callback", func(t *testing.T) {
			reset(t)
			if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_dispatch SET status='failed',error_message='未找到原消息' WHERE id=10`); err != nil {
				t.Fatal(err)
			}
			assertFailure(t)
			assertParent(t, sysin.CollectEventStatusFailed, "未找到原消息", true)
		})
		t.Run("missing_dispatch_is_not_admission", func(t *testing.T) {
			reset(t)
			if _, err := tx.Exec(`DELETE FROM hg_youban_publish_collect_dispatch`); err != nil {
				t.Fatal(err)
			}
			assertRetry(t)
			assertParent(t, sysin.CollectEventStatusMediaReady, "", false)
		})
		t.Run("all_fast_sent_before_finalize_completes_parent", func(t *testing.T) {
			reset(t)
			if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_dispatch SET status='sent';
				INSERT INTO hg_youban_publish_collect_dispatch(id,event_id,status,updated_at,deleted_at) VALUES (11,1,'sent',now(),NULL)`); err != nil {
				t.Fatal(err)
			}
			assertParent(t, sysin.CollectEventStatusMediaReady, "", false)
			if err := service.finishCollectEventDispatchHandoff(ctx, 1); err != nil {
				t.Fatal(err)
			}
			assertParent(t, sysin.CollectEventStatusProcessed, "", true)
		})
		t.Run("sent_and_skipped_before_finalize_completes_parent", func(t *testing.T) {
			reset(t)
			if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_dispatch SET status='sent';
				INSERT INTO hg_youban_publish_collect_dispatch(id,event_id,status,updated_at,deleted_at) VALUES (11,1,'skipped',now(),NULL)`); err != nil {
				t.Fatal(err)
			}
			if err := service.finishCollectEventDispatchHandoff(ctx, 1); err != nil {
				t.Fatal(err)
			}
			assertParent(t, sysin.CollectEventStatusProcessed, "", true)
		})
		t.Run("one_sent_one_pending_is_only_dispatched", func(t *testing.T) {
			reset(t)
			if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_dispatch SET status='sent';
				INSERT INTO hg_youban_publish_collect_dispatch(id,event_id,status,updated_at,deleted_at) VALUES (11,1,'pending',now(),NULL)`); err != nil {
				t.Fatal(err)
			}
			if err := service.finishCollectEventDispatchHandoff(ctx, 1); err != nil {
				t.Fatal(err)
			}
			assertParent(t, sysin.CollectEventStatusDispatched, "", false)
		})
		t.Run("sent_and_failed_preserves_terminal_failure", func(t *testing.T) {
			reset(t)
			if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_dispatch SET status='sent';
				INSERT INTO hg_youban_publish_collect_dispatch(id,event_id,status,updated_at,deleted_at,error_message)
				VALUES (11,1,'failed',now(),NULL,'未找到原消息')`); err != nil {
				t.Fatal(err)
			}
			assertFailure(t)
			assertParent(t, sysin.CollectEventStatusFailed, "未找到原消息", true)
		})
		t.Run("completed_callback_is_not_regressed", func(t *testing.T) {
			reset(t)
			if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_dispatch SET status='sent';
				UPDATE hg_youban_publish_collect_event SET status='processed',processed_at=now()`); err != nil {
				t.Fatal(err)
			}
			if err := service.finishCollectEventDispatchHandoff(ctx, 1); err != nil {
				t.Fatal(err)
			}
			assertParent(t, sysin.CollectEventStatusProcessed, "", true)
			callback(t)
			assertParent(t, sysin.CollectEventStatusFailed, "未找到原消息", true)
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
