-- Run standalone with psql autocommit. Never use -1 / BEGIN / migration TX.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_batch_rotation
ON hg_youban_publish_channel_profile
  (tenant_id,channel_id,status,batch_order,profile_id)
WHERE status='active' AND batch_order>0;
