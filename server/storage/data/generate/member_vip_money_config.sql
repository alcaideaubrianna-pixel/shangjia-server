INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'member_vip', '会员认证价格', 'float64', 'memberVipMoney', '30', '30', 4, '会员认证支付配置', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM `hg_sys_config` WHERE `group` = 'member_vip' AND `key` = 'memberVipMoney'
);
