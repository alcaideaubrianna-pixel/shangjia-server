// Command collect_channel_counts backfills the transactional channel projection.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/lib/pq"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	apply := flag.Bool("apply", false, "apply bounded batches; default is dry-run")
	all := flag.Bool("all", false, "explicitly permit all tenant/account scopes")
	tenant := flag.Int64("tenant", 0, "tenant scope")
	account := flag.Int64("account", 0, "account scope")
	rule := flag.Int64("rule", 0, "optional rule scope")
	audit := flag.Bool("audit", false, "read-only compare summaries to live owned profile relations")
	history := flag.Bool("reconcile-history", false, "repair missing relations from legacy evidence for one channel")
	channel := flag.String("channel", "", "explicit Telegram channel for historical reconciliation")
	batch := flag.Int("batch", 200, "rows per transaction, maximum 1000")
	maxBatches := flag.Int("max-batches", 10, "maximum transactions per invocation")
	flag.Parse()
	if *batch < 1 || *batch > 1000 || *maxBatches < 1 || *maxBatches > 10000 {
		return fmt.Errorf("invalid batch bounds")
	}
	if !*all && (*tenant <= 0 || *account <= 0) {
		return fmt.Errorf("specify tenant/account or explicitly pass --all")
	}
	if *all && (*tenant != 0 || *account != 0 || *rule != 0) {
		return fmt.Errorf("--all cannot be combined with a scoped selection")
	}
	if *rule < 0 {
		return fmt.Errorf("rule must not be negative")
	}
	if *history && (*all || *rule <= 0 || *channel == "" || *audit) {
		return fmt.Errorf("historical reconciliation requires tenant/account/rule/channel and cannot use --all or --audit")
	}
	dsn := os.Getenv("YOUBAN_COLLECT_COUNT_DSN")
	if dsn == "" {
		return fmt.Errorf("YOUBAN_COLLECT_COUNT_DSN is required; credentials are never printed")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open database failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if *audit {
		return auditCounts(db, *tenant, *account, *rule, *all)
	}
	if *history {
		return reconcileHistory(db, *tenant, *account, *rule, *channel, *batch, *maxBatches, *apply)
	}
	where := "NOT count_projected"
	if !*all {
		where += fmt.Sprintf(" AND tenant_id=%d AND account_id=%d", *tenant, *account)
	}
	if *rule > 0 {
		where += fmt.Sprintf(" AND rule_id=%d", *rule)
	}
	query := `WITH settings AS MATERIALIZED (SELECT set_config('youban.collect_count_backfill','on',true)), candidates AS (
 SELECT c.id FROM hg_youban_publish_collect_channel_profile c CROSS JOIN settings WHERE ` + where + `
 ORDER BY tenant_id,account_id,ybp_collect_count_chat(channel_id),rule_id,id LIMIT $1 FOR UPDATE OF c SKIP LOCKED
 ) UPDATE hg_youban_publish_collect_channel_profile c SET count_projected=true
 FROM candidates x WHERE c.id=x.id RETURNING c.id`
	if !*apply {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var n int
		if err = db.QueryRowContext(ctx, "SELECT count(*) FROM (SELECT id FROM hg_youban_publish_collect_channel_profile WHERE "+where+" LIMIT $1) c", *batch).Scan(&n); err != nil {
			return fmt.Errorf("dry-run query failed: %w", err)
		}
		fmt.Printf("dry-run next_batch=%d maximum_rows=%d\n", n, *batch**maxBatches)
		return nil
	}
	total := 0
	for i := 0; i < *maxBatches; i++ {
		var n int
		for attempt := 0; attempt < 3; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			rows, queryErr := db.QueryContext(ctx, query, *batch)
			n = 0
			if queryErr == nil {
				for rows.Next() {
					var id int64
					if queryErr = rows.Scan(&id); queryErr != nil {
						break
					}
					n++
				}
				if queryErr == nil {
					queryErr = rows.Err()
				}
				rows.Close()
			}
			cancel()
			if queryErr == nil {
				err = nil
				break
			}
			err = queryErr
			if e, ok := queryErr.(*pq.Error); !ok || (e.Code != "40P01" && e.Code != "40001") {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("backfill batch failed (previous batches remain committed): %w", err)
		}
		total += n
		fmt.Printf("batch=%d rows=%d total=%d\n", i+1, n, total)
		if n == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var completed int
	if err = db.QueryRowContext(ctx, "SELECT ybp_collect_count_finish_backfill()").Scan(&completed); err != nil {
		return fmt.Errorf("finalize backfill failed: %w", err)
	}
	fmt.Printf("completed_scopes=%d\n", completed)
	return nil
}
