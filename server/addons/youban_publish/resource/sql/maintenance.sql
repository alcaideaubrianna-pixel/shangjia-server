ALTER TABLE `hg_youban_publish_tg_job_log`
  ADD KEY `idx_ybp_tg_job_log_tenant` (`tenant_id`,`id`),
  ADD KEY `idx_ybp_tg_job_log_account` (`tenant_id`,`account_id`,`id`),
  ADD KEY `idx_ybp_tg_job_log_created` (`created_at`,`id`);

ALTER TABLE `hg_youban_publish_tenant`
  ADD KEY `idx_ybp_tenant_remark` (`remark`);

ALTER TABLE `hg_youban_publish_account`
  ADD KEY `idx_ybp_account_username` (`account_type`,`username`,`tenant_id`);

ALTER TABLE `hg_content_profile`
  ADD UNIQUE KEY `uk_content_profile_no` (`profile_no`);

ALTER TABLE `hg_youban_publish_task`
  ADD KEY `idx_ybp_task_collect_order` (`collect_source_id`,`collect_source_chat_id`,`collect_source_message_id`,`id`);

ALTER TABLE `hg_youban_publish_tg_job`
  ADD KEY `idx_ybp_tg_job_collect_order` (`channel_id`,`target_chat_id`,`collect_source_id`,`collect_source_chat_id`,`collect_source_message_id`,`status`,`id`);

UPDATE `hg_youban_publish_task` t
JOIN `hg_youban_publish_collect_event` e
  ON t.`tenant_id`=e.`tenant_id`
  AND t.`account_id`=e.`account_id`
  AND t.`client_request_id` LIKE CONCAT('collect:', e.`source_unique_key`, ':%')
SET
  t.`collect_event_id`=e.`id`,
  t.`collect_source_id`=e.`source_id`,
  t.`collect_source_chat_id`=e.`source_chat_id`,
  t.`collect_source_message_id`=e.`source_message_id`
WHERE t.`collect_source_message_id`=0;

UPDATE `hg_youban_publish_tg_job` j
JOIN `hg_youban_publish_task` t ON t.`id`=j.`task_id`
SET
  j.`collect_event_id`=t.`collect_event_id`,
  j.`collect_source_id`=t.`collect_source_id`,
  j.`collect_source_chat_id`=t.`collect_source_chat_id`,
  j.`collect_source_message_id`=t.`collect_source_message_id`
WHERE j.`collect_source_message_id`=0 AND t.`collect_source_message_id`>0;

ALTER TABLE `hg_youban_publish_media` ADD KEY `idx_ybp_media_similar_tenant` (`tenant_id`,`media_type`,`account_id`,`profile_id`,`id`);
ALTER TABLE `hg_youban_publish_media` ADD KEY `idx_ybp_media_similar_account` (`account_id`,`media_type`,`profile_id`,`id`);
