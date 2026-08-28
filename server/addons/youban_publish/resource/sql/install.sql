CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '租户名称',
  `contact_name` varchar(128) NOT NULL DEFAULT '' COMMENT '联系人',
  `contact_phone` varchar(64) NOT NULL DEFAULT '' COMMENT '联系电话',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tenant_status` (`status`,`id`),
  KEY `idx_ybp_tenant_remark` (`remark`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架租户';
CREATE TABLE IF NOT EXISTS `hg_youban_publish_merchant` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '商家名称',
  `contact_name` varchar(128) NOT NULL DEFAULT '' COMMENT '联系人',
  `contact_phone` varchar(64) NOT NULL DEFAULT '' COMMENT '联系电话',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_merchant_status` (`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架商家兼容表';
INSERT INTO `hg_youban_publish_tenant` (`id`, `name`, `contact_name`, `contact_phone`, `remark`, `status`, `created_by`, `updated_by`, `deleted_by`, `created_at`, `updated_at`, `deleted_at`)
SELECT m.`id`, m.`name`, m.`contact_name`, m.`contact_phone`, m.`remark`, m.`status`, m.`created_by`, m.`updated_by`, m.`deleted_by`, m.`created_at`, m.`updated_at`, m.`deleted_at`
FROM `hg_youban_publish_merchant` m
WHERE NOT EXISTS (SELECT 1 FROM `hg_youban_publish_tenant` t WHERE t.`id` = m.`id`);
CREATE TABLE IF NOT EXISTS `hg_youban_publish_account` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `parent_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '父账号ID',
  `account_type` varchar(32) NOT NULL DEFAULT 'uploader' COMMENT '账号类型',
  `nickname` varchar(128) NOT NULL DEFAULT '' COMMENT '昵称',
  `username` varchar(128) NOT NULL DEFAULT '' COMMENT '用户名',
  `password_hash` varchar(128) NOT NULL DEFAULT '' COMMENT '密码hash',
  `salt` varchar(16) NOT NULL DEFAULT '' COMMENT '密码盐',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `avatar_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '头像地址',
  `contact_telegram` varchar(128) NOT NULL DEFAULT '' COMMENT '联系TG',
  `contact_wechat` varchar(128) NOT NULL DEFAULT '' COMMENT '联系微信',
  `contact_phone` varchar(64) NOT NULL DEFAULT '' COMMENT '联系电话',
  `contact_other` text COMMENT '其他联系方式',
  `follow_approval_required` tinyint(1) NOT NULL DEFAULT '0' COMMENT '关注我是否需要审批',
  `public_follow_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否公开关注',
  `daily_publish_limit` int(11) NOT NULL DEFAULT '0' COMMENT '每日上架额度',
  `can_direct_publish` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否可直接发布',
  `allowed_channel_json` text COMMENT '可发布频道JSON',
  `allowed_region_json` text COMMENT '可发布地区JSON',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_account_tenant` (`tenant_id`,`account_type`,`status`),
  KEY `idx_ybp_account_username` (`account_type`,`username`,`tenant_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架账号';
ALTER TABLE `hg_youban_publish_account` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `id`, ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
ALTER TABLE `hg_youban_publish_account` ADD COLUMN `password_hash` varchar(128) NOT NULL DEFAULT '' COMMENT '密码hash' AFTER `username`, ADD COLUMN `salt` varchar(16) NOT NULL DEFAULT '' COMMENT '密码盐' AFTER `password_hash`;
ALTER TABLE `hg_youban_publish_account` ADD COLUMN `avatar_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '头像地址' AFTER `telegram_username`, ADD COLUMN `contact_telegram` varchar(128) NOT NULL DEFAULT '' COMMENT '联系TG' AFTER `avatar_url`, ADD COLUMN `contact_wechat` varchar(128) NOT NULL DEFAULT '' COMMENT '联系微信' AFTER `contact_telegram`, ADD COLUMN `contact_phone` varchar(64) NOT NULL DEFAULT '' COMMENT '联系电话' AFTER `contact_wechat`, ADD COLUMN `contact_other` text COMMENT '其他联系方式' AFTER `contact_phone`, ADD COLUMN `follow_approval_required` tinyint(1) NOT NULL DEFAULT '0' COMMENT '关注我是否需要审批' AFTER `contact_other`, ADD COLUMN `public_follow_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否公开关注' AFTER `follow_approval_required`;
UPDATE `hg_youban_publish_account` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;
ALTER TABLE `hg_youban_publish_account` ADD KEY `idx_ybp_account_tenant` (`tenant_id`,`account_type`,`status`);
ALTER TABLE `hg_youban_publish_account` ADD KEY `idx_ybp_account_username` (`account_type`,`username`,`tenant_id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_source` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '所属账号ID',
  `source_type` varchar(32) NOT NULL DEFAULT 'bot' COMMENT '来源类型：account/bot/follow',
  `title` varchar(255) NOT NULL DEFAULT '' COMMENT '采集源名称',
  `source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '来源频道/群聊ID',
  `source_username` varchar(128) NOT NULL DEFAULT '' COMMENT '来源用户名',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '协议号ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '机器人ID',
  `bot_collect_scope` varchar(16) NOT NULL DEFAULT 'chat' COMMENT 'Bot采集范围：chat/private',
  `follow_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '关注账号ID',
  `collect_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否开启采集',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `event_total` bigint(20) NOT NULL DEFAULT '0' COMMENT '事件总数',
  `success_total` bigint(20) NOT NULL DEFAULT '0' COMMENT '成功数',
  `failed_total` bigint(20) NOT NULL DEFAULT '0' COMMENT '失败数',
  `last_event_at` datetime DEFAULT NULL COMMENT '最后事件时间',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_collect_source_owner` (`tenant_id`,`account_id`,`source_type`,`status`),
  KEY `idx_ybp_collect_source_bot_chat` (`bot_id`,`source_chat_id`),
  KEY `idx_ybp_collect_source_bot_scope` (`bot_id`,`bot_collect_scope`,`source_chat_id`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集源';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_rule` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '所属账号ID',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '规则名称',
  `global_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否全局应用',
  `review_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否需要审核',
  `dedupe_enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否图文去重',
  `dedupe_days` int(11) NOT NULL DEFAULT '7' COMMENT '去重天数',
  `full_match_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '全量匹配',
  `block_link` tinyint(1) NOT NULL DEFAULT '1' COMMENT '屏蔽链接',
  `block_username` tinyint(1) NOT NULL DEFAULT '1' COMMENT '屏蔽用户名',
  `block_plain_text` tinyint(1) NOT NULL DEFAULT '1' COMMENT '屏蔽纯文本',
  `header_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否启用前置文案',
  `header_markdown` text COMMENT '前置Markdown文案',
  `footer_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否启用后置文案',
  `footer_markdown` text COMMENT '后置Markdown文案',
  `sort` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_collect_rule_owner` (`tenant_id`,`account_id`,`status`),
  KEY `idx_ybp_collect_rule_global` (`tenant_id`,`global_enabled`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集规则';

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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_source_rule` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID',
  `rule_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '规则ID',
  `sort` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_collect_source_rule` (`source_id`,`rule_id`),
  KEY `idx_ybp_collect_source_rule_source` (`tenant_id`,`source_id`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集源规则';

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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_event` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '所属账号ID',
  `source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID',
  `source_type` varchar(32) NOT NULL DEFAULT '' COMMENT '来源类型',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '机器人ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '协议号ID',
  `source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '来源频道/群聊ID',
  `source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '来源消息ID',
  `source_grouped_id` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体组ID',
  `source_unique_key` varchar(255) NOT NULL DEFAULT '' COMMENT '来源唯一键',
  `material_role` varchar(16) NOT NULL DEFAULT 'pending' COMMENT '资料组角色：pending/display/verify',
  `material_parent_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '验证资料所属展示事件ID',
  `material_group_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '资料组状态',
  `raw_text` text COMMENT '原始文本',
  `media_count` int(11) NOT NULL DEFAULT '0' COMMENT '媒体数量',
  `text_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '文本哈希',
  `dedupe_key` varchar(128) NOT NULL DEFAULT '' COMMENT '去重键',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `error_message` text COMMENT '错误信息',
  `received_at` datetime DEFAULT NULL COMMENT '接收时间',
  `processed_at` datetime DEFAULT NULL COMMENT '处理时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_collect_event_unique` (`source_unique_key`),
  KEY `idx_ybp_collect_event_source` (`tenant_id`,`source_id`,`status`,`id`),
  KEY `idx_ybp_collect_event_chat` (`source_chat_id`,`source_message_id`),
  KEY `idx_ybp_collect_event_order` (`tenant_id`,`account_id`,`source_id`,`source_chat_id`,`source_message_id`),
  KEY `idx_ybp_collect_event_material` (`source_id`,`source_chat_id`,`material_role`,`material_parent_event_id`,`source_message_id`),
  KEY `idx_ybp_collect_event_dedupe` (`tenant_id`,`dedupe_key`,`created_at`),
  KEY `idx_ybp_collect_event_text_hash` (`tenant_id`,`account_id`,`text_hash`,`received_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集事件';

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
  `source_kind` varchar(32) NOT NULL DEFAULT '' COMMENT 'TG来源媒体类型',
  `source_media_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG媒体ID',
  `source_access_hash` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG媒体访问哈希',
  `source_file_reference` blob COMMENT 'TG文件引用',
  `source_thumb_size` varchar(32) NOT NULL DEFAULT '' COMMENT 'TG缩略图规格',
  `source_mime_type` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG媒体MIME',
  `source_dc_id` int(11) NOT NULL DEFAULT '0' COMMENT 'TG数据中心ID',
  `source_size` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG媒体大小',
  `file_md5` varchar(64) NOT NULL DEFAULT '' COMMENT '文件MD5',
  `file_phash` varchar(128) NOT NULL DEFAULT '' COMMENT '图片感知哈希',
  `meta_json` text COMMENT '仅用于调试的媒体元数据',
  `sort_index` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `cache_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '缓存状态',
  `download_duration_ms` bigint(20) NOT NULL DEFAULT '0' COMMENT '下载耗时毫秒',
  `download_bytes` bigint(20) NOT NULL DEFAULT '0' COMMENT '下载字节数',
  `download_attempts` int(11) NOT NULL DEFAULT '0' COMMENT '下载尝试次数',
  `cache_hit` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否命中缓存',
  `download_error_type` varchar(64) NOT NULL DEFAULT '' COMMENT '下载错误分类',
  `next_retry_at` datetime DEFAULT NULL COMMENT '下次重试时间',
  `error_message` text COMMENT '错误信息',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_collect_event_media_event` (`event_id`,`sort_index`,`id`),
  KEY `idx_ybp_collect_event_media_owner` (`tenant_id`,`source_id`,`cache_status`,`id`),
  KEY `idx_ybp_collect_event_media_source` (`source_chat_id`,`source_message_id`,`source_media_key`),
  KEY `idx_ybp_collect_event_media_file` (`source_file_id`),
  KEY `idx_ybp_collect_event_media_cache` (`cache_status`,`updated_at`,`id`),
  KEY `idx_ybp_collect_event_media_event_retry` (`event_id`,`cache_status`,`next_retry_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集事件媒体';

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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_content` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '所属账号ID',
  `first_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '首次事件ID',
  `last_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '最近事件ID',
  `source_type` varchar(32) NOT NULL DEFAULT '' COMMENT '来源类型',
  `raw_text` text COMMENT '原始文本',
  `normalized_text` text COMMENT '归一化文本',
  `media_count` int(11) NOT NULL DEFAULT '0' COMMENT '媒体数量',
  `text_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '文本哈希',
  `dedupe_key` varchar(128) NOT NULL DEFAULT '' COMMENT '去重键',
  `duplicate_total` int(11) NOT NULL DEFAULT '0' COMMENT '重复命中数',
  `status` varchar(32) NOT NULL DEFAULT 'active' COMMENT '状态',
  `first_seen_at` datetime DEFAULT NULL COMMENT '首次出现时间',
  `previous_seen_at` datetime DEFAULT NULL COMMENT '上次出现时间',
  `last_seen_at` datetime DEFAULT NULL COMMENT '最近出现时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_collect_content_dedupe` (`tenant_id`,`account_id`,`dedupe_key`),
  KEY `idx_ybp_collect_content_text` (`tenant_id`,`account_id`,`text_hash`,`first_seen_at`),
  KEY `idx_ybp_collect_content_seen` (`tenant_id`,`account_id`,`last_seen_at`),
  KEY `idx_ybp_collect_content_previous` (`tenant_id`,`account_id`,`previous_seen_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集内容池';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_review` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '所属账号ID',
  `source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID',
  `rule_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '规则ID',
  `event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID',
  `dispatch_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '分发记录ID',
  `raw_text` text COMMENT '原始文本',
  `media_count` int(11) NOT NULL DEFAULT '0' COMMENT '媒体数量',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '审核状态',
  `review_reason` varchar(500) NOT NULL DEFAULT '' COMMENT '审核原因',
  `reviewed_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '审核人',
  `reviewed_at` datetime DEFAULT NULL COMMENT '审核时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_collect_review_owner` (`tenant_id`,`account_id`,`status`,`id`),
  KEY `idx_ybp_collect_review_event` (`event_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集审核';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_dispatch` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '所属账号ID',
  `source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID',
  `rule_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '规则ID',
  `event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID',
  `review_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '审核ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架任务ID',
  `match_json` longtext COMMENT '命中详情JSON',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `error_message` text COMMENT '错误信息',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `finished_at` datetime DEFAULT NULL COMMENT '完成时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_collect_dispatch_event` (`event_id`,`rule_id`),
  KEY `idx_ybp_collect_dispatch_owner` (`tenant_id`,`account_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集分发';

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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_account_follow` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `follower_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '关注人账号ID',
  `following_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '被关注账号ID',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `approval_required_snapshot` tinyint(1) NOT NULL DEFAULT '0' COMMENT '申请时是否需要审批',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `blocked_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '拉黑人',
  `approved_at` datetime DEFAULT NULL COMMENT '通过时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_account_follow_pair` (`follower_account_id`,`following_account_id`),
  KEY `idx_ybp_account_follow_follower` (`tenant_id`,`follower_account_id`,`status`),
  KEY `idx_ybp_account_follow_following` (`tenant_id`,`following_account_id`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴账号关注';
CREATE TABLE IF NOT EXISTS `hg_youban_publish_profile_state` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `customer_remark` text,
  `anti_scan_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `publish_at` datetime DEFAULT NULL,
  `publish_operation_no` varchar(128) NOT NULL DEFAULT '' COMMENT '当前上架操作号',
  `publish_task_status` varchar(32) NOT NULL DEFAULT '' COMMENT '当前上架任务状态',
  `publish_task_updated_at` datetime DEFAULT NULL COMMENT '上架任务状态更新时间',
  `created_by` bigint(20) NOT NULL DEFAULT '0',
  `updated_by` bigint(20) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_profile_state_profile` (`profile_id`),
  KEY `idx_ybp_profile_state_owner` (`tenant_id`,`account_id`,`profile_id`),
  KEY `idx_ybp_profile_state_publish_active` (`publish_task_status`,`publish_task_updated_at`,`profile_id`)
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
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `job_id` bigint(20) NOT NULL DEFAULT '0',
  `task_id` bigint(20) NOT NULL DEFAULT '0',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `channel_id` bigint(20) NOT NULL DEFAULT '0',
  `bot_id` bigint(20) NOT NULL DEFAULT '0',
  `operation_no` varchar(128) NOT NULL DEFAULT '',
  `target_chat_id` varchar(128) NOT NULL DEFAULT '',
  `action` varchar(32) NOT NULL DEFAULT 'profile_publish',
  `status` varchar(16) NOT NULL DEFAULT 'success',
  `message` varchar(255) NOT NULL DEFAULT '',
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_success_record_job` (`job_id`),
  KEY `idx_ybp_success_record_owner` (`tenant_id`,`account_id`,`id`),
  KEY `idx_ybp_success_record_profile` (`profile_id`,`id`),
  KEY `idx_ybp_success_record_monitor` (`created_at`,`status`,`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='上架成功发布记录';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_full_push_batch` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `batch_no` varchar(128) NOT NULL,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `channel_id` bigint(20) NOT NULL DEFAULT '0',
  `requested_by` bigint(20) NOT NULL DEFAULT '0',
  `snapshot_max_profile_id` bigint(20) NOT NULL DEFAULT '0',
  `cursor_profile_id` bigint(20) NOT NULL DEFAULT '0',
  `total_count` int(11) NOT NULL DEFAULT '0',
  `queued_count` int(11) NOT NULL DEFAULT '0',
  `retry_count` int(11) NOT NULL DEFAULT '0',
  `status` varchar(16) NOT NULL DEFAULT 'pending',
  `active_key` varchar(64) DEFAULT NULL,
  `error_message` text,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `finished_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_full_push_batch_no` (`batch_no`),
  UNIQUE KEY `uk_ybp_full_push_active` (`active_key`),
  KEY `idx_ybp_full_push_schedule` (`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='频道全量推送批次';

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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_import_task` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID',
  `source_name` varchar(64) NOT NULL DEFAULT 'lyy_cms' COMMENT '来源名称',
  `base_url` varchar(255) NOT NULL DEFAULT '' COMMENT '旧站域名',
  `server_ip` varchar(64) NOT NULL DEFAULT '' COMMENT '旧站服务器IP，DNS失效时使用',
  `username` varchar(128) NOT NULL DEFAULT '' COMMENT '旧站账号',
  `password_cipher` varchar(512) NOT NULL DEFAULT '' COMMENT '旧站密码密文',
  `cookie_cipher` text COMMENT '旧站Cookie密文',
  `limit_count` int(11) NOT NULL DEFAULT '0' COMMENT '测试采集数量',
  `per_page` int(11) NOT NULL DEFAULT '12' COMMENT '每页数量',
  `proxy_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否启用代理',
  `proxy_pool` text COMMENT '代理池',
  `media_concurrency` int(11) NOT NULL DEFAULT '4' COMMENT '媒体并发数',
  `channel_id_json` text COMMENT '匹配频道ID JSON',
  `tg_start_at` datetime DEFAULT NULL COMMENT 'TG匹配开始时间',
  `tg_end_at` datetime DEFAULT NULL COMMENT 'TG匹配结束时间',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '任务状态',
  `stage` varchar(32) NOT NULL DEFAULT 'created' COMMENT '执行阶段',
  `progress_total` int(11) NOT NULL DEFAULT '0' COMMENT '总进度',
  `progress_done` int(11) NOT NULL DEFAULT '0' COMMENT '已完成进度',
  `page_total` int(11) NOT NULL DEFAULT '0' COMMENT '总页数',
  `page_done` int(11) NOT NULL DEFAULT '0' COMMENT '已完成页数',
  `item_total` int(11) NOT NULL DEFAULT '0' COMMENT '资料总数',
  `item_done` int(11) NOT NULL DEFAULT '0' COMMENT '已处理资料数',
  `imported` int(11) NOT NULL DEFAULT '0' COMMENT '导入数量',
  `duplicate` int(11) NOT NULL DEFAULT '0' COMMENT '重复数量',
  `media_total` int(11) NOT NULL DEFAULT '0' COMMENT '媒体总数',
  `media_done` int(11) NOT NULL DEFAULT '0' COMMENT '已处理媒体数',
  `media_imported` int(11) NOT NULL DEFAULT '0' COMMENT '媒体导入数量',
  `tg_total` int(11) NOT NULL DEFAULT '0' COMMENT 'TG消息总数',
  `tg_done` int(11) NOT NULL DEFAULT '0' COMMENT 'TG已处理数',
  `tg_matched` int(11) NOT NULL DEFAULT '0' COMMENT 'TG匹配数量',
  `last_source_note_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '最近旧站资料ID',
  `error_message` text COMMENT '错误信息',
  `result_json` longtext COMMENT '执行结果JSON',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `started_at` datetime DEFAULT NULL COMMENT '开始时间',
  `finished_at` datetime DEFAULT NULL COMMENT '结束时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_import_task_scope` (`tenant_id`,`account_id`,`status`,`id`),
  KEY `idx_ybp_import_task_status` (`status`,`updated_at`),
  KEY `idx_ybp_import_task_source` (`source_name`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架旧站导入任务';

ALTER TABLE `hg_youban_publish_import_task` ADD COLUMN `server_ip` varchar(64) NOT NULL DEFAULT '' COMMENT '旧站服务器IP，DNS失效时使用' AFTER `base_url`;
ALTER TABLE `hg_youban_publish_import_task` ADD COLUMN `cookie_cipher` text COMMENT '旧站Cookie密文' AFTER `password_cipher`;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_import_run` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '导入任务ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID',
  `source_name` varchar(64) NOT NULL DEFAULT 'lyy_cms' COMMENT '来源名称',
  `base_url` varchar(255) NOT NULL DEFAULT '' COMMENT '旧站域名',
  `username` varchar(128) NOT NULL DEFAULT '' COMMENT '旧站账号',
  `run_type` varchar(32) NOT NULL DEFAULT 'import' COMMENT '执行类型',
  `import_mode` varchar(32) NOT NULL DEFAULT 'incremental' COMMENT '导入方式',
  `scan_mode` varchar(32) NOT NULL DEFAULT 'recent' COMMENT '扫描范围',
  `recent_count` int(11) NOT NULL DEFAULT '100' COMMENT '最近数量',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `stage` varchar(32) NOT NULL DEFAULT 'created' COMMENT '阶段',
  `progress_total` int(11) NOT NULL DEFAULT '0' COMMENT '总进度',
  `progress_done` int(11) NOT NULL DEFAULT '0' COMMENT '已完成进度',
  `page_total` int(11) NOT NULL DEFAULT '0' COMMENT '总页数',
  `page_done` int(11) NOT NULL DEFAULT '0' COMMENT '已完成页数',
  `item_total` int(11) NOT NULL DEFAULT '0' COMMENT '资料总数',
  `item_done` int(11) NOT NULL DEFAULT '0' COMMENT '已处理资料数',
  `imported` int(11) NOT NULL DEFAULT '0' COMMENT '导入数量',
  `duplicate` int(11) NOT NULL DEFAULT '0' COMMENT '重复数量',
  `media_total` int(11) NOT NULL DEFAULT '0' COMMENT '媒体总数',
  `media_done` int(11) NOT NULL DEFAULT '0' COMMENT '已处理媒体数',
  `media_imported` int(11) NOT NULL DEFAULT '0' COMMENT '媒体导入数量',
  `media_missing_storage` int(11) NOT NULL DEFAULT '0' COMMENT '未迁移到当前存储媒体数',
  `tg_total` int(11) NOT NULL DEFAULT '0' COMMENT 'TG消息总数',
  `tg_done` int(11) NOT NULL DEFAULT '0' COMMENT 'TG已处理数',
  `tg_matched` int(11) NOT NULL DEFAULT '0' COMMENT 'TG匹配数量',
  `error_message` text COMMENT '错误信息',
  `params_json` longtext COMMENT '执行参数JSON',
  `result_json` longtext COMMENT '执行结果JSON',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `started_at` datetime DEFAULT NULL COMMENT '开始时间',
  `finished_at` datetime DEFAULT NULL COMMENT '结束时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_import_run_scope` (`tenant_id`,`account_id`,`status`,`id`),
  KEY `idx_ybp_import_run_task` (`task_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架旧站导入执行记录';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_import_run_log` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `run_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '执行记录ID',
  `level` varchar(16) NOT NULL DEFAULT 'info' COMMENT '日志级别',
  `stage` varchar(32) NOT NULL DEFAULT '' COMMENT '阶段',
  `message` text COMMENT '消息',
  `context` longtext COMMENT '上下文JSON',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_import_run_log_run` (`run_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架旧站导入执行日志';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_material_import_task` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料归属账号ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG协议账号ID',
  `source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '来源频道/群聊ID',
  `source_title` varchar(255) NOT NULL DEFAULT '' COMMENT '来源频道/群聊名称',
  `source_username` varchar(128) NOT NULL DEFAULT '' COMMENT '来源用户名',
  `channel_id_json` text COMMENT '导入资料默认上架频道ID JSON',
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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_material_import_group_media` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '导入任务ID',
  `group_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '导入分组ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `purpose` varchar(16) NOT NULL DEFAULT 'display' COMMENT '媒体用途',
  `media_type` varchar(32) NOT NULL DEFAULT '' COMMENT '媒体类型',
  `sort_index` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `source_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'TG来源文件ID',
  `file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '访问地址',
  `storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '存储路径',
  `poster_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '封面地址',
  `source_kind` varchar(32) NOT NULL DEFAULT '' COMMENT 'TG来源媒体类型',
  `source_media_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG媒体ID',
  `source_access_hash` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG访问哈希',
  `source_file_reference` blob COMMENT 'TG文件引用',
  `source_thumb_size` varchar(32) NOT NULL DEFAULT '' COMMENT 'TG缩略图规格',
  `source_mime_type` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG媒体MIME',
  `source_dc_id` int(11) NOT NULL DEFAULT '0' COMMENT 'TG数据中心ID',
  `source_size` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG媒体大小',
  `file_md5` varchar(64) NOT NULL DEFAULT '' COMMENT '文件MD5',
  `file_phash` varchar(128) NOT NULL DEFAULT '' COMMENT '图片感知哈希',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_material_import_group_media_group` (`group_id`,`sort_index`,`id`),
  KEY `idx_ybp_material_import_group_media_task` (`task_id`,`group_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG资料导入分组媒体';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_import_match_run` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `import_run_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '导入执行记录ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `stage` varchar(32) NOT NULL DEFAULT 'created' COMMENT '阶段',
  `channel_id_json` text COMMENT '频道ID JSON',
  `scan_days` int(11) NOT NULL DEFAULT '180' COMMENT '扫描天数',
  `threshold` int(11) NOT NULL DEFAULT '80' COMMENT '自动匹配阈值',
  `profile_total` int(11) NOT NULL DEFAULT '0' COMMENT '资料总数',
  `profile_done` int(11) NOT NULL DEFAULT '0' COMMENT '已处理资料',
  `candidate_total` int(11) NOT NULL DEFAULT '0' COMMENT '候选总数',
  `auto_matched` int(11) NOT NULL DEFAULT '0' COMMENT '自动匹配数',
  `manual_pending` int(11) NOT NULL DEFAULT '0' COMMENT '待人工确认数',
  `confirmed` int(11) NOT NULL DEFAULT '0' COMMENT '已确认数',
  `skipped` int(11) NOT NULL DEFAULT '0' COMMENT '已跳过数',
  `error_message` text COMMENT '错误信息',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `finished_at` datetime DEFAULT NULL COMMENT '完成时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_import_match_run_import` (`import_run_id`,`id`),
  KEY `idx_ybp_import_match_run_scope` (`tenant_id`,`account_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架导入TG匹配执行';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_import_match_item` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `match_run_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '匹配执行ID',
  `import_run_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '导入执行记录ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID',
  `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID',
  `display_group_key` varchar(128) NOT NULL DEFAULT '' COMMENT '展示资料TG组',
  `verify_group_key` varchar(128) NOT NULL DEFAULT '' COMMENT '验证资料TG组',
  `display_score` int(11) NOT NULL DEFAULT '0' COMMENT '展示资料分数',
  `verify_score` int(11) NOT NULL DEFAULT '0' COMMENT '验证资料分数',
  `total_score` int(11) NOT NULL DEFAULT '0' COMMENT '总分',
  `match_status` varchar(32) NOT NULL DEFAULT 'manual_pending' COMMENT '匹配状态',
  `match_mode` varchar(32) NOT NULL DEFAULT '' COMMENT '匹配方式',
  `reason_json` text COMMENT '匹配原因JSON',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_import_match_item_scope` (`match_run_id`,`profile_id`,`channel_id`),
  KEY `idx_ybp_import_match_item_profile` (`profile_id`,`task_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架导入TG匹配明细';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_import_match_candidate` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `match_run_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '匹配执行ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID',
  `group_key` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体组键',
  `media_group_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG媒体组ID',
  `first_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '首条消息ID',
  `last_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '末条消息ID',
  `message_date` datetime DEFAULT NULL COMMENT '消息时间',
  `caption_text` text COMMENT '文案',
  `media_count` int(11) NOT NULL DEFAULT '0' COMMENT '媒体数量',
  `media_types` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体类型',
  `preview_json` text COMMENT '预览JSON',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_import_match_candidate_group` (`match_run_id`,`channel_id`,`group_key`),
  KEY `idx_ybp_import_match_candidate_channel` (`match_run_id`,`channel_id`,`message_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架导入TG候选组';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_account_setting` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `enable_suffix` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否启用发送后缀',
  `suffix_content` text COMMENT '发送后缀内容',
  `enable_title_mark` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否启用编号标识',
  `mark_mode` varchar(16) NOT NULL DEFAULT 'nickname' COMMENT '标识模式',
  `number_source` varchar(16) NOT NULL DEFAULT 'sequence' COMMENT '编号来源',
  `custom_mark_text` varchar(128) NOT NULL DEFAULT '' COMMENT '自定义标识文字',
  `mark_position` varchar(16) NOT NULL DEFAULT 'bottom' COMMENT '显示位置',
	`shared_resource_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '上架账号是否可管理租户共享资料',
	`telegram_binding_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '上架账号是否可绑定并使用Telegram',
  `default_recycle_days` int(11) NOT NULL DEFAULT '0' COMMENT '默认循环天数',
  `cycle_publish_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架',
  `cycle_publish_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环上架天数',
  `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环上架时间',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_account_setting_account` (`tenant_id`,`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架账号推送设置';
ALTER TABLE `hg_youban_publish_account_setting` ADD COLUMN `cycle_publish_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架' AFTER `default_recycle_days`;
ALTER TABLE `hg_youban_publish_account_setting` ADD COLUMN `cycle_publish_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环上架天数' AFTER `cycle_publish_enabled`;
ALTER TABLE `hg_youban_publish_account_setting` ADD COLUMN `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环上架时间' AFTER `cycle_publish_days`;
ALTER TABLE `hg_youban_publish_account_setting` ADD COLUMN `publish_config_json` text NOT NULL COMMENT '账号级推送配置JSON';
ALTER TABLE `hg_youban_publish_account_setting` ADD COLUMN IF NOT EXISTS `shared_resource_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '上架账号是否可管理租户共享资料';
ALTER TABLE `hg_youban_publish_account_setting` ADD COLUMN IF NOT EXISTS `telegram_binding_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '上架账号是否可绑定并使用Telegram';

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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_media` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `task_id` bigint(20) DEFAULT NULL COMMENT '采集任务ID，普通资料媒体为空',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `attachment_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'HotGo附件ID',
  `original_attachment_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '原始HotGo附件ID',
  `edited_attachment_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '编辑后HotGo附件ID',
  `media_type` varchar(16) NOT NULL DEFAULT 'image' COMMENT '媒体类型',
  `must_send` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否每次推送必发',
  `purpose` varchar(16) NOT NULL DEFAULT 'display' COMMENT '用途：display展示 verify验证',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT '文件名',
  `file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '访问地址',
  `original_file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '原始访问地址',
  `edited_file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '编辑后访问地址',
  `poster_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面',
  `poster_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面存储路径',
  `tg_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'Telegram文件ID',
  `tg_thumb_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'Telegram缩略图文件ID',
  `tg_cache_asset_hash` varchar(1024) NOT NULL DEFAULT '' COMMENT 'TG缓存素材Hash',
  `tg_cache_status` varchar(16) NOT NULL DEFAULT 'invalid' COMMENT 'TG缓存状态',
  `storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '存储路径',
  `original_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '原始存储路径',
  `edited_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '编辑后存储路径',
  `mime_type` varchar(128) NOT NULL DEFAULT '' COMMENT 'MIME',
  `md5` varchar(64) NOT NULL DEFAULT '' COMMENT 'MD5',
  `perceptual_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '图片感知哈希',
  `processing_status` varchar(16) NOT NULL DEFAULT 'ready' COMMENT '媒体处理状态：uploaded/processing/ready/failed',
  `processing_error` text COMMENT '媒体处理错误',
  `processing_started_at` datetime DEFAULT NULL COMMENT '媒体处理开始时间',
  `edit_config_json` text COMMENT '图片编辑配置',
  `edit_status` varchar(16) NOT NULL DEFAULT 'raw' COMMENT '编辑状态：raw/edited',
  `size` bigint(20) NOT NULL DEFAULT '0' COMMENT '大小',
  `sort_index` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_media_task_attachment` (`task_id`,`attachment_id`),
  KEY `idx_ybp_media_task_sort` (`task_id`,`sort_index`,`id`),
  KEY `idx_ybp_media_profile` (`profile_id`,`id`),
  KEY `idx_ybp_media_profile_current` (`profile_id`,`task_id`,`purpose`,`sort_index`,`id`),
  KEY `idx_ybp_media_phash` (`perceptual_hash`),
  KEY `idx_ybp_media_similar_tenant` (`tenant_id`,`media_type`,`account_id`,`profile_id`,`id`),
  KEY `idx_ybp_media_similar_account` (`account_id`,`media_type`,`profile_id`,`id`),
  KEY `idx_ybp_media_purpose` (`task_id`,`purpose`,`sort_index`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架媒体';

ALTER TABLE `hg_youban_publish_media` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `id`, ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `perceptual_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '图片感知哈希' AFTER `md5`, ADD COLUMN `purpose` varchar(16) NOT NULL DEFAULT 'display' COMMENT '用途：display展示 verify验证' AFTER `media_type`, ADD COLUMN `poster_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面' AFTER `file_url`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `poster_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面存储路径' AFTER `poster_url`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `tg_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'Telegram文件ID' AFTER `poster_url`, ADD COLUMN `tg_thumb_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'Telegram缩略图文件ID' AFTER `tg_file_id`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `tg_cache_asset_hash` varchar(1024) NOT NULL DEFAULT '' COMMENT 'TG缓存素材Hash' AFTER `tg_thumb_file_id`, ADD COLUMN `tg_cache_status` varchar(16) NOT NULL DEFAULT 'invalid' COMMENT 'TG缓存状态' AFTER `tg_cache_asset_hash`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `original_attachment_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '原始HotGo附件ID' AFTER `attachment_id`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `original_file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '原始访问地址' AFTER `file_url`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `original_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '原始存储路径' AFTER `storage_path`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `edited_attachment_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '编辑后HotGo附件ID' AFTER `original_attachment_id`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `edited_file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '编辑后访问地址' AFTER `original_file_url`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `edited_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '编辑后存储路径' AFTER `original_storage_path`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `edit_config_json` text COMMENT '图片编辑配置' AFTER `perceptual_hash`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `edit_status` varchar(16) NOT NULL DEFAULT 'raw' COMMENT '编辑状态：raw/edited' AFTER `edit_config_json`;
UPDATE `hg_youban_publish_media` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;
UPDATE `hg_youban_publish_media` SET `original_attachment_id` = `attachment_id`, `original_file_url` = `file_url`, `original_storage_path` = `storage_path` WHERE `original_attachment_id` = 0 AND `attachment_id` > 0;
UPDATE `hg_youban_publish_media` SET `edit_status` = 'edited' WHERE (`edit_status` = '' OR `edit_status` = 'raw' OR `edit_status` IS NULL) AND (`edited_attachment_id` > 0 OR `edited_storage_path` <> '' OR `edited_file_url` <> '' OR lower(`name`) LIKE '%-edited.%' OR lower(`name`) LIKE '%_edited.%');
UPDATE `hg_youban_publish_media` SET `tg_cache_status` = 'valid', `tg_cache_asset_hash` = COALESCE(NULLIF(`md5`, ''), NULLIF(`storage_path`, ''), NULLIF(`file_url`, '')) WHERE `tg_file_id` <> '' AND (`tg_cache_status` = '' OR `tg_cache_status` = 'invalid' OR `tg_cache_status` IS NULL) AND `edit_status` = 'raw';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_media_face` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `media_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '媒体ID',
  `face_index` int(11) NOT NULL DEFAULT '0' COMMENT '图片内人脸序号',
  `bbox_json` text NOT NULL COMMENT '人脸框JSON',
  `embedding_model` varchar(64) NOT NULL DEFAULT '' COMMENT '特征模型',
  `embedding_vector` longtext NOT NULL COMMENT '人脸向量',
  `feature_hash` varchar(128) NOT NULL DEFAULT '' COMMENT '特征哈希',
  `quality_score` decimal(10,4) NOT NULL DEFAULT '0.0000' COMMENT '质量分',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_media_face_media` (`media_id`,`face_index`),
  KEY `idx_ybp_media_face_profile` (`tenant_id`,`account_id`,`profile_id`),
  KEY `idx_ybp_media_face_feature` (`feature_hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架媒体人脸特征';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_anti_scan_cache` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `image_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '图片感知哈希',
  `config_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '防扫图配置哈希',
  `provider` varchar(32) NOT NULL DEFAULT '' COMMENT '视觉服务商',
  `face_count` int(11) NOT NULL DEFAULT '0' COMMENT '人脸数量',
  `face_json` longtext NOT NULL COMMENT '人脸检测响应',
  `segment_json` longtext NOT NULL COMMENT '人像分割响应',
  `original_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '原图地址',
  `preview_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '预览图地址',
  `warnings_json` text NOT NULL COMMENT '预览告警',
  `cloud_raw_saved` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否已保存云识别结果',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_anti_scan_image` (`image_hash`),
  KEY `idx_ybp_anti_scan_config` (`image_hash`,`config_hash`),
  KEY `idx_ybp_anti_scan_provider` (`provider`,`cloud_raw_saved`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴防扫图预览缓存';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_cloud_resource_usage` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID，0为系统调用',
  `resource_type` varchar(32) NOT NULL DEFAULT '' COMMENT '资源类型',
  `scene` varchar(32) NOT NULL DEFAULT '' COMMENT '调用场景',
  `usage_date` date NOT NULL COMMENT '统计日期',
  `request_count` bigint(20) NOT NULL DEFAULT '0' COMMENT '请求次数',
  `success_count` bigint(20) NOT NULL DEFAULT '0' COMMENT '成功次数',
  `failure_count` bigint(20) NOT NULL DEFAULT '0' COMMENT '失败次数',
  `total_duration_ms` bigint(20) NOT NULL DEFAULT '0' COMMENT '累计耗时毫秒',
  `last_called_at` datetime DEFAULT NULL COMMENT '最后调用时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_cloud_usage_daily` (`tenant_id`,`account_id`,`resource_type`,`scene`,`usage_date`),
  KEY `idx_ybp_cloud_usage_date` (`usage_date`,`resource_type`,`account_id`),
  KEY `idx_ybp_cloud_usage_account` (`account_id`,`usage_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴云资源每日调用统计';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_anti_scan_material` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `type` varchar(32) NOT NULL DEFAULT 'sticker' COMMENT '素材类型',
  `name` varchar(120) NOT NULL DEFAULT '' COMMENT '素材名称',
  `url` varchar(1024) NOT NULL DEFAULT '' COMMENT '素材地址',
  `sort` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_anti_scan_material_owner` (`tenant_id`,`account_id`,`type`,`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴防扫图素材库';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tag` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name` varchar(64) NOT NULL DEFAULT '' COMMENT '标签名称',
  `review_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '审核状态',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `use_count` int(11) NOT NULL DEFAULT '0' COMMENT '使用数量',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tag_name_deleted` (`name`,`deleted_at`),
  KEY `idx_ybp_tag_review_status` (`review_status`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架标签';

ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `name` varchar(64) NOT NULL DEFAULT '' COMMENT '标签名称' AFTER `id`, ADD COLUMN `review_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '审核状态' AFTER `name`, ADD COLUMN `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态' AFTER `review_status`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `use_count` int(11) NOT NULL DEFAULT '0' COMMENT '使用数量' AFTER `status`, ADD COLUMN `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人' AFTER `use_count`, ADD COLUMN `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人' AFTER `created_by`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人' AFTER `updated_by`, ADD COLUMN `created_at` datetime DEFAULT NULL COMMENT '创建时间' AFTER `deleted_by`, ADD COLUMN `updated_at` datetime DEFAULT NULL COMMENT '更新时间' AFTER `created_at`, ADD COLUMN `deleted_at` datetime DEFAULT NULL COMMENT '删除时间' AFTER `updated_at`;
ALTER TABLE `hg_youban_publish_tag` ADD UNIQUE KEY `uk_ybp_tag_name_deleted` (`name`,`deleted_at`);
ALTER TABLE `hg_youban_publish_tag` ADD KEY `idx_ybp_tag_review_status` (`review_status`,`status`,`id`);

INSERT INTO `hg_youban_publish_tag` (`name`, `review_status`, `status`, `use_count`, `created_by`, `updated_by`, `deleted_by`, `created_at`, `updated_at`, `deleted_at`)
SELECT seed.`name`, 'approved', 1, 0, 0, 0, 0, NOW(), NOW(), NULL
FROM (
  SELECT '颜值' AS `name` UNION ALL SELECT '穿搭' UNION ALL SELECT '美食' UNION ALL SELECT '探店' UNION ALL SELECT '旅行' UNION ALL
  SELECT '运动' UNION ALL SELECT '健身' UNION ALL SELECT '摄影' UNION ALL SELECT '音乐' UNION ALL SELECT '舞蹈' UNION ALL
  SELECT '日常' UNION ALL SELECT '生活' UNION ALL SELECT '情感' UNION ALL SELECT '职场' UNION ALL SELECT '学习' UNION ALL
  SELECT '数码' UNION ALL SELECT '游戏' UNION ALL SELECT '电影' UNION ALL SELECT '宠物' UNION ALL SELECT '家居'
) seed
WHERE NOT EXISTS (SELECT 1 FROM `hg_youban_publish_tag`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_job` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键', `task_id` bigint(20) DEFAULT NULL COMMENT '采集任务ID，普通资料推送为空',
  `operation_no` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG操作号',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID', `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID', `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID', `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `target_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '目标Chat ID', `tg_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG消息ID',
  `collect_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID', `collect_source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID',
  `collect_source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '采集来源Chat ID', `collect_source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集来源消息ID',
  `asynq_task_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'Asynq任务ID', `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `retry_count` int(11) NOT NULL DEFAULT '0' COMMENT '重试次数', `next_retry_at` datetime DEFAULT NULL COMMENT '下次重试时间',
  `sent_at` datetime DEFAULT NULL COMMENT '发送成功时间', `cycle_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架',
  `cycle_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环天数', `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环发布时间',
  `next_cycle_at` datetime DEFAULT NULL COMMENT '下次循环时间', `priority` int(11) NOT NULL DEFAULT '100' COMMENT '调度优先级',
  `queue_name` varchar(64) NOT NULL DEFAULT '' COMMENT '队列名称', `dispatch_status` varchar(32) NOT NULL DEFAULT 'idle' COMMENT '调度状态',
  `dispatched_at` datetime DEFAULT NULL COMMENT '调度时间', `dispatch_count` int(11) NOT NULL DEFAULT '0' COMMENT '调度次数',
  `send_phase` varchar(32) NOT NULL DEFAULT '' COMMENT '发送阶段', `reconcile_count` int(11) NOT NULL DEFAULT '0' COMMENT '对账次数',
  `last_dispatch_error` varchar(512) NOT NULL DEFAULT '' COMMENT '最后调度错误', `error_message` text COMMENT '错误信息',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间', `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tg_job_operation_channel` (`task_id`,`operation_no`,`channel_id`),
  UNIQUE KEY `uk_ybp_tg_job_profile_operation_channel` (`profile_id`,`operation_no`,`channel_id`),
  KEY `idx_ybp_tg_job_task_channel` (`task_id`,`channel_id`,`id`),
  KEY `idx_ybp_tg_job_status_retry` (`status`,`next_retry_at`,`id`),
  KEY `idx_ybp_tg_job_task` (`task_id`),
  KEY `idx_ybp_tg_job_profile_operation` (`profile_id`,`task_id`,`operation_no`,`status`,`id`),
  KEY `idx_ybp_tg_job_profile_cleanup` (`profile_id`,`tenant_id`,`created_at`,`id`),
  KEY `idx_ybp_tg_job_cycle` (`cycle_enabled`,`next_cycle_at`,`id`),
  KEY `idx_ybp_tg_job_cycle_due` (`cycle_enabled`,`status`,`next_cycle_at`,`id`),
  KEY `idx_ybp_tg_job_operation` (`operation_no`,`status`,`id`),
  KEY `idx_ybp_tg_job_scheduler` (`dispatch_status`,`status`,`priority`,`next_retry_at`,`id`),
  KEY `idx_ybp_tg_job_tenant_channel_status` (`tenant_id`,`channel_id`,`status`,`id`),
  KEY `idx_ybp_tg_job_channel_dispatch` (`target_chat_id`,`dispatch_status`,`status`,`updated_at`),
  KEY `idx_ybp_tg_job_collect_order` (`channel_id`,`target_chat_id`,`collect_source_id`,`collect_source_chat_id`,`collect_source_message_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG发布任务';

ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `task_id`, ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `operation_no` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG操作号' AFTER `task_id`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `collect_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID' AFTER `target_chat_id`, ADD COLUMN `collect_source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID' AFTER `collect_event_id`, ADD COLUMN `collect_source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '采集来源Chat ID' AFTER `collect_source_id`, ADD COLUMN `collect_source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集来源消息ID' AFTER `collect_source_chat_id`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID' AFTER `profile_id`, ADD COLUMN `asynq_task_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'Asynq任务ID' AFTER `tg_message_id`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `sent_at` datetime DEFAULT NULL COMMENT '发送成功时间' AFTER `next_retry_at`, ADD COLUMN `cycle_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架' AFTER `sent_at`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `cycle_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环天数' AFTER `cycle_enabled`, ADD COLUMN `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环发布时间' AFTER `cycle_days`, ADD COLUMN `next_cycle_at` datetime DEFAULT NULL COMMENT '下次循环时间' AFTER `cycle_publish_time`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `priority` int(11) NOT NULL DEFAULT '100' COMMENT '调度优先级' AFTER `next_cycle_at`, ADD COLUMN `queue_name` varchar(64) NOT NULL DEFAULT '' COMMENT '队列名称' AFTER `priority`, ADD COLUMN `dispatch_status` varchar(32) NOT NULL DEFAULT 'idle' COMMENT '调度状态' AFTER `queue_name`, ADD COLUMN `dispatched_at` datetime DEFAULT NULL COMMENT '调度时间' AFTER `dispatch_status`, ADD COLUMN `dispatch_count` int(11) NOT NULL DEFAULT '0' COMMENT '调度次数' AFTER `dispatched_at`, ADD COLUMN `last_dispatch_error` varchar(512) NOT NULL DEFAULT '' COMMENT '最后调度错误' AFTER `dispatch_count`;
UPDATE `hg_youban_publish_tg_job` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;
ALTER TABLE `hg_youban_publish_tg_job` DROP INDEX `uk_ybp_tg_job_task_channel`;
ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_task_channel` (`task_id`,`channel_id`,`id`);
ALTER TABLE `hg_youban_publish_tg_job` ADD UNIQUE KEY `uk_ybp_tg_job_operation_channel` (`task_id`,`operation_no`,`channel_id`);
ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_cycle` (`cycle_enabled`,`next_cycle_at`,`id`);
ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_operation` (`operation_no`,`status`,`id`);
ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_scheduler` (`dispatch_status`,`status`,`priority`,`next_retry_at`,`id`);
ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_tenant_channel_status` (`tenant_id`,`channel_id`,`status`,`id`);
ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_channel_dispatch` (`target_chat_id`,`dispatch_status`,`status`,`updated_at`);
ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_collect_order` (`channel_id`,`target_chat_id`,`collect_source_id`,`collect_source_chat_id`,`collect_source_message_id`,`status`,`id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_queue_stat` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `stat_time` datetime DEFAULT NULL COMMENT '统计时间',
  `queue_name` varchar(64) NOT NULL DEFAULT '' COMMENT '队列名称',
  `priority_level` int(11) NOT NULL DEFAULT '0' COMMENT '优先级',
  `status` varchar(32) NOT NULL DEFAULT '' COMMENT '状态',
  `job_count` int(11) NOT NULL DEFAULT '0' COMMENT '任务数',
  `oldest_job_at` datetime DEFAULT NULL COMMENT '最早任务时间',
  `latest_job_at` datetime DEFAULT NULL COMMENT '最新任务时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tg_queue_stat` (`queue_name`,`priority_level`,`status`),
  KEY `idx_ybp_tg_queue_stat_count` (`job_count`,`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG队列统计';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_channel_stat` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID',
  `target_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '目标Chat ID',
  `channel_title` varchar(255) NOT NULL DEFAULT '' COMMENT '频道名称',
  `pending_count` int(11) NOT NULL DEFAULT '0' COMMENT '待调度数',
  `queued_count` int(11) NOT NULL DEFAULT '0' COMMENT '已调度数',
  `sending_count` int(11) NOT NULL DEFAULT '0' COMMENT '发送中数',
  `sent_count` int(11) NOT NULL DEFAULT '0' COMMENT '成功数',
  `failed_count` int(11) NOT NULL DEFAULT '0' COMMENT '失败数',
  `retry_count` int(11) NOT NULL DEFAULT '0' COMMENT '重试数',
  `rate_limit_count` int(11) NOT NULL DEFAULT '0' COMMENT '限流数',
  `last_sent_at` datetime DEFAULT NULL COMMENT '最后成功时间',
  `last_error_at` datetime DEFAULT NULL COMMENT '最后错误时间',
  `last_error_message` text NOT NULL COMMENT '最后错误',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tg_channel_stat` (`channel_id`,`target_chat_id`),
  KEY `idx_ybp_tg_channel_stat_tenant` (`tenant_id`,`account_id`,`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG频道统计';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_bot_stat` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `bot_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'Bot名称',
  `bot_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'Bot用户名',
  `pending_count` int(11) NOT NULL DEFAULT '0' COMMENT '待调度数',
  `queued_count` int(11) NOT NULL DEFAULT '0' COMMENT '已调度数',
  `sending_count` int(11) NOT NULL DEFAULT '0' COMMENT '发送中数',
  `sent_count` int(11) NOT NULL DEFAULT '0' COMMENT '成功数',
  `failed_count` int(11) NOT NULL DEFAULT '0' COMMENT '失败数',
  `retry_count` int(11) NOT NULL DEFAULT '0' COMMENT '重试数',
  `rate_limit_count` int(11) NOT NULL DEFAULT '0' COMMENT '限流数',
  `last_sent_at` datetime DEFAULT NULL COMMENT '最后成功时间',
  `last_error_at` datetime DEFAULT NULL COMMENT '最后错误时间',
  `last_error_message` text NOT NULL COMMENT '最后错误',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tg_bot_stat` (`tenant_id`,`bot_id`),
  KEY `idx_ybp_tg_bot_stat_updated` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG Bot统计';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_message` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键', `job_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG任务ID',
  `task_id` bigint(20) DEFAULT NULL COMMENT '旧任务ID，资料索引为空', `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID', `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID', `target_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '目标Chat ID',
  `tg_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG消息ID', `media_group_id` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体组ID',
  `media_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '媒体ID', `purpose` varchar(16) NOT NULL DEFAULT '' COMMENT 'display/verify',
  `tg_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'TG文件ID', `status` varchar(32) NOT NULL DEFAULT 'sent' COMMENT '状态',
  `sent_at` datetime DEFAULT NULL COMMENT '发送时间', `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间', `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tg_message_job_message` (`job_id`,`tg_message_id`),
  KEY `idx_ybp_tg_message_job` (`job_id`,`status`,`id`),
  KEY `idx_ybp_tg_message_task` (`task_id`,`id`),
  KEY `idx_ybp_tg_message_profile` (`tenant_id`,`account_id`,`profile_id`),
  KEY `idx_ybp_tg_message_target_message` (`target_chat_id`,`tg_message_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG消息记录';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_message_repair_run` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架任务ID',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `stage` varchar(32) NOT NULL DEFAULT '' COMMENT '阶段',
  `progress` int(11) NOT NULL DEFAULT '0' COMMENT '进度',
  `channel_count` int(11) NOT NULL DEFAULT '0' COMMENT '频道数',
  `scanned_count` int(11) NOT NULL DEFAULT '0' COMMENT '扫描消息数',
  `matched_count` int(11) NOT NULL DEFAULT '0' COMMENT '匹配消息数',
  `error_message` varchar(1000) NOT NULL DEFAULT '' COMMENT '错误信息',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `finished_at` datetime DEFAULT NULL COMMENT '完成时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tg_msg_repair_scope` (`tenant_id`,`account_id`,`profile_id`,`id`),
  KEY `idx_ybp_tg_msg_repair_status` (`status`,`updated_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG消息修复任务';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_message_cache` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG协议号ID',
  `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道配置ID',
  `target_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '目标Chat ID',
  `tg_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG消息ID',
  `message_text` text COMMENT '消息文本',
  `media_type` varchar(32) NOT NULL DEFAULT '' COMMENT '媒体类型',
  `message_date` datetime DEFAULT NULL COMMENT '消息时间',
  `media_group_id` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体组ID',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tg_msg_cache_msg` (`tenant_id`,`channel_id`,`tg_message_id`),
  KEY `idx_ybp_tg_msg_cache_channel` (`tenant_id`,`tg_account_id`,`channel_id`,`message_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG历史消息缓存';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_job_log` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键', `job_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG任务ID',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID', `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID', `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID', `action` varchar(32) NOT NULL DEFAULT '' COMMENT '动作',
  `status` varchar(32) NOT NULL DEFAULT '' COMMENT '状态', `message` text COMMENT '日志内容',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tg_job_log_tenant` (`tenant_id`,`id`),
  KEY `idx_ybp_tg_job_log_account` (`tenant_id`,`account_id`,`id`),
  KEY `idx_ybp_tg_job_log_created` (`created_at`,`id`),
  KEY `idx_ybp_tg_job_log_job` (`job_id`,`id`),
  KEY `idx_ybp_tg_job_log_task` (`task_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG任务日志';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_daily_stat` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键', `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID', `stat_date` date NOT NULL COMMENT '统计日期',
  `new_profile_count` int(11) NOT NULL DEFAULT '0' COMMENT '新资料数量', `published_count` int(11) NOT NULL DEFAULT '0' COMMENT '上架成功数量',
  `failed_count` int(11) NOT NULL DEFAULT '0' COMMENT '失败数量', `down_count` int(11) NOT NULL DEFAULT '0' COMMENT '下架数量',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间', `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_daily_stat_account_date` (`tenant_id`,`account_id`,`stat_date`),
  KEY `idx_ybp_daily_stat_date` (`stat_date`,`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴每日上架统计';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_bot` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID，0表示全局默认',
  `bot_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'Bot名称',
  `bot_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'Bot用户名',
  `bot_token` varchar(255) NOT NULL DEFAULT '' COMMENT 'Bot Token',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_bot_tenant` (`tenant_id`,`status`,`id`),
  KEY `idx_ybp_bot_status` (`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架Bot配置';

ALTER TABLE `hg_youban_publish_bot` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID，0表示全局默认' AFTER `id`;
ALTER TABLE `hg_youban_publish_bot` ADD KEY `idx_ybp_bot_tenant` (`tenant_id`,`status`,`id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_login` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `login_token` varchar(128) NOT NULL DEFAULT '' COMMENT '登录令牌',
  `qr_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '二维码地址',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `session_key` varchar(255) NOT NULL DEFAULT '' COMMENT '会话存储键',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `error_message` text COMMENT '错误信息',
  `expires_at` datetime DEFAULT NULL COMMENT '过期时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tg_login_token` (`login_token`),
  KEY `idx_ybp_tg_login_account` (`account_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架TG扫码登录';

ALTER TABLE `hg_youban_publish_tg_login` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `id`;
ALTER TABLE `hg_youban_publish_tg_login` ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
UPDATE `hg_youban_publish_tg_login` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_account` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建账号ID',
  `display_name` varchar(128) NOT NULL DEFAULT '' COMMENT '显示名称',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `telegram_first_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG名',
  `telegram_last_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG姓',
  `telegram_phone` varchar(64) NOT NULL DEFAULT '' COMMENT 'TG手机号',
  `telegram_is_bot` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否Bot',
  `session_key` varchar(255) NOT NULL DEFAULT '' COMMENT '会话存储键',
  `login_token` varchar(128) NOT NULL DEFAULT '' COMMENT '登录令牌',
  `qr_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '二维码地址',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `error_message` text COMMENT '错误信息',
  `last_login_at` datetime DEFAULT NULL COMMENT '最后授权时间',
  `expires_at` datetime DEFAULT NULL COMMENT '二维码过期时间',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tg_account_tenant` (`tenant_id`,`status`,`id`),
  KEY `idx_ybp_tg_account_login` (`login_token`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架TG账号';

ALTER TABLE `hg_youban_publish_tg_account` ADD COLUMN `telegram_first_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG名' AFTER `telegram_username`;
ALTER TABLE `hg_youban_publish_tg_account` ADD COLUMN `telegram_last_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG姓' AFTER `telegram_first_name`;
ALTER TABLE `hg_youban_publish_tg_account` ADD COLUMN `telegram_phone` varchar(64) NOT NULL DEFAULT '' COMMENT 'TG手机号' AFTER `telegram_last_name`;
ALTER TABLE `hg_youban_publish_tg_account` ADD COLUMN `telegram_is_bot` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否Bot' AFTER `telegram_phone`;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_session` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `session_key` varchar(255) NOT NULL DEFAULT '' COMMENT '会话存储键',
  `session_data` longblob NOT NULL COMMENT '会话数据',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_ybp_tg_session_key` (`session_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架TG会话';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_channel` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG账号ID',
  `channel_title` varchar(128) NOT NULL DEFAULT '' COMMENT '频道名称',
  `channel_username` varchar(128) NOT NULL DEFAULT '' COMMENT '频道用户名',
  `target_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '目标Chat ID',
  `publish_direction` varchar(16) NOT NULL DEFAULT 'up' COMMENT '上架/下架频道',
  `cycle_publish_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架',
  `cycle_publish_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环上架天数',
  `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环上架时间',
  `is_default_selected` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否默认选中',
  `publish_visible` tinyint(1) NOT NULL DEFAULT '1' COMMENT '上架端资料选择可见',
  `anti_scan_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '频道防扫图开关',
  `text_obfuscation_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '频道文本混淆开关',
  `auto_delete_enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '频道自动删除开关',
  `preserve_history_messages` tinyint(1) NOT NULL DEFAULT '0' COMMENT '下架和循环上架时保留旧消息',
  `bot_id_json` text COMMENT '绑定Bot ID JSON',
  `bot_permission_status_json` text NOT NULL COMMENT '频道Bot权限检测结果JSON',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `last_refresh_status` varchar(32) NOT NULL DEFAULT '' COMMENT '最近刷新状态',
  `last_refresh_message` varchar(500) NOT NULL DEFAULT '' COMMENT '最近刷新信息',
  `last_refresh_at` datetime DEFAULT NULL COMMENT '最近刷新时间',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_channel_tenant_account` (`tenant_id`,`tg_account_id`,`status`,`id`),
  KEY `idx_ybp_channel_tenant_direction` (`tenant_id`,`publish_direction`,`status`,`id`),
  KEY `idx_ybp_channel_chat` (`tenant_id`,`target_chat_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架频道配置';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_feature_permission` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint NOT NULL DEFAULT '0',
  `feature_code` varchar(64) NOT NULL DEFAULT '',
  `status` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tenant_feature` (`tenant_id`,`feature_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴租户功能权限';

ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_publish_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架' AFTER `publish_direction`;
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_publish_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环上架天数' AFTER `cycle_publish_enabled`;
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环上架时间' AFTER `cycle_publish_days`;
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_next_run_at` datetime DEFAULT NULL COMMENT '下次循环上架时间';
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_last_run_at` datetime DEFAULT NULL COMMENT '上次循环上架时间';
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_active_run_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '当前循环批次ID';
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_last_error_message` text COMMENT '循环上架最近错误';
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `is_default_selected` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否默认选中' AFTER `cycle_publish_time`;
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `publish_visible` tinyint(1) NOT NULL DEFAULT '1' COMMENT '上架端资料选择可见' AFTER `is_default_selected`;
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `anti_scan_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '频道防扫图开关' AFTER `publish_visible`;
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `text_obfuscation_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '频道文本混淆开关' AFTER `anti_scan_enabled`;
ALTER TABLE `hg_youban_publish_channel` ADD KEY `idx_ybp_channel_tenant_direction` (`tenant_id`,`publish_direction`,`status`,`id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_channel` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG账号ID',
  `channel_id` varchar(128) NOT NULL DEFAULT '' COMMENT '频道ID',
  `access_hash` varchar(128) NOT NULL DEFAULT '' COMMENT 'AccessHash',
  `channel_title` varchar(128) NOT NULL DEFAULT '' COMMENT '频道名称',
  `channel_username` varchar(128) NOT NULL DEFAULT '' COMMENT '频道用户名',
  `management_role` varchar(16) NOT NULL DEFAULT 'member' COMMENT '当前TG账号角色：owner/admin/member',
  `is_broadcast` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否频道',
  `is_megagroup` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否群组',
  `can_post_messages` tinyint(1) NOT NULL DEFAULT '0' COMMENT '账号可发频道消息',
  `can_invite_users` tinyint(1) NOT NULL DEFAULT '0' COMMENT '账号可邀请用户',
  `can_add_admins` tinyint(1) NOT NULL DEFAULT '0' COMMENT '账号可添加管理员',
  `last_sync_at` datetime DEFAULT NULL COMMENT '最后同步时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tg_channel_account_channel` (`tenant_id`,`tg_account_id`,`channel_id`),
  KEY `idx_ybp_tg_channel_search` (`tenant_id`,`tg_account_id`,`channel_title`,`channel_username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架TG账号频道缓存';

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', 'Telegram App ID', 'int', 'appId', '0', '0', 10, '扫码登录使用的 Telegram API ID，来自 my.telegram.org', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='appId');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', 'Telegram App Hash', 'string', 'appHash', '', '', 20, '扫码登录使用的 Telegram App Hash，来自 my.telegram.org', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='appHash');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', '代理地址', 'string', 'proxyUrl', '', '', 30, '本地开发可配置 socks5://127.0.0.1:7890，也支持 http/https 代理', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='proxyUrl');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', 'Bot运行模式', 'string', 'botRuntimeMode', 'auto', 'auto', 40, 'auto/develop 使用 pull，production 使用 webhook；也可显式配置 pull/webhook', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='botRuntimeMode');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', 'Webhook Base URL', 'string', 'webhookBaseUrl', '', '', 50, '线上 webhook 的公网域名，例如 https://api.example.com', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='webhookBaseUrl');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', 'Webhook Secret', 'string', 'webhookSecret', '', '', 60, 'Telegram webhook secret token，可选', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='webhookSecret');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', '默认推送 Chat ID', 'string', 'defaultTargetChat', '', '', 70, '资料发布后默认推送的 Telegram chat_id，可由后续频道配置覆盖', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='defaultTargetChat');
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
SELECT seed.`addon_name`, seed.`group`, seed.`name`, seed.`type`, seed.`key`, seed.`value`, seed.`default_value`, seed.`sort`, seed.`tip`, 0, 1, NOW(), NOW()
FROM (
  SELECT 'youban_publish' AS `addon_name`, 'cloudResource' AS `group`, '腾讯云人脸检测开关' AS `name`, 'int' AS `type`, 'tencentVisionEnabled' AS `key`, '0' AS `value`, '0' AS `default_value`, 80 AS `sort`, '是否启用腾讯云人脸检测，用于二维码和贴图避开人脸' AS `tip` UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云站点', 'string', 'tencentCloudSite', 'mainland', 'mainland', 85, 'mainland 国内云；intl 国际云' UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云 SecretId', 'string', 'tencentSecretId', '', '', 90, 'CAM 子用户 SecretId，仅授予 IAI 必要权限' UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云 SecretKey', 'string', 'tencentSecretKey', '', '', 100, 'CAM 子用户 SecretKey，页面回显会脱敏' UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云 Region', 'string', 'tencentRegion', 'ap-guangzhou', 'ap-guangzhou', 110, '国内云默认 ap-guangzhou，国际云默认 ap-singapore' UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云 BDA Endpoint', 'string', 'tencentBdaEndpoint', 'bda.tencentcloudapi.com', 'bda.tencentcloudapi.com', 120, '旧国内版人体分析接口域名，当前默认不强制使用' UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云 IAI Endpoint', 'string', 'tencentIaiEndpoint', 'iai.tencentcloudapi.com', 'iai.tencentcloudapi.com', 130, '国内云 iai.tencentcloudapi.com；国际云 iai.intl.tencentcloudapi.com' UNION ALL
  SELECT 'youban_publish', 'cloudResource', 'FAPIHub抠图开关', 'int', 'fapiHubEnabled', '0', '0', 140, '是否启用 FAPIHub 抠图，用于背景替换和人像背景贴图' UNION ALL
  SELECT 'youban_publish', 'cloudResource', 'FAPIHub API Key', 'string', 'fapiHubApiKey', '', '', 150, 'FAPIHub API Key，页面回显会脱敏' UNION ALL
  SELECT 'youban_publish', 'cloudResource', 'FAPIHub Endpoint', 'string', 'fapiHubEndpoint', 'https://fapihub.com/v2/rembg/', 'https://fapihub.com/v2/rembg/', 160, 'FAPIHub 抠图接口地址' UNION ALL
  SELECT 'youban_publish', 'cloudResource', 'FAPIHub Model', 'string', 'fapiHubModel', 'falcon', 'falcon', 170, 'FAPIHub 抠图模型，默认 falcon'
) seed
WHERE NOT EXISTS (
  SELECT 1 FROM `hg_sys_addons_config` c
  WHERE c.`addon_name` = seed.`addon_name`
    AND c.`group` = seed.`group`
    AND c.`key` = seed.`key`
);

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT seed.`addon_name`, seed.`group`, seed.`name`, seed.`type`, seed.`key`, seed.`value`, seed.`default_value`, seed.`sort`, seed.`tip`, 0, 1, NOW(), NOW()
FROM (
  SELECT 'youban_publish' AS `addon_name`, 'publish' AS `group`, '下架不推送到下架频道' AS `name`, 'int' AS `type`, 'skipDownChannelEnabled' AS `key`, '1' AS `value`, '1' AS `default_value`, 130 AS `sort`, '资料下架时是否跳过下架频道推送' AS `tip` UNION ALL
  SELECT 'youban_publish', 'publish', '发送间隔秒数', 'int', 'sendIntervalSeconds', '3', '3', 140, 'Telegram 消息发送间隔' UNION ALL
  SELECT 'youban_publish', 'publish', '发送时间窗口开关', 'int', 'sendWindowEnabled', '0', '0', 150, '是否限制自动发送执行时间段' UNION ALL
  SELECT 'youban_publish', 'publish', '发送开始时间', 'string', 'sendWindowStart', '', '', 160, '自动发送允许开始时间' UNION ALL
  SELECT 'youban_publish', 'publish', '发送结束时间', 'string', 'sendWindowEnd', '', '', 170, '自动发送允许结束时间' UNION ALL
  SELECT 'youban_publish', 'publish', '失败处理策略', 'string', 'failureStrategy', 'continue', 'continue', 180, 'continue 继续后续任务，stop 停止后续任务' UNION ALL
  SELECT 'youban_publish', 'publish', '失败重试开关', 'int', 'retryEnabled', '1', '1', 190, '发送失败后是否重试' UNION ALL
  SELECT 'youban_publish', 'publish', '最大重试次数', 'int', 'maxRetryCount', '3', '3', 200, '发送失败最大重试次数' UNION ALL
  SELECT 'youban_publish', 'publish', '重试间隔分钟', 'int', 'retryIntervalMinutes', '5', '5', 210, '发送失败重试间隔' UNION ALL
  SELECT 'youban_publish', 'publish', '默认防扫图开关', 'int', 'defaultAntiScanEnabled', '0', '0', 220, '新发布内容默认是否启用防扫图' UNION ALL
  SELECT 'youban_publish', 'autoDelete', '自动删除关键词', '[]string', 'keywords', '["发现重复资料","验证视频","资料近期已上架","信息已推送成功","视频已推送成功","视频已自动转发到视频频道","小时内已提交过相同内容","内容已自动转发到发布频道","提交成功！","信息视频验证保存成功","收录失败！","处理媒体组失败:","推送媒体组到群","信息投稿推送","推送视频失败","收录成功","本频道弃用，订阅如下新频道","投稿已自动审核通过","重复投稿","重复提交","信息已存在请勿重复提交","采集失败","检测到重复资料","正在分析中","已被人脸过滤","提交成功","去重阶段","重复帖子已拦截","提交失败"]', '["发现重复资料","验证视频","资料近期已上架","信息已推送成功","视频已推送成功","视频已自动转发到视频频道","小时内已提交过相同内容","内容已自动转发到发布频道","提交成功！","信息视频验证保存成功","收录失败！","处理媒体组失败:","推送媒体组到群","信息投稿推送","推送视频失败","收录成功","本频道弃用，订阅如下新频道","投稿已自动审核通过","重复投稿","重复提交","信息已存在请勿重复提交","采集失败","检测到重复资料","正在分析中","已被人脸过滤","提交成功","去重阶段","重复帖子已拦截","提交失败"]', 220, '所有租户继承的默认自动删除关键词' UNION ALL
  SELECT 'youban_publish', 'autoDelete', '自动删除规则', '[]string', 'rules', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', 230, '仅匹配整条消息的自动删除规则' UNION ALL
  SELECT 'youban_publish', 'antiScan', '防扫图总开关', 'int', 'antiScanEnabled', '0', '0', 300, '是否启用防扫图能力' UNION ALL
  SELECT 'youban_publish', 'antiScan', '新笔记默认防扫图', 'int', 'defaultNewNoteEnabled', '0', '0', 310, '新笔记默认是否开启防扫图' UNION ALL
  SELECT 'youban_publish', 'antiScan', '已有资料批量处理', 'int', 'existingBatchEnabled', '0', '0', 320, '是否对已有资料触发批量处理意图' UNION ALL
  SELECT 'youban_publish', 'antiScan', '发送前强制处理', 'int', 'forceBeforeSendEnabled', '0', '0', 330, '发送前是否强制生成防扫图副本' UNION ALL
  SELECT 'youban_publish', 'antiScan', '单条资料允许覆盖', 'int', 'allowSingleOverrideEnabled', '0', '0', 340, '单条资料是否允许覆盖全局开关' UNION ALL
  SELECT 'youban_publish', 'antiScan', '移除图片元信息', 'int', 'metadataStripEnabled', '0', '0', 350, '是否移除 EXIF 等图片元信息' UNION ALL
  SELECT 'youban_publish', 'antiScan', '尺寸微调', 'int', 'resizeEnabled', '0', '0', 360, '是否轻微调整图片尺寸' UNION ALL
  SELECT 'youban_publish', 'antiScan', '尺寸缩放比例', 'int', 'resizeScale', '96', '96', 370, '尺寸缩放比例，80-100' UNION ALL
  SELECT 'youban_publish', 'antiScan', '轻微裁剪', 'int', 'cropEnabled', '0', '0', 380, '是否轻微裁剪图片边缘' UNION ALL
  SELECT 'youban_publish', 'antiScan', '裁剪比例', 'int', 'cropPercent', '2', '2', 390, '边缘裁剪比例，1-8' UNION ALL
  SELECT 'youban_publish', 'antiScan', '人像背景贴图', 'int', 'portraitBackgroundEnabled', '0', '0', 410, '是否启用人像背景贴图' UNION ALL
  SELECT 'youban_publish', 'antiScan', '人像背景替换', 'int', 'backgroundReplaceEnabled', '0', '0', 420, '是否启用替换背景处理' UNION ALL
  SELECT 'youban_publish', 'antiScan', '背景模糊', 'int', 'backgroundBlurEnabled', '0', '0', 430, '是否模糊背景' UNION ALL
  SELECT 'youban_publish', 'antiScan', '背景纹理叠加', 'int', 'backgroundTextureEnabled', '0', '0', 440, '是否叠加背景纹理' UNION ALL
  SELECT 'youban_publish', 'antiScan', '背景纹理预设', 'string', 'backgroundTexturePreset', 'rabbit', 'rabbit', 445, '背景纹理预设 rabbit/heart/dot/grid' UNION ALL
  SELECT 'youban_publish', 'antiScan', '素材库背景贴图', 'string', 'backgroundTextureImage', '', '', 446, '素材库背景贴图地址，留空使用预设' UNION ALL
  SELECT 'youban_publish', 'antiScan', '内容遮挡', 'int', 'maskEnabled', '0', '0', 450, '是否启用二维码或贴图遮挡' UNION ALL
  SELECT 'youban_publish', 'antiScan', '打码方式', 'string', 'maskMode', 'qr', 'qr', 460, '打码方式：qr二维码模式，sticker贴图模式' UNION ALL
  SELECT 'youban_publish', 'antiScan', '打码数量', 'int', 'maskCount', '1', '1', 470, '同一张图打码数量，最多3个' UNION ALL
  SELECT 'youban_publish', 'antiScan', '二维码文案', 'string', 'qrText', '', '', 480, '二维码模式的展示文案' UNION ALL
  SELECT 'youban_publish', 'antiScan', '贴图图片', 'string', 'stickerImage', '', '', 490, '贴图模式使用的正方形贴图' UNION ALL
  SELECT 'youban_publish', 'antiScan', '手动遮挡素材', 'string', 'maskItemsJson', '[]', '[]', 495, '手动摆放二维码或贴图 JSON' UNION ALL
  SELECT 'youban_publish', 'antiScan', '贴图透明度', 'int', 'stickerOpacity', '18', '18', 500, '防扫图贴图透明度，1-100' UNION ALL
  SELECT 'youban_publish', 'antiScan', '水印开关', 'int', 'watermarkEnabled', '0', '0', 510, '是否启用背景水印' UNION ALL
  SELECT 'youban_publish', 'antiScan', '资料编号水印', 'int', 'profileNoWatermarkEnabled', '0', '0', 520, '是否叠加资料编号水印' UNION ALL
  SELECT 'youban_publish', 'antiScan', '水印字体大小', 'int', 'watermarkFontSize', '22', '22', 530, '水印字体大小，12-56' UNION ALL
  SELECT 'youban_publish', 'antiScan', '水印透明度', 'int', 'watermarkOpacity', '28', '28', 540, '水印透明度，5-80' UNION ALL
  SELECT 'youban_publish', 'antiScan', '水印文案', 'string', 'watermarkText', '', '', 550, '防扫图水印文案' UNION ALL
  SELECT 'youban_publish', 'antiScan', '贴纸文案', 'string', 'stickerText', '', '', 560, '防扫图贴纸文案' UNION ALL
  SELECT 'youban_publish', 'antiScan', '噪点扰动', 'int', 'noiseEnabled', '0', '0', 570, '是否启用轻微噪点扰动' UNION ALL
  SELECT 'youban_publish', 'antiScan', '噪点强度', 'int', 'noiseStrength', '18', '18', 580, '噪点扰动强度' UNION ALL
  SELECT 'youban_publish', 'antiScan', '压缩重采样', 'int', 'compressionEnabled', '0', '0', 590, '是否启用压缩重采样' UNION ALL
  SELECT 'youban_publish', 'antiScan', '输出质量', 'int', 'compressionQuality', '82', '82', 600, '压缩重采样输出质量' UNION ALL
  SELECT 'youban_publish', 'antiScan', 'JPEG质量控制', 'int', 'jpegQualityControlEnabled', '0', '0', 610, '是否启用 JPEG 质量控制' UNION ALL
  SELECT 'youban_publish', 'antiScan', '色彩轻扰动', 'int', 'colorJitterEnabled', '0', '0', 620, '是否启用色彩轻扰动' UNION ALL
  SELECT 'youban_publish', 'antiScan', '色彩扰动强度', 'int', 'colorJitterStrength', '12', '12', 630, '色彩扰动强度' UNION ALL
  SELECT 'youban_publish', 'antiScan', '锐化模糊微扰', 'int', 'sharpenBlurEnabled', '0', '0', 640, '是否启用锐化或模糊微扰' UNION ALL
  SELECT 'youban_publish', 'antiScan', '微扰方式', 'string', 'sharpenBlurMode', 'blur', 'blur', 650, 'blur 或 sharpen' UNION ALL
  SELECT 'youban_publish', 'antiScan', '微扰强度', 'int', 'sharpenBlurStrength', '8', '8', 660, '锐化模糊微扰强度'
) seed
WHERE NOT EXISTS (
  SELECT 1 FROM `hg_sys_addons_config` c
  WHERE c.`addon_name` = seed.`addon_name`
    AND c.`group` = seed.`group`
    AND c.`key` = seed.`key`
);
DELETE FROM `hg_sys_addons_config`
WHERE `addon_name` = 'youban_publish'
  AND `group` = 'publish'
  AND `key` IN ('cyclePublishEnabled', 'cyclePublishDays', 'cyclePublishTime');

UPDATE `hg_sys_addons_config`
SET `status` = 2,
    `updated_at` = NOW()
WHERE `addon_name` = 'youban_publish'
  AND `group` = 'account'
  AND `key` IN ('defaultRoleId', 'defaultDeptId');

UPDATE `hg_sys_addons_config`
SET `status` = 2,
    `updated_at` = NOW()
WHERE `addon_name` = 'youban_publish'
  AND `group` = 'antiScan'
  AND `key` IN ('enabled', 'mode', 'method', 'stickerStyle', 'stickerDensity', 'qrMaskEnabled', 'qrCustomStickerEnabled', 'qrCount', 'customStickerEnabled');

UPDATE `hg_sys_addons_config`
SET `status` = 2,
    `updated_at` = NOW()
WHERE `addon_name` = 'youban_publish'
  AND `group` = 'autoDelete'
  AND `key` IN ('enabled', 'autoDeleteEnabled', 'botIds');

CREATE TABLE IF NOT EXISTS `hg_youban_publish_cycle_run` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `plan_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '循环计划ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架任务ID',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `stage` varchar(32) NOT NULL DEFAULT 'created' COMMENT '阶段',
  `scheduled_at` datetime DEFAULT NULL COMMENT '计划执行时间',
  `started_at` datetime DEFAULT NULL COMMENT '开始时间',
  `finished_at` datetime DEFAULT NULL COMMENT '完成时间',
  `error_message` text COMMENT '错误信息',
  `retry_count` int(11) NOT NULL DEFAULT '0' COMMENT '重试次数',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ybp_cycle_run_plan` (`plan_id`,`id`),
  KEY `idx_ybp_cycle_run_owner` (`tenant_id`,`account_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='上架循环执行记录';
ALTER TABLE `hg_youban_publish_cycle_run` ADD COLUMN IF NOT EXISTS `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID' AFTER `profile_id`;
ALTER TABLE `hg_youban_publish_cycle_run` ADD COLUMN IF NOT EXISTS `cursor_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料索引游标';
ALTER TABLE `hg_youban_publish_cycle_run` ADD COLUMN IF NOT EXISTS `total_count` int(11) NOT NULL DEFAULT '0' COMMENT '资料总数';
ALTER TABLE `hg_youban_publish_cycle_run` ADD COLUMN IF NOT EXISTS `queued_count` int(11) NOT NULL DEFAULT '0' COMMENT '已生成任务数';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_channel_profile` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `channel_id` bigint(20) NOT NULL DEFAULT '0',
  `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `last_job_id` bigint(20) NOT NULL DEFAULT '0',
  `status` varchar(16) NOT NULL DEFAULT 'active',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_channel_profile` (`channel_id`,`profile_id`),
  KEY `idx_ybp_channel_profile_scan` (`channel_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='频道当前上架资料索引';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_cycle_run_log` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `run_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '执行ID',
  `plan_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '计划ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID',
  `level` varchar(16) NOT NULL DEFAULT 'info' COMMENT '级别',
  `stage` varchar(32) NOT NULL DEFAULT '' COMMENT '阶段',
  `message` text COMMENT '内容',
  `context_json` json DEFAULT NULL COMMENT '上下文',
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ybp_cycle_run_log_run` (`run_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='上架循环执行日志';
ALTER TABLE `hg_youban_publish_cycle_run_log` ADD COLUMN IF NOT EXISTS `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID' AFTER `profile_id`;
DELETE FROM `hg_youban_publish_cycle_run_log`;
DELETE FROM `hg_youban_publish_cycle_run`;
DROP TABLE IF EXISTS `hg_youban_publish_cycle_plan`;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_template` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `serial_no` varchar(32) NOT NULL DEFAULT '' COMMENT 'Inline模板编号',
  `push_mode` varchar(16) NOT NULL DEFAULT 'bot' COMMENT '推送方式：bot/account',
  `source_message_record_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '来源TG消息记录ID',
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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_media` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `template_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '模板ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `source_message_record_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '来源TG消息记录ID',
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

-- 资料编号统一为 A00001 格式，建议保持全局唯一；已存在索引时由数据库忽略或升级脚本处理。

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
  KEY `idx_ybp_media_phash_bucket_lookup` (`tenant_id`,`media_type`,`bucket_pos`,`bucket_value`,`account_id`,`profile_id`,`task_id`,`media_id`),
  KEY `idx_ybp_media_phash_bucket_profile_id` (`profile_id`)
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
  KEY `idx_ybp_media_phash_lsh_lookup` (`tenant_id`,`media_type`,`bucket_pos`,`bucket_value`,`account_id`,`profile_id`,`media_id`,`hash_value`),
  KEY `idx_ybp_media_phash_lsh_profile_id` (`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='媒体感知哈希LSH索引';

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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `level` int NOT NULL DEFAULT 0,
  `status` int NOT NULL DEFAULT 2,
  `opened_at` datetime DEFAULT NULL,
  `expired_at` datetime DEFAULT NULL,
  `remark` text NOT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tenant_vip_tenant` (`tenant_id`),
  KEY `idx_ybp_vip_expired` (`status`,`expired_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE `hg_youban_publish_message_push_plan` ADD COLUMN IF NOT EXISTS `push_mode` varchar(16) NOT NULL DEFAULT 'bot';
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN IF NOT EXISTS `push_mode` varchar(16) NOT NULL DEFAULT 'bot';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip_log` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `operator_id` bigint NOT NULL DEFAULT 0,
  `source` varchar(32) NOT NULL DEFAULT '',
  `action` varchar(32) NOT NULL DEFAULT '',
  `before_status` int NOT NULL DEFAULT 0,
  `before_level` int NOT NULL DEFAULT 0,
  `before_expired_at` datetime DEFAULT NULL,
  `after_status` int NOT NULL DEFAULT 0,
  `after_level` int NOT NULL DEFAULT 0,
  `after_expired_at` datetime DEFAULT NULL,
  `remark` text NOT NULL,
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tenant_vip_log_tenant` (`tenant_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip_coupon` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(64) NOT NULL DEFAULT '',
  `use_type` varchar(16) NOT NULL DEFAULT 'single',
  `amount` decimal(10,2) NOT NULL DEFAULT 0,
  `total_count` int NOT NULL DEFAULT 1,
  `used_count` int NOT NULL DEFAULT 0,
  `status` int NOT NULL DEFAULT 1,
  `remark` text NOT NULL,
  `expired_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tenant_vip_coupon_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip_event` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `event_key` varchar(160) NOT NULL DEFAULT '',
  `event_type` varchar(48) NOT NULL DEFAULT '',
  `activity_code` varchar(64) NOT NULL DEFAULT '',
  `activity_generation` int NOT NULL DEFAULT 1,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `account_id` bigint NOT NULL DEFAULT 0,
  `trigger_tenant_id` bigint NOT NULL DEFAULT 0,
  `trigger_account_id` bigint NOT NULL DEFAULT 0,
  `reference_type` varchar(32) NOT NULL DEFAULT '',
  `reference_id` varchar(64) NOT NULL DEFAULT '',
  `change_days` int NOT NULL DEFAULT 0,
  `before_expired_at` datetime DEFAULT NULL,
  `after_expired_at` datetime DEFAULT NULL,
  `notify_status` varchar(16) NOT NULL DEFAULT 'pending',
  `notify_retry_count` int NOT NULL DEFAULT 0,
  `notify_next_retry_at` datetime DEFAULT NULL,
  `notified_at` datetime DEFAULT NULL,
  `error_message` text NOT NULL,
  `remark` text NOT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_vip_event_key` (`event_key`),
  KEY `idx_ybp_vip_event_activity` (`activity_code`,`activity_generation`,`id`),
  KEY `idx_ybp_vip_event_tenant` (`tenant_id`,`event_type`,`id`),
  KEY `idx_ybp_vip_event_notify` (`notify_status`,`notify_next_retry_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_activity_generation` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `activity_code` varchar(64) NOT NULL DEFAULT '',
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `generation` int NOT NULL DEFAULT 1,
  `reset_reason` text NOT NULL,
  `updated_by` bigint NOT NULL DEFAULT 0,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_activity_generation` (`activity_code`,`tenant_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_cms_app` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT, `app_id` varchar(64) NOT NULL DEFAULT '',
  `app_secret` varchar(255) NOT NULL DEFAULT '', `name` varchar(128) NOT NULL DEFAULT '',
  `base_url` varchar(512) NOT NULL DEFAULT '', `instance_id` varchar(128) DEFAULT NULL,
  `enroll_hash` varchar(64) NOT NULL DEFAULT '', `source_ip` varchar(64) NOT NULL DEFAULT '',
  `cms_version` varchar(64) NOT NULL DEFAULT '', `last_heartbeat_at` datetime DEFAULT NULL,
  `status` tinyint NOT NULL DEFAULT 1, `created_at` datetime DEFAULT NULL, `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_cms_app_app_id` (`app_id`),
  UNIQUE KEY `uk_ybp_cms_app_instance_id` (`instance_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='XC-CMS开放应用';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_cms_binding_code` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT, `app_id` varchar(64) NOT NULL DEFAULT '',
  `code_hash` varchar(64) NOT NULL DEFAULT '', `code_hint` varchar(16) NOT NULL DEFAULT '',
  `version` int NOT NULL DEFAULT 1, `status` tinyint NOT NULL DEFAULT 1,
  `created_at` datetime DEFAULT NULL, `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_cms_binding_code_app` (`app_id`),
  UNIQUE KEY `uk_ybp_cms_binding_code_hash` (`code_hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='XC-CMS租户绑定码';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_cms_tenant_binding` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT, `app_id` varchar(64) NOT NULL DEFAULT '',
  `tenant_id` bigint NOT NULL DEFAULT 0, `code_version` int NOT NULL DEFAULT 1,
  `status` varchar(16) NOT NULL DEFAULT 'pending', `reason` varchar(500) NOT NULL DEFAULT '',
  `requested_at` datetime DEFAULT NULL, `reviewed_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT NULL, `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_cms_tenant_binding` (`app_id`,`tenant_id`),
  KEY `idx_ybp_cms_binding_tenant_status` (`tenant_id`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='XC-CMS租户绑定关系';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_bot_message_source` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT, `tenant_id` bigint NOT NULL DEFAULT 0, `received_bot_id` bigint NOT NULL DEFAULT 0,
  `chat_id` varchar(64) NOT NULL DEFAULT '', `message_id` bigint NOT NULL DEFAULT 0, `media_group_id` varchar(128) NOT NULL DEFAULT '',
  `sender_user_id` bigint NOT NULL DEFAULT 0, `sender_username` varchar(255) NOT NULL DEFAULT '', `sender_chat_id` bigint NOT NULL DEFAULT 0,
  `sender_chat_title` varchar(255) NOT NULL DEFAULT '', `reply_to_message_id` bigint NOT NULL DEFAULT 0, `reply_job_id` bigint NOT NULL DEFAULT 0,
  `reply_profile_id` bigint NOT NULL DEFAULT 0, `reply_purpose` varchar(16) NOT NULL DEFAULT '', `update_type` varchar(64) NOT NULL DEFAULT '',
  `message_text` text, `received_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP, `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_bot_message_source` (`chat_id`,`message_id`),
  KEY `idx_ybp_bot_message_source_chat_time` (`chat_id`,`received_at`), KEY `idx_ybp_bot_message_source_reply` (`reply_job_id`,`reply_profile_id`,`reply_to_message_id`), KEY `idx_ybp_bot_message_source_profile_time` (`tenant_id`,`reply_profile_id`,`received_at`,`id`),
  KEY `idx_ybp_bot_message_source_sender` (`sender_user_id`,`sender_chat_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
