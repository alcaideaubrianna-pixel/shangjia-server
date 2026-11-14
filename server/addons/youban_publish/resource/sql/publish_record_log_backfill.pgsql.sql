-- Run repeatedly in small batches. The cursor is the last processed success_record.id.
-- Replace :last_id and :batch_size in the calling tool (defaults: 0 and 500).
WITH batch AS MATERIALIZED (
    SELECT r.*
    FROM hg_youban_publish_success_record r
    WHERE r.id > :last_id
    ORDER BY r.id
    LIMIT :batch_size
), snapshot AS (
    SELECT b.id AS source_id,
           b.job_id,b.task_id,b.tenant_id,b.account_id,b.profile_id,b.channel_id,b.bot_id,
           b.operation_no,b.target_chat_id,b.action,b.status,b.message,b.created_at,
           COALESCE(NULLIF(p.title,''),'') AS title,
           COALESCE(NULLIF(a.nickname,''),NULLIF(a.username,''),'') AS account_name,
           COALESCE(NULLIF(owner.username,''),NULLIF(t.name,''),'') AS tenant_name,
           COALESCE(NULLIF(c.channel_title,''),'') AS channel_title,
           COALESCE(NULLIF(c.channel_username,''),'') AS channel_username,
           COALESCE(NULLIF(bot.bot_name,''),'') AS bot_name,
           COALESCE(NULLIF(bot.bot_username,''),'') AS bot_username,
           COALESCE(j.retry_count,0) AS retry_count
    FROM batch b
    LEFT JOIN hg_youban_publish_tg_job j ON j.id=b.job_id
    LEFT JOIN hg_content_profile p ON p.id=b.profile_id
    LEFT JOIN hg_youban_publish_account a ON a.id=b.account_id AND a.deleted_at IS NULL
    LEFT JOIN hg_youban_publish_tenant t ON t.id=b.tenant_id AND t.deleted_at IS NULL
    LEFT JOIN hg_youban_publish_account owner ON owner.tenant_id=b.tenant_id AND owner.account_type='admin' AND owner.deleted_at IS NULL
    LEFT JOIN hg_youban_publish_channel c ON c.id=COALESCE(NULLIF(b.channel_id,0),j.channel_id)
    LEFT JOIN hg_youban_publish_bot bot ON bot.id=b.bot_id AND bot.deleted_at IS NULL
)
INSERT INTO hg_youban_publish_record_log
(job_id,task_id,tenant_id,account_id,profile_id,channel_id,bot_id,operation_no,target_chat_id,action,
 title,account_name,tenant_name,channel_title,channel_username,bot_name,bot_username,status,message,retry_count,created_at,updated_at)
SELECT job_id,task_id,tenant_id,account_id,profile_id,channel_id,bot_id,operation_no,target_chat_id,action,
       title,account_name,tenant_name,channel_title,channel_username,bot_name,bot_username,status,
       LEFT(COALESCE(message,''),255),retry_count,created_at,created_at
FROM snapshot
ON CONFLICT (job_id) DO NOTHING;
