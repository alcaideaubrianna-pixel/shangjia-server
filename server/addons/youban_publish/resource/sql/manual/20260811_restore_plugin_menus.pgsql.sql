BEGIN;

UPDATE "hg_admin_menu"
SET "hidden" = 0,
    "status" = 1,
    "redirect" = '/develop/addons/index',
    "updated_at" = NOW()
WHERE "name" = 'addons';

UPDATE "hg_admin_menu" AS menu
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
WHERE menu."name" = 'develop_addons';

WITH installed_roots("addon_name", "menu_name") AS (
    VALUES
        ('youban_publish', 'youbanPublish'),
        ('youban_two_way_bot', 'youbanTwoWayBot'),
        ('youban_bot', 'youbanBot'),
        ('youban_feiniu_sync', 'youbanFeiniuSync'),
        ('lazysheep_tggo', 'addons_lazysheep_tggo')
), active_roots AS (
    SELECT menu."id", menu."tree", menu."name"
    FROM installed_roots mapping
    JOIN "hg_sys_addons_install" install
      ON install."name" = mapping."addon_name"
     AND install."status" = 1
    JOIN "hg_admin_menu" menu
      ON menu."name" = mapping."menu_name"
)
UPDATE "hg_admin_menu" AS menu
SET "status" = 1,
    "hidden" = CASE WHEN menu."id" = root."id" THEN 2 ELSE menu."hidden" END,
    "updated_at" = NOW()
FROM active_roots root
WHERE menu."id" = root."id"
   OR menu."tree" LIKE root."tree" || 'tr_' || root."id"::text || ' %';

WITH target_roles AS (
    SELECT "id" AS "role_id"
    FROM "hg_admin_role"
    WHERE "id" IN (1, 2)

    UNION

    SELECT role_menu."role_id"
    FROM "hg_admin_role_menu" role_menu
    JOIN "hg_admin_menu" root ON root."id" = role_menu."menu_id"
    WHERE root."name" = 'addons'
), target_menus AS (
    SELECT menu."id" AS "menu_id"
    FROM "hg_admin_menu" menu
    WHERE menu."name" = 'develop_addons'

    UNION

    SELECT menu."id"
    FROM "hg_admin_menu" menu
    JOIN "hg_admin_menu" root
      ON root."name" IN (
          'youbanPublish',
          'youbanTwoWayBot',
          'youbanBot',
          'youbanFeiniuSync',
          'addons_lazysheep_tggo'
      )
    JOIN (VALUES
        ('youbanPublish', 'youban_publish'),
        ('youbanTwoWayBot', 'youban_two_way_bot'),
        ('youbanBot', 'youban_bot'),
        ('youbanFeiniuSync', 'youban_feiniu_sync'),
        ('addons_lazysheep_tggo', 'lazysheep_tggo')
    ) AS mapping("menu_name", "addon_name") ON mapping."menu_name" = root."name"
    JOIN "hg_sys_addons_install" install
      ON install."name" = mapping."addon_name"
     AND install."status" = 1
    WHERE menu."id" = root."id"
       OR menu."tree" LIKE root."tree" || 'tr_' || root."id"::text || ' %'
)
INSERT INTO "hg_admin_role_menu" ("role_id", "menu_id")
SELECT roles."role_id", menus."menu_id"
FROM target_roles roles
CROSS JOIN target_menus menus
ON CONFLICT ("role_id", "menu_id") DO NOTHING;

DELETE FROM "hg_admin_role_menu" role_menu
USING "hg_admin_menu" menu
JOIN "hg_admin_menu" example ON example."name" = 'hgexample'
WHERE role_menu."menu_id" = menu."id"
  AND (
      menu."id" = example."id"
      OR menu."tree" LIKE example."tree" || 'tr_' || example."id"::text || ' %'
  );

UPDATE "hg_admin_menu" AS menu
SET "status" = 2,
    "hidden" = 1,
    "updated_at" = NOW()
FROM (
    SELECT "id", "tree"
    FROM "hg_admin_menu"
    WHERE "name" = 'hgexample'
    LIMIT 1
) AS example
WHERE menu."id" = example."id"
   OR menu."tree" LIKE example."tree" || 'tr_' || example."id"::text || ' %';

COMMIT;
