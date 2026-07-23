INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'pay', '彩虹易支付网关地址', 'string', 'payRainbowGateway', 'https://pay.v8jisu.cn', 'https://pay.v8jisu.cn', 940, '彩虹易支付网关地址', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_config` WHERE `group` = 'pay' AND `key` = 'payRainbowGateway');

INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'pay', '彩虹易支付商户ID', 'string', 'payRainbowPid', '', '', 950, '彩虹易支付商户ID', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_config` WHERE `group` = 'pay' AND `key` = 'payRainbowPid');

INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'pay', '彩虹易支付MD5密钥', 'string', 'payRainbowKey', '', '', 960, '彩虹易支付 V1 接口 MD5 签名密钥', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_config` WHERE `group` = 'pay' AND `key` = 'payRainbowKey');

INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)

INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'pay', 'GMPay 默认币种', 'string', 'payRainbowToken', 'usdt', 'usdt', 980, 'GMPay 创建订单时的默认币种', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_config` WHERE `group` = 'pay' AND `key` = 'payRainbowToken');

INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'pay', 'GMPay 默认网络', 'string', 'payRainbowNetwork', 'tron', 'tron', 990, 'GMPay 创建订单时的默认网络', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_config` WHERE `group` = 'pay' AND `key` = 'payRainbowNetwork');
