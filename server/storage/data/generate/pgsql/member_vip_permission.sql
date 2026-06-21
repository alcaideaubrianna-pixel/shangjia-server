-- 用户列表：修改会员按钮权限。
-- 可重复执行。

INSERT INTO hg_admin_menu (
  pid, level, tree, title, name, path, icon, type, redirect, permissions,
  permission_name, component, always_show, active_menu, is_root, is_frame,
  frame_src, keep_alive, hidden, affix, sort, remark, status, created_at, updated_at
)
SELECT
  id, level + 1, tree || 'tr_' || id || ' ', '修改会员', 'org_user_vip', '',
  '', 3, '', '/member/vip', '', '', 1, '', 0, 2, '', 0, 0, 0, 55,
  '', 1, now(), now()
FROM hg_admin_menu
WHERE name = 'user'
  AND NOT EXISTS (SELECT 1 FROM hg_admin_menu WHERE name = 'org_user_vip');

INSERT INTO hg_admin_role_menu (role_id, menu_id)
SELECT r.id, m.id
FROM hg_admin_role r
JOIN hg_admin_menu m ON m.name = 'org_user_vip'
WHERE r.id IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM hg_admin_role_menu rm WHERE rm.role_id = r.id AND rm.menu_id = m.id
  );
