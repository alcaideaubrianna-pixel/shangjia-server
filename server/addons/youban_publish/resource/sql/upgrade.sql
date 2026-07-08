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

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'collect', '采集总开关', 'int', 'collectEnabled', '1', '1', 10, '是否启用采集能力', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='collectEnabled');

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
