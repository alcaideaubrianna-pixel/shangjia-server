-- 关系级频道归并：一次只处理一个 tenant + canonical Chat ID。
-- 通过 psql -v tenant_id=... -v canonical_chat_id=... 调用。
-- 该文件不接受“全库 apply”，避免长事务和误合并。
\set ON_ERROR_STOP on
\if :{?tenant_id}
\else
  \echo '缺少 -v tenant_id=...'
  \quit 2
\endif
\if :{?canonical_chat_id}
\else
  \echo '缺少 -v canonical_chat_id=...'
  \quit 2
\endif
\if :{?confirm}
\else
  \echo '缺少 -v confirm=I_UNDERSTAND_COLLECT_CHANNEL_MERGE'
  \quit 3
\endif

BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
SELECT pg_advisory_xact_lock(hashtextextended('youban:collect_channel:merge:' || :'tenant_id' || ':' || :'canonical_chat_id', 0));

CREATE TEMP TABLE _cc_channels ON COMMIT DROP AS
SELECT id, tenant_id, telegram_chat_id,
       regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id
FROM hg_youban_publish_collect_channel
WHERE tenant_id = :'tenant_id'::bigint
  AND btrim(telegram_chat_id) <> ''
  AND regexp_replace(btrim(telegram_chat_id), '^-100', '') = regexp_replace(btrim(:'canonical_chat_id'), '^-100', '')
FOR UPDATE;

DO $$
DECLARE n bigint; active_n bigint;
BEGIN
  SELECT count(*), count(*) FILTER (WHERE deleted_at IS NULL)
    INTO n, active_n
  FROM hg_youban_publish_collect_channel c
  JOIN _cc_channels x ON x.id=c.id;
  IF n < 2 THEN RAISE EXCEPTION 'REFUSED: duplicate group not found'; END IF;
  IF active_n < 1 THEN RAISE EXCEPTION 'REFUSED: duplicate group has no active collect_channel'; END IF;
END $$;

-- 活跃的 canonical 记录优先；否则保留信息最完整、ID 最小的活动记录。
CREATE TEMP TABLE _cc_survivor ON COMMIT DROP AS
SELECT id AS survivor_id
FROM (
  SELECT c.id,
         row_number() OVER (ORDER BY (c.deleted_at IS NULL) DESC,
                                     (c.telegram_chat_id LIKE '-100%') DESC,
                                     c.updated_at DESC NULLS LAST, c.id) AS rn
  FROM hg_youban_publish_collect_channel c
  JOIN _cc_channels x ON x.id=c.id
) q WHERE rn=1;

CREATE TEMP TABLE _cc_old ON COMMIT DROP AS
SELECT x.id AS old_id, s.survivor_id
FROM _cc_channels x CROSS JOIN _cc_survivor s
WHERE x.id <> s.survivor_id;

-- rule_channel 是硬冲突：同一租户/账户/规则/频道组出现两条关系时整组回滚。
DO $$
DECLARE n bigint;
BEGIN
  SELECT count(*) INTO n
  FROM (
    SELECT r.tenant_id, r.account_id, r.rule_id,
           count(*) AS relation_count
    FROM hg_youban_publish_collect_rule_channel r
    JOIN _cc_channels x ON x.id=r.channel_id
    GROUP BY r.tenant_id,r.account_id,r.rule_id
    HAVING count(*) > 1
  ) q;
  IF n > 0 THEN
    RAISE EXCEPTION 'REFUSED: rule_channel conflict in % rule groups; group rolled back', n;
  END IF;
END $$;

-- account_channel：同一 tenant/account/source/canonical 以 updated_at 最新为主。
CREATE TEMP TABLE _cc_account_winner ON COMMIT DROP AS
SELECT id AS winner_id, channel_id AS winner_channel_id
FROM (
  SELECT r.id, r.channel_id,
         row_number() OVER (
           PARTITION BY r.tenant_id,r.account_id,r.source_id
           ORDER BY r.updated_at DESC NULLS LAST,r.id DESC
         ) AS rn
  FROM hg_youban_publish_collect_account_channel r
  JOIN _cc_channels x ON x.id=r.channel_id
  WHERE r.deleted_at IS NULL
) q WHERE rn=1;

-- 该唯一索引不是部分索引，软删不会释放唯一键；旧重复关系必须直接删除。
DELETE FROM hg_youban_publish_collect_account_channel r
WHERE r.id IN (
  SELECT r2.id
  FROM hg_youban_publish_collect_account_channel r2
  JOIN _cc_channels x ON x.id=r2.channel_id
  LEFT JOIN _cc_account_winner w ON w.winner_id=r2.id
  WHERE r2.deleted_at IS NULL AND w.winner_id IS NULL
);
UPDATE hg_youban_publish_collect_account_channel r
SET channel_id=s.survivor_id, updated_at=CURRENT_TIMESTAMP
FROM _cc_survivor s
WHERE r.id IN (SELECT winner_id FROM _cc_account_winner)
  AND r.channel_id <> s.survivor_id;

