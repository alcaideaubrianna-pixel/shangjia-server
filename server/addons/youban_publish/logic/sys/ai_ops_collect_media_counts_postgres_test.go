package sys

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// Only a fixed disposable localhost PostgreSQL is accepted; never a remote DSN.
func TestAIOpsCollectMediaCountsPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires local PostgreSQL 127.0.0.1:35432")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", "host=127.0.0.1 port=35432 dbname=postgres sslmode=disable user="+os.Getenv("USER"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// Transaction-local temporary tables shadow production names only on this
	// fixed local connection. No schema changes survive the test.
	_, err = tx.ExecContext(ctx, `
CREATE TEMP TABLE hg_youban_publish_collect_event (
 id bigint PRIMARY KEY, material_parent_event_id bigint, material_role text
) ON COMMIT DROP;
CREATE TEMP TABLE hg_youban_publish_collect_event_media (
 id bigint PRIMARY KEY, event_id bigint
) ON COMMIT DROP;
INSERT INTO hg_youban_publish_collect_event VALUES
 (1,0,'display'),(2,1,'verify'),(3,1,'verify'),(4,1,'display'),
 (5,0,'verify'),(6,0,NULL),(7,0,'pending'),(8,8,'verify'),
 (10,0,'display'),(11,9,'verify');
INSERT INTO hg_youban_publish_collect_event_media VALUES
 (1,1),(2,1),(3,2),(4,2),(5,3),(6,4),(7,5),(8,6),(9,7),
 (10,8),(11,8),(12,9),(13,9),(14,11),(15,9999);
`)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name            string
		root            int64
		display, verify int
	}{
		{"multiple verify children and excluded display child", 1, 2, 3},
		{"verify root", 5, 0, 1},
		{"null role root", 6, 1, 0},
		{"non verify role root", 7, 1, 0},
		{"self parent counted once", 8, 0, 2},
		{"missing root event with media and child", 9, 2, 1},
		{"event without media", 10, 0, 0},
		{"missing event without media", 999, 0, 0},
	}
	const oldQuery = `SELECT
 COUNT(*) FILTER (WHERE COALESCE(e.material_role,'') <> 'verify'),
 COUNT(*) FILTER (WHERE e.material_role='verify')
FROM hg_youban_publish_collect_event_media em
LEFT JOIN hg_youban_publish_collect_event e ON e.id=em.event_id
WHERE em.event_id=$1 OR (e.material_parent_event_id=$1 AND e.material_role='verify')`
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query, args := aiOpsCollectMediaCountsSQL(tc.root)
			for i := range args {
				query = strings.Replace(query, "?", fmt.Sprintf("$%d", i+1), 1)
			}
			var display, verify, oldDisplay, oldVerify int
			if err := tx.QueryRowContext(ctx, query, args...).Scan(&display, &verify); err != nil {
				t.Fatal(err)
			}
			if err := tx.QueryRowContext(ctx, oldQuery, tc.root).Scan(&oldDisplay, &oldVerify); err != nil {
				t.Fatal(err)
			}
			if display != tc.display || verify != tc.verify {
				t.Fatalf("counts=(%d,%d), want=(%d,%d)", display, verify, tc.display, tc.verify)
			}
			if display != oldDisplay || verify != oldVerify {
				t.Fatalf("new counts=(%d,%d), old=(%d,%d)", display, verify, oldDisplay, oldVerify)
			}
		})
	}
}
