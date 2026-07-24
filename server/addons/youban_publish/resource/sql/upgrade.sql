ALTER TABLE `hg_youban_publish_collect_rule`
  ADD COLUMN `full_match_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '全量匹配' AFTER `dedupe_days`;

ALTER TABLE `hg_youban_publish_collect_rule`
  ADD COLUMN `delete_text_json` text COMMENT '删除文本JSON' AFTER `replace_json`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD COLUMN `history_collect_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '账号历史采集开关' AFTER `collect_enabled`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD COLUMN `history_collect_mode` varchar(32) NOT NULL DEFAULT 'recent_days' COMMENT '账号历史采集模式' AFTER `history_collect_enabled`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD COLUMN `history_collect_days` int(11) NOT NULL DEFAULT '30' COMMENT '账号历史采集天数' AFTER `history_collect_mode`;

ALTER TABLE `hg_youban_publish_task`
  ADD COLUMN `collect_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID' AFTER `channel_id_json`,
  ADD COLUMN `collect_source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID' AFTER `collect_event_id`,
  ADD COLUMN `collect_source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '采集来源Chat ID' AFTER `collect_source_id`,
  ADD COLUMN `collect_source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集来源消息ID' AFTER `collect_source_chat_id`;

ALTER TABLE `hg_youban_publish_tg_job`
  ADD COLUMN `collect_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID' AFTER `target_chat_id`,
  ADD COLUMN `collect_source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID' AFTER `collect_event_id`,
  ADD COLUMN `collect_source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '采集来源Chat ID' AFTER `collect_source_id`,
  ADD COLUMN `collect_source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集来源消息ID' AFTER `collect_source_chat_id`;

ALTER TABLE `hg_youban_publish_tg_channel`
  ADD COLUMN `management_role` varchar(16) NOT NULL DEFAULT 'member' COMMENT '当前TG账号角色：owner/admin/member' AFTER `channel_username`;

