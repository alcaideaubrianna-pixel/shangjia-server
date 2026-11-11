-- 本地 Hook 发送依赖：虚拟 Bot + 100 个 Bot 频道，不连接真实 Telegram。
-- Gateway必须作为真实安装插件启动，否则只有服务注册、没有Webhook路由和更新消费者。
INSERT INTO hg_sys_addons_install(name,version,status)
SELECT 'youban_tg_bot_gateway','v1.0.0',1
WHERE NOT EXISTS (SELECT 1 FROM hg_sys_addons_install WHERE name='youban_tg_bot_gateway');
-- 真实 scheduler 只执行后台启用的定时任务；回放库必须带上循环调度任务。
INSERT INTO hg_sys_cron(group_id,title,name,params,pattern,policy,count,sort,status,created_at,updated_at)
SELECT 1,'本地循环调度','youbanPublishCycleScheduler','','@every 1m',2,0,21,1,now(),now()
WHERE NOT EXISTS (SELECT 1 FROM hg_sys_cron WHERE name='youbanPublishCycleScheduler');
UPDATE hg_sys_cron
SET pattern='@every 1m',status=1,updated_at=now()
WHERE name='youbanPublishCycleScheduler';
-- Stream 模式按到期 Outbox 读取，补充本地压力测试索引，避免大批量夹具触发全表扫描。
CREATE INDEX IF NOT EXISTS idx_local_tg_outbox_stream_ready
    ON hg_youban_publish_tg_outbox(event_type,status,available_at,id,job_id);
ANALYZE hg_youban_publish_tg_outbox;
ANALYZE hg_youban_publish_tg_job;
-- 账户调度夹具：统一 Stream 模式的循环调度按账户目录入队。
-- 没有账户及其 push_engine 配置时，scheduler 虽然正常执行，但不会创建账户调度任务。
INSERT INTO hg_youban_publish_account
    (id,tenant_id,merchant_id,parent_id,account_type,nickname,username,status,created_at,updated_at,last_active_at,deleted_at)
VALUES
    (999,999,0,0,'uploader','local-regression-account','local_regression_999',1,now(),now(),now(),NULL)
ON CONFLICT (id) DO UPDATE SET
    tenant_id=EXCLUDED.tenant_id,
    status=1,
    deleted_at=NULL,
    last_active_at=now(),
    updated_at=now();
INSERT INTO hg_youban_publish_account_setting
    (tenant_id,account_id,cycle_publish_enabled,cycle_publish_days,cycle_publish_time,push_engine,created_at,updated_at,deleted_at)
VALUES
    (999,999,1,1,'00:00','unified_stream',now(),now(),NULL)
ON CONFLICT (tenant_id,account_id) WHERE deleted_at IS NULL DO UPDATE SET
    cycle_publish_enabled=1,
    cycle_publish_days=1,
    push_engine='unified_stream',
    deleted_at=NULL,
    updated_at=now();
INSERT INTO hg_youban_publish_bot(tenant_id,bot_name,bot_username,bot_token,status,created_at,updated_at)
SELECT 999,'local-hook-bot','local_hook_bot','LOCAL:TEST:TOKEN',1,now(),now()
WHERE NOT EXISTS (SELECT 1 FROM hg_youban_publish_bot WHERE tenant_id=999 AND bot_username='local_hook_bot');
INSERT INTO hg_youban_publish_channel(tenant_id,tg_account_id,channel_title,target_chat_id,publish_direction,status,bot_id_json,created_at,updated_at)
SELECT 999,0,'local-hook-channel-'||g,'-100999'||lpad(g::text,6,'0'),'up',1,'[1]',now(),now()
FROM generate_series(1,100) g
WHERE NOT EXISTS (SELECT 1 FROM hg_youban_publish_channel c WHERE c.tenant_id=999 AND c.target_chat_id='-100999'||lpad(g::text,6,'0'));
UPDATE hg_youban_publish_tg_job
SET bot_id=(SELECT id FROM hg_youban_publish_bot WHERE tenant_id=999 AND bot_username='local_hook_bot' LIMIT 1),
    -- 先移到临时负数，避免把逻辑频道编号替换成真实主键时触发唯一索引冲突。
    channel_id=-id,
    target_chat_id='-100999'||lpad((channel_id%100+1)::text,6,'0'), updated_at=now()
WHERE account_id=999;
-- 压力任务使用虚拟 300/301/302 分组；映射到真实插入的频道主键，避免被频道存在性校验跳过。
UPDATE hg_youban_publish_tg_job j
SET channel_id=c.id, updated_at=now()
FROM hg_youban_publish_channel c
WHERE j.account_id=999 AND c.tenant_id=999 AND c.target_chat_id=j.target_chat_id;
DELETE FROM hg_youban_publish_profile_channel WHERE account_id=999;
INSERT INTO hg_youban_publish_profile_channel
    (tenant_id,account_id,profile_id,channel_id,is_manual,created_at,updated_at)
SELECT 999,999,p.id,c.id,0,now(),now()
FROM hg_content_profile p
CROSS JOIN generate_series(1,100) logical_channel
JOIN hg_youban_publish_channel c
  ON c.tenant_id=999
 AND c.target_chat_id='-100999'||lpad(logical_channel::text,6,'0')
WHERE p.source_type='local_regression'
  -- 基础映射只保留少量循环基线；大规模资料由压力场景按需创建，
  -- 避免账户修复器在每轮调度扫描十万条历史夹具。
  AND p.id <= 20;
UPDATE hg_youban_publish_tg_outbox o SET updated_at=now()
FROM hg_youban_publish_tg_job j WHERE j.id=o.job_id AND j.account_id=999;
SELECT count(*) bots FROM hg_youban_publish_bot WHERE tenant_id=999;
SELECT count(*) channels FROM hg_youban_publish_channel WHERE tenant_id=999;
SELECT count(*) jobs_ready FROM hg_youban_publish_tg_job WHERE account_id=999 AND bot_id>0;
