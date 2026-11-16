-- Read-only relationship-level preview for collect_channel canonicalization.
-- Usage: psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f preview_collect_channel_duplicates.sql
\pset pager off
\x off

\echo '== duplicate groups: survivor, old channels, decision =='
WITH channels AS (
    SELECT c.*,
           regexp_replace(btrim(c.telegram_chat_id), '^-100', '') AS canonical_chat_id,
           row_number() OVER (
               PARTITION BY c.tenant_id, regexp_replace(btrim(c.telegram_chat_id), '^-100', '')
               ORDER BY (c.deleted_at IS NULL) DESC,
                        (btrim(c.telegram_chat_id) LIKE '-100%') DESC,
                        (c.title IS NOT NULL AND btrim(c.title) <> '') DESC,
                        (c.username IS NOT NULL AND btrim(c.username) <> '') DESC,
                        c.id
           ) AS survivor_rank
    FROM hg_youban_publish_collect_channel c
    WHERE btrim(c.telegram_chat_id) <> ''
), groups AS (
    SELECT tenant_id, canonical_chat_id, count(*) AS channel_count
    FROM channels
    GROUP BY tenant_id, canonical_chat_id
    HAVING count(*) > 1
), scoped AS (
    SELECT c.*, g.channel_count,
           max(c.id) FILTER (WHERE c.survivor_rank = 1) OVER (PARTITION BY c.tenant_id, c.canonical_chat_id) AS survivor_channel_id
    FROM channels c
    JOIN groups g USING (tenant_id, canonical_chat_id)
), relation_groups AS (
    SELECT s.tenant_id, s.canonical_chat_id,
           count(ac.id) AS account_relation_count,
           count(*) FILTER (WHERE ac.collect_enabled = 1 AND ac.status = 'enabled' AND ac.approval_status = 'approved') AS allowed_count,
           count(*) FILTER (WHERE ac.status = 'disabled' OR ac.approval_status = 'rejected' OR coalesce(ac.disabled_reason, '') <> '') AS blocked_or_manual_count,
           count(*) FILTER (WHERE coalesce(ac.disabled_reason, '') <> '' OR ac.disabled_at IS NOT NULL) AS manual_decision_count
    FROM scoped s
    LEFT JOIN hg_youban_publish_collect_account_channel ac
      ON ac.tenant_id = s.tenant_id AND ac.channel_id = s.id AND ac.deleted_at IS NULL
    GROUP BY s.tenant_id, s.canonical_chat_id
), decision AS (
    SELECT s.tenant_id, s.canonical_chat_id,
           bool_or(s.id <> s.survivor_channel_id AND EXISTS (
               SELECT 1 FROM hg_youban_publish_collect_bot_channel bc
               WHERE bc.tenant_id = s.tenant_id AND bc.channel_id = s.id AND bc.deleted_at IS NULL
           )) AS old_bot_ref,
           bool_or(s.id <> s.survivor_channel_id AND EXISTS (
               SELECT 1 FROM hg_youban_publish_collect_account_channel ac
               WHERE ac.tenant_id = s.tenant_id AND ac.channel_id = s.id AND ac.deleted_at IS NULL
           )) AS old_account_ref,
           bool_or(s.id <> s.survivor_channel_id AND EXISTS (
               SELECT 1 FROM hg_youban_publish_collect_rule_channel rc
               WHERE rc.tenant_id = s.tenant_id AND rc.channel_id = s.id
           )) AS old_rule_ref,
           bool_or(EXISTS (
               SELECT 1 FROM hg_youban_publish_collect_channel_profile cp
               WHERE cp.tenant_id = s.tenant_id
                 AND regexp_replace(btrim(cp.channel_id), '^-100', '') = s.canonical_chat_id
                 AND cp.deleted_at IS NULL
           )) AS profile_ref,
           bool_or(EXISTS (
               SELECT 1 FROM hg_youban_publish_collect_channel_count cc
               WHERE cc.tenant_id = s.tenant_id
                 AND regexp_replace(btrim(cc.channel_id), '^-100', '') = s.canonical_chat_id
           )) AS count_ref,
           bool_or(EXISTS (
               SELECT 1 FROM hg_youban_publish_collect_channel_count_member cm
               WHERE cm.tenant_id = s.tenant_id
                 AND regexp_replace(btrim(cm.channel_id), '^-100', '') = s.canonical_chat_id
           )) AS count_member_ref
    FROM scoped s
    GROUP BY s.tenant_id, s.canonical_chat_id
)
SELECT rg.tenant_id,
       rg.canonical_chat_id,
       max(s.survivor_channel_id) AS survivor_channel_id,
       string_agg(s.id::text, ',' ORDER BY s.id) FILTER (WHERE s.id <> s.survivor_channel_id) AS old_channel_ids,
       rg.account_relation_count,
       rg.allowed_count,
       rg.blocked_or_manual_count,
       rg.manual_decision_count,
       d.old_bot_ref AS old_channel_has_bot_ref,
       d.old_account_ref AS old_channel_has_account_ref,
       d.old_rule_ref AS old_channel_has_rule_ref,
       d.profile_ref,
       d.count_ref,
       d.count_member_ref,
       CASE
           WHEN rg.manual_decision_count > 0
             OR (rg.allowed_count > 0 AND rg.blocked_or_manual_count > 0)
             OR d.old_bot_ref OR d.old_account_ref OR d.old_rule_ref
           THEN 'manual_conflict'
           ELSE 'safe_auto_merge'
       END AS merge_decision
