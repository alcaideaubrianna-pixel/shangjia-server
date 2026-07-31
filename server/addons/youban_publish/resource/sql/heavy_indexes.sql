ALTER TABLE `hg_content_profile` ADD KEY `idx_content_profile_source_uuid` (`source_note_uuid`);
ALTER TABLE `hg_youban_publish_media_phash_lsh` ADD KEY `idx_ybp_media_phash_lsh_lookup` (`tenant_id`,`media_type`,`bucket_pos`,`bucket_value`,`account_id`,`profile_id`,`media_id`,`hash_value`);
