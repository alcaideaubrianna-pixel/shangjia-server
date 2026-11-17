package sys

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	pdao "hotgo/addons/youban_publish/internal/dao"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// The default GoFrame driver is process-cached. Never replace it in the parent
// test process, and never accept a DSN or host inherited from the environment.
func TestCollectSourceQueueCancelBotAccountZeroLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT")
	if port == "" {
		t.Skip("requires dedicated loopback PostgreSQL 127.0.0.1:55489")
	}
	if port != "55489" {
		t.Fatal("only dedicated loopback PostgreSQL port 55489 is allowed")
	}
	const childEnv = "YOUBAN_COLLECT_SOURCE_QUEUE_CANCEL_PG_CHILD"
	if os.Getenv(childEnv) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.count=1")
		cmd.Env = append(os.Environ(), childEnv+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated source cancellation regression: %v\n%s", err, output)
		}
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55489", User: "postgres", Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	historyTable := pdao.YoubanPublishCollectHistoryTask.Table()
	sourceTable := pdao.YoubanPublishCollectSource.Table()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Five session-local tables shadow any same-named permanent tables. The
		// source row documents the owner already checked by the outer caller.
		for _, ddl := range []string{
			"CREATE TEMP TABLE hg_tg_collector_event(id bigint PRIMARY KEY,tenant_id bigint,source_id bigint,account_id bigint,status text,error_message text,lease_owner text,lease_until timestamp,next_run_at timestamp,processed_at timestamp,updated_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_tg_collector_delivery(id bigint PRIMARY KEY,event_id bigint,tenant_id bigint,source_id bigint,account_id bigint,status text,error_message text,lease_owner text,lease_until timestamp,next_run_at timestamp,updated_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_tg_collector_account_task(id bigint PRIMARY KEY,history_task_id bigint,account_id bigint,status text,error_message text,updated_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE " + historyTable + "(id bigint PRIMARY KEY,tenant_id bigint,source_id bigint,account_id bigint,status text) ON COMMIT DROP",
			"CREATE TEMP TABLE " + sourceTable + "(id bigint,tenant_id bigint,account_id bigint,PRIMARY KEY(tenant_id,id)) ON COMMIT DROP",
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		for _, seed := range []string{
			"INSERT INTO " + sourceTable + " VALUES(351,78,615),(1370,78,615),(351,79,615)",
			"INSERT INTO " + historyTable + " VALUES(10,78,351,615,'pending'),(11,78,351,616,'pending'),(12,78,1370,615,'pending'),(13,79,351,615,'pending')",
			"INSERT INTO hg_tg_collector_event(id,tenant_id,source_id,account_id,status,error_message,lease_owner,lease_until,next_run_at) VALUES(1,78,351,0,'received','unchanged','original-owner',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '1 hour'),(2,78,351,22,'processing','unchanged','original-owner',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '1 hour'),(3,78,1370,0,'received','unchanged','original-owner',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '1 hour'),(4,79,351,0,'ready','unchanged','original-owner',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '1 hour'),(5,78,351,0,'processed','unchanged','original-owner',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '1 hour')",
			"INSERT INTO hg_tg_collector_delivery(id,event_id,tenant_id,source_id,account_id,status,error_message,lease_owner,lease_until,next_run_at) VALUES(101,1,78,351,0,'pending','unchanged','original-owner',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '1 hour'),(102,2,78,351,22,'processing','unchanged','original-owner',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '1 hour'),(103,3,78,1370,0,'pending','unchanged','original-owner',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '1 hour'),(104,4,79,351,0,'failed_retry','unchanged','original-owner',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '1 hour'),(105,5,78,351,0,'delivered','unchanged','original-owner',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '1 hour')",
			"INSERT INTO hg_tg_collector_account_task VALUES(20,10,615,'pending','unchanged',NULL),(21,11,616,'pending','unchanged',NULL),(22,12,615,'pending','unchanged',NULL),(23,13,615,'processing','unchanged',NULL),(24,10,615,'succeeded','unchanged',NULL)",
		} {
			if _, err := tx.Exec(seed); err != nil {
				return err
			}
		}
		if err := cancelCollectorSourceTasks(ctx, 351, 78, 615, gtime.Now()); err != nil {
			return err
		}
		for _, check := range []struct {
			table   string
			id      int64
			status  string
			changed bool
		}{
			{"hg_tg_collector_event", 1, "dead", true},
			{"hg_tg_collector_event", 2, "dead", true},
			{"hg_tg_collector_event", 3, "received", false},
			{"hg_tg_collector_event", 4, "ready", false},
			{"hg_tg_collector_event", 5, "processed", false},
			{"hg_tg_collector_delivery", 101, "dead", true},
			{"hg_tg_collector_delivery", 102, "dead", true},
			{"hg_tg_collector_delivery", 103, "pending", false},
			{"hg_tg_collector_delivery", 104, "failed_retry", false},
			{"hg_tg_collector_delivery", 105, "delivered", false},
			{"hg_tg_collector_account_task", 20, "cancelled", true},
			{"hg_tg_collector_account_task", 21, "pending", false},
			{"hg_tg_collector_account_task", 22, "pending", false},
			{"hg_tg_collector_account_task", 23, "processing", false},
			{"hg_tg_collector_account_task", 24, "succeeded", false},
		} {
			row, err := tx.GetOne("SELECT * FROM "+check.table+" WHERE id=?", check.id)
			if err != nil {
				return err
			}
			if row.IsEmpty() {
				t.Errorf("%s id=%d missing", check.table, check.id)
				continue
			}
			if got := row["status"].String(); got != check.status {
				t.Errorf("%s id=%d status=%q want %q", check.table, check.id, got, check.status)
			}
			if check.changed {
				if row["error_message"].String() != collectSourceCanceledMessage || row["updated_at"].IsNil() {
					t.Errorf("%s id=%d missing durable cancellation projection", check.table, check.id)
				}
				if check.table != "hg_tg_collector_account_task" {
					if row["lease_owner"].String() != "" || !row["lease_until"].IsNil() || !row["next_run_at"].IsNil() {
						t.Errorf("%s id=%d retained execution lease/schedule", check.table, check.id)
					}
					if check.table == "hg_tg_collector_event" && row["processed_at"].IsNil() {
						t.Errorf("raw id=%d missing processed_at", check.id)
					}
				}
			} else if row["error_message"].String() != "unchanged" || !row["updated_at"].IsNil() {
				t.Errorf("%s id=%d unrelated/terminal row was modified", check.table, check.id)
			}
		}
		// History ownership is only used to scope account-task cancellation; this
		// helper does not cancel another owner's tasks or rewrite history rows.
		for _, table := range []string{historyTable, sourceTable} {
			count, err := tx.GetValue("SELECT COUNT(*) FROM " + table)
			if err != nil {
				return err
			}
			want := int64(4)
			if table == sourceTable {
				want = 3
			}
			if count.Int64() != want {
				t.Errorf("fixture scope table changed: %s", table)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