FROM relation_groups rg
JOIN scoped s USING (tenant_id, canonical_chat_id)
JOIN decision d USING (tenant_id, canonical_chat_id)
GROUP BY rg.tenant_id, rg.canonical_chat_id, rg.account_relation_count,
         rg.allowed_count, rg.blocked_or_manual_count, rg.manual_decision_count,
         d.old_bot_ref, d.old_account_ref, d.old_rule_ref, d.profile_ref,
         d.count_ref, d.count_member_ref
ORDER BY rg.tenant_id, rg.canonical_chat_id;

\echo '== duplicate group channel records and selected survivor =='
WITH channels AS (
    SELECT c.*,
           regexp_replace(btrim(c.telegram_chat_id), '^-100', '') AS canonical_chat_id,
           row_number() OVER (
               PARTITION BY c.tenant_id, regexp_replace(btrim(c.telegram_chat_id), '^-100', '')
               ORDER BY (c.deleted_at IS NULL) DESC,
                        (btrim(c.telegram_chat_id) LIKE '-100%') DESC,
                        (c.title IS NOT NULL AND btrim(c.title) <> '') DESC,
                        (c.username IS NOT NULL AND btrim(c.username) <> '') DESC,
                        c.id
           ) AS survivor_rank
    FROM hg_youban_publish_collect_channel c
    WHERE btrim(c.telegram_chat_id) <> ''
), groups AS (
    SELECT tenant_id, canonical_chat_id FROM channels GROUP BY tenant_id, canonical_chat_id HAVING count(*) > 1
)
SELECT c.tenant_id, c.canonical_chat_id,
       CASE WHEN c.survivor_rank = 1 THEN 'survivor' ELSE 'old_channel' END AS channel_role,
       c.id AS channel_id, c.telegram_chat_id, c.title, c.username, c.status, c.deleted_at
FROM channels c JOIN groups g USING (tenant_id, canonical_chat_id)
ORDER BY c.tenant_id, c.canonical_chat_id, c.survivor_rank, c.id;

\echo '== account_channel relationship details =='
WITH channels AS (
    SELECT id, tenant_id,
           regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id,
           row_number() OVER (
               PARTITION BY tenant_id, regexp_replace(btrim(telegram_chat_id), '^-100', '')
               ORDER BY (deleted_at IS NULL) DESC, (btrim(telegram_chat_id) LIKE '-100%') DESC, id
           ) AS survivor_rank
    FROM hg_youban_publish_collect_channel
    WHERE btrim(telegram_chat_id) <> ''
), groups AS (
    SELECT tenant_id, canonical_chat_id FROM channels GROUP BY tenant_id, canonical_chat_id HAVING count(*) > 1
)
SELECT ac.tenant_id, ac.account_id, ac.source_id, c.canonical_chat_id,
       c.id AS channel_id, ac.id AS relation_id,
       CASE WHEN c.survivor_rank = 1 THEN 'survivor_relation_candidate' ELSE 'old_channel_relation' END AS relation_role,
       ac.collect_enabled, ac.status, ac.approval_status,
       ac.approved_at, ac.disabled_at, ac.disabled_reason,
       ac.created_at, ac.updated_at, ac.deleted_at,
       CASE
           WHEN coalesce(ac.disabled_reason, '') <> '' OR ac.disabled_at IS NOT NULL
             OR ac.status = 'disabled' OR ac.approval_status = 'rejected'
           THEN 'manual_or_blocked'
           WHEN ac.collect_enabled = 1 AND ac.status = 'enabled' AND ac.approval_status = 'approved'
           THEN 'allowed'
           ELSE 'pending_or_other'
       END AS relation_decision
FROM hg_youban_publish_collect_account_channel ac
JOIN channels c ON c.tenant_id = ac.tenant_id AND c.id = ac.channel_id
JOIN groups g USING (tenant_id, canonical_chat_id)
WHERE ac.deleted_at IS NULL
ORDER BY ac.tenant_id, ac.account_id, ac.source_id, c.canonical_chat_id, ac.id;

\echo '== bot_channel references by duplicate group =='
WITH channels AS (
    SELECT id, tenant_id, regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id
    FROM hg_youban_publish_collect_channel WHERE btrim(telegram_chat_id) <> ''
), groups AS (
    SELECT tenant_id, canonical_chat_id FROM channels GROUP BY tenant_id, canonical_chat_id HAVING count(*) > 1
)
SELECT bc.tenant_id, c.canonical_chat_id, bc.bot_id, bc.id AS bot_channel_id,
       bc.channel_id, bc.status, bc.first_seen_at, bc.last_seen_at, bc.deleted_at
