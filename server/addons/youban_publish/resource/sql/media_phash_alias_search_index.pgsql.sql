-- 防扫图图片近似匹配索引。生产环境显式执行，不能放在事务或请求初始化中。
-- psql -X -v ON_ERROR_STOP=1 -f media_phash_alias_search_index.pgsql.sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_phash_alias_search
ON hg_youban_publish_media_phash_alias_bucket
    (tenant_id, media_type, bucket_pos, bucket_value)
INCLUDE (account_id, profile_id, media_id, hash_value);

-- 并发创建失败可能留下无效索引，执行后必须检查 indisvalid 和 indisready。
SELECT c.relname, i.indisvalid, i.indisready
FROM pg_index i
JOIN pg_class c ON c.oid = i.indexrelid
WHERE c.relname = 'idx_ybp_phash_alias_search';
