CREATE TABLE IF NOT EXISTS `hg_yb_invite_config` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否启用',
  `base_url` varchar(500) NOT NULL DEFAULT '' COMMENT '邀请链接域名',
  `level1_min` int(11) NOT NULL DEFAULT '1' COMMENT '一档最小单数',
  `level1_max` int(11) NOT NULL DEFAULT '5' COMMENT '一档最大单数',
  `level1_rate` decimal(8,4) NOT NULL DEFAULT '2.0000' COMMENT '一档返现比例',
  `level2_min` int(11) NOT NULL DEFAULT '6' COMMENT '二档最小单数',
  `level2_rate` decimal(8,4) NOT NULL DEFAULT '3.0000' COMMENT '二档返现比例',
  `manual_audit` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否人工审核',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴邀请返现配置';

CREATE TABLE IF NOT EXISTS `hg_yb_invite_rebate` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `inviter_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '邀请人ID',
  `invitee_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '被邀请人ID',
  `invite_code` varchar(64) NOT NULL DEFAULT '' COMMENT '邀请码',
  `order_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '订单ID',
  `order_sn` varchar(64) NOT NULL DEFAULT '' COMMENT '订单号',
  `trade_type` varchar(64) NOT NULL DEFAULT 'member_vip' COMMENT '交易类型',
  `order_amount` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '订单金额',
  `rebate_rate` decimal(8,4) NOT NULL DEFAULT '0.0000' COMMENT '返现比例',
  `rebate_amount` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '返现金额',
  `settle_status` varchar(32) NOT NULL DEFAULT 'settled' COMMENT '结算状态',
  `settled_at` datetime DEFAULT NULL COMMENT '结算时间',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_yb_invite_rebate_order` (`trade_type`,`order_sn`),
  KEY `idx_yb_invite_rebate_inviter` (`inviter_id`),
  KEY `idx_yb_invite_rebate_invitee` (`invitee_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴邀请返现记录';

INSERT INTO `hg_yb_invite_config` (`id`, `enabled`, `base_url`, `level1_min`, `level1_max`, `level1_rate`, `level2_min`, `level2_rate`, `manual_audit`, `remark`, `created_at`, `updated_at`)
SELECT 1, 1, 'https://yuebanby.com', 1, 5, 2.0000, 6, 3.0000, 0, '默认邀请返现配置', NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_yb_invite_config` WHERE `id` = 1);
