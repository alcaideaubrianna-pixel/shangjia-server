-- Run after upgrade_online.sql. Kept separate to avoid a large deployment transaction.
INSERT INTO `hg_youban_publish_collect_channel_profile`
  (`tenant_id`,`account_id`,`source_id`,`rule_id`,`channel_id`,`profile_id`,`created_at`,`updated_at`)
SELECT d.`tenant_id`,d.`account_id`,d.`source_id`,d.`rule_id`,
  IF(e.`source_chat_id` LIKE '-100%',e.`source_chat_id`,CONCAT('-100',e.`source_chat_id`)),
  d.`profile_id`,COALESCE(d.`created_at`,CURRENT_TIMESTAMP),COALESCE(d.`updated_at`,CURRENT_TIMESTAMP)
FROM `hg_youban_publish_collect_dispatch` d
JOIN `hg_youban_publish_collect_event` e
  ON e.`id`=d.`event_id`
 AND e.`tenant_id`=d.`tenant_id`
 AND e.`account_id`=d.`account_id`
WHERE d.`profile_id`>0 AND e.`source_chat_id`<>'' AND d.`rule_id`>0
ON DUPLICATE KEY UPDATE
  `source_id`=VALUES(`source_id`),
  `updated_at`=GREATEST(`updated_at`,VALUES(`updated_at`));
