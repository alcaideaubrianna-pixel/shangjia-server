-- 循环调度分片迁移：先执行本文件，再部署使用分片字段的 publish。
-- 回滚：删除新增索引和字段即可，不改动现有 next_cycle_at/cycle_next_run_at。

ALTER TABLE "hg_youban_publish_tg_job"
    ADD COLUMN IF NOT EXISTS "cycle_scheduler_shard" smallint NOT NULL DEFAULT 0;

ALTER TABLE "hg_youban_publish_channel"
    ADD COLUMN IF NOT EXISTS "cycle_scheduler_shard" smallint NOT NULL DEFAULT 0;

UPDATE "hg_youban_publish_tg_job"
SET "cycle_scheduler_shard" = mod(abs(hashtext("tenant_id"::text || ':' || "account_id"::text)), 64)
WHERE "cycle_enabled" = 1;

UPDATE "hg_youban_publish_channel"
SET "cycle_scheduler_shard" = mod(abs(hashtext("tenant_id"::text || ':' || "id"::text)), 64)
WHERE "cycle_publish_enabled" = 1;

CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_cycle_shard_due"
    ON "hg_youban_publish_tg_job" ("cycle_scheduler_shard", "next_cycle_at", "id")
    WHERE "cycle_enabled" = 1 AND "next_cycle_at" IS NOT NULL;

CREATE INDEX IF NOT EXISTS "idx_ybp_channel_cycle_shard_due"
    ON "hg_youban_publish_channel" ("cycle_scheduler_shard", "cycle_next_run_at", "id")
    WHERE "cycle_publish_enabled" = 1 AND "deleted_at" IS NULL;

ANALYZE "hg_youban_publish_tg_job";
ANALYZE "hg_youban_publish_channel";
