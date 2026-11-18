-- psql vars: tenant_id, channel_id, batch_size. Invoke until assigned=0.
-- Stage the COMPLETE fixed PK range first. Unknown sent_at rows are an explicit
-- deterministic fallback, NOT evidence of actual historical success order.
BEGIN;
WITH page AS MATERIALIZED (
  SELECT s.mapping_id,s.sent_at,s.last_job_id
  FROM hg_ybp_batch_rotation_stage s
  WHERE s.tenant_id=:tenant_id AND s.channel_id=:channel_id
  ORDER BY s.sent_at NULLS FIRST,s.mapping_id
  LIMIT LEAST(GREATEST(:batch_size,1),1000)
  FOR UPDATE
), ordered AS MATERIALIZED (
  SELECT mapping_id,last_job_id,nextval('hg_ybp_batch_order_seq') AS new_order
  FROM page ORDER BY sent_at NULLS FIRST,mapping_id
), assigned AS (
  UPDATE hg_youban_publish_channel_profile cp
  SET batch_order=ordered.new_order,
      batch_last_sent_job_id=GREATEST(cp.batch_last_sent_job_id,ordered.last_job_id)
  FROM ordered
  WHERE cp.id=ordered.mapping_id AND cp.batch_order=0
    AND cp.last_job_id=ordered.last_job_id
  RETURNING cp.id
), removed AS (
  DELETE FROM hg_ybp_batch_rotation_stage s USING page
  WHERE s.mapping_id=page.mapping_id RETURNING s.mapping_id
)
SELECT (SELECT count(*) FROM assigned) AS assigned,
       (SELECT count(*) FROM removed) AS consumed,
       (SELECT count(*) FROM page p
        JOIN hg_youban_publish_channel_profile cp ON cp.id=p.mapping_id
        WHERE cp.batch_order=0 AND cp.last_job_id<>p.last_job_id) AS cas_skipped;
COMMIT;
