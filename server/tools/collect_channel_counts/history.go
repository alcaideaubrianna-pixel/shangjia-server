package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func auditCounts(db *sql.DB, tenant, account, rule int64, all bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	where := "true"
	if !all {
		where = fmt.Sprintf("tenant_id=%d AND account_id=%d", tenant, account)
	}
	ruleFilter := "true"
	if rule > 0 {
		ruleFilter = fmt.Sprintf("rule_id=%d", rule)
	}
	query := `WITH live AS (
 SELECT c.tenant_id,c.account_id,c.rule_id,ybp_collect_count_chat(c.channel_id) channel_id,c.profile_id
 FROM hg_youban_publish_collect_channel_profile c WHERE ` + strings.ReplaceAll(strings.ReplaceAll(where, "tenant_id", "c.tenant_id"), "account_id", "c.account_id") + `
 AND c.deleted_at IS NULL AND c.rule_id>0 AND c.profile_id>0
 AND EXISTS(SELECT 1 FROM hg_content_profile p JOIN hg_youban_publish_profile_state ps ON ps.profile_id=p.id
 WHERE p.id=c.profile_id AND p.deleted_at IS NULL AND ps.tenant_id=c.tenant_id AND ps.account_id=c.account_id AND ps.deleted_at IS NULL)
 ), facts AS (
 SELECT tenant_id,account_id,r.rule_id,channel_id,count(DISTINCT profile_id) expected FROM live
 CROSS JOIN LATERAL unnest(ARRAY[live.rule_id,0::bigint]) r(rule_id)
 GROUP BY tenant_id,account_id,r.rule_id,channel_id
 ), counters AS (SELECT * FROM hg_youban_publish_collect_channel_count WHERE ` + where + ` AND ` + ruleFilter + `)
 SELECT coalesce(f.tenant_id,s.tenant_id),coalesce(f.account_id,s.account_id),coalesce(f.rule_id,s.rule_id),coalesce(f.channel_id,s.channel_id),coalesce(f.expected,0),coalesce(s.profile_count,0),coalesce(s.ready,false)
 FROM (SELECT * FROM facts WHERE ` + ruleFilter + `) f FULL JOIN counters s USING(tenant_id,account_id,rule_id,channel_id)
 WHERE coalesce(f.expected,0)<>coalesce(s.profile_count,0) OR NOT coalesce(s.ready,false) LIMIT 20`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("audit failed: %w", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var t, a, r, want, got int64
		var c string
		var ready bool
		if err = rows.Scan(&t, &a, &r, &c, &want, &got, &ready); err != nil {
			return err
		}
		n++
		fmt.Printf("mismatch tenant=%d account=%d rule=%d channel=%s expected=%d actual=%d ready=%t\n", t, a, r, c, want, got, ready)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("audit mismatches=%d (maximum 20 shown); do not deploy the reader", n)
	}
	fmt.Println("audit mismatches=0")
	return nil
}

// Expensive evidence scans are opt-in, bounded and outside the HTTP path.
// Do not remove account/rule checks: message IDs and profiles are shared.
func reconcileHistory(db *sql.DB, tenant, account, rule int64, chat string, batch, maxBatches int, apply bool) error {
	chat = strings.TrimPrefix(strings.TrimSpace(chat), "-100")
	if chat == "" {
		return fmt.Errorf("channel is empty")
	}
	query := `WITH evidence AS (
 SELECT coalesce(nullif(d.source_id,0),e.source_id) source_id,d.profile_id FROM hg_youban_publish_collect_dispatch d
 LEFT JOIN hg_youban_publish_collect_event e ON e.id=d.event_id AND e.tenant_id=d.tenant_id AND e.account_id=d.account_id
 WHERE d.tenant_id=$1 AND d.account_id=$2 AND d.rule_id=$3
 AND coalesce(nullif(d.source_chat_id_snapshot,''),e.source_chat_id) IN($4,$5)
 UNION
 SELECT coalesce(nullif(d.source_id,0),e.source_id),j.profile_id FROM hg_youban_publish_tg_job j
 JOIN hg_youban_publish_collect_dispatch d ON d.event_id=j.collect_event_id AND d.profile_id=j.profile_id AND d.tenant_id=j.tenant_id AND d.account_id=j.account_id AND d.rule_id=$3
 LEFT JOIN hg_youban_publish_collect_event e ON e.id=d.event_id AND e.tenant_id=d.tenant_id AND e.account_id=d.account_id
 WHERE j.tenant_id=$1 AND j.account_id=$2 AND j.collect_source_chat_id IN($4,$5)
 UNION
 SELECT coalesce(nullif(d.source_id,0),e.source_id),o.profile_id FROM hg_youban_publish_profile_origin o
 JOIN hg_youban_publish_collect_dispatch d ON d.event_id=o.event_id AND d.profile_id=o.profile_id AND d.tenant_id=o.tenant_id AND d.account_id=o.account_id AND d.rule_id=$3
 LEFT JOIN hg_youban_publish_collect_event e ON e.id=d.event_id AND e.tenant_id=d.tenant_id AND e.account_id=d.account_id
 WHERE o.tenant_id=$1 AND o.account_id=$2 AND o.origin_kind='collect' AND o.chat_id IN($4,$5)
 ), candidates AS (
 SELECT DISTINCT ON(e.profile_id) e.source_id,e.profile_id FROM evidence e
 WHERE e.source_id>0 AND e.profile_id>0
 AND EXISTS(SELECT 1 FROM hg_content_profile p JOIN hg_youban_publish_profile_state ps ON ps.profile_id=p.id
 WHERE p.id=e.profile_id AND p.deleted_at IS NULL AND ps.tenant_id=$1 AND ps.account_id=$2 AND ps.deleted_at IS NULL)
 AND NOT EXISTS(SELECT 1 FROM hg_youban_publish_collect_channel_profile c WHERE c.tenant_id=$1 AND c.account_id=$2 AND c.rule_id=$3 AND c.channel_id IN($4,$5) AND c.profile_id=e.profile_id AND c.deleted_at IS NULL)
 ORDER BY e.profile_id,e.source_id LIMIT $6)
 `
	if !apply {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var n int
		if err := db.QueryRowContext(ctx, query+`SELECT count(*) FROM candidates`, tenant, account, rule, chat, "-100"+chat, batch).Scan(&n); err != nil {
			return fmt.Errorf("history preview failed: %w", err)
		}
		fmt.Printf("dry-run missing_relations_next_batch=%d channel=%s\n", n, chat)
		return nil
	}
	for i := 0; i < maxBatches; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result, err := db.ExecContext(ctx, query+`INSERT INTO hg_youban_publish_collect_channel_profile(tenant_id,account_id,source_id,rule_id,channel_id,profile_id)
 SELECT $1,$2,source_id,$3,$4,profile_id FROM candidates
 ON CONFLICT(tenant_id,account_id,rule_id,channel_id,profile_id) DO UPDATE SET deleted_at=NULL`, tenant, account, rule, chat, "-100"+chat, batch)
		cancel()
		if err != nil {
			return fmt.Errorf("history reconciliation failed: %w", err)
		}
		n, _ := result.RowsAffected()
		fmt.Printf("history batch=%d rows=%d channel=%s\n", i+1, n, chat)
		if n == 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}
