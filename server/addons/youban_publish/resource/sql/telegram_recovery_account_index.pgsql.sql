-- 账户级恢复扫描索引。部分索引避免历史终态任务进入恢复排序。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_recovery_failed_display_account
    ON hg_youban_publish_tg_job (account_id, updated_at, id)
    WHERE status = 'failed'
      AND send_phase = 'display_confirmed'
      AND (last_dispatch_error IS NULL OR last_dispatch_error <> '展示后失败的历史任务已释放频道，禁止孤立补发验证；问题资料需整组重新上架');

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_recovery_missing_state_account
    ON hg_youban_publish_tg_job (account_id, id DESC)
    WHERE task_id IS NULL
      AND operation_no <> ''
      AND status IN ('pending', 'sending', 'failed_retry', 'unknown');

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_profile_state_recovery_account
    ON hg_youban_publish_profile_state (tenant_id, account_id, profile_id)
    WHERE deleted_at IS NULL AND publish_task_status = '';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_outbox_expired_account
    ON hg_youban_publish_tg_outbox (status, lease_expires_at, id, job_id)
    WHERE event_type = 'telegram_job.publish' AND status = 'processing';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_dispatch_recovery_account
    ON hg_youban_publish_tg_job (account_id, dispatch_status, status, dispatched_at, id)
    WHERE status IN ('pending','failed_retry','unknown')
      AND dispatch_status IN ('queued','processing');
