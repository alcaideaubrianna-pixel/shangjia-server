-- Explicit prerequisite to deploying batch-rotation writers. No startup DDL.
BEGIN;
CREATE SEQUENCE IF NOT EXISTS hg_ybp_batch_order_seq AS bigint START WITH 1 CACHE 1;
ALTER TABLE hg_youban_publish_channel_profile
  ADD COLUMN IF NOT EXISTS batch_order bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS batch_last_sent_job_id bigint NOT NULL DEFAULT 0;
COMMENT ON COLUMN hg_youban_publish_channel_profile.batch_order IS
  'Rotation position, advanced on snapshot reservation and successful send; zero is unmigrated, NOT historical publication order';
COMMENT ON COLUMN hg_youban_publish_channel_profile.batch_last_sent_job_id IS
  'Monotonic successful job watermark; duplicate/older callbacks cannot move queue position';
COMMIT;
-- Never reset/setval from MAX(table). Re-running this migration preserves sequence.
