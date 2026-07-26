ALTER TABLE `hg_content_profile` ADD KEY `idx_content_profile_source_uuid` (`source_note_uuid`);
ALTER TABLE `hg_youban_publish_task` ADD KEY `idx_ybp_task_profile_status` (`profile_id`,`status`,`id`);
ALTER TABLE `hg_youban_publish_media` ADD KEY `idx_ybp_media_task_active` (`task_id`,`deleted_at`,`sort_index`,`id`);
