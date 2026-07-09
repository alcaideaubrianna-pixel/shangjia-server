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
ALTER TABLE `hg_youban_publish_task`
  ADD KEY `idx_ybp_task_collect_order` (`collect_source_id`,`collect_source_chat_id`,`collect_source_message_id`,`id`);

ALTER TABLE `hg_youban_publish_tg_job`
  ADD COLUMN `collect_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID' AFTER `target_chat_id`,
  ADD COLUMN `collect_source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID' AFTER `collect_event_id`,
  ADD COLUMN `collect_source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '采集来源Chat ID' AFTER `collect_source_id`,
  ADD COLUMN `collect_source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集来源消息ID' AFTER `collect_source_chat_id`;
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

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'collect', '采集总开关', 'int', 'collectEnabled', '1', '1', 10, '是否启用采集能力', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='collectEnabled');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'collect', '实时采集推送延迟', 'int', 'realtimePushDelaySec', '60', '60', 20, '实时采集命中规则后延迟推送的秒数，用于等待媒体组和保持来源顺序', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='realtimePushDelaySec');

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
