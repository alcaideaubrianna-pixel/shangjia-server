-- 历史采集公平调度增量迁移。
--
-- 运行约束：
-- 1. 所有 DDL 均可在事务中执行并可回滚；不使用 CREATE INDEX CONCURRENTLY。
-- 2. 不在自动迁移阶段对 delivery 做无界 UPDATE。
-- 3. scheduler_pool 的批量分类见文件末尾的“手动分批回填”说明；每次只处理
--    有界主键批次，重复执行直到 UPDATE 返回 0 行即可。
-- 4. 旧记录默认保持 history。只有 raw_update 明确标记 history=false，或关联
--    event 明确属于 account 且没有 history=true 时，才安全地改为 live。

ALTER TABLE "hg_tg_collector_delivery"
    ADD COLUMN IF NOT EXISTS "account_id" BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS "source_id" BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS "scheduler_pool" VARCHAR(16) NOT NULL DEFAULT 'history',
    ADD COLUMN IF NOT EXISTS "max_concurrency" INT NOT NULL DEFAULT 4,
    ADD COLUMN IF NOT EXISTS "running_count" INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS "dispatch_owner" VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS "dispatch_until" TIMESTAMP NULL,
    ADD COLUMN IF NOT EXISTS "dispatch_at" TIMESTAMP NULL;

-- 只增加调度元数据，不修改 delivery 业务状态，不删除历史记录。
-- 该回填原先是无界 UPDATE，已移除；account_id/source_id 由后续有界批次补齐。

-- 调度查询和手动回填均按 delivery.id 走有界主键范围。
CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_scheduler"
    ON "hg_tg_collector_delivery" ("status", "scheduler_pool", "priority" DESC, "created_at", "id");

CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_account_source"
    ON "hg_tg_collector_delivery" ("status", "account_id", "source_id", "priority" DESC, "created_at", "id");

CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_dispatch_lease"
    ON "hg_tg_collector_delivery" ("dispatch_until", "status", "id");

-- 支持按主键批量回填 account/source 元数据；只命中仍缺失的行。
CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_event_metadata_backfill"
    ON "hg_tg_collector_delivery" ("id", "event_id")
    WHERE "account_id" = 0 OR "source_id" = 0;

-- 支持 pending 调度器按 pool 和主键分批处理。
CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_pending_pool_id"
    ON "hg_tg_collector_delivery" ("scheduler_pool", "id")
    WHERE "status" = 'pending';

CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_pending_dispatch"
    ON "hg_tg_collector_delivery" ("status", "dispatch_until", "priority" DESC, "created_at", "id")
    WHERE "status" = 'pending';

-- 历史调度专用索引：只覆盖 pending/history，避免 background 扫描实时池。
-- 键顺序覆盖 scheduler_pool、租约时间、优先级和稳定分页字段。
CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_pending_history_dispatch"
    ON "hg_tg_collector_delivery" ("scheduler_pool", "dispatch_until", "priority" DESC, "created_at", "id")
    WHERE "status" = 'pending' AND "scheduler_pool" = 'history';

-- 实时恢复按来源公平取数；source_id 放在分区键前，减少窗口查询的回表范围。
CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_pending_realtime_source"
    ON "hg_tg_collector_delivery" ("scheduler_pool", "source_id", "priority" DESC, "created_at", "id")
    WHERE "status" IN ('pending', 'failed_retry') AND "scheduler_pool" = 'realtime';

-- ============================================================================
-- 手动分批回填（每次执行一条；不要把以下语句包在长事务中）
-- ============================================================================
--
-- 设计：
-- * account_id/source_id 只从关联 event 回填，按 delivery.id 排序，LIMIT 500。
-- * scheduler_pool 只把“明确实时”的记录从默认 history 改为 live：
--     - raw_update.history = true  -> history，永不改成 live；
--     - raw_update.history = false -> live；
--     - account source 且没有显式 history=true -> live；
--     - bot source 没有明确 history=false 时保留 history，避免误把旧 Bot
--       历史重放任务放入实时池。
-- * 使用 FOR UPDATE SKIP LOCKED，多个迁移/维护进程可以安全地并行执行。
-- * 语句是幂等的：已回填的行不再命中，重复执行不会改变结果。
--
-- 1) 回填账户和来源元数据：
--
-- WITH batch AS (
--     SELECT d.id, e.account_id, e.source_id
--     FROM "hg_tg_collector_delivery" AS d
--     JOIN "hg_tg_collector_event" AS e ON e.id = d.event_id
--     WHERE (d.account_id = 0 OR d.source_id = 0)
--     ORDER BY d.id
--     LIMIT 500
--     FOR UPDATE OF d SKIP LOCKED
-- )
-- UPDATE "hg_tg_collector_delivery" AS d
-- SET "account_id" = batch.account_id,
--     "source_id" = batch.source_id
-- FROM batch
-- WHERE d.id = batch.id;
--
-- 2) 只把明确实时的旧 delivery 分批迁移到 live pool：
--
-- WITH batch AS (
--     SELECT d.id
--     FROM "hg_tg_collector_delivery" AS d
--     JOIN "hg_tg_collector_event" AS e ON e.id = d.event_id
--     WHERE d.scheduler_pool <> 'live'
--       AND (
--           lower(COALESCE(e.raw_update->>'history', '')) = 'false'
--           OR (
--               lower(COALESCE(e.raw_update->>'history', '')) NOT IN ('true', 'false')
--               AND lower(COALESCE(e.source_type, '')) = 'account'
--           )
--       )
--     ORDER BY d.id
--     LIMIT 500
--     FOR UPDATE OF d SKIP LOCKED
-- )
-- UPDATE "hg_tg_collector_delivery" AS d
-- SET "scheduler_pool" = 'live'
-- FROM batch
-- WHERE d.id = batch.id
--   AND d.scheduler_pool <> 'live';
--
-- 建议先在低峰期执行上述两条语句，每条重复执行，直到返回 UPDATE 0 行。
-- 如果迁移执行器不支持手动循环，则只安装本文件中的索引，使用后台维护任务
-- 调用同样的 LIMIT 500 查询逐批完成回填；不要把整个 UPDATE 改回无界语句。
