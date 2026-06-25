INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'pay', '彩虹易支付网关地址', 'string', 'payRainbowGateway', 'https://pay.v8jisu.cn', 'https://pay.v8jisu.cn', 940, '彩虹易支付网关地址', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_config` WHERE `group` = 'pay' AND `key` = 'payRainbowGateway');

INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'pay', '彩虹易支付商户ID', 'string', 'payRainbowPid', '', '', 950, '彩虹易支付商户ID', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_config` WHERE `group` = 'pay' AND `key` = 'payRainbowPid');

INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'pay', '彩虹易支付接口类型', 'string', 'payRainbowMethod', 'jump', 'jump', 960, '彩虹易支付接口类型，如 jump', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_config` WHERE `group` = 'pay' AND `key` = 'payRainbowMethod');

INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'pay', '彩虹易支付商户私钥', 'string', 'payRainbowPrivateKey', '', '', 970, '用于 SHA256WithRSA 签名，可填写 PEM 内容或服务器文件路径', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_config` WHERE `group` = 'pay' AND `key` = 'payRainbowPrivateKey');

INSERT INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'pay', '彩虹易支付平台公钥', 'string', 'payRainbowPlatformPublicKey', '', '', 980, '用于验证彩虹回调签名，可填写 PEM 内容或服务器文件路径', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_config` WHERE `group` = 'pay' AND `key` = 'payRainbowPlatformPublicKey');
