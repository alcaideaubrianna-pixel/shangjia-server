package sys

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// The actual single-row UPDATE RETURNING is exercised across separate database
// connections. A TEMP table would be invisible to the competing connections.
// Like the existing batch fixtures, this test runs in its own child process and
// accepts only the disposable loopback PostgreSQL; it never reads a DSN from ENV.
func TestRealtimeEventClaimLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT")
	if port == "" {
		t.Skip("requires dedicated loopback PostgreSQL 55489")
	}
	if port != "55489" {
		t.Fatal("requires dedicated loopback PostgreSQL 55489")
	}
	if !collectorBatchPostgresChild(t) {
		return
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55489", User: "postgres", Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if historyEventBatchEnabled(ctx) {
		t.Fatal("single realtime claim must not use history SKIP LOCKED")
	}
	isolation, err := g.DB().GetValue(ctx, `SHOW default_transaction_isolation`)
	if err != nil {
		t.Fatal(err)
	}
	if isolation.String() != "read committed" {
		t.Fatalf("fixture must use READ COMMITTED, got %s", isolation.String())
	}
	// Fail instead of deleting a table left by another fixture. Cleanup is
	// registered only after this test successfully creates its own table.
	if _, err = g.DB().Exec(ctx, `CREATE TABLE hg_tg_collector_event (
 id bigint PRIMARY KEY,tenant_id bigint,source_id bigint,source_type text,account_id bigint,
 status text,lease_owner text DEFAULT '',lease_until timestamp,attempt_count integer DEFAULT 0,
 updated_at timestamp,processed_at timestamp,next_run_at timestamp,error_message text,raw_update jsonb)`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancelCleanup := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelCleanup()
		if _, err := g.DB().Exec(cleanup, `DROP TABLE hg_tg_collector_event`); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
	}()
	if _, err = g.DB().Exec(ctx, `INSERT INTO hg_tg_collector_event(id,tenant_id,status,raw_update)
 VALUES (1,78,'received','{}'),(2,78,'received','{}'),(3,78,'ready','{}'),(4,78,'ignored','{}'),(5,78,'failed_retry','{}')`); err != nil {
		t.Fatal(err)
	}

	type claimResult struct {
		id      int64
		owner   string
		attempt int
		count   int
		err     error
	}
	claim := func(ctx context.Context, id int64, owner string) claimResult {
		rows, err := executeHistoryEventMutation(ctx, historyEventClaim, []historyEventMutation{{Kind: historyEventClaim, ID: id, Owner: owner}})
		result := claimResult{count: len(rows), err: err}
		if len(rows) == 1 {
			result.id = rows[0].Id
			result.owner = rows[0].LeaseOwner
			result.attempt = rows[0].AttemptCount
		}
		return result
	}
	lockRow := func(t *testing.T, ctx context.Context, id int64) gdb.TX {
		t.Helper()
		tx, err := g.DB().Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.GetOne(`SELECT id FROM hg_tg_collector_event WHERE id=? FOR UPDATE`, id); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		return tx
	}
	// Wait until PostgreSQL confirms the statements really reached a row lock.
	// This avoids a timing-only assertion passing because a goroutine never ran.
	waitForBlockedClaims := func(t *testing.T, ctx context.Context, want int) {
		t.Helper()
		deadline := time.NewTimer(3 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			rows, err := g.DB().GetAll(ctx, `SELECT pid FROM pg_stat_activity
 WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock'
 AND query LIKE '%WITH input(event_id,owner)%' LIMIT 3`)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) >= want {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-deadline.C:
				t.Fatalf("did not observe %d waiting claims", want)
			case <-tick.C:
			}
		}
	}
	receive := func(t *testing.T, ctx context.Context, ch <-chan claimResult) claimResult {
		t.Helper()
		select {
		case result := <-ch:
			if result.err != nil {
				t.Fatal(result.err)
			}
			return result
		case <-ctx.Done():
			t.Fatal(ctx.Err())
			return claimResult{}
		}
	}

	t.Run("same_id_waits_and_has_one_winner", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
		defer cancel()
		holder := lockRow(t, ctx, 1)
		defer holder.Rollback()
		results := make(chan claimResult, 2)
		for _, owner := range []string{"live-a", "live-b"} {
			go func(owner string) { results <- claim(ctx, 1, owner) }(owner)
		}
		waitForBlockedClaims(t, ctx, 2)
		select {
		case got := <-results:
			t.Fatalf("locked claim returned early (zero-row ACK risk): %+v", got)
		default:
		}
		if err := holder.Commit(); err != nil {
			t.Fatal(err)
		}
		first, second := receive(t, ctx, results), receive(t, ctx, results)
		if first.count+second.count != 1 {
			t.Fatalf("same snapshot competitors must have one winner: %+v %+v", first, second)
		}
		winner := first
		if winner.count == 0 {
			winner = second
		}
		if winner.id != 1 || winner.attempt != 1 || (winner.owner != "live-a" && winner.owner != "live-b") {
			t.Fatalf("bad returned ownership: %+v", winner)
		}
		row, err := g.DB().GetOne(ctx, `SELECT status,attempt_count,lease_owner FROM hg_tg_collector_event WHERE id=1`)
		if err != nil {
			t.Fatal(err)
		}
		if row["status"].String() != "processing" || row["attempt_count"].Int() != 1 || row["lease_owner"].String() != winner.owner {
			t.Fatalf("loser overwrote winner: %v", row)
		}
	})

	t.Run("locked_id_does_not_block_different_id", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
		defer cancel()
		holder := lockRow(t, ctx, 2)
		defer holder.Rollback()
		blocked := make(chan claimResult, 1)
		go func() { blocked <- claim(ctx, 2, "blocked-owner") }()
		waitForBlockedClaims(t, ctx, 1)
		independentCtx, cancelIndependent := context.WithTimeout(ctx, time.Second)
		defer cancelIndependent()
		got := claim(independentCtx, 5, "independent-owner")
		if got.err != nil || got.id != 5 || got.count != 1 || got.attempt != 1 {
			t.Fatalf("unrelated ID blocked: %+v", got)
		}
		select {
		case got := <-blocked:
			t.Fatalf("same-ID waiter returned before unlock: %+v", got)
		default:
		}
		if err := holder.Commit(); err != nil {
			t.Fatal(err)
		}
		got = receive(t, ctx, blocked)
		if got.id != 2 || got.count != 1 || got.owner != "blocked-owner" {
			t.Fatalf("waiter failed after unlock: %+v", got)
		}
	})

	t.Run("ready_and_ignored_cannot_be_reclaimed", func(t *testing.T) {
		for id, status := range map[int64]string{3: "ready", 4: "ignored"} {
			got := claim(ctx, id, fmt.Sprintf("forbidden-%d", id))
			if got.err != nil || got.count != 0 {
				t.Fatalf("%s reclaimed: %+v", status, got)
			}
			row, err := g.DB().GetOne(ctx, `SELECT status,attempt_count,lease_owner FROM hg_tg_collector_event WHERE id=?`, id)
			if err != nil {
				t.Fatal(err)
			}
			if row["status"].String() != status || row["attempt_count"].Int() != 0 || row["lease_owner"].String() != "" {
				t.Fatalf("terminal row changed: %v", row)
			}
		}
	})
}
