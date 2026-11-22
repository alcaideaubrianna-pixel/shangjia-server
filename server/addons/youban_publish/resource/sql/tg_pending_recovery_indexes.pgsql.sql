-- Run outside a transaction: CONCURRENTLY keeps publishing writes available.
-- Matches telegramPendingRecoveryPostgresSQL eligibility and DISTINCT ON order.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_pending_recovery_head
ON hg_youban_publish_tg_job (
    tenant_id, account_id, channel_id,
    (REPLACE(COALESCE(target_chat_id,''),'-100','')),
    created_at, id
)
INCLUDE (target_chat_id, bot_id, push_mode, operation_no, next_retry_at, updated_at)
WHERE status IN ('pending','failed_retry')
  AND COALESCE(dispatch_status,'') IN ('','idle')
  AND COALESCE(asynq_task_id,'')='';

-- Keep this predicate aligned with telegramChannelBlockingJobStateSQL.
-- Historical terminal phases must not enter this index.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_execution_blocker
ON hg_youban_publish_tg_job (
    tenant_id, account_id, channel_id,
    (REPLACE(COALESCE(target_chat_id,''),'-100',''))
)
WHERE COALESCE(status,'') NOT IN ('sent','failed','superseded','cancelled','deleted')
  AND (
      status IN ('sending','confirming','unknown','reconciling')
      OR dispatch_status='processing'
      OR send_phase IN (
          'display_sending','display_confirming',
          'verify_sending','verify_confirming',
          'cleanup_sending','cleanup_confirming','reconciling'
      )
  );

-- Supports the bounded pending-head timeout sweep. The query uses a keyset
-- cursor on updated_at/id and only considers jobs explicitly blocked by the
-- channel-order wait reason. It is intentionally separate from the channel
-- head index above because updated_at must be an index key for range scans.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_pending_head_timeout
ON hg_youban_publish_tg_job (updated_at, id)
INCLUDE (
    tenant_id,
    account_id,
    profile_id,
    channel_id,
    target_chat_id,
    operation_no,
    next_retry_at
)
WHERE status = 'pending'
  AND COALESCE(dispatch_status,'') IN ('','idle')
  AND COALESCE(asynq_task_id,'')=''
  AND COALESCE(send_phase,'')=''
  AND last_dispatch_error = '等待同频道前序推送任务完成';

-- IF NOT EXISTS does not repair an invalid index left by interrupted creation.
-- Inspect pg_index.indisvalid/indisready after execution before declaring success.
