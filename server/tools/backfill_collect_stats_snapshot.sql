-- 慢速回填采集统计快照。可重复执行；仅更新指定租户/账户范围。
-- 用法：psql "$DATABASE_URL" -v tenant_id=0 -v account_id=0 -f backfill_collect_stats_snapshot.sql
-- tenant_id/account_id 为 0 时回填全部账户。

WITH scopes AS (
    SELECT DISTINCT tenant_id, account_id
    FROM hg_youban_publish_collect_source
    WHERE deleted_at IS NULL
      AND account_id > 0
      AND (:tenant_id::bigint = 0 OR tenant_id = :tenant_id::bigint)
      AND (:account_id::bigint = 0 OR account_id = :account_id::bigint)
), source_stats AS (
    SELECT tenant_id, account_id,
           COUNT(*) FILTER (WHERE collect_enabled=1 AND status=1) AS collecting_count,
           COALESCE(SUM(event_total),0) AS event_count,
           COALESCE(SUM(blocked_total),0) AS blocked_count
    FROM hg_youban_publish_collect_source
    WHERE deleted_at IS NULL
      AND account_id > 0
      AND (:tenant_id::bigint = 0 OR tenant_id = :tenant_id::bigint)
      AND (:account_id::bigint = 0 OR account_id = :account_id::bigint)
    GROUP BY tenant_id, account_id
), rule_stats AS (
    SELECT tenant_id, account_id, COUNT(*) AS rule_count
    FROM hg_youban_publish_collect_rule
    WHERE deleted_at IS NULL
      AND (:tenant_id::bigint = 0 OR tenant_id = :tenant_id::bigint)
      AND (:account_id::bigint = 0 OR account_id = :account_id::bigint)
    GROUP BY tenant_id, account_id
), review_stats AS (
    SELECT tenant_id, account_id,
           COUNT(*) FILTER (WHERE status='pending') AS pending_review_count
    FROM hg_youban_publish_collect_review
    WHERE tenant_id IS NOT NULL
      AND (:tenant_id::bigint = 0 OR tenant_id = :tenant_id::bigint)
      AND (:account_id::bigint = 0 OR account_id = :account_id::bigint)
    GROUP BY tenant_id, account_id
), dispatch_stats AS (
    SELECT tenant_id, account_id,
           COUNT(*) FILTER (WHERE status='failed') AS failed_dispatch_count,
           COUNT(*) FILTER (WHERE status='sent') AS sent_count,
           COUNT(*) FILTER (WHERE status='sent' AND finished_at >= CURRENT_DATE) AS today_pushed_count
    FROM hg_youban_publish_collect_dispatch
    WHERE (:tenant_id::bigint = 0 OR tenant_id = :tenant_id::bigint)
      AND (:account_id::bigint = 0 OR account_id = :account_id::bigint)
    GROUP BY tenant_id, account_id
)
INSERT INTO hg_youban_publish_collect_stats_snapshot
    (tenant_id,account_id,collecting_count,rule_count,event_count,blocked_count,
     pending_review_count,failed_dispatch_count,sent_count,today_pushed_count,updated_at)
SELECT s.tenant_id,s.account_id,
       COALESCE(ss.collecting_count,0),COALESCE(rs.rule_count,0),
       COALESCE(ss.event_count,0),COALESCE(ss.blocked_count,0),
       COALESCE(rv.pending_review_count,0),COALESCE(ds.failed_dispatch_count,0),
       COALESCE(ds.sent_count,0),COALESCE(ds.today_pushed_count,0),CURRENT_TIMESTAMP
FROM scopes s
LEFT JOIN source_stats ss USING (tenant_id,account_id)
LEFT JOIN rule_stats rs USING (tenant_id,account_id)
LEFT JOIN review_stats rv USING (tenant_id,account_id)
LEFT JOIN dispatch_stats ds USING (tenant_id,account_id)
ON CONFLICT (tenant_id,account_id) DO UPDATE SET
 collecting_count=EXCLUDED.collecting_count,rule_count=EXCLUDED.rule_count,
 event_count=EXCLUDED.event_count,blocked_count=EXCLUDED.blocked_count,
 pending_review_count=EXCLUDED.pending_review_count,
 failed_dispatch_count=EXCLUDED.failed_dispatch_count,
 sent_count=EXCLUDED.sent_count,today_pushed_count=EXCLUDED.today_pushed_count,
 updated_at=EXCLUDED.updated_at;
