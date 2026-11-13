-- 固定批次真实回放夹具：1000 个独立 profile×channel 组合。
-- 仅操作本地 tenant/account=999，禁止连接线上数据库。
-- 与复用同一 profile/channel 的旧压测不同，本脚本每个 Job 都有唯一的资料×频道组合，
-- 并排除已有活动任务，避免被历史队首阻塞或被业务幂等规则 superseded。
--
-- 使用示例：
-- psql "$YOUBAN_LOCAL_DATABASE_URL" -v ON_ERROR_STOP=1 \
--   -f tools/local-regression/scenarios/fixed_1000_run4.sql

\set ON_ERROR_STOP on

BEGIN;

-- 只清理本轮自己的证据，允许脚本重复执行，不触碰其它回放或业务数据。
CREATE TEMP TABLE fixed_run4_old_jobs ON COMMIT DROP AS
SELECT id
FROM hg_youban_publish_tg_job
WHERE tenant_id = 999
  AND account_id = 999
  AND operation_no LIKE 'fixed_accept:run4:%';

DELETE FROM hg_youban_publish_tg_outbox o
USING fixed_run4_old_jobs old
WHERE o.job_id = old.id;

DELETE FROM hg_youban_publish_tg_job j
USING fixed_run4_old_jobs old
WHERE j.id = old.id;

-- 选取有有效展示媒体的资料，并排除已有活动任务。
-- 1000 行代表 1000 个不同的 (profile_id, channel_id) 组合；
-- 同一资料允许投递到多个频道，但同一组合只出现一次。
CREATE TEMP TABLE fixed_run4_pairs (
    row_no bigint PRIMARY KEY,
    profile_id bigint NOT NULL,
    channel_id bigint NOT NULL,
    target_chat_id varchar(128) NOT NULL,
    profile_no varchar(32) NOT NULL,
    title text NOT NULL
) ON COMMIT DROP;

INSERT INTO fixed_run4_pairs (
    row_no,
    profile_id,
    channel_id,
    target_chat_id,
    profile_no,
    title
)
SELECT
    row_number() OVER (ORDER BY pc.profile_id, pc.channel_id),
    pc.profile_id,
    pc.channel_id,
    c.target_chat_id,
    p.profile_no,
    COALESCE(p.title, '')
FROM hg_youban_publish_profile_channel pc
JOIN hg_content_profile p
  ON p.id = pc.profile_id
 AND p.deleted_at IS NULL
 AND p.status = 1
JOIN hg_youban_publish_channel c
  ON c.id = pc.channel_id
 AND c.tenant_id = 999
 AND c.status = 1
 AND c.deleted_at IS NULL
WHERE pc.tenant_id = 999
  AND pc.account_id = 999
  AND pc.deleted_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM hg_youban_publish_media m
      WHERE m.tenant_id = 999
        AND m.account_id = 999
        AND m.profile_id = pc.profile_id
        AND m.deleted_at IS NULL
        AND m.status = 1
        AND m.purpose = 'display'
        AND m.processing_status = 'ready'
        AND m.tg_file_id <> ''
  )
  AND NOT EXISTS (
      SELECT 1
      FROM hg_youban_publish_tg_job active
      WHERE active.tenant_id = pc.tenant_id
        AND active.account_id = pc.account_id
        AND active.profile_id = pc.profile_id
        AND active.channel_id = pc.channel_id
        AND active.status IN ('pending', 'sending', 'failed_retry', 'unknown')
  )
ORDER BY pc.profile_id, pc.channel_id
LIMIT 1000;

DO $$
DECLARE
    pair_count bigint;
BEGIN
    SELECT count(*) INTO pair_count FROM fixed_run4_pairs;
    IF pair_count <> 1000 THEN
        RAISE EXCEPTION
            '固定 run4 需要 1000 个独立 profile×channel 组合，实际只有 %',
            pair_count;
    END IF;
END
$$;

-- 通过真实本地 Bot 和频道映射创建 Job。
CREATE TEMP TABLE fixed_run4_jobs (
    id bigint PRIMARY KEY,
    row_no bigint NOT NULL,
    profile_id bigint NOT NULL,
    channel_id bigint NOT NULL
) ON COMMIT DROP;

