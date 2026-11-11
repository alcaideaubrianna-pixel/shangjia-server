-- 上线前显式执行；不在应用启动期间运行DDL。不清空旧任务或旧计划。
ALTER TABLE hg_youban_publish_channel_profile ADD COLUMN IF NOT EXISTS next_cycle_at timestamp;
ALTER TABLE hg_youban_publish_channel_profile ADD COLUMN IF NOT EXISTS cycle_generation bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_publish_channel ADD COLUMN IF NOT EXISTS cycle_round_end_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_publish_cycle_run ADD COLUMN IF NOT EXISTS upper_profile_id bigint NOT NULL DEFAULT 0;
-- CONCURRENTLY必须在事务外执行；计划由账户级分页修复补齐，避免升级时全表回填。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_cp_account_due
ON hg_youban_publish_channel_profile(tenant_id,account_id,next_cycle_at,id)
WHERE status='active' AND next_cycle_at IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_cp_account_missing
ON hg_youban_publish_channel_profile(tenant_id,account_id,id)
WHERE status='active' AND next_cycle_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_job_account_repair
ON hg_youban_publish_tg_job(tenant_id,account_id,id)
WHERE status='sent';
