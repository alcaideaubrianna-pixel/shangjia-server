INSERT INTO "hg_sys_config" ("group", "name", "type", "key", "value", "default_value", "sort", "tip", "status", "created_by", "created_at", "updated_at")
SELECT 'upload', 'COS公开访问域名', 'string', 'uploadCosPublicURL', 'https://img.yuebanby.com', '', 490, '前端展示/CDN域名，服务端上传仍使用COS Bucket域名', 1, 1, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM "hg_sys_config" WHERE "group" = 'upload' AND "key" = 'uploadCosPublicURL'
);

UPDATE "hg_sys_config"
SET "name" = 'COS上传Bucket域名',
    "tip" = '服务端SDK上传地址，例如：https://bucket-appid.cos.ap-hongkong.myqcloud.com',
    "updated_at" = NOW()
WHERE "group" = 'upload' AND "key" = 'uploadCosBucketURL';
