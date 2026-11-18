package sys

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCollectDispatchRetryRepairsEventAfterDispatchCommitLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	previousDB := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", previousDB)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres",
	}})
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_event(
				id bigint PRIMARY KEY,source_id bigint,status text,error_message text,processed_at timestamp,updated_at timestamp
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,event_id bigint,
				status text,error_message text,finished_at timestamp,updated_at timestamp
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch_channel(dispatch_id bigint,channel_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,collect_event_id bigint,
				channel_id bigint,operation_no text,status text,dispatch_status text
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(
				profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp
			) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_event VALUES
				(1,0,'dispatched','',NULL,NULL),(2,0,'dispatched','',NULL,NULL)`,
			`INSERT INTO hg_youban_publish_collect_dispatch VALUES
				(100,1,2,3,1,'sent','',CURRENT_TIMESTAMP,NULL),
				(101,1,2,3,2,'pending','',NULL,NULL)`,
			`INSERT INTO hg_youban_publish_collect_dispatch_channel VALUES(100,10),(101,11)`,
			`INSERT INTO hg_youban_publish_tg_job VALUES(1,1,2,3,1,10,'collect:100','sent','done')`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		s := new(sSysPublish)
		operation := telegramJobRecord{TenantId: 1, AccountId: 2, ProfileId: 3, OperationNo: "collect:100"}
		for i := 0; i < 2; i++ {
			if err := s.reconcileCollectOperationDispatch(ctx, operation); err != nil {
				return err
			}
			status, err := tx.Model(publishCollectEventTable).Where("id", 1).Value("status")
			if err != nil {
				return err
			}
			if status.String() != "processed" {
				t.Fatal("already-sent dispatch must still repair its event, including on repeated calls")
			}
		}
		operation.OperationNo = "collect:101"
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_tg_job VALUES(2,1,2,3,2,11,'collect:101','sent','done')`); err != nil {
			return err
		}
		operation.TenantId = 99
		if err := s.reconcileCollectOperationDispatch(ctx, operation); err != nil {
			return err
		}
		status, err := tx.Model(publishCollectDispatchTable).Where("id", 101).Value("status")
		if err != nil {
			return err
		}
		if status.String() != "pending" {
			t.Fatal("another tenant must not acknowledge an otherwise complete dispatch")
		}
		if _, err := tx.Exec(`DELETE FROM hg_youban_publish_tg_job WHERE id=2`); err != nil {
			return err
		}
		operation.TenantId = 1
		// A missing target Job, or a superseded target, is never sent proof.
		for i := 0; i < 2; i++ {
			if i == 1 {
				if _, err := tx.Exec(`INSERT INTO hg_youban_publish_tg_job VALUES(2,1,2,3,2,11,'collect:101','superseded','done')`); err != nil {
					return err
				}
			}
			if err := s.reconcileCollectOperationDispatch(ctx, operation); err != nil {
				return err
			}
			status, err := tx.Model(publishCollectDispatchTable).Where("id", 101).Value("status")
			if err != nil {
				return err
			}
			if status.String() != "pending" {
				t.Fatal("missing or superseded target must not acknowledge dispatch delivery")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
