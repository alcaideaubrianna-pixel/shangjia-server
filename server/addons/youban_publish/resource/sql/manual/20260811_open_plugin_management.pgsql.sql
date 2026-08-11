BEGIN;

UPDATE "hg_admin_menu"
SET "redirect" = '/develop/addons/index',
    "updated_at" = NOW()
WHERE "name" = 'addons';

UPDATE "hg_admin_menu" AS m
SET "pid" = root."id",
    "title" = '插件管理',
    "path" = 'addons',
    "type" = 2,
    "redirect" = '',
    "permissions" = '/addons/selects,/addons/list',
    "component" = '/develop/addons/index',
    "always_show" = 1,
    "hidden" = 0,
    "level" = 2,
    "tree" = 'tr_' || root."id"::text || ' ',
    "status" = 1,
    "updated_at" = NOW()
FROM (
    SELECT "id"
    FROM "hg_admin_menu"
    WHERE "name" = 'addons'
    LIMIT 1
) AS root
WHERE m."name" = 'develop_addons';

DELETE FROM "hg_admin_role_menu" AS rm
USING "hg_admin_menu" AS m
JOIN "hg_admin_menu" AS example ON example."name" = 'hgexample'
WHERE rm."menu_id" = m."id"
  AND (
      m."id" = example."id"
      OR m."tree" LIKE example."tree" || 'tr_' || example."id"::text || ' %'
  );

UPDATE "hg_admin_menu" AS m
SET "status" = 2,
    "hidden" = 1,
    "updated_at" = NOW()
FROM (
    SELECT "id", "tree"
    FROM "hg_admin_menu"
    WHERE "name" = 'hgexample'
    LIMIT 1
) AS example
WHERE m."name" = 'hgexample'
   OR m."tree" LIKE example."tree" || 'tr_' || example."id"::text || ' %';

WITH target AS (
    SELECT "id"
    FROM "hg_admin_menu"
    WHERE "name" = 'develop_addons'
    LIMIT 1
), eligible_roles AS (
    SELECT "id" AS role_id
    FROM "hg_admin_role"
    WHERE "id" IN (1, 2)
    UNION
    SELECT DISTINCT rm."role_id"
    FROM "hg_admin_role_menu" AS rm
    JOIN "hg_admin_menu" AS root ON root."id" = rm."menu_id"
    WHERE root."name" = 'addons'
)
INSERT INTO "hg_admin_role_menu" ("role_id", "menu_id")
SELECT roles.role_id, target."id"
FROM eligible_roles AS roles
CROSS JOIN target
WHERE NOT EXISTS (
    SELECT 1
    FROM "hg_admin_role_menu" AS rm
    WHERE rm."role_id" = roles.role_id
      AND rm."menu_id" = target."id"
);

SELECT "id", "pid", "title", "name", "hidden", "status", "redirect", "component"
FROM "hg_admin_menu"
WHERE "name" IN ('addons', 'develop_addons', 'hgexample')
ORDER BY "id";

SELECT rm."role_id", m."name", m."title"
FROM "hg_admin_role_menu" AS rm
JOIN "hg_admin_menu" AS m ON m."id" = rm."menu_id"
WHERE m."name" = 'develop_addons'
ORDER BY rm."role_id";

COMMIT;
