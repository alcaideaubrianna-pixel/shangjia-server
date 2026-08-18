ALTER TABLE `hg_youban_publish_collect_rule`
  ADD COLUMN `full_match_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '全量匹配' AFTER `dedupe_days`;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_material_import_group_media` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT, `task_id` bigint(20) NOT NULL DEFAULT '0', `group_id` bigint(20) NOT NULL DEFAULT '0',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0', `account_id` bigint(20) NOT NULL DEFAULT '0', `purpose` varchar(16) NOT NULL DEFAULT 'display',
  `media_type` varchar(32) NOT NULL DEFAULT '', `sort_index` int(11) NOT NULL DEFAULT '0', `source_file_id` varchar(255) NOT NULL DEFAULT '',
  `file_url` varchar(1024) NOT NULL DEFAULT '', `storage_path` varchar(1024) NOT NULL DEFAULT '', `poster_url` varchar(1024) NOT NULL DEFAULT '',
  `source_kind` varchar(32) NOT NULL DEFAULT '', `source_media_id` bigint(20) NOT NULL DEFAULT '0', `source_access_hash` bigint(20) NOT NULL DEFAULT '0',
  `source_file_reference` blob, `source_thumb_size` varchar(32) NOT NULL DEFAULT '', `source_mime_type` varchar(128) NOT NULL DEFAULT '',
  `source_dc_id` int(11) NOT NULL DEFAULT '0', `source_size` bigint(20) NOT NULL DEFAULT '0', `file_md5` varchar(64) NOT NULL DEFAULT '',
  `file_phash` varchar(128) NOT NULL DEFAULT '', `created_at` datetime DEFAULT NULL, `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), KEY `idx_ybp_material_import_group_media_group` (`group_id`,`sort_index`,`id`),
  KEY `idx_ybp_material_import_group_media_task` (`task_id`,`group_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG资料导入分组媒体';