FROM hg_youban_publish_collect_bot_channel bc
JOIN channels c ON c.tenant_id = bc.tenant_id AND c.id = bc.channel_id
JOIN groups g USING (tenant_id, canonical_chat_id)
WHERE bc.deleted_at IS NULL
ORDER BY bc.tenant_id, c.canonical_chat_id, bc.bot_id, bc.id;

\echo '== rule_channel references by duplicate group =='
WITH channels AS (
    SELECT id, tenant_id, regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id
    FROM hg_youban_publish_collect_channel WHERE btrim(telegram_chat_id) <> ''
), groups AS (
    SELECT tenant_id, canonical_chat_id FROM channels GROUP BY tenant_id, canonical_chat_id HAVING count(*) > 1
)
SELECT rc.tenant_id, rc.account_id, rc.rule_id, c.canonical_chat_id,
       rc.id AS rule_channel_id, rc.channel_id, rc.created_at
FROM hg_youban_publish_collect_rule_channel rc
JOIN channels c ON c.tenant_id = rc.tenant_id AND c.id = rc.channel_id
JOIN groups g USING (tenant_id, canonical_chat_id)
ORDER BY rc.tenant_id, c.canonical_chat_id, rc.account_id, rc.rule_id, rc.id;

\echo '== profile/count references by duplicate group =='
WITH groups AS (
    SELECT tenant_id, regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id
    FROM hg_youban_publish_collect_channel WHERE btrim(telegram_chat_id) <> ''
    GROUP BY tenant_id, regexp_replace(btrim(telegram_chat_id), '^-100', '')
    HAVING count(*) > 1
), refs AS (
    SELECT 'profile' AS ref_type, tenant_id, account_id, rule_id,
           regexp_replace(btrim(channel_id), '^-100', '') AS canonical_chat_id,
           count(*) AS reference_count
    FROM hg_youban_publish_collect_channel_profile
    WHERE deleted_at IS NULL AND btrim(channel_id) <> ''
    GROUP BY tenant_id, account_id, rule_id, regexp_replace(btrim(channel_id), '^-100', '')
    UNION ALL
    SELECT 'count', tenant_id, account_id, rule_id,
           regexp_replace(btrim(channel_id), '^-100', ''), count(*)
    FROM hg_youban_publish_collect_channel_count
    WHERE btrim(channel_id) <> ''
    GROUP BY tenant_id, account_id, rule_id, regexp_replace(btrim(channel_id), '^-100', '')
    UNION ALL
    SELECT 'count_member', tenant_id, account_id, rule_id,
           regexp_replace(btrim(channel_id), '^-100', ''), count(*)
    FROM hg_youban_publish_collect_channel_count_member
    WHERE btrim(channel_id) <> ''
    GROUP BY tenant_id, account_id, rule_id, regexp_replace(btrim(channel_id), '^-100', '')
)
SELECT r.ref_type, r.tenant_id, r.account_id, r.rule_id, r.canonical_chat_id, r.reference_count
FROM refs r JOIN groups g USING (tenant_id, canonical_chat_id)
ORDER BY r.tenant_id, r.canonical_chat_id, r.ref_type, r.account_id, r.rule_id;

\echo '== decision blockers and summary =='
WITH channels AS (
    SELECT id, tenant_id, regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id
    FROM hg_youban_publish_collect_channel WHERE btrim(telegram_chat_id) <> ''
), groups AS (
    SELECT tenant_id, canonical_chat_id FROM channels GROUP BY tenant_id, canonical_chat_id HAVING count(*) > 1
), relation_groups AS (
    SELECT g.tenant_id, g.canonical_chat_id,
           count(ac.id) AS relation_count,
           count(*) FILTER (WHERE ac.collect_enabled = 1 AND ac.status = 'enabled' AND ac.approval_status = 'approved') AS allowed_count,
           count(*) FILTER (WHERE ac.status = 'disabled' OR ac.approval_status = 'rejected' OR coalesce(ac.disabled_reason, '') <> '' OR ac.disabled_at IS NOT NULL) AS blocked_count
    FROM groups g
    JOIN channels c USING (tenant_id, canonical_chat_id)
    LEFT JOIN hg_youban_publish_collect_account_channel ac
      ON ac.tenant_id = c.tenant_id AND ac.channel_id = c.id AND ac.deleted_at IS NULL
    GROUP BY g.tenant_id, g.canonical_chat_id
)
SELECT count(*) AS duplicate_group_count,
       count(*) FILTER (WHERE relation_count = 0 OR (allowed_count > 0 AND blocked_count = 0)) AS relation_safe_group_count,
       count(*) FILTER (WHERE blocked_count > 0 OR (allowed_count > 0 AND blocked_count > 0)) AS manual_conflict_group_count
FROM relation_groups;
