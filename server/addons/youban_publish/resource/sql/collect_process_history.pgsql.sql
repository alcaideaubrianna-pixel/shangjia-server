-- Run outside an explicit transaction; concurrent indexes preserve live writes.
-- Apply before deploying realtime collection routing. NULL denotes legacy work.
-- No existing task state, queue, or payload is reset.
ALTER TABLE hg_youban_publish_collect_process_outbox ADD COLUMN IF NOT EXISTS history BOOLEAN NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_process_outbox_fresh
 ON hg_youban_publish_collect_process_outbox (created_at,available_at,id)
 WHERE status='pending' AND COALESCE(history,FALSE)=FALSE;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_process_outbox_history
 ON hg_youban_publish_collect_process_outbox (available_at,id)
 WHERE status='pending' AND history=TRUE;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_process_outbox_history_status_updated
 ON hg_youban_publish_collect_process_outbox (history,status,updated_at,id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_source_stats_cover
 ON hg_youban_publish_collect_event (source_id)
 INCLUDE (status,material_role,received_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_window_ready_updated
 ON hg_youban_publish_collect_event
 (tenant_id,source_id,account_id,source_chat_id,updated_at,id)
 WHERE processed_at IS NULL
   AND status <> 'ignored'
   AND (material_role IS NULL OR material_role IN ('pending','display'))
   AND material_group_status IS DISTINCT FROM 'waiting_verify';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_window_ready_account_updated
 ON hg_youban_publish_collect_event
 (tenant_id,account_id,source_id,source_chat_id,updated_at,id)
 WHERE processed_at IS NULL
   AND status <> 'ignored'
   AND (material_role IS NULL OR material_role IN ('pending','display'))
   AND material_group_status IS DISTINCT FROM 'waiting_verify';
