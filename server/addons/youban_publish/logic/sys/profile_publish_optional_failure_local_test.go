package sys

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCompleteProfileOperationWithoutFailedStateLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	t.Setenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED", "true")
	previousDB := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", previousDB)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres",
	}})
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_youban_publish_profile_state(
				profile_id bigint,tenant_id bigint,account_id bigint,publish_operation_no text,
				publish_task_status text,deleted_at timestamp
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job(
				tenant_id bigint,account_id bigint,profile_id bigint,operation_no text
			) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_profile_state VALUES(3,1,2,'collect:100','pending',NULL)`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		// No failed-state row is normal. With no jobs yet, completion is a
		// harmless false/nil, rather than leaking sql.ErrNoRows to projection.
		completed, err := new(sSysPublish).completeProfileTelegramOperation(ctx, telegramJobRecord{
			Id: 1, TenantId: 1, AccountId: 2, ProfileId: 3, OperationNo: "collect:100",
		}, false)
		if err != nil {
			return err
		}
		if completed {
			t.Fatal("an operation with no jobs cannot be completed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
