# Account visibility read-model acceptance checks

These are read-only PostgreSQL statements for DBX. Run each statement separately against `railway` after the visibility migration and backfill. They compare distinct visibility pairs, so a pair carrying multiple relation labels is counted once.

## Relation counts

```sql
SELECT relation_type, count(*) AS rows
FROM hg_youban_publish_account_visibility
GROUP BY relation_type
ORDER BY relation_type;
```

## Cross-tenant derived edges

The result must be zero.

```sql
SELECT count(*) AS cross_tenant_edges
FROM hg_youban_publish_account_visibility v
JOIN hg_youban_publish_account viewer ON viewer.id = v.viewer_account_id
JOIN hg_youban_publish_account visible ON visible.id = v.visible_account_id
WHERE viewer.tenant_id <> v.tenant_id
   OR visible.tenant_id <> v.tenant_id;
```

## Missing and extra pairs

The following query rebuilds the expected distinct pairs from the authoritative account hierarchy and approved follow relations. Following a tenant administrator grants the administrator's tenant scope, matching `expandFollowNoteAccountIds`.

```sql
WITH active_accounts AS (
  SELECT id, tenant_id, parent_id, account_type
  FROM hg_youban_publish_account
  WHERE status = 1 AND deleted_at IS NULL
), expected AS (
  SELECT tenant_id, id AS viewer_account_id, id AS visible_account_id
  FROM active_accounts
  UNION
  SELECT c.tenant_id, c.parent_id, c.id
  FROM active_accounts c
  JOIN active_accounts p ON p.id = c.parent_id AND p.tenant_id = c.tenant_id
  UNION
  -- A tenant administrator's own scope includes every active account in the
  -- tenant, not only direct children. This matches the tenant_scope rows.
  SELECT admin.tenant_id, admin.id, child.id
  FROM active_accounts admin
  JOIN active_accounts child ON child.tenant_id = admin.tenant_id
  WHERE admin.account_type = 'admin'
  UNION
  SELECT f.tenant_id, f.follower_account_id, f.following_account_id
  FROM hg_youban_publish_account_follow f
  JOIN active_accounts viewer ON viewer.id = f.follower_account_id AND viewer.tenant_id = f.tenant_id
  JOIN active_accounts visible ON visible.id = f.following_account_id AND visible.tenant_id = f.tenant_id
  WHERE f.status = 'approved' AND f.deleted_at IS NULL
  UNION
  SELECT f.tenant_id, f.follower_account_id, child.id
  FROM hg_youban_publish_account_follow f
  JOIN active_accounts admin ON admin.id = f.following_account_id
    AND admin.tenant_id = f.tenant_id AND admin.account_type = 'admin'
  JOIN active_accounts child ON child.tenant_id = admin.tenant_id
  WHERE f.status = 'approved' AND f.deleted_at IS NULL
), model AS (
  SELECT DISTINCT tenant_id, viewer_account_id, visible_account_id
  FROM hg_youban_publish_account_visibility
)
SELECT 'missing' AS difference, count(*) AS pairs
FROM (SELECT * FROM expected EXCEPT SELECT * FROM model) d
UNION ALL
SELECT 'extra' AS difference, count(*) AS pairs
FROM (SELECT * FROM model EXCEPT SELECT * FROM expected) d;
```

Both `missing` and `extra` must be zero. If relation labels are required for debugging, inspect the raw rows for the pair returned by the difference query before changing the projection.

## Tenant-scope and managed spot checks

```sql
SELECT relation_type, count(*) AS rows
FROM hg_youban_publish_account_visibility
WHERE relation_type IN ('managed', 'tenant_scope', 'self', 'following', 'mutual')
GROUP BY relation_type
ORDER BY relation_type;

SELECT count(*) AS stale_managed_or_scope_edges
FROM hg_youban_publish_account_visibility v
LEFT JOIN hg_youban_publish_account visible
  ON visible.id = v.visible_account_id
 AND visible.tenant_id = v.tenant_id
 AND visible.status = 1
 AND visible.deleted_at IS NULL
WHERE v.relation_type IN ('managed', 'tenant_scope')
  AND visible.id IS NULL;
```

`stale_managed_or_scope_edges` must be zero. Do not delete source account or follow rows as part of these checks; the visibility table is a derived projection.
