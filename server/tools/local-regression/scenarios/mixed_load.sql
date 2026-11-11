-- 先执行phase=history建立500条积压，再执行phase=live插入100采集+100循环+20手工。
-- 所有输入都是本地夹具；不清理已有运行证据。任务由真实Outbox/Stream/Asynq和角色执行。
BEGIN;
CREATE TEMP TABLE mixed_new_jobs(id bigint PRIMARY KEY) ON COMMIT DROP;
WITH profiles AS (
 SELECT id,profile_no,title,row_number() OVER(ORDER BY id) n
 FROM hg_content_profile WHERE source_type='local_regression'
 ORDER BY id LIMIT 720
), channels AS (
 SELECT id,target_chat_id,row_number() OVER(ORDER BY id) n
 FROM hg_youban_publish_channel WHERE tenant_id=999 AND channel_title LIKE 'local-hook-channel-%' AND status=1 AND deleted_at IS NULL
 ORDER BY id LIMIT 10
), inserted AS (
 INSERT INTO hg_youban_publish_tg_job(tenant_id,account_id,profile_id,channel_id,target_chat_id,bot_id,operation_no,status,priority,dispatch_status,push_mode,profile_no_snapshot,title_snapshot,created_at,updated_at)
 SELECT 999,999,p.id,c.id,c.target_chat_id,
 (SELECT id FROM hg_youban_publish_bot WHERE tenant_id=999 AND bot_username='local_hook_bot' LIMIT 1),
 (CASE WHEN p.n<=500 THEN 'collect' WHEN p.n<=600 THEN 'cycle_batch' WHEN p.n<=700 THEN 'collect' ELSE 'profile' END)||':local:mix:'||:'run_id'||':'||p.n,
 'pending',CASE WHEN p.n<=500 THEN 95 WHEN p.n<=600 THEN 90 WHEN p.n<=700 THEN 10 ELSE 1 END,'idle','bot',p.profile_no,p.title,now(),now()
 FROM profiles p JOIN channels c ON c.n=(p.n-1)%10+1
 WHERE (:'phase'='history' AND p.n<=500) OR (:'phase'='live' AND p.n>500)
 RETURNING id
) INSERT INTO mixed_new_jobs SELECT id FROM inserted;
INSERT INTO hg_youban_publish_tg_outbox(job_id,event_type,dedupe_key,status,available_at,created_at,updated_at)
SELECT id,'telegram_job.publish','local-mix:'||id,'pending',now(),now(),now() FROM mixed_new_jobs;
UPDATE hg_youban_publish_profile_state state SET publish_operation_no=j.operation_no,publish_task_status='pending',publish_task_updated_at=now()
FROM hg_youban_publish_tg_job j JOIN mixed_new_jobs n ON n.id=j.id
WHERE state.tenant_id=999 AND state.account_id=999 AND state.profile_id=j.profile_id;
SELECT count(*) created_jobs FROM mixed_new_jobs;
COMMIT;
