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
