-- Explicit deployment step after 20261118_profile_cycle_override.pgsql.sql.
-- Prerequisite: ../batch_rotation.pgsql.sql has added batch_order
-- and created hg_ybp_batch_order_seq; saves use that sequence when disabling.
-- Run standalone with autocommit, outside any migration-runner transaction.
-- Keep the existing batch indexes; never execute this DDL from startup.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_batch_inherited_rotation
  ON hg_youban_publish_channel_profile (tenant_id, channel_id, status, batch_order, profile_id)
  WHERE status = 'active' AND batch_order > 0 AND cycle_override_enabled = false;
