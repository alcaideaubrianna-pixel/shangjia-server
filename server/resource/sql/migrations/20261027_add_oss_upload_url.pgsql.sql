-- Browser uploads may use an OSS CNAME while SAE workers keep the internal endpoint.
INSERT INTO hg_sys_config ("group", key, name, type, value, default_value, sort, tip, is_default, status, created_at, updated_at)
SELECT 'upload', 'uploadOssUploadURL', 'OSS 浏览器上传域名', 'string',
       'https://img.xiaohuiji.cc', 'https://img.xiaohuiji.cc', 487,
       '可选，浏览器直传使用的 OSS CNAME，例如：https://img.xiaohuiji.cc；留空时使用 OSS 公网 Endpoint',
       0, 1, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM hg_sys_config WHERE "group" = 'upload' AND key = 'uploadOssUploadURL'
);
