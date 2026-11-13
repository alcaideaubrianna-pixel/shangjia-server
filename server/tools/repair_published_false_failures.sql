WITH candidates AS (
 SELECT ps.ctid,ps.tenant_id,ps.account_id,ps.profile_id FROM hg_youban_publish_profile_state ps
 WHERE ps.publish_task_status='failed' AND EXISTS (
  SELECT 1 FROM hg_youban_publish_tg_job j WHERE j.tenant_id=ps.tenant_id AND j.account_id=ps.account_id AND j.profile_id=ps.profile_id AND j.status='sent'
 ) LIMIT :batch_size
)
UPDATE hg_youban_publish_profile_state ps SET publish_task_status='published',publish_task_updated_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP FROM candidates c WHERE ps.ctid=c.ctid;

WITH candidates AS (
 SELECT n.ctid FROM hg_youban_publish_note_index n JOIN hg_youban_publish_profile_state ps ON ps.tenant_id=n.tenant_id AND ps.account_id=n.account_id AND ps.profile_id=n.profile_id
 WHERE n.task_status='failed' AND ps.publish_task_status='published' LIMIT :batch_size
)
UPDATE hg_youban_publish_note_index n SET task_status='published',updated_at=CURRENT_TIMESTAMP FROM candidates c WHERE n.ctid=c.ctid;
