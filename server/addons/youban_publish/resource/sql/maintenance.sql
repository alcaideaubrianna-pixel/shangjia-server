ALTER TABLE `hg_youban_publish_tg_job_log`
  ADD KEY `idx_ybp_tg_job_log_tenant` (`tenant_id`,`id`),
  ADD KEY `idx_ybp_tg_job_log_account` (`tenant_id`,`account_id`,`id`),
  ADD KEY `idx_ybp_tg_job_log_created` (`created_at`,`id`);

ALTER TABLE `hg_youban_publish_tenant`
  ADD KEY `idx_ybp_tenant_remark` (`remark`);

ALTER TABLE `hg_youban_publish_account`
  ADD KEY `idx_ybp_account_username` (`account_type`,`username`,`tenant_id`);

ALTER TABLE `hg_youban_publish_profile_state`
  ADD KEY `idx_ybp_profile_state_tenant_profile_open` (`tenant_id`,`profile_id`,`deleted_at`);

ALTER TABLE `hg_content_profile`
  ADD UNIQUE KEY `uk_content_profile_no` (`profile_no`);

ALTER TABLE `hg_youban_publish_tg_job`
  ADD KEY `idx_ybp_tg_job_collect_order` (`channel_id`,`target_chat_id`,`collect_source_id`,`collect_source_chat_id`,`collect_source_message_id`,`status`,`id`);

ALTER TABLE `hg_youban_publish_media` ADD KEY `idx_ybp_media_similar_tenant` (`tenant_id`,`media_type`,`account_id`,`profile_id`,`id`);
ALTER TABLE `hg_youban_publish_media` ADD KEY `idx_ybp_media_similar_account` (`account_id`,`media_type`,`profile_id`,`id`);
