CREATE TABLE IF NOT EXISTS `hg_member_vip` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `member_id` bigint(20) NOT NULL COMMENT '用户ID',
  `level` int(11) NOT NULL DEFAULT '1' COMMENT '会员等级',
  `status` tinyint(1) NOT NULL DEFAULT '2' COMMENT '状态',
  `opened_at` datetime DEFAULT NULL COMMENT '开通时间',
  `expired_at` datetime DEFAULT NULL COMMENT '到期时间',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_member_id` (`member_id`),
  KEY `idx_status_expired_at` (`status`,`expired_at`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户VIP会员';

CREATE TABLE IF NOT EXISTS `hg_member_vip_log` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `member_id` bigint(20) NOT NULL COMMENT '用户ID',
  `operator_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '操作人ID',
  `source` varchar(32) NOT NULL DEFAULT '' COMMENT '来源',
  `action` varchar(32) NOT NULL DEFAULT '' COMMENT '动作',
  `before_status` tinyint(1) NOT NULL DEFAULT '2' COMMENT '变更前状态',
  `after_status` tinyint(1) NOT NULL DEFAULT '2' COMMENT '变更后状态',
  `before_level` int(11) NOT NULL DEFAULT '0' COMMENT '变更前等级',
  `after_level` int(11) NOT NULL DEFAULT '0' COMMENT '变更后等级',
  `before_expired_at` datetime DEFAULT NULL COMMENT '变更前到期时间',
  `after_expired_at` datetime DEFAULT NULL COMMENT '变更后到期时间',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  KEY `idx_member_id` (`member_id`),
  KEY `idx_operator_id` (`operator_id`),
  KEY `idx_source` (`source`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户VIP会员日志';

SET @now := now();
SET @orgId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'Org' LIMIT 1);

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @orgId, '会员日志', 'OrgVipLog', 'vip-log', '', '2', '', '/member/vipLogList', '', '/org/vipLog/index', '1', '', '0', '0', '', '0', '0', '0', '2', CONCAT('tr_', @orgId, ' '), '15', '', '1', @now, @now
WHERE @orgId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'OrgVipLog');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, p.`id`, '修改会员', 'org_user_vip', '', '', '3', '', '/member/vip', '', '', '1', '', '0', '2', '', '0', '0', '0', p.`level` + 1, CONCAT(p.`tree`, ' tr_', p.`id`, ' '), '55', '', '1', @now, @now
FROM `hg_admin_menu` p
WHERE p.`name` = 'user' AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'org_user_vip');

INSERT IGNORE INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` IN ('OrgVipLog', 'org_user_vip')
WHERE r.`id` IN (1, 2);
