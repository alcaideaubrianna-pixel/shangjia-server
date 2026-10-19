-- 在线维护，须在事务外执行；普通 VACUUM 不阻塞正常读写，禁止改为 FULL。
SET lock_timeout = '2s';
ALTER TABLE hg_youban_publish_media_phash_alias_bucket SET (
    autovacuum_vacuum_scale_factor = 0.01,
    autovacuum_vacuum_insert_scale_factor = 0.01,
    autovacuum_analyze_scale_factor = 0.02
);
VACUUM (ANALYZE) hg_youban_publish_media_phash_alias_bucket;
