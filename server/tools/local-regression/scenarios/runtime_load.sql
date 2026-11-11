-- 本地新增一轮任务，不删除历史测试证据；仅账户999的local_regression资料和虚拟Bot频道。
-- 调用时必须传 run_id、profiles、channels；运行相同run_id不会重复创建。
\if :{?profile_offset}
\else
\set profile_offset 0
\endif
\if :{?channel_offset}
\else
\set channel_offset 0
\endif
BEGIN;
CREATE TEMP TABLE runtime_round_jobs(id bigint PRIMARY KEY) ON COMMIT DROP;
WITH profiles AS (
 SELECT id,profile_no,title FROM hg_content_profile
 WHERE source_type='local_regression' ORDER BY id OFFSET :profile_offset LIMIT :profiles
), selected_channels AS (
 SELECT id FROM hg_youban_publish_channel
 WHERE tenant_id=999 AND channel_title LIKE 'local-hook-channel-%' AND status=1 AND deleted_at IS NULL
 ORDER BY id OFFSET :channel_offset LIMIT :channels
), channels AS (
 -- 循环夹具也必须有有效循环配置，不能依赖旧版本缺少发送前拦截的漏洞。
 UPDATE hg_youban_publish_channel c SET cycle_publish_enabled=1,cycle_publish_mode='time',cycle_publish_days=30
 WHERE c.id IN(SELECT id FROM selected_channels) RETURNING c.id,c.target_chat_id
), inserted AS (
 INSERT INTO hg_youban_publish_tg_job(tenant_id,account_id,profile_id,channel_id,target_chat_id,bot_id,operation_no,status,priority,dispatch_status,cycle_enabled,cycle_days,push_mode,profile_no_snapshot,title_snapshot,created_at,updated_at)
 SELECT 999,999,p.id,c.id,c.target_chat_id,
 (SELECT id FROM hg_youban_publish_bot WHERE tenant_id=999 AND bot_username='local_hook_bot' LIMIT 1),
 'profile_cycle:local:runtime:'||:'run_id'||':'||p.id,
 'pending',90,'idle',1,1,'bot',p.profile_no,p.title,now(),now()
 FROM profiles p CROSS JOIN channels c
 WHERE NOT EXISTS (SELECT 1 FROM hg_youban_publish_tg_job j WHERE j.account_id=999 AND j.profile_id=p.id AND j.channel_id=c.id AND j.operation_no='profile_cycle:local:runtime:'||:'run_id'||':'||p.id)
 RETURNING id
) INSERT INTO runtime_round_jobs SELECT id FROM inserted;
INSERT INTO hg_youban_publish_tg_outbox(job_id,event_type,dedupe_key,status,available_at,created_at,updated_at)
SELECT id,'telegram_job.publish','local-runtime:'||id,'pending',now(),now(),now() FROM runtime_round_jobs;
-- 与真实创建操作一致，记录当前资料操作，验证汇总投影而非只验证job发送。
UPDATE hg_youban_publish_profile_state state SET
 publish_operation_no='profile_cycle:local:runtime:'||:'run_id'||':'||state.profile_id,
 publish_task_status='pending',publish_task_updated_at=now()
WHERE state.tenant_id=999 AND state.account_id=999
 AND EXISTS (SELECT 1 FROM runtime_round_jobs r JOIN hg_youban_publish_tg_job j ON j.id=r.id WHERE j.profile_id=state.profile_id);
SELECT count(*) AS created_jobs FROM runtime_round_jobs;
-- 故障用例：模拟PG已记录通知投递成功，但Redis丢失了通知；事务内注入，避免与消费者竞态。
\if :{?simulate_ready_loss}
UPDATE hg_youban_publish_tg_outbox SET status='delivered',delivered_at=now()-interval '1 minute'
WHERE job_id IN (SELECT id FROM runtime_round_jobs);
UPDATE hg_youban_publish_tg_job SET updated_at=now()-interval '1 minute'
WHERE id IN (SELECT id FROM runtime_round_jobs);
\endif
COMMIT;
