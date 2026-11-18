package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"

	"hotgo/internal/dao"
)

// Reuse the explicit-opt-in, disposable loopback PostgreSQL fixture. These
// checks do not start workers, use Redis, or call Telegram.
func TestTelegramFallbackFullPushOperationScopePostgres(t *testing.T) {
	for _, tc := range []struct {
		name           string
		change         string
		cancelDisabled bool
		wantClosed     bool
	}{
		{name: "independent channel operation"},
		{name: "other channel completed", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status=''"},
		{name: "offline profile", change: "UPDATE " + dao.ContentProfile.Table() + " SET status=2,visibility='private'", wantClosed: true},
		{name: "deleted profile", change: "UPDATE " + dao.ContentProfile.Table() + " SET deleted_at=LOCALTIMESTAMP", wantClosed: true},
		{name: "unpublishing", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='unpublishing'", wantClosed: true},
		{name: "canceling", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='canceling'", wantClosed: true},
		{name: "canceled", change: "UPDATE " + publishProfileStateTable + " SET publish_task_status='canceled'", wantClosed: true},
		{name: "exact cancellation", change: "INSERT INTO " + publishProfileCancelTable + " (tenant_id,account_id,profile_id,operation_no,status) VALUES(1,2,3,'full_push:7:100:3','canceled')", wantClosed: true},
		{name: "new ordinary operation", change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='profile:new'", wantClosed: true},
		{name: "new batch on same channel", change: "INSERT INTO " + publishFullPushBatchTable + " VALUES(3,1,7,'full_push:7:102',3,'pending','1:7')", wantClosed: true},
		{name: "disabled retains operation comparison", cancelDisabled: true, wantClosed: true},
		{name: "disabled same operation", cancelDisabled: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='full_push:7:100:3'"},
		{name: "disabled canceled same operation", cancelDisabled: true, change: "UPDATE " + publishProfileStateTable + " SET publish_operation_no='full_push:7:100:3',publish_task_status='canceled'", wantClosed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, run := profileCancelPostgresFixture(t)
			if tc.cancelDisabled {
				t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "false")
			}
			err := run(func(ctx context.Context, tx gdb.TX) error {
				for _, query := range []string{
					"CREATE TABLE " + publishFullPushBatchTable + " (id bigint PRIMARY KEY,tenant_id bigint,channel_id bigint,batch_no text,snapshot_max_profile_id bigint,status text,active_key text UNIQUE)",
					"INSERT INTO " + publishFullPushBatchTable + " VALUES(1,1,7,'full_push:7:100',3,'completed',NULL),(2,1,8,'full_push:8:101',3,'dispatching','1:8')",
					"UPDATE " + publishProfileStateTable + " SET publish_operation_no='full_push:8:101:3',publish_task_status='publishing'",
					"UPDATE " + publishTgJobTable + " SET operation_no='full_push:7:100:3' WHERE id=10",
					"INSERT INTO " + telegramFallbackAccountTaskTable + " (id,tenant_id,task_key,status) VALUES(30,1,'fallback-scope-test','processing')",
				} {
					if _, err := tx.Exec(query); err != nil {
						return err
					}
				}
				if tc.change != "" {
					if _, err := tx.Exec(tc.change); err != nil {
						return err
					}
				}
				var job telegramJobRecord
				if err := tx.Model(publishTgJobTable).Where("id", 10).Scan(&job); err != nil {
					return err
				}
				// Match the handoff/callback order for accepted operations:
				// profile fence, AccountTask, Job, then fallback revalidation.
				if !tc.wantClosed {
					if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
						return err
					}
					if _, err := tx.Model(telegramFallbackAccountTaskTable).
						Where("id", 30).LockUpdate().One(); err != nil {
						return err
					}
					if err := tx.Model(publishTgJobTable).Where("id", job.Id).
						LockUpdate().Scan(&job); err != nil {
						return err
					}
				}
				err := checkTelegramFallbackProfileOperationTx(ctx, tx, job)
				if tc.wantClosed {
					if !errors.Is(err, errTelegramJobSuperseded) {
						return fmt.Errorf("expected superseded fallback, got %v", err)
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

func TestTelegramFallbackOrdinaryOperationComparisonPostgres(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		for _, tc := range []struct {
			name       string
			operation  string
			status     string
			wantClosed bool
		}{
			{name: "same operation", operation: "opaque:a:b", status: "publishing"},
			{name: "other operation", operation: "opaque:new", status: "publishing", wantClosed: true},
			{name: "empty operation", status: "publishing"},
			{name: "canceling same operation", operation: "opaque:a:b", status: "canceling", wantClosed: true},
			{name: "canceled same operation", operation: "opaque:a:b", status: "canceled", wantClosed: true},
		} {
			t.Run(fmt.Sprintf("cancel_enabled_%t/%s", enabled, tc.name), func(t *testing.T) {
				_, run := profileCancelPostgresFixture(t)
				t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", fmt.Sprint(enabled))
				err := run(func(ctx context.Context, tx gdb.TX) error {
					if _, err := tx.Exec("UPDATE "+publishProfileStateTable+" SET publish_operation_no=?,publish_task_status=?", tc.operation, tc.status); err != nil {
						return err
					}
					job := telegramJobRecord{Id: 10, TenantId: 1, AccountId: 2, ProfileId: 3, ChannelId: 7, OperationNo: "opaque:a:b"}
					err := checkTelegramFallbackProfileOperationTx(ctx, tx, job)
					if tc.wantClosed {
						if !errors.Is(err, errTelegramJobSuperseded) {
							return fmt.Errorf("expected legacy fallback rejection, got %v", err)
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
}