ALTER TABLE `hg_youban_publish_account`
  MODIFY COLUMN `public_follow_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否公开关注';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_note_index` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `task_id` bigint(20) NOT NULL DEFAULT '0',
  `uuid` varchar(128) NOT NULL DEFAULT '',
  `profile_no` varchar(64) NOT NULL DEFAULT '',
  `title` varchar(255) NOT NULL DEFAULT '',
  `summary` text,
  `plain_text` text,
  `tag` text,
  `province` varchar(64) NOT NULL DEFAULT '',
  `city` varchar(64) NOT NULL DEFAULT '',
  `status` smallint NOT NULL DEFAULT '1',
  `visibility` varchar(32) NOT NULL DEFAULT '',
  `review_status` varchar(32) NOT NULL DEFAULT '',
  `task_status` varchar(32) NOT NULL DEFAULT '',
  `cover_media_id` bigint(20) NOT NULL DEFAULT '0',
  `published_at` datetime DEFAULT NULL,
  `source_updated_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_note_index_scope_profile` (`tenant_id`,`account_id`,`profile_id`),
  KEY `idx_ybp_note_index_tenant_updated` (`tenant_id`,`updated_at`,`id`),
  KEY `idx_ybp_note_index_account_updated` (`account_id`,`updated_at`,`id`),
  KEY `idx_ybp_note_index_profile` (`profile_id`),
  KEY `idx_ybp_note_index_title` (`title`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='资料列表读模型';

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'collect', '采集总开关', 'int', 'collectEnabled', '1', '1', 10, '是否启用采集能力', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='collectEnabled');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'collect', '实时采集推送延迟', 'int', 'realtimePushDelaySec', '600', '600', 20, '实时采集命中规则后延迟推送的秒数，用于等待媒体组和保持来源顺序', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='realtimePushDelaySec');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'autoDelete', '自动删除规则', '[]string', 'rules', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', 230, '仅匹配整条消息的自动删除规则', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='rules');

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_history_task` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '所属账号ID',
  `source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '协议号ID',
  `source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '来源频道ID',
  `mode` varchar(32) NOT NULL DEFAULT 'recent_days' COMMENT '采集模式',
  `days` int(11) NOT NULL DEFAULT '30' COMMENT '采集天数',
  `offset_id` int(11) NOT NULL DEFAULT '0' COMMENT '历史游标',
  `scanned_count` int(11) NOT NULL DEFAULT '0' COMMENT '扫描数量',
  `event_count` int(11) NOT NULL DEFAULT '0' COMMENT '事件数量',
  `duplicate_count` int(11) NOT NULL DEFAULT '0' COMMENT '重复数量',
  `failed_count` int(11) NOT NULL DEFAULT '0' COMMENT '失败数量',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `error_message` text COMMENT '错误信息',
  `next_run_at` datetime DEFAULT NULL COMMENT '下次执行时间',
  `started_at` datetime DEFAULT NULL COMMENT '开始时间',
  `finished_at` datetime DEFAULT NULL COMMENT '完成时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_collect_history_owner` (`tenant_id`,`account_id`,`status`,`id`),
  KEY `idx_ybp_collect_history_source` (`source_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴历史采集任务';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_history_log` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '所属账号ID',
  `level` varchar(16) NOT NULL DEFAULT 'info' COMMENT '日志等级',
  `stage` varchar(32) NOT NULL DEFAULT '' COMMENT '阶段',
  `message` text COMMENT '日志内容',
  `meta_json` text COMMENT '上下文JSON',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_collect_history_log_task` (`task_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴历史采集日志';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_event_media` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID',
  `source_type` varchar(32) NOT NULL DEFAULT '' COMMENT '采集源类型',
  `event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID',
  `source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '来源频道/群聊ID',
  `source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '来源消息ID',
  `source_grouped_id` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体组ID',
  `source_media_key` varchar(255) NOT NULL DEFAULT '' COMMENT '来源媒体键',
  `media_type` varchar(32) NOT NULL DEFAULT '' COMMENT '媒体类型',
  `source_ref_type` varchar(32) NOT NULL DEFAULT '' COMMENT '来源引用类型',
  `source_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT '来源文件ID',
  `source_message_ref` varchar(255) NOT NULL DEFAULT '' COMMENT '来源消息引用',
  `backup_channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '备份频道ID',
  `backup_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '备份聊天ID',
  `backup_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '备份消息ID',
  `file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '文件访问地址',
  `storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '存储路径',
  `poster_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '封面地址',
  `meta_json` text COMMENT '媒体元数据',
  `sort_index` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `cache_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '缓存状态',
  `error_message` text COMMENT '错误信息',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_collect_event_media_event` (`event_id`,`sort_index`,`id`),
  KEY `idx_ybp_collect_event_media_owner` (`tenant_id`,`source_id`,`cache_status`,`id`),
  KEY `idx_ybp_collect_event_media_source` (`source_chat_id`,`source_message_id`,`source_media_key`),
  KEY `idx_ybp_collect_event_media_file` (`source_file_id`),
  KEY `idx_ybp_collect_event_media_cache` (`cache_status`,`updated_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集事件媒体';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_event_log` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID',
  `dispatch_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '分发ID',
  `stage` varchar(64) NOT NULL DEFAULT '' COMMENT '阶段',
  `status` varchar(32) NOT NULL DEFAULT '' COMMENT '状态',
  `message` text COMMENT '日志内容',
  `meta_text` text COMMENT '上下文文本',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_collect_event_log_event` (`event_id`,`id`),
  KEY `idx_ybp_collect_event_log_owner` (`tenant_id`,`account_id`,`created_at`),
  KEY `idx_ybp_collect_event_log_stage` (`event_id`,`stage`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集事件日志';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_template` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '模板名称',
  `text` text COMMENT '消息文本',
  `media_count` int(11) NOT NULL DEFAULT '0' COMMENT '媒体数量',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态：1启用 2停用',
  `created_by` bigint(20) NOT NULL DEFAULT '0',
  `updated_by` bigint(20) NOT NULL DEFAULT '0',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ybp_msg_tpl_owner` (`tenant_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息推送模板';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_media` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `template_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '模板ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `media_type` varchar(16) NOT NULL DEFAULT 'image' COMMENT '媒体类型',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT '文件名',
  `file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '访问地址',
  `storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '存储路径',
  `poster_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面',
  `poster_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面路径',
  `tg_file_id` varchar(1024) NOT NULL DEFAULT '' COMMENT 'TG文件ID',
  `tg_thumb_file_id` varchar(1024) NOT NULL DEFAULT '' COMMENT 'TG封面文件ID',
  `asset_hash` varchar(1024) NOT NULL DEFAULT '' COMMENT '素材Hash',
  `sort_index` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ybp_msg_media_tpl` (`template_id`,`sort_index`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息推送模板媒体';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_push_plan` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '计划名称',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG账号ID',
  `template_ids` text COMMENT '模板ID列表JSON',
  `target_chat_ids` text COMMENT '目标群聊或频道Chat ID列表JSON',
  `times` text COMMENT '每天推送时间JSON',
  `interval_seconds` int(11) NOT NULL DEFAULT '60' COMMENT '推送间隔秒数',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态：1启用 2停用',
  `next_run_at` datetime DEFAULT NULL COMMENT '下次执行时间',
  `last_run_at` datetime DEFAULT NULL COMMENT '最后执行时间',
  `last_result` text COMMENT '最后执行结果',
  `locked_at` datetime DEFAULT NULL COMMENT '锁定时间',
  `created_by` bigint(20) NOT NULL DEFAULT '0',
  `updated_by` bigint(20) NOT NULL DEFAULT '0',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ybp_msg_plan_due` (`status`,`next_run_at`,`id`),
  KEY `idx_ybp_msg_plan_owner` (`tenant_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息自动推送计划';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_listen_plan` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '计划名称',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '监听TG账号ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '推送Bot ID',
  `bind_code` varchar(32) NOT NULL DEFAULT '' COMMENT '通知目标绑定ID',
  `notify_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '通知目标Chat ID',
  `notify_chat_type` varchar(32) NOT NULL DEFAULT '' COMMENT '通知目标类型',
  `notify_chat_title` varchar(255) NOT NULL DEFAULT '' COMMENT '通知目标标题',
  `notify_bound_at` datetime DEFAULT NULL COMMENT '通知目标绑定时间',
  `keywords_json` text COMMENT '关键字JSON',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态：1启用 2停用',
  `last_trigger_at` datetime DEFAULT NULL COMMENT '最近触发时间',
  `last_result` text COMMENT '最近执行结果',
  `created_by` bigint(20) NOT NULL DEFAULT '0',
  `updated_by` bigint(20) NOT NULL DEFAULT '0',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_msg_listen_plan_code` (`bind_code`),
  KEY `idx_ybp_msg_listen_plan_owner` (`tenant_id`,`status`,`id`),
  KEY `idx_ybp_msg_listen_plan_account` (`tg_account_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息监听计划';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_listen_target` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `plan_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '计划ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `target_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '目标Chat ID',
  `target_chat_type` varchar(32) NOT NULL DEFAULT '' COMMENT '目标Chat类型',
  `target_chat_title` varchar(255) NOT NULL DEFAULT '' COMMENT '目标Chat标题',
  `target_chat_username` varchar(255) NOT NULL DEFAULT '' COMMENT '目标Chat用户名',
  `last_matched_at` datetime DEFAULT NULL COMMENT '最近命中时间',
  `last_matched_text` text COMMENT '最近命中文本',
  `last_matched_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT '最近命中用户ID',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0',
  `updated_by` bigint(20) NOT NULL DEFAULT '0',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_msg_listen_target_chat` (`plan_id`,`target_chat_id`),
  KEY `idx_ybp_msg_listen_target_plan` (`plan_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息监听目标';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_listen_notice` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `plan_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '计划ID',
  `target_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '目标ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '监听TG账号ID',
  `source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '来源Chat ID',
  `source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '来源消息ID',
  `sender_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT '发送用户ID',
  `sender_username` varchar(128) NOT NULL DEFAULT '' COMMENT '发送用户名',
  `normalized_text_hash` varchar(128) NOT NULL DEFAULT '' COMMENT '文本Hash',
  `media_hash` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体Hash',
  `dedupe_key` varchar(255) NOT NULL DEFAULT '' COMMENT '去重键',
  `match_keywords_json` text COMMENT '命中关键字JSON',
  `notify_result` text COMMENT '推送结果',
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_msg_listen_notice_dedupe` (`dedupe_key`),
  KEY `idx_ybp_msg_listen_notice_plan` (`plan_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息监听命中记录';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_listen_sender` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '监听TG账号ID',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `telegram_first_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG名',
  `telegram_last_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG姓',
  `display_name` varchar(255) NOT NULL DEFAULT '' COMMENT '显示名称',
  `last_seen_at` datetime DEFAULT NULL COMMENT '最近出现时间',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_msg_listen_sender_user` (`tg_account_id`,`telegram_user_id`),
  KEY `idx_ybp_msg_listen_sender_tenant` (`tenant_id`,`tg_account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息监听发送者缓存';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_quick_push_plan` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '计划名称',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG账号ID',
  `target_chat_ids` text COMMENT '目标群聊或频道Chat ID JSON',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_quick_plan_owner` (`tenant_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴快速推送计划';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_material_import_task` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料归属账号ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG协议账号ID',
  `source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '来源频道/群聊ID',
  `source_title` varchar(255) NOT NULL DEFAULT '' COMMENT '来源频道/群聊名称',
  `source_username` varchar(128) NOT NULL DEFAULT '' COMMENT '来源用户名',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '任务状态',
  `stage` varchar(32) NOT NULL DEFAULT 'created' COMMENT '执行阶段',
  `pull_offset_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '消息拉取偏移ID',
  `pull_limit_days` int(11) NOT NULL DEFAULT '365' COMMENT '拉取天数',
  `message_total` int(11) NOT NULL DEFAULT '0' COMMENT '消息总数',
  `message_done` int(11) NOT NULL DEFAULT '0' COMMENT '已处理消息数',
  `group_total` int(11) NOT NULL DEFAULT '0' COMMENT '分组总数',
  `group_done` int(11) NOT NULL DEFAULT '0' COMMENT '已入库分组数',
  `media_total` int(11) NOT NULL DEFAULT '0' COMMENT '媒体总数',
  `media_done` int(11) NOT NULL DEFAULT '0' COMMENT '已下载媒体数',
  `media_failed` int(11) NOT NULL DEFAULT '0' COMMENT '媒体失败数',
  `imported` int(11) NOT NULL DEFAULT '0' COMMENT '导入资料数',
  `duplicate` int(11) NOT NULL DEFAULT '0' COMMENT '重复资料数',
  `error_message` text COMMENT '错误信息',
  `next_run_at` datetime DEFAULT NULL COMMENT '下次执行时间',
  `result_json` longtext COMMENT '结果JSON',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `started_at` datetime DEFAULT NULL COMMENT '开始时间',
  `finished_at` datetime DEFAULT NULL COMMENT '结束时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_material_import_owner` (`tenant_id`,`account_id`,`status`,`id`),
  KEY `idx_ybp_material_import_tg` (`tenant_id`,`tg_account_id`,`source_chat_id`,`id`),
  KEY `idx_ybp_material_import_status` (`status`,`next_run_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴资料导入任务';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_material_import_group` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '导入任务ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料归属账号ID',
  `source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '来源频道/群聊ID',
  `source_grouped_id` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体组ID',
  `source_message_ids` text COMMENT '来源消息ID JSON',
  `source_unique_key` varchar(255) NOT NULL DEFAULT '' COMMENT '来源唯一键',
  `title` varchar(255) NOT NULL DEFAULT '' COMMENT '标题',
  `nickname` varchar(128) NOT NULL DEFAULT '' COMMENT '昵称',
  `profile_no` varchar(64) NOT NULL DEFAULT '' COMMENT '编号',
  `raw_text` text COMMENT '原始文本',
  `profile_text` text COMMENT '资料正文',
  `verify_text` text COMMENT '验证资料文本',
  `media_json` longtext COMMENT '媒体JSON',
  `media_total` int(11) NOT NULL DEFAULT '0' COMMENT '媒体总数',
  `media_done` int(11) NOT NULL DEFAULT '0' COMMENT '已下载媒体数',
  `media_failed` int(11) NOT NULL DEFAULT '0' COMMENT '媒体失败数',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `task_profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架任务ID',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `error_message` text COMMENT '错误信息',
  `message_at` datetime DEFAULT NULL COMMENT 'TG消息时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_material_import_group` (`tenant_id`,`source_unique_key`),
  KEY `idx_ybp_material_import_group_task` (`task_id`,`status`,`id`),
  KEY `idx_ybp_material_import_group_profile` (`profile_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴资料导入分组';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_media_phash_bucket` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `media_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '媒体ID',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID',
  `media_type` varchar(16) NOT NULL DEFAULT '' COMMENT '媒体类型',
  `hash_value` varchar(64) NOT NULL DEFAULT '' COMMENT '感知哈希',
  `bucket_pos` smallint(6) NOT NULL DEFAULT '0' COMMENT '分桶位置',
  `bucket_value` varchar(1) NOT NULL DEFAULT '' COMMENT '分桶值',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_media_phash_bucket_media_pos` (`media_id`,`bucket_pos`),
  KEY `idx_ybp_media_phash_bucket_lookup` (`tenant_id`,`media_type`,`bucket_pos`,`bucket_value`,`account_id`,`profile_id`,`task_id`,`media_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='媒体感知哈希分桶';
