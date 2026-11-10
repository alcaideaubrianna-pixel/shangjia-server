-- Explicit, bounded cutover compensation. Run after the schema migration and
-- before retiring the legacy worker. Repeat only while inserted > 0.
-- Uses the existing idx_ybp_media_processing_recovery_v2 partial index.
-- bootstrap delivery checks legacy Asynq ownership, including archived tasks.
INSERT INTO hg_youban_publish_media_process_outbox(media_id,bootstrap)
SELECT m.id,true FROM hg_youban_publish_media m
WHERE m.deleted_at IS NULL
 AND m.processing_status IN ('uploaded','retry','processing')
 AND ((m.processing_status IN ('uploaded','retry') AND m.updated_at<statement_timestamp()-interval '1 minute')
   OR (m.processing_status='processing' AND m.updated_at<statement_timestamp()-interval '11 minutes'))
 AND NOT EXISTS(SELECT 1 FROM hg_youban_publish_media_process_outbox o WHERE o.media_id=m.id)
ORDER BY m.updated_at,m.id LIMIT 1000
ON CONFLICT(media_id) DO NOTHING;