-- bot_channel：tenant/bot/canonical 同样按最新更新时间保留。
CREATE TEMP TABLE _cc_bot_winner ON COMMIT DROP AS
SELECT id AS winner_id
FROM (
  SELECT r.id,
         row_number() OVER (
           PARTITION BY r.tenant_id,r.bot_id
           ORDER BY r.updated_at DESC NULLS LAST,r.id DESC
         ) AS rn
  FROM hg_youban_publish_collect_bot_channel r
  JOIN _cc_channels x ON x.id=r.channel_id
  WHERE r.deleted_at IS NULL
) q WHERE rn=1;
DELETE FROM hg_youban_publish_collect_bot_channel r
WHERE r.id IN (
  SELECT r2.id FROM hg_youban_publish_collect_bot_channel r2
  JOIN _cc_channels x ON x.id=r2.channel_id
  LEFT JOIN _cc_bot_winner w ON w.winner_id=r2.id
  WHERE r2.deleted_at IS NULL AND w.winner_id IS NULL
);
UPDATE hg_youban_publish_collect_bot_channel r
SET channel_id=s.survivor_id, updated_at=CURRENT_TIMESTAMP
FROM _cc_survivor s
WHERE r.id IN (SELECT winner_id FROM _cc_bot_winner)
  AND r.channel_id <> s.survivor_id;

-- 唯一 rule_channel 关系迁移；冲突已在前面整组拒绝。
UPDATE hg_youban_publish_collect_rule_channel r
SET channel_id=s.survivor_id
FROM _cc_survivor s
WHERE r.channel_id IN (SELECT old_id FROM _cc_old);

-- profile：先按 canonical Chat ID + 业务维度保留最新一条，再重写活动记录。
CREATE TEMP TABLE _cc_profile_winner ON COMMIT DROP AS
SELECT id AS winner_id
FROM (
  SELECT p.id,
         row_number() OVER (
           PARTITION BY p.tenant_id,p.account_id,p.rule_id,p.profile_id
           ORDER BY p.updated_at DESC NULLS LAST,p.id DESC
         ) AS rn
  FROM hg_youban_publish_collect_channel_profile p
  WHERE p.deleted_at IS NULL
    AND regexp_replace(btrim(p.channel_id), '^-100', '') = (SELECT canonical_chat_id FROM _cc_channels LIMIT 1)
) q WHERE rn=1;
UPDATE hg_youban_publish_collect_channel_profile p
SET deleted_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP
WHERE p.deleted_at IS NULL
  AND regexp_replace(btrim(p.channel_id), '^-100', '') = (SELECT canonical_chat_id FROM _cc_channels LIMIT 1)
  AND p.id NOT IN (SELECT winner_id FROM _cc_profile_winner);
UPDATE hg_youban_publish_collect_channel_profile p
SET channel_id='-100' || (SELECT canonical_chat_id FROM _cc_channels LIMIT 1),
    updated_at=CURRENT_TIMESTAMP
WHERE p.id IN (SELECT winner_id FROM _cc_profile_winner);

-- count/member 是投影：删除该 canonical 组后按去重后的 profile 明细重建，绝不相加。
DELETE FROM hg_youban_publish_collect_channel_count_member
WHERE tenant_id=:'tenant_id'::bigint
  AND regexp_replace(btrim(channel_id), '^-100', '') = (SELECT canonical_chat_id FROM _cc_channels LIMIT 1);
DELETE FROM hg_youban_publish_collect_channel_count
WHERE tenant_id=:'tenant_id'::bigint
  AND regexp_replace(btrim(channel_id), '^-100', '') = (SELECT canonical_chat_id FROM _cc_channels LIMIT 1);
INSERT INTO hg_youban_publish_collect_channel_count_member
  (tenant_id,account_id,rule_id,channel_id,profile_id,reference_count)
SELECT tenant_id,account_id,rule_id,
       '-100' || (SELECT canonical_chat_id FROM _cc_channels LIMIT 1),
       profile_id,count(*)
FROM hg_youban_publish_collect_channel_profile
WHERE tenant_id=:'tenant_id'::bigint AND deleted_at IS NULL
  AND regexp_replace(btrim(channel_id), '^-100', '') = (SELECT canonical_chat_id FROM _cc_channels LIMIT 1)
GROUP BY tenant_id,account_id,rule_id,profile_id;
INSERT INTO hg_youban_publish_collect_channel_count
  (tenant_id,account_id,rule_id,channel_id,profile_count,ready,created_at,updated_at,last_seen_at)
SELECT tenant_id,account_id,rule_id,channel_id,count(*)::bigint,true,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
FROM hg_youban_publish_collect_channel_count_member
WHERE tenant_id=:'tenant_id'::bigint
  AND channel_id='-100' || (SELECT canonical_chat_id FROM _cc_channels LIMIT 1)
GROUP BY tenant_id,account_id,rule_id,channel_id;

-- 最后规范化 survivor，并软删旧频道实体。
UPDATE hg_youban_publish_collect_channel c
SET telegram_chat_id='-100' || (SELECT canonical_chat_id FROM _cc_channels LIMIT 1), updated_at=CURRENT_TIMESTAMP
FROM _cc_survivor s WHERE c.id=s.survivor_id;
UPDATE hg_youban_publish_collect_channel c
SET deleted_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP
FROM _cc_old o WHERE c.id=o.old_id AND c.deleted_at IS NULL;

-- 最终校验；任何失败都会回滚这一组。
DO $$
DECLARE n bigint;
BEGIN
  SELECT count(*) INTO n FROM hg_youban_publish_collect_channel c
  WHERE c.tenant_id=(SELECT tenant_id FROM _cc_channels LIMIT 1)
    AND c.deleted_at IS NULL
    AND regexp_replace(btrim(c.telegram_chat_id), '^-100', '')=(SELECT canonical_chat_id FROM _cc_channels LIMIT 1);
  IF n <> 1 THEN RAISE EXCEPTION 'VERIFY FAILED: active collect_channel count=%',n; END IF;
END $$;

COMMIT;
