-- Explicit reusable psql operation; one bounded PK page per invocation.
-- Required vars: tenant_id, channel_id, after_id, upper_id, batch_size.
-- Stage ALL pages for the selected scope before assigning ANY sequence values.
-- Stop scheduling/writers for this scope until assignment finishes.
-- This durable staging table is intentionally separate from runtime reads.
CREATE TABLE IF NOT EXISTS hg_ybp_batch_rotation_stage (
  mapping_id bigint PRIMARY KEY,
  tenant_id bigint NOT NULL,
  channel_id bigint NOT NULL,
  sent_at timestamp,
  last_job_id bigint NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ybp_batch_rotation_stage_order
ON hg_ybp_batch_rotation_stage (tenant_id,channel_id,sent_at NULLS FIRST,mapping_id);
BEGIN;
WITH page AS MATERIALIZED (
  SELECT id,tenant_id,channel_id,last_job_id
  FROM hg_youban_publish_channel_profile
  WHERE id > :after_id AND id <= :upper_id
  ORDER BY id LIMIT LEAST(GREATEST(:batch_size,1),1000)
)
INSERT INTO hg_ybp_batch_rotation_stage
SELECT cp.id,cp.tenant_id,cp.channel_id,j.sent_at,cp.last_job_id
FROM page cp
LEFT JOIN hg_youban_publish_tg_job j ON j.id=cp.last_job_id
WHERE cp.tenant_id=:tenant_id AND cp.channel_id=:channel_id
ON CONFLICT (mapping_id) DO NOTHING;
COMMIT;
-- The caller advances after_id using the same PK page, including rows belonging
-- to other scopes. Do not advance using only inserted staging rows.
SELECT id AS next_after_id FROM hg_youban_publish_channel_profile
WHERE id > :after_id AND id <= :upper_id
ORDER BY id LIMIT LEAST(GREATEST(:batch_size,1),1000);
