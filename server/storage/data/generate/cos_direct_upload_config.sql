INSERT IGNORE INTO `hg_sys_config` (`group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
VALUES
    ('upload', 'COS Bucket', 'string', 'uploadCosBucket', '', '', 475, '存储桶名称，必须包含APPID后缀，例如：bucket-1250000000', 0, 1, NOW(), NOW()),
    ('upload', 'COS Region', 'string', 'uploadCosRegion', '', '', 476, '存储桶所属地域，例如：ap-hongkong', 0, 1, NOW(), NOW()),
    ('upload', 'COS浏览器上传域名', 'string', 'uploadCosUploadURL', '', '', 485, '可选；为空直接上传COS源站，填写CDN前需放行COS写请求和CORS', 0, 1, NOW(), NOW());
