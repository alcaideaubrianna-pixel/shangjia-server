-- 在独立 cycle_replay 数据库执行；不会触碰业务库。
TRUNCATE hg_youban_publish_profile_cycle_run_channel, hg_youban_publish_profile_cycle_run RESTART IDENTITY;

INSERT INTO hg_youban_publish_profile_cycle_run
    (tenant_id, account_id, profile_id, run_key, status, due_at, created_at, updated_at)
SELECT (g % 20) + 1,
       (g % 100) + 1,
       g,
       'replay-' || g,
       CASE WHEN g % 10 = 0 THEN 'completed' ELSE 'pending' END,
       CURRENT_TIMESTAMP + ((g % 60) || ' minutes')::interval,
       CURRENT_TIMESTAMP,
       CURRENT_TIMESTAMP
FROM generate_series(1, 10000) AS g;

INSERT INTO hg_youban_publish_profile_cycle_run_channel
    (run_id, tenant_id, account_id, profile_id, channel_id, status, created_at, updated_at)
SELECT r.id,
       r.tenant_id,
       r.account_id,
       r.profile_id,
       r.profile_id * 10 + channel_no,
       CASE WHEN channel_no = 1 AND r.profile_id % 10 = 0 THEN 'sent' ELSE 'pending' END,
       CURRENT_TIMESTAMP,
       CURRENT_TIMESTAMP
FROM hg_youban_publish_profile_cycle_run r
CROSS JOIN generate_series(1, 4) AS channel_no;

UPDATE hg_youban_publish_profile_cycle_run
SET due_at = CURRENT_TIMESTAMP
WHERE id <= 1000 AND status = 'pending';

ANALYZE hg_youban_publish_profile_cycle_run;
ANALYZE hg_youban_publish_profile_cycle_run_channel;

SELECT 'runs' AS kind, COUNT(*) AS count
FROM hg_youban_publish_profile_cycle_run
UNION ALL
SELECT 'channel_tasks', COUNT(*)
FROM hg_youban_publish_profile_cycle_run_channel;

EXPLAIN (ANALYZE, BUFFERS, TIMING OFF)
SELECT id, profile_id, due_at
FROM hg_youban_publish_profile_cycle_run
WHERE status = 'pending'
  AND due_at <= CURRENT_TIMESTAMP
ORDER BY due_at, id
LIMIT 500;
