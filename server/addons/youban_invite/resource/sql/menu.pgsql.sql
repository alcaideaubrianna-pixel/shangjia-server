INSERT INTO "hg_admin_menu" ("pid", "level", "title", "name", "path", "component", "redirect", "icon", "type", "permission", "sort", "hidden", "keepalive", "always_show", "remark", "status", "created_at", "updated_at")
SELECT 0, 1, '邀请返现', 'youbanInvite', '/addons/youbanInvite', '/addons/youbanInvite/index', '', 'icon-park-outline:share', 1, '', 750, 2, 1, 1, '邀请返现插件后台', 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" WHERE "name" = 'youbanInvite');
