INSERT INTO hg_admin_menu (
  pid, title, name, path, icon, type, redirect, permissions, permission_name,
  component, always_show, active_menu, is_root, is_frame, frame_src, keep_alive,
  hidden, affix, level, tree, sort, remark, status, created_at, updated_at
)
SELECT
  0, '悦伴客服', 'YoubanChat', '/youban-chat', 'MessageOutlined', 1,
  '/youban-chat/workbench', '', '', 'LAYOUT', 1, '',
  0, 0, '', 0, 0, 0, 1, '', 91, '', 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM hg_admin_menu WHERE name = 'YoubanChat');

UPDATE hg_admin_menu
SET title = '悦伴客服',
    pid = 0,
    path = '/youban-chat',
    icon = 'MessageOutlined',
    type = 1,
    redirect = '/youban-chat/workbench',
    permissions = '',
    component = 'LAYOUT',
    always_show = 1,
    hidden = 0,
    level = 1,
    tree = '',
    sort = 91,
    status = 1,
    updated_at = NOW()
WHERE name = 'YoubanChat';

INSERT INTO hg_admin_menu (
  pid, title, name, path, icon, type, redirect, permissions, permission_name,
  component, always_show, active_menu, is_root, is_frame, frame_src, keep_alive,
  hidden, affix, level, tree, sort, remark, status, created_at, updated_at
)
SELECT
  root.id, '客服工作台', 'YoubanChatWorkbench', 'workbench', '', 2,
  '', '/youban_chat/chat/list', '', '/addons/youban_chat/index', 1, '',
  0, 0, '', 0, 0, 0, 2, CONCAT('tr_', root.id, ' '), 10, '', 1, NOW(), NOW()
FROM (SELECT id FROM hg_admin_menu WHERE name = 'YoubanChat' LIMIT 1) root
WHERE root.id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM hg_admin_menu WHERE name = 'YoubanChatWorkbench');

UPDATE hg_admin_menu
SET pid = (SELECT id FROM hg_admin_menu WHERE name = 'YoubanChat' LIMIT 1),
    title = '客服工作台',
    path = 'workbench',
    type = 2,
    permissions = '/youban_chat/chat/list',
    component = '/addons/youban_chat/index',
    always_show = 1,
    hidden = 0,
    level = 2,
    tree = CONCAT('tr_', (SELECT id FROM hg_admin_menu WHERE name = 'YoubanChat' LIMIT 1), ' '),
    sort = 10,
    status = 1,
    updated_at = NOW()
WHERE name = 'YoubanChatWorkbench';

INSERT INTO hg_admin_menu (
  pid, title, name, path, icon, type, redirect, permissions, permission_name,
  component, always_show, active_menu, is_root, is_frame, frame_src, keep_alive,
  hidden, affix, level, tree, sort, remark, status, created_at, updated_at
)
SELECT
  list.id, item.title, item.name, '', '', 3, '', item.permissions, '',
  '', 1, '', 0, 0, '', 0, 1, 0, 3, CONCAT('tr_', root.id, ' tr_', list.id, ' '),
  item.sort, '', 1, NOW(), NOW()
FROM (SELECT id FROM hg_admin_menu WHERE name = 'YoubanChat' LIMIT 1) root
JOIN (SELECT id FROM hg_admin_menu WHERE name = 'YoubanChatWorkbench' LIMIT 1) list ON 1=1
JOIN (
  VALUES
    ('客服会话详情', 'YoubanChatConversationView', '/youban_chat/chat/view,/youban_chat/chat/messages,/youban_chat/chat/clear', 10),
    ('Bot管理', 'YoubanChatBot', '/youban_chat/chat/botList,/youban_chat/chat/saveBot', 25),
    ('频道群绑定', 'YoubanChatBinding', '/youban_chat/chat/bindingList,/youban_chat/chat/saveBinding,/youban_chat/chat/channelOptions', 30),
    ('客服绑定', 'YoubanChatOperator', '/youban_chat/chat/operatorList,/youban_chat/chat/saveOperator', 40),
    ('功能配置', 'YoubanChatFeature', '/youban_chat/chat/featureList,/youban_chat/chat/saveFeature', 50)
) AS item(title, name, permissions, sort) ON 1=1
WHERE root.id IS NOT NULL
  AND list.id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM hg_admin_menu m WHERE m.name = item.name);

UPDATE hg_admin_menu
SET permissions = '/youban_chat/chat/view,/youban_chat/chat/messages,/youban_chat/chat/clear',
    updated_at = NOW()
WHERE name = 'YoubanChatConversationView';

INSERT INTO hg_admin_role_menu (role_id, menu_id)
SELECT r.id, m.id
FROM hg_admin_role r
JOIN hg_admin_menu m ON m.name IN (
  'YoubanChat',
  'YoubanChatWorkbench',
  'YoubanChatConversationView',
  'YoubanChatBot',
  'YoubanChatBinding',
  'YoubanChatOperator',
  'YoubanChatFeature'
)
WHERE r.id IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM hg_admin_role_menu rm WHERE rm.role_id = r.id AND rm.menu_id = m.id
  );
INSERT INTO hg_admin_menu (pid,title,name,path,icon,type,redirect,permissions,permission_name,component,always_show,active_menu,is_root,is_frame,frame_src,keep_alive,hidden,affix,level,tree,sort,remark,status,created_at,updated_at)
SELECT root.id,'悦伴客服','youbanChatAdmin','youbanChatAdmin','lucide:messages-square',2,'','/youban_chat/chat/list,/youban_chat/chat/view,/youban_chat/chat/messages,/youban_chat/chat/clear,/youban_chat/chat/botList,/youban_chat/chat/saveBot,/youban_chat/chat/bindingList,/youban_chat/chat/saveBinding,/youban_chat/chat/channelOptions,/youban_chat/chat/operatorList,/youban_chat/chat/saveOperator,/youban_chat/chat/featureList,/youban_chat/chat/saveFeature','','/addons/youban_chat/index',1,'',0,0,'',1,2,0,2,'tr_'||root.id::text||' ',36,'悦伴客服插件后台',1,NOW(),NOW()
FROM (SELECT id FROM hg_admin_menu WHERE name='addons' LIMIT 1) root WHERE NOT EXISTS (SELECT 1 FROM hg_admin_menu WHERE name='youbanChatAdmin');

UPDATE hg_admin_menu SET title='悦伴客服',path='youbanChatAdmin',icon='lucide:messages-square',type=2,component='/addons/youban_chat/index',permissions='/youban_chat/chat/list,/youban_chat/chat/view,/youban_chat/chat/messages,/youban_chat/chat/clear,/youban_chat/chat/botList,/youban_chat/chat/saveBot,/youban_chat/chat/bindingList,/youban_chat/chat/saveBinding,/youban_chat/chat/channelOptions,/youban_chat/chat/operatorList,/youban_chat/chat/saveOperator,/youban_chat/chat/featureList,/youban_chat/chat/saveFeature',hidden=2,status=1,updated_at=NOW() WHERE name='youbanChatAdmin';

INSERT INTO hg_admin_role_menu (role_id,menu_id) SELECT r.id,m.id FROM hg_admin_role r JOIN hg_admin_menu m ON m.name='youbanChatAdmin' WHERE r.id IN (1,2) AND NOT EXISTS (SELECT 1 FROM hg_admin_role_menu rm WHERE rm.role_id=r.id AND rm.menu_id=m.id);
