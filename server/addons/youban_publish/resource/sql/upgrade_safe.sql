ALTER TABLE `hg_youban_publish_collect_event_media`
  ADD COLUMN IF NOT EXISTS `next_retry_at` datetime DEFAULT NULL COMMENT '下次重试时间';

ALTER TABLE `hg_youban_publish_tg_job`
  ADD KEY `idx_ybp_tg_job_cycle_due` (`cycle_enabled`,`status`,`next_cycle_at`,`id`);
