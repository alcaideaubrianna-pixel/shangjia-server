ALTER TABLE `hg_youban_publish_collect_event_media`
  ADD COLUMN IF NOT EXISTS `next_retry_at` datetime DEFAULT NULL COMMENT '下次重试时间';
