UPDATE "hg_sys_config"
SET "value" = 'https://img.yuebanby.com', "updated_at" = NOW()
WHERE "group" = 'upload' AND "key" = 'uploadCosBucketURL';