WITH inserted AS (
    INSERT INTO hg_youban_publish_tg_job (
        tenant_id,
        account_id,
        profile_id,
        channel_id,
        target_chat_id,
        bot_id,
        operation_no,
        status,
        priority,
        dispatch_status,
        cycle_enabled,
        cycle_days,
        push_mode,
        operation_type,
        profile_no_snapshot,
        title_snapshot,
        created_at,
        updated_at
    )
    SELECT
        999,
        999,
        p.profile_id,
        p.channel_id,
        p.target_chat_id,
        (
            SELECT b.id
            FROM hg_youban_publish_bot b
            WHERE b.tenant_id = 999
              AND b.bot_username = 'local_hook_bot'
              AND b.status = 1
            ORDER BY b.id
            LIMIT 1
        ),
        'fixed_accept:run4:' || p.row_no,
        'pending',
        90,
        'idle',
        0,
        0,
        'bot',
        'publish',
        p.profile_no,
        p.title,
        now(),
        now()
    FROM fixed_run4_pairs p
    RETURNING id, profile_id, channel_id, operation_no
)
INSERT INTO fixed_run4_jobs (id, row_no, profile_id, channel_id)
SELECT
    i.id,
    p.row_no,
    i.profile_id,
    i.channel_id
FROM inserted i
JOIN fixed_run4_pairs p
  ON p.profile_id = i.profile_id
 AND p.channel_id = i.channel_id;

DO $$
DECLARE
    job_count bigint;
    missing_bot_count bigint;
BEGIN
    SELECT count(*), count(*) FILTER (WHERE j.bot_id = 0)
    INTO job_count, missing_bot_count
    FROM hg_youban_publish_tg_job j
    WHERE j.operation_no LIKE 'fixed_accept:run4:%';
    IF job_count <> 1000 OR missing_bot_count <> 0 THEN
        RAISE EXCEPTION
            '固定 run4 Job/Bot 夹具不完整，Job=%，缺少 Bot=%',
            job_count, missing_bot_count;
    END IF;
END
$$;

-- Job 与 Outbox 成对写入，模拟业务入口的持久化结果。
INSERT INTO hg_youban_publish_tg_outbox (
    job_id,
    event_type,
    dedupe_key,
    status,
    available_at,
    created_at,
    updated_at
)
SELECT
    j.id,
    'telegram_job.publish',
    'fixed_accept:run4:outbox:' || j.id,
    'pending',
    now(),
    now(),
    now()
FROM fixed_run4_jobs j;

COMMIT;

-- 夹具构造后的硬校验：组合唯一、媒体完整、Job/Outbox 一一对应。
SELECT 'pairs' AS item, count(*)::text AS value
FROM (
    SELECT DISTINCT profile_id, channel_id
    FROM hg_youban_publish_tg_job
    WHERE tenant_id = 999
      AND account_id = 999
      AND operation_no LIKE 'fixed_accept:run4:%'
) pairs
UNION ALL
SELECT 'profiles', count(DISTINCT profile_id)::text
FROM hg_youban_publish_tg_job
WHERE tenant_id = 999
  AND account_id = 999
  AND operation_no LIKE 'fixed_accept:run4:%'
UNION ALL
SELECT 'channels', count(DISTINCT channel_id)::text
FROM hg_youban_publish_tg_job
WHERE tenant_id = 999
  AND account_id = 999
  AND operation_no LIKE 'fixed_accept:run4:%'
UNION ALL
SELECT 'jobs', count(*)::text
FROM hg_youban_publish_tg_job
WHERE tenant_id = 999
  AND account_id = 999
  AND operation_no LIKE 'fixed_accept:run4:%'
UNION ALL
SELECT 'outbox', count(*)::text
FROM hg_youban_publish_tg_outbox o
JOIN hg_youban_publish_tg_job j ON j.id = o.job_id
WHERE j.tenant_id = 999
  AND j.account_id = 999
  AND j.operation_no LIKE 'fixed_accept:run4:%'
UNION ALL
SELECT 'jobs_with_ready_media', count(*)::text
FROM hg_youban_publish_tg_job j
WHERE j.tenant_id = 999
  AND j.account_id = 999
  AND j.operation_no LIKE 'fixed_accept:run4:%'
  AND EXISTS (
      SELECT 1
      FROM hg_youban_publish_media m
      WHERE m.tenant_id = j.tenant_id
        AND m.account_id = j.account_id
        AND m.profile_id = j.profile_id
        AND m.deleted_at IS NULL
        AND m.status = 1
        AND m.purpose = 'display'
        AND m.processing_status = 'ready'
        AND m.tg_file_id <> ''
  )
ORDER BY item;
