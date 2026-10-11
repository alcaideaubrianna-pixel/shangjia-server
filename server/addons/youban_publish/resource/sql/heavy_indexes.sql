-- Run only through the explicit heavy-index maintenance command, never interactive upgrade.
ALTER TABLE `hg_youban_publish_import_run_log` ADD KEY `idx_ybp_import_run_log_created` (`created_at`,`id`);
ALTER TABLE `hg_youban_publish_collect_event_log` ADD KEY `idx_ybp_collect_event_log_created` (`created_at`,`id`);
ALTER TABLE `hg_youban_publish_collect_history_log` ADD KEY `idx_ybp_collect_history_log_created` (`created_at`,`id`);
ALTER TABLE `hg_youban_publish_cycle_run_log` ADD KEY `idx_ybp_cycle_run_log_created` (`created_at`,`id`);
ALTER TABLE `hg_addon_lazysheep_tggo_push_log` ADD KEY `idx_lazysheep_tggo_push_log_created` (`created_at`,`id`);

ALTER TABLE `hg_content_profile` ADD KEY `idx_content_profile_source_uuid` (`source_note_uuid`);
ALTER TABLE `hg_youban_publish_media_phash_lsh` ADD KEY `idx_ybp_media_phash_lsh_lookup` (`tenant_id`,`media_type`,`bucket_pos`,`bucket_value`,`account_id`,`profile_id`,`media_id`,`hash_value`);
ALTER TABLE `hg_youban_publish_tg_message` ADD KEY `idx_ybp_tg_message_file_unique` (`tg_file_unique_id`,`account_id`,`id`);
