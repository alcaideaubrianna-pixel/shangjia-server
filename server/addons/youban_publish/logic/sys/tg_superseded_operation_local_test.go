package sys

import (
	"context"
	"os"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestSupersededOperationLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SUPERSEDED_LOCAL_PG") != "1" {
		t.Skip("isolated local PostgreSQL required")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "55489", User: "test", Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, q := range []string{
			`CREATE TEMP TABLE hg_youban_publish_profile_state(tenant_id bigint,account_id bigint,profile_id bigint,publish_operation_no text,publish_task_status text,publish_task_updated_at timestamp,updated_at timestamp,deleted_at timestamp)`,
			`CREATE TEMP TABLE hg_youban_publish_note_index(tenant_id bigint,account_id bigint,profile_id bigint,task_status text,updated_at timestamp,deleted_at timestamp)`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job(tenant_id bigint,account_id bigint,profile_id bigint,operation_no text,status text)`,
			`CREATE TEMP TABLE hg_content_profile(id bigint,status int,visibility text)`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		cases := []struct {
			name, operation, state, jobStatus string
			business                          int
			clear                             bool
		}{
			{"online", "old", "pending", "superseded", 1, true},
			{"offline", "old", "publishing", "superseded", 2, true},
			{"new_operation", "new", "pending", "superseded", 1, false},
			{"active", "old", "pending", "sending", 1, false},
			{"unpublishing", "old", "unpublishing", "superseded", 2, false},
			{"no_jobs", "old", "pending", "", 1, false},
		}
		for i, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				id := int64(i + 1)
				for _, q := range []struct {
					sql  string
					args []interface{}
				}{
					{`INSERT INTO hg_youban_publish_profile_state VALUES(10,435,?,?,?,NULL,NULL,NULL)`, []interface{}{id, c.operation, c.state}},
					{`INSERT INTO hg_youban_publish_note_index VALUES(10,435,?,?,NULL,NULL)`, []interface{}{id, c.state}},
					{`INSERT INTO hg_content_profile VALUES(?,?,'public')`, []interface{}{id, c.business}},
				} {
					if _, e := tx.Exec(q.sql, q.args...); e != nil {
						t.Fatal(e)
					}
				}
				if c.jobStatus != "" {
					if _, e := tx.Exec(`INSERT INTO hg_youban_publish_tg_job VALUES(10,435,?,'old',?)`, id, c.jobStatus); e != nil {
						t.Fatal(e)
					}
				}
				if e := new(sSysPublish).clearSupersededProfileOperationTx(ctx, tx, telegramJobRecord{TenantId: 10, AccountId: 435, ProfileId: id, OperationNo: "old"}); e != nil {
					t.Fatal(e)
				}
				row, e := tx.GetOne(`SELECT publish_operation_no,publish_task_status FROM hg_youban_publish_profile_state WHERE profile_id=?`, id)
				if e != nil {
					t.Fatal(e)
				}
				index, e := tx.GetValue(`SELECT task_status FROM hg_youban_publish_note_index WHERE profile_id=?`, id)
				if e != nil {
					t.Fatal(e)
				}
				wantOp, wantStatus := c.operation, c.state
				if c.clear {
					wantOp, wantStatus = "", ""
				}
				if row["publish_operation_no"].String() != wantOp || row["publish_task_status"].String() != wantStatus || index.String() != wantStatus {
					t.Fatalf("unexpected projections: %v index=%s", row, index.String())
				}
				business, e := tx.GetValue(`SELECT status FROM hg_content_profile WHERE id=?`, id)
				if e != nil {
					t.Fatal(e)
				}
				if business.Int() != c.business {
					t.Fatal("business status changed")
				}
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