ALTER TABLE `hg_youban_publish_collect_event_media`
  ADD COLUMN `source_kind` varchar(32) NOT NULL DEFAULT '' COMMENT 'TG来源媒体类型' AFTER `poster_url`,
  ADD COLUMN `source_media_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG媒体ID' AFTER `source_kind`,
  ADD COLUMN `source_access_hash` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG媒体访问哈希' AFTER `source_media_id`,
  ADD COLUMN `source_file_reference` blob COMMENT 'TG文件引用' AFTER `source_access_hash`,
  ADD COLUMN `source_thumb_size` varchar(32) NOT NULL DEFAULT '' COMMENT 'TG缩略图规格' AFTER `source_file_reference`,
  ADD COLUMN `source_mime_type` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG媒体MIME' AFTER `source_thumb_size`,
  ADD COLUMN `source_dc_id` int(11) NOT NULL DEFAULT '0' COMMENT 'TG数据中心ID' AFTER `source_mime_type`,
  ADD COLUMN `source_size` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG媒体大小' AFTER `source_dc_id`,
  ADD COLUMN `file_md5` varchar(64) NOT NULL DEFAULT '' COMMENT '文件MD5' AFTER `source_size`,
  ADD COLUMN `file_phash` varchar(128) NOT NULL DEFAULT '' COMMENT '图片感知哈希' AFTER `file_md5`;

ALTER TABLE `hg_youban_publish_collect_rule`
  ADD COLUMN `delete_text_json` text COMMENT '删除文本JSON' AFTER `replace_json`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD COLUMN `bot_collect_scope` varchar(16) NOT NULL DEFAULT 'chat' COMMENT 'Bot采集范围：chat/private' AFTER `bot_id`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD COLUMN `history_collect_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '账号历史采集开关' AFTER `collect_enabled`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD COLUMN `history_collect_mode` varchar(32) NOT NULL DEFAULT 'recent_days' COMMENT '账号历史采集模式' AFTER `history_collect_enabled`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD COLUMN `history_collect_days` int(11) NOT NULL DEFAULT '30' COMMENT '账号历史采集天数' AFTER `history_collect_mode`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD KEY `idx_ybp_collect_source_bot_scope` (`bot_id`,`bot_collect_scope`,`source_chat_id`,`status`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_bot_channel_cache` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '机器人ID',
  `chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '聊天ID',
  `chat_type` varchar(32) NOT NULL DEFAULT '' COMMENT '聊天类型',
  `chat_title` varchar(255) NOT NULL DEFAULT '' COMMENT '聊天标题',
  `chat_username` varchar(128) NOT NULL DEFAULT '' COMMENT '聊天用户名',
  `is_broadcast` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否频道',
  `is_megagroup` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否超级群',
  `message_count` int(11) NOT NULL DEFAULT '0' COMMENT '消息数量',
  `last_message_text` text COMMENT '最后消息文本',
  `last_message_at` datetime DEFAULT NULL COMMENT '最后消息时间',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_bot_channel_cache_bot_chat` (`tenant_id`,`bot_id`,`chat_id`),
  KEY `idx_ybp_bot_channel_cache_list` (`tenant_id`,`bot_id`,`last_message_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Bot采集频道缓存';

ALTER TABLE `hg_youban_publish_tg_job`
  ADD COLUMN `collect_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID' AFTER `target_chat_id`,
  ADD COLUMN `collect_source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID' AFTER `collect_event_id`,
  ADD COLUMN `collect_source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '采集来源Chat ID' AFTER `collect_source_id`,
  ADD COLUMN `collect_source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集来源消息ID' AFTER `collect_source_chat_id`;

ALTER TABLE `hg_youban_publish_tg_channel`
  ADD COLUMN `management_role` varchar(16) NOT NULL DEFAULT 'member' COMMENT '当前TG账号角色：owner/admin/member' AFTER `channel_username`;

ALTER TABLE `hg_youban_publish_channel`
  ADD COLUMN `bot_permission_status_json` text NOT NULL COMMENT '频道Bot权限检测结果JSON' AFTER `bot_id_json`;

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
SELECT 'youban_publish', 'collect', '采集推送总开关', 'int', 'collectPushEnabled', '1', '1', 15, '是否允许采集资料推送到 Telegram 频道', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='collectPushEnabled');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'collect', '实时采集推送延迟', 'int', 'realtimePushDelaySec', '600', '600', 20, '实时采集命中规则后延迟推送的秒数，用于等待媒体组和保持来源顺序', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='realtimePushDelaySec');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'autoDelete', '自动删除规则', '[]string', 'rules', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', 230, '仅匹配整条消息的自动删除规则', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='rules');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'autoDelete', '自动删除关键词', '[]string', 'keywords', '["发现重复资料","验证视频","资料近期已上架","信息已推送成功","视频已推送成功","视频已自动转发到视频频道","小时内已提交过相同内容","内容已自动转发到发布频道","提交成功！","信息视频验证保存成功","收录失败！","处理媒体组失败:","推送媒体组到群","信息投稿推送","推送视频失败","收录成功","本频道弃用，订阅如下新频道","投稿已自动审核通过","重复投稿","重复提交","信息已存在请勿重复提交","采集失败","检测到重复资料","正在分析中","已被人脸过滤","提交成功","去重阶段","重复帖子已拦截","提交失败"]', '["发现重复资料","验证视频","资料近期已上架","信息已推送成功","视频已推送成功","视频已自动转发到视频频道","小时内已提交过相同内容","内容已自动转发到发布频道","提交成功！","信息视频验证保存成功","收录失败！","处理媒体组失败:","推送媒体组到群","信息投稿推送","推送视频失败","收录成功","本频道弃用，订阅如下新频道","投稿已自动审核通过","重复投稿","重复提交","信息已存在请勿重复提交","采集失败","检测到重复资料","正在分析中","已被人脸过滤","提交成功","去重阶段","重复帖子已拦截","提交失败"]', 220, '所有租户继承的默认自动删除关键词', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `group`='autoDelete' AND `key`='keywords');

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_auto_delete_config` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否启用',
  `bot_ids_json` text COMMENT '租户Bot ID JSON',
  `custom_keywords_json` text COMMENT '租户自定义关键词JSON',
  `custom_rules_json` text COMMENT '租户自定义规则JSON',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tenant_auto_delete_config_tenant` (`tenant_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='租户消息自动删除配置';

UPDATE `hg_sys_addons_config`
SET `status` = 2,
    `updated_at` = NOW()
WHERE `addon_name` = 'youban_publish'
  AND `group` = 'autoDelete'
  AND `key` IN ('enabled', 'autoDeleteEnabled', 'botIds');

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
  `serial_no` varchar(32) NOT NULL DEFAULT '' COMMENT 'Inline模板编号',
  `push_mode` varchar(16) NOT NULL DEFAULT 'bot' COMMENT '推送方式：bot/account',
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
  ,KEY `idx_ybp_msg_tpl_serial` (`serial_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息推送模板';
ALTER TABLE `hg_youban_publish_message_template` ADD COLUMN `source_message_record_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '来源TG消息记录ID';

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
ALTER TABLE `hg_youban_publish_message_media` ADD COLUMN `source_message_record_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '来源TG消息记录ID';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_push_plan` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '计划名称',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG账号ID',
  `template_ids` text COMMENT '模板ID列表JSON',
  `target_chat_ids` text COMMENT '目标群聊或频道Chat ID列表JSON',
  `times` text COMMENT '执行日推送时间JSON',
  `interval_days` int(11) NOT NULL DEFAULT '1' COMMENT '执行间隔天数',
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

ALTER TABLE `hg_youban_publish_message_push_plan`
  ADD COLUMN IF NOT EXISTS `interval_days` int(11) NOT NULL DEFAULT '1' COMMENT '执行间隔天数' AFTER `times`;

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
  KEY `idx_ybp_msg_listen_notice_plan` (`plan_id`,`id`),
  KEY `idx_ybp_msg_listen_notice_cooldown` (`plan_id`,`sender_user_id`,`normalized_text_hash`,`created_at`)
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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_media_phash_lsh` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `media_id` bigint(20) NOT NULL DEFAULT '0',
  `task_id` bigint(20) NOT NULL DEFAULT '0',
  `media_type` varchar(16) NOT NULL DEFAULT '',
  `hash_value` varchar(64) NOT NULL DEFAULT '',
  `bucket_pos` smallint(6) NOT NULL DEFAULT '0',
  `bucket_value` int(11) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_media_phash_lsh_media_pos` (`media_id`,`bucket_pos`),
  KEY `idx_ybp_media_phash_lsh_search` (`tenant_id`,`media_type`,`bucket_pos`,`bucket_value`,`media_id`,`profile_id`,`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='媒体感知哈希LSH索引';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_profile_state` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `customer_remark` text,
  `anti_scan_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `publish_at` datetime DEFAULT NULL,
  `created_by` bigint(20) NOT NULL DEFAULT '0',
  `updated_by` bigint(20) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_profile_state_profile` (`profile_id`),
  KEY `idx_ybp_profile_state_owner` (`tenant_id`,`account_id`,`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='上架资料归属和发布配置';
CREATE TABLE IF NOT EXISTS `hg_youban_publish_profile_channel` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `channel_id` bigint(20) NOT NULL DEFAULT '0',
  `is_manual` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否资料手动选择',
  `created_by` bigint(20) NOT NULL DEFAULT '0',
  `updated_by` bigint(20) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_profile_channel` (`tenant_id`,`profile_id`,`channel_id`),
  KEY `idx_ybp_profile_channel_owner` (`tenant_id`,`account_id`,`profile_id`),
  KEY `idx_ybp_profile_channel_channel` (`tenant_id`,`channel_id`,`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='资料推送频道映射';
CREATE TABLE IF NOT EXISTS `hg_youban_publish_success_record` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT, `job_id` bigint(20) NOT NULL DEFAULT '0', `task_id` bigint(20) NOT NULL DEFAULT '0',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0', `account_id` bigint(20) NOT NULL DEFAULT '0', `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `channel_id` bigint(20) NOT NULL DEFAULT '0', `bot_id` bigint(20) NOT NULL DEFAULT '0', `operation_no` varchar(128) NOT NULL DEFAULT '',
  `target_chat_id` varchar(128) NOT NULL DEFAULT '', `action` varchar(32) NOT NULL DEFAULT 'profile_publish',
  `status` varchar(16) NOT NULL DEFAULT 'success', `message` varchar(255) NOT NULL DEFAULT '', `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_success_record_job` (`job_id`),
  KEY `idx_ybp_success_record_owner` (`tenant_id`,`account_id`,`id`), KEY `idx_ybp_success_record_profile` (`profile_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='上架成功发布记录';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_full_push_batch` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT, `batch_no` varchar(128) NOT NULL,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0', `channel_id` bigint(20) NOT NULL DEFAULT '0', `requested_by` bigint(20) NOT NULL DEFAULT '0',
  `snapshot_max_profile_id` bigint(20) NOT NULL DEFAULT '0', `cursor_profile_id` bigint(20) NOT NULL DEFAULT '0',
  `total_count` int(11) NOT NULL DEFAULT '0', `queued_count` int(11) NOT NULL DEFAULT '0', `retry_count` int(11) NOT NULL DEFAULT '0',
  `status` varchar(16) NOT NULL DEFAULT 'pending', `active_key` varchar(64) DEFAULT NULL, `error_message` text,
  `created_at` datetime DEFAULT NULL, `updated_at` datetime DEFAULT NULL, `finished_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_full_push_batch_no` (`batch_no`), UNIQUE KEY `uk_ybp_full_push_active` (`active_key`),
  KEY `idx_ybp_full_push_schedule` (`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='频道全量推送批次';
ALTER TABLE `hg_youban_publish_full_push_batch` ADD COLUMN IF NOT EXISTS `snapshot_max_profile_id` bigint(20) NOT NULL DEFAULT '0';
ALTER TABLE `hg_youban_publish_full_push_batch` ADD COLUMN IF NOT EXISTS `cursor_profile_id` bigint(20) NOT NULL DEFAULT '0';
ALTER TABLE `hg_youban_publish_full_push_batch` ADD COLUMN IF NOT EXISTS `snapshot_max_task_id` bigint(20) NOT NULL DEFAULT '0';
ALTER TABLE `hg_youban_publish_full_push_batch` ADD COLUMN IF NOT EXISTS `cursor_task_id` bigint(20) NOT NULL DEFAULT '0';
UPDATE `hg_youban_publish_full_push_batch` SET `snapshot_max_profile_id`=`snapshot_max_task_id` WHERE `snapshot_max_profile_id`=0 AND `snapshot_max_task_id`>0;
UPDATE `hg_youban_publish_full_push_batch` SET `cursor_profile_id`=`cursor_task_id` WHERE `cursor_profile_id`=0 AND `cursor_task_id`>0;
ALTER TABLE `hg_youban_publish_full_push_batch` DROP COLUMN IF EXISTS `snapshot_max_task_id`;
ALTER TABLE `hg_youban_publish_full_push_batch` DROP COLUMN IF EXISTS `cursor_task_id`;
ALTER TABLE `hg_youban_publish_tg_job` MODIFY COLUMN `task_id` bigint(20) DEFAULT NULL COMMENT '采集任务ID，普通资料推送为空';
ALTER TABLE `hg_youban_publish_tg_message` MODIFY COLUMN `task_id` bigint(20) DEFAULT NULL COMMENT '旧任务ID，资料索引为空';
ALTER TABLE `hg_youban_publish_channel_profile` DROP COLUMN IF EXISTS `task_id`;
DELETE FROM `hg_admin_menu` WHERE `name` = 'youbanPublishTask';

-- 规范化本地媒体存储路径，避免把静态根目录写入业务字段。
UPDATE `hg_youban_publish_media`
SET `storage_path` = CASE WHEN `storage_path` LIKE 'resource/public/%' THEN SUBSTRING(`storage_path`, CHAR_LENGTH('resource/public/') + 1) ELSE `storage_path` END,
    `updated_at` = NOW()
WHERE `storage_path` LIKE 'resource/public/%';

UPDATE `hg_youban_publish_media`
SET `poster_storage_path` = CASE WHEN `poster_storage_path` LIKE 'resource/public/%' THEN SUBSTRING(`poster_storage_path`, CHAR_LENGTH('resource/public/') + 1) ELSE `poster_storage_path` END,
    `updated_at` = NOW()
WHERE `poster_storage_path` LIKE 'resource/public/%';

UPDATE `hg_youban_publish_media`
SET `original_storage_path` = CASE WHEN `original_storage_path` LIKE 'resource/public/%' THEN SUBSTRING(`original_storage_path`, CHAR_LENGTH('resource/public/') + 1) ELSE `original_storage_path` END,
    `updated_at` = NOW()
WHERE `original_storage_path` LIKE 'resource/public/%';

UPDATE `hg_youban_publish_media`
SET `edited_storage_path` = CASE WHEN `edited_storage_path` LIKE 'resource/public/%' THEN SUBSTRING(`edited_storage_path`, CHAR_LENGTH('resource/public/') + 1) ELSE `edited_storage_path` END,
    `updated_at` = NOW()
WHERE `edited_storage_path` LIKE 'resource/public/%';
ALTER TABLE `hg_youban_publish_collect_event` ADD KEY `idx_ybp_collect_event_text_hash` (`tenant_id`,`account_id`,`text_hash`,`received_at`,`id`);
ALTER TABLE `hg_youban_publish_collect_event` ADD KEY `idx_ybp_collect_event_order` (`tenant_id`,`account_id`,`source_id`,`source_chat_id`,`source_message_id`);
ALTER TABLE `hg_youban_publish_collect_event` ADD COLUMN `material_role` varchar(16) NOT NULL DEFAULT 'pending' COMMENT '资料组角色：pending/display/verify' AFTER `source_unique_key`;
ALTER TABLE `hg_youban_publish_collect_event` ADD COLUMN `material_parent_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '验证资料所属展示事件ID' AFTER `material_role`;
ALTER TABLE `hg_youban_publish_collect_event` ADD COLUMN `material_group_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '资料组状态' AFTER `material_parent_event_id`;
ALTER TABLE `hg_youban_publish_collect_event` ADD KEY `idx_ybp_collect_event_material` (`source_id`,`source_chat_id`,`material_role`,`material_parent_event_id`,`source_message_id`);
ALTER TABLE `hg_youban_publish_collect_review` DROP COLUMN IF EXISTS `media_json`;

ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN IF NOT EXISTS `download_duration_ms` bigint(20) NOT NULL DEFAULT '0' COMMENT '下载耗时毫秒';
ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN IF NOT EXISTS `download_bytes` bigint(20) NOT NULL DEFAULT '0' COMMENT '下载字节数';
ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN IF NOT EXISTS `download_attempts` int(11) NOT NULL DEFAULT '0' COMMENT '下载尝试次数';
ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN IF NOT EXISTS `cache_hit` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否命中缓存';
ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN IF NOT EXISTS `download_error_type` varchar(64) NOT NULL DEFAULT '' COMMENT '下载错误分类';
CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_media_stat` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG账号ID',
  `source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID',
  `event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID',
  `status` varchar(32) NOT NULL DEFAULT '' COMMENT '任务状态',
  `media_total` int(11) NOT NULL DEFAULT '0' COMMENT '媒体总数',
  `success_count` int(11) NOT NULL DEFAULT '0' COMMENT '成功数',
  `failed_count` int(11) NOT NULL DEFAULT '0' COMMENT '失败数',
  `pending_count` int(11) NOT NULL DEFAULT '0' COMMENT '等待重试数',
  `cache_hit_count` int(11) NOT NULL DEFAULT '0' COMMENT '缓存命中数',
  `retry_count` int(11) NOT NULL DEFAULT '0' COMMENT '重试次数',
  `bytes` bigint(20) NOT NULL DEFAULT '0' COMMENT '字节数',
  `duration_ms` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务耗时毫秒',
  `p50_ms` bigint(20) NOT NULL DEFAULT '0' COMMENT '媒体P50耗时毫秒',
  `p95_ms` bigint(20) NOT NULL DEFAULT '0' COMMENT '媒体P95耗时毫秒',
  `throughput_mbps` decimal(18,4) NOT NULL DEFAULT '0' COMMENT '吞吐Mbps',
  `success_rate` decimal(8,5) NOT NULL DEFAULT '0' COMMENT '成功率',
  `failure_rate` decimal(8,5) NOT NULL DEFAULT '0' COMMENT '失败率',
  `error_summary_json` text COMMENT '错误分类JSON',
  `started_at` datetime DEFAULT NULL COMMENT '开始时间',
  `finished_at` datetime DEFAULT NULL COMMENT '结束时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_collect_media_stat_event` (`event_id`),
  KEY `idx_ybp_collect_media_stat_owner` (`tenant_id`,`account_id`,`tg_account_id`,`created_at`),
  KEY `idx_ybp_collect_media_stat_source` (`source_id`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集媒体性能统计';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_rule_channel` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `rule_id` bigint(20) NOT NULL DEFAULT '0',
  `channel_id` bigint(20) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_collect_rule_channel` (`rule_id`,`channel_id`),
  KEY `idx_ybp_collect_rule_channel_owner` (`tenant_id`,`account_id`,`rule_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集规则频道';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_rule_item` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `rule_id` bigint(20) NOT NULL DEFAULT '0',
  `item_type` varchar(32) NOT NULL DEFAULT '',
  `value` text,
  `replacement` text,
  `sort` int(11) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ybp_collect_rule_item_rule` (`rule_id`,`item_type`,`sort`,`id`),
  KEY `idx_ybp_collect_rule_item_owner` (`tenant_id`,`account_id`,`rule_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集规则项';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_dispatch_channel` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `dispatch_id` bigint(20) NOT NULL DEFAULT '0',
  `channel_id` bigint(20) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_collect_dispatch_channel` (`dispatch_id`,`channel_id`),
  KEY `idx_ybp_collect_dispatch_channel_owner` (`tenant_id`,`account_id`,`dispatch_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集分发频道';

-- 采集媒体统一收敛到事件媒体快照和正式媒体表
ALTER TABLE `hg_youban_publish_collect_content` DROP COLUMN IF EXISTS `media_signature`;
ALTER TABLE `hg_youban_publish_collect_content` DROP COLUMN IF EXISTS `media_json`;
DROP TABLE IF EXISTS `hg_youban_publish_collect_content_media`;
ALTER TABLE `hg_youban_publish_collect_event` ADD KEY `idx_ybp_collect_event_queue` (`tenant_id`,`account_id`,`source_id`,`status`,`processed_at`,`source_chat_id`,`source_message_id`,`id`);
ALTER TABLE `hg_youban_publish_collect_dispatch` ADD KEY `idx_ybp_collect_dispatch_dedupe` (`tenant_id`,`account_id`,`event_id`,`status`,`id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_notice` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `type` tinyint(4) NOT NULL DEFAULT '1' COMMENT '消息类型：1通知，2公告，3私信',
  `title` varchar(255) NOT NULL DEFAULT '' COMMENT '消息标题',
  `content` text COMMENT '消息内容',
  `tag` bigint(20) NOT NULL DEFAULT '0' COMMENT '消息标签',
  `receiver` text COMMENT '私信接收账号ID JSON',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `sort` bigint(20) NOT NULL DEFAULT '0' COMMENT '排序',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  `publish_at` datetime DEFAULT NULL COMMENT '发布时间',
  `expire_at` datetime DEFAULT NULL COMMENT '过期时间',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_notice_list` (`status`,`type`,`sort`,`id`),
  KEY `idx_ybp_notice_time` (`status`,`publish_at`,`expire_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架通知公告';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_notice_read` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `notice_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '公告ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID',
  `clicks` int(11) NOT NULL DEFAULT '0' COMMENT '阅读次数',
  `created_at` datetime DEFAULT NULL COMMENT '首次阅读时间',
  `updated_at` datetime DEFAULT NULL COMMENT '最近阅读时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_notice_read_account` (`notice_id`,`account_id`),
  KEY `idx_ybp_notice_read_account` (`account_id`,`notice_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架通知公告已读记录';

ALTER TABLE `hg_youban_publish_profile_state` ADD COLUMN IF NOT EXISTS `publish_operation_no` varchar(128) NOT NULL DEFAULT '' COMMENT '当前上架操作号';
ALTER TABLE `hg_youban_publish_profile_state` ADD COLUMN IF NOT EXISTS `publish_task_status` varchar(32) NOT NULL DEFAULT '' COMMENT '当前上架任务状态';
ALTER TABLE `hg_youban_publish_profile_state` ADD COLUMN IF NOT EXISTS `publish_task_updated_at` datetime DEFAULT NULL COMMENT '上架任务状态更新时间';
ALTER TABLE `hg_youban_publish_profile_state` ADD INDEX `idx_ybp_profile_state_publish_active` (`publish_task_status`,`publish_task_updated_at`,`profile_id`);

ALTER TABLE `hg_youban_publish_channel` ADD COLUMN IF NOT EXISTS `anti_scan_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '频道防扫图开关';
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN IF NOT EXISTS `text_obfuscation_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '频道文本混淆开关';
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN IF NOT EXISTS `auto_delete_enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '频道自动删除开关';
UPDATE `hg_youban_publish_channel`
SET `auto_delete_enabled` = 1
WHERE `auto_delete_enabled` = 0
  AND `publish_direction` = 'up'
  AND `status` = 1
  AND `deleted_at` IS NULL;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN IF NOT EXISTS `must_send` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否每次推送必发' AFTER `media_type`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN IF NOT EXISTS `processing_status` varchar(16) NOT NULL DEFAULT 'ready' COMMENT '媒体处理状态：uploaded/processing/ready/failed';
ALTER TABLE `hg_youban_publish_media` ADD COLUMN IF NOT EXISTS `processing_error` text COMMENT '媒体处理错误';
ALTER TABLE `hg_youban_publish_media` ADD COLUMN IF NOT EXISTS `processing_started_at` datetime DEFAULT NULL COMMENT '媒体处理开始时间';
UPDATE `hg_youban_publish_media` SET `must_send` = 0 WHERE EXISTS (SELECT 1 FROM `information_schema`.`columns` WHERE `table_schema` = DATABASE() AND `table_name` = 'hg_youban_publish_media' AND `column_name` = 'must_send' AND `column_default` = '1');
ALTER TABLE `hg_youban_publish_media` ALTER COLUMN `must_send` SET DEFAULT '0';
CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_feature_permission` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT '0',`feature_code` varchar(64) NOT NULL DEFAULT '',`status` tinyint NOT NULL DEFAULT '2',`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_tenant_feature` (`tenant_id`,`feature_code`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN IF NOT EXISTS `send_phase` varchar(32) NOT NULL DEFAULT '' COMMENT '发送阶段' AFTER `dispatch_count`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN IF NOT EXISTS `reconcile_count` int(11) NOT NULL DEFAULT '0' COMMENT '对账次数' AFTER `send_phase`;
DELETE older FROM `hg_youban_publish_tg_message` older INNER JOIN `hg_youban_publish_tg_message` newer ON older.`job_id` = newer.`job_id` AND older.`tg_message_id` = newer.`tg_message_id` AND older.`id` < newer.`id`;
ALTER TABLE `hg_youban_publish_tg_message` ADD UNIQUE INDEX `uk_ybp_tg_message_job_message` (`job_id`,`tg_message_id`);
ALTER TABLE `hg_youban_publish_tg_channel_stat` MODIFY COLUMN `last_error_message` text NOT NULL COMMENT '最后错误';
ALTER TABLE `hg_youban_publish_tg_bot_stat` MODIFY COLUMN `last_error_message` text NOT NULL COMMENT '最后错误';
ALTER TABLE `hg_youban_publish_tg_job` ADD INDEX `idx_ybp_tg_job_tenant_channel_status` (`tenant_id`,`channel_id`,`status`,`id`);
CREATE TABLE IF NOT EXISTS `hg_youban_publish_media_phash_alias_bucket` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT, `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0', `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `media_id` bigint(20) NOT NULL DEFAULT '0', `media_type` varchar(16) NOT NULL DEFAULT '',
  `fingerprint_key` varchar(64) NOT NULL DEFAULT '', `hash_value` varchar(64) NOT NULL DEFAULT '',
  `bucket_pos` smallint(6) NOT NULL DEFAULT '0', `bucket_value` int(11) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL, `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_phash_alias_media_key_pos` (`media_id`,`fingerprint_key`,`bucket_pos`),
  KEY `idx_ybp_phash_alias_search` (`tenant_id`,`media_type`,`bucket_pos`,`bucket_value`,`account_id`,`profile_id`,`media_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='防扫图媒体搜索指纹';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_cms_app` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `app_id` varchar(64) NOT NULL DEFAULT '',
  `app_secret` varchar(255) NOT NULL DEFAULT '',
  `name` varchar(128) NOT NULL DEFAULT '',
  `base_url` varchar(512) NOT NULL DEFAULT '',
  `instance_id` varchar(128) DEFAULT NULL,
  `enroll_hash` varchar(64) NOT NULL DEFAULT '',
  `source_ip` varchar(64) NOT NULL DEFAULT '',
  `cms_version` varchar(64) NOT NULL DEFAULT '',
  `last_heartbeat_at` datetime DEFAULT NULL,
  `status` tinyint(4) NOT NULL DEFAULT '1',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_cms_app_app_id` (`app_id`),
  UNIQUE KEY `uk_ybp_cms_app_instance_id` (`instance_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='XC-CMS开放应用';

ALTER TABLE `hg_youban_publish_cms_app`
  ADD COLUMN IF NOT EXISTS `instance_id` varchar(128) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS `enroll_hash` varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS `source_ip` varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS `cms_version` varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS `last_heartbeat_at` datetime DEFAULT NULL,
  ADD UNIQUE KEY IF NOT EXISTS `uk_ybp_cms_app_instance_id` (`instance_id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_cms_binding_code` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `app_id` varchar(64) NOT NULL DEFAULT '',
  `code_hash` varchar(64) NOT NULL DEFAULT '',
  `code_hint` varchar(16) NOT NULL DEFAULT '',
  `version` int(11) NOT NULL DEFAULT '1',
  `status` tinyint(4) NOT NULL DEFAULT '1',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_cms_binding_code_app` (`app_id`),
  UNIQUE KEY `uk_ybp_cms_binding_code_hash` (`code_hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='XC-CMS租户绑定码';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_cms_tenant_binding` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `app_id` varchar(64) NOT NULL DEFAULT '',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `code_version` int(11) NOT NULL DEFAULT '1',
  `status` varchar(16) NOT NULL DEFAULT 'pending',
  `reason` varchar(500) NOT NULL DEFAULT '',
  `requested_at` datetime DEFAULT NULL,
  `reviewed_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_cms_tenant_binding` (`app_id`,`tenant_id`),
  KEY `idx_ybp_cms_binding_tenant_status` (`tenant_id`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='XC-CMS租户绑定关系';
