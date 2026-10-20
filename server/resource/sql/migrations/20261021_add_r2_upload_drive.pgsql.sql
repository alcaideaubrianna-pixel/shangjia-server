-- Add Cloudflare R2 to the upload driver dictionary. Idempotent by option key/group.
INSERT INTO hg_sys_dict_data (label, value, value_type, type, list_class, is_default, sort, remark, status, created_at, updated_at)
SELECT 'Cloudflare R2', 'r2', 'string', 'config_upload_drive', 'info', 2, 70, 'Cloudflare R2 S3-compatible storage', 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM hg_sys_dict_data WHERE type = 'config_upload_drive' AND value = 'r2');
