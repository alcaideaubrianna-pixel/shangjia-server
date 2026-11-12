-- 显式部署执行，禁止放在实例启动路径。CONCURRENTLY不能放在事务内。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_channel_profile_active_job
ON hg_youban_publish_channel_profile (last_job_id)
WHERE status='active';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_profile_cycle_channel_job
ON hg_youban_publish_profile_cycle_run_channel (tenant_id,account_id,job_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_channel_account_batch_due
ON hg_youban_publish_channel (tenant_id,tg_account_id,cycle_next_run_at,id)
WHERE cycle_publish_enabled=1 AND cycle_publish_mode='batch' AND status=1 AND cycle_active_run_id=0 AND deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_profile_cycle_run_account_due
ON hg_youban_publish_profile_cycle_run (tenant_id,account_id,status,due_at,id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_account_cycle_due
ON hg_youban_publish_tg_job (tenant_id,account_id,next_cycle_at,id)
WHERE cycle_enabled=1 AND status IN ('sent','superseded') AND next_cycle_at IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_profile_state_account_publish_due
ON hg_youban_publish_profile_state (tenant_id,account_id,publish_at,id)
WHERE publish_at IS NOT NULL AND deleted_at IS NULL;
-- 恢复器按账户读取异常任务，避免全局扫描历史 TG 任务。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_failed_display_recovery
ON hg_youban_publish_tg_job (tenant_id,account_id,updated_at,id)
WHERE status='failed' AND send_phase='display_confirmed';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_missing_operation_state
ON hg_youban_publish_tg_job (tenant_id,account_id,id)
WHERE task_id IS NULL AND operation_no<>'' AND status IN ('pending','sending','failed_retry','unknown');
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_profile_state_missing_operation
ON hg_youban_publish_profile_state (tenant_id,account_id,profile_id)
WHERE deleted_at IS NULL AND publish_task_status='';
