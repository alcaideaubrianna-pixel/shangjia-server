-- Complete the R2 upload settings omitted by the original R2 migration.
INSERT INTO hg_sys_config ("group", key, name, type, value, default_value, sort, tip, is_default, status, created_at, updated_at)
SELECT 'upload', 'uploadR2AccessKey', 'R2 Access Key ID', 'string', '', '', 473, 'Cloudflare R2 Access Key ID', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM hg_sys_config WHERE "group" = 'upload' AND key = 'uploadR2AccessKey');

INSERT INTO hg_sys_config ("group", key, name, type, value, default_value, sort, tip, is_default, status, created_at, updated_at)
SELECT 'upload', 'uploadR2SecretKey', 'R2 Secret Access Key', 'string', '', '', 474, 'Cloudflare R2 Secret Access Key', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM hg_sys_config WHERE "group" = 'upload' AND key = 'uploadR2SecretKey');

INSERT INTO hg_sys_config ("group", key, name, type, value, default_value, sort, tip, is_default, status, created_at, updated_at)
SELECT 'upload', 'uploadR2PublicURL', 'R2 公开访问域名', 'string', '', '', 480, 'R2 自定义域名或公开访问地址', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM hg_sys_config WHERE "group" = 'upload' AND key = 'uploadR2PublicURL');
