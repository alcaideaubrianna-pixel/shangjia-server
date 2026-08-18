CREATE TABLE IF NOT EXISTS `hg_youban_chat_visitor` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `app_id` varchar(128) NOT NULL,
  `external_user_id` varchar(128) NOT NULL,
  `name` varchar(128) NOT NULL DEFAULT '',
  `email` varchar(255) NOT NULL DEFAULT '',
  `avatar_url` varchar(500) NOT NULL DEFAULT '',
  `created_at` datetime NULL,
  `updated_at` datetime NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybcv_app_user` (`app_id`,`external_user_id`)
);
