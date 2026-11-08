-- 历史采集公平调度增量迁移。
-- 只增加调度元数据，不修改 delivery 业务状态，不删除历史记录。

ALTER TABLE "hg_tg_collector_delivery"
    ADD COLUMN IF NOT EXISTS "account_id" BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS "source_id" BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS "scheduler_pool" VARCHAR(16) NOT NULL DEFAULT 'history',
    ADD COLUMN IF NOT EXISTS "max_concurrency" INT NOT NULL DEFAULT 4,
    ADD COLUMN IF NOT EXISTS "running_count" INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS "dispatch_owner" VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS "dispatch_until" TIMESTAMP NULL,
    ADD COLUMN IF NOT EXISTS "dispatch_at" TIMESTAMP NULL;

-- 回填只补充缺失的调度元数据，重复执行安全。
UPDATE "hg_tg_collector_delivery" AS d
SET "account_id" = e."account_id",
    "source_id" = e."source_id"
FROM "hg_tg_collector_event" AS e
WHERE d."event_id" = e."id"
  AND (d."account_id" = 0 OR d."source_id" = 0);

CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_scheduler"
    ON "hg_tg_collector_delivery" ("status", "scheduler_pool", "priority" DESC, "created_at", "id");

CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_account_source"
    ON "hg_tg_collector_delivery" ("status", "account_id", "source_id", "priority" DESC, "created_at", "id");

CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_dispatch_lease"
    ON "hg_tg_collector_delivery" ("dispatch_until", "status", "id");
