-- 本地专用压力夹具：不连接线上，不调用 Telegram。
-- 生成 1000 资料、每资料 2 个媒体、3 个频道映射，以及不同优先级的推送任务。
BEGIN;
-- 仅清理本地夹具上一次运行的数据，保证脚本可重复执行。
DELETE FROM hg_youban_publish_tg_outbox WHERE job_id IN (SELECT id FROM hg_youban_publish_tg_job WHERE account_id=999 AND operation_no LIKE '%:local:%');
DELETE FROM hg_youban_publish_tg_job WHERE account_id=999 AND operation_no LIKE '%:local:%';
DELETE FROM hg_youban_publish_profile_channel WHERE account_id=999 AND profile_id IN (SELECT id FROM hg_content_profile WHERE source_type='local_regression');
DELETE FROM hg_youban_publish_profile_state WHERE account_id=999 AND profile_id IN (SELECT id FROM hg_content_profile WHERE source_type='local_regression');
DELETE FROM hg_youban_publish_media WHERE account_id=999 AND profile_id IN (SELECT id FROM hg_content_profile WHERE source_type='local_regression');
DELETE FROM hg_content_profile WHERE source_type='local_regression';
CREATE TEMP TABLE local_seed_profiles(profile_id bigint PRIMARY KEY, profile_no text, title text) ON COMMIT DROP;
INSERT INTO local_seed_profiles(profile_id,profile_no,title)
SELECT nextval('hg_content_profile_id_seq'),'LOCAL'||lpad(g::text,6,'0'),'local-regression-'||g
FROM generate_series(1,1000) g;
INSERT INTO hg_content_profile(id,profile_no,source_type,title,plain_text,image_count,visibility,status,created_at,updated_at)
SELECT profile_id,profile_no,'local_regression',title,'本地压力测试资料',
       CASE WHEN profile_id%10=0 THEN 0 ELSE 2 END,'public',1,now(),now()
FROM local_seed_profiles;
INSERT INTO hg_youban_publish_profile_state(tenant_id,account_id,profile_id,expected_display_media_count,expected_verify_media_count,media_intent_status,created_at,updated_at)
SELECT 999,999,profile_id,CASE WHEN profile_id%10=0 THEN 0 ELSE 2 END,0,'ready',now(),now()
FROM local_seed_profiles;
INSERT INTO hg_youban_publish_media(tenant_id,account_id,profile_id,media_type,purpose,name,file_url,original_file_url,storage_path,original_storage_path,tg_file_id,tg_cache_status,processing_status,sort_index,status,created_at,updated_at)
SELECT 999,999,p.profile_id,'image','display','local-display-'||p.profile_id||'-'||m.n,
       '/tmp/youban-testdata/image.jpg','/tmp/youban-testdata/image.jpg','/tmp/youban-testdata/image.jpg','/tmp/youban-testdata/image.jpg',
       'local-file-'||p.profile_id||'-'||m.n,'valid','ready',m.n,1,now(),now()
FROM local_seed_profiles p CROSS JOIN generate_series(1,2) m(n)
WHERE p.profile_id%10<>0;
INSERT INTO hg_youban_publish_profile_channel(tenant_id,account_id,profile_id,channel_id,is_manual,created_at,updated_at)
SELECT 999,999,p.profile_id,c.channel_id,0,now(),now()
FROM local_seed_profiles p CROSS JOIN generate_series(1,100) c(channel_id);
INSERT INTO hg_youban_publish_tg_job(tenant_id,account_id,profile_id,channel_id,target_chat_id,operation_no,status,priority,dispatch_status,cycle_enabled,cycle_days,push_mode,profile_no_snapshot,title_snapshot,created_at,updated_at)
SELECT 999,999,p.profile_id,c.channel_id,'-100LOCAL'||c.channel_id,
       CASE WHEN p.profile_id%20=0 THEN 'manual:local:'||p.profile_id WHEN p.profile_id%3=0 THEN 'collect:local:'||p.profile_id ELSE 'profile_cycle:local:'||p.profile_id END,
       'pending',CASE WHEN p.profile_id%20=0 THEN 1 WHEN p.profile_id%3=0 THEN 10 ELSE 90 END,'idle',1,1,'bot',p.profile_no,p.title,now(),now()
FROM local_seed_profiles p CROSS JOIN generate_series(1,100) c(channel_id);
INSERT INTO hg_youban_publish_tg_outbox(job_id,event_type,dedupe_key,status,available_at,created_at,updated_at)
SELECT j.id,'telegram_job.publish','local-regression:'||j.id,'pending',now(),now(),now()
FROM hg_youban_publish_tg_job j WHERE j.operation_no LIKE '%:local:%' AND j.created_at>=now()-interval '1 minute';
COMMIT;
SELECT count(*) profiles FROM hg_content_profile WHERE source_type='local_regression';
SELECT count(*) media FROM hg_youban_publish_media WHERE account_id=999;
SELECT count(*) jobs FROM hg_youban_publish_tg_job WHERE account_id=999;
SELECT count(*) outbox FROM hg_youban_publish_tg_outbox WHERE dedupe_key LIKE 'local-regression:%';
