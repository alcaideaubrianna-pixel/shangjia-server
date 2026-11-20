CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybb_manager_task_notice_due
    ON hg_youban_bot_manager_task (updated_at, id)
    WHERE notified = 0 AND status IN ('success', 'failed');
