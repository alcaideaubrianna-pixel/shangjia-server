-- 最小真实采集回放夹具：一条 BOT 消息命中两条规则。
-- 导入到本地 youban_tgfake_test 后，由真实 collector/collect-process/media/publish 角色处理。
BEGIN;

-- 两条规则共享一个 BOT 来源，但分别绑定不同目标频道。
INSERT INTO hg_youban_publish_collect_rule
(id,tenant_id,account_id,name,global_enabled,review_enabled,dedupe_enabled,dedupe_days,full_match_enabled,block_link,block_username,block_plain_text,status,created_at,updated_at)
VALUES
(900088,999,999,'本地回放规则一',0,0,0,7,0,0,0,0,1,NOW(),NOW()),
(900089,999,999,'本地回放规则二',0,0,0,7,0,0,0,0,1,NOW(),NOW())
ON CONFLICT (id) DO UPDATE SET status=1,updated_at=EXCLUDED.updated_at;

INSERT INTO hg_youban_publish_collect_rule_item
(tenant_id,account_id,rule_id,item_type,value,replacement,sort,created_at)
VALUES
(999,999,900088,'keyword','回放资料','',1,NOW()),
(999,999,900089,'keyword','回放资料','',1,NOW())
ON CONFLICT DO NOTHING;

INSERT INTO hg_youban_publish_collect_source_rule
(tenant_id,source_id,rule_id,sort,status,created_at,updated_at)
VALUES
(999,900087,900088,1,1,NOW(),NOW()),
(999,900087,900089,2,1,NOW(),NOW())
ON CONFLICT (source_id,rule_id) DO UPDATE SET status=1,updated_at=EXCLUDED.updated_at;

INSERT INTO hg_youban_publish_collect_rule_channel
(tenant_id,account_id,rule_id,channel_id,created_at)
VALUES
(999,999,900088,2,NOW()),
(999,999,900089,79,NOW())
ON CONFLICT (rule_id,channel_id) DO NOTHING;

-- 清理本夹具旧数据，保证可重复执行。
DELETE FROM hg_youban_publish_collect_process_outbox WHERE event_id=990088;
DELETE FROM hg_youban_publish_collect_event_media WHERE event_id=990088;
DELETE FROM hg_youban_publish_collect_event WHERE id=990088;

INSERT INTO hg_youban_publish_collect_event
(id,tenant_id,account_id,source_id,source_type,bot_id,tg_account_id,source_chat_id,source_message_id,source_unique_key,material_role,material_group_status,raw_text,media_count,status,received_at,created_at,updated_at)
VALUES
(990088,999,999,900087,'bot',1,0,'-1003954550954',990088,'local:collect:990088','display','ready','回放资料：命中两条规则',1,'pending',NOW(),NOW(),NOW());

INSERT INTO hg_youban_publish_collect_event_media
(tenant_id,account_id,source_id,source_type,event_id,source_chat_id,source_message_id,source_media_key,media_type,source_ref_type,source_file_id,file_url,storage_path,source_kind,source_mime_type,source_size,sort_index,cache_status,cache_hit,created_at,updated_at)
VALUES
(999,999,900087,'bot',990088,'-1003954550954',990088,'local-media-990088','photo','bot','local-file-990088','http://127.0.0.1:19100/local-media-990088','/tmp/local-media-990088.jpg','photo','image/jpeg',1024,0,'ready',1,NOW(),NOW());

INSERT INTO hg_youban_publish_collect_process_outbox
(event_id,tenant_id,account_id,source_id,source_chat_id,status,available_at,created_at,updated_at)
VALUES
(990088,999,999,900087,'-1003954550954','pending',NOW(),NOW(),NOW())
ON CONFLICT (event_id) DO UPDATE SET status='pending',available_at=NOW(),lease_owner='',lease_expires_at=NULL,last_error=NULL,updated_at=NOW();

COMMIT;

SELECT 'collect_multi_rule_replay' AS fixture, count(*) AS events
FROM hg_youban_publish_collect_event WHERE id=990088;
SELECT rule_id,channel_id FROM hg_youban_publish_collect_rule_channel WHERE rule_id IN (900088,900089) ORDER BY rule_id;
