INSERT INTO "hg_admin_menu" ("pid","title","name","path","icon","type","permissions","component","level","sort","remark","status","created_at","updated_at")
SELECT parent."id",'开放中心','youbanOpen','youbanOpen','icon-park-outline:cloud-server',2,'','/addons/youbanopen/index',2,80,'开放平台管理',1,NOW(),NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name"='addons' LIMIT 1) parent
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" WHERE "name"='youbanOpen');
INSERT INTO "hg_admin_menu" ("pid","title","name","path","icon","type","permissions","component","level","sort","remark","status","created_at","updated_at")
SELECT p."id",'CMS 应用','youbanOpenCmsApps','cms-apps','icon-park-outline:connection-point-two',2,'/youban_open/cmsApp/list,/youban_open/cmsApp/save,/youban_open/cmsApp/resetSecret','/addons/youbanopen/index',3,1,'管理 CMS 授权应用',1,NOW(),NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name"='youbanOpen' LIMIT 1) p
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" WHERE "name"='youbanOpenCmsApps');
INSERT INTO "hg_admin_role_menu" ("role_id","menu_id") SELECT r."id",m."id" FROM "hg_admin_role" r JOIN "hg_admin_menu" m ON m."name" IN ('youbanOpen','youbanOpenCmsApps') WHERE r."id" IN (1,2) AND NOT EXISTS (SELECT 1 FROM "hg_admin_role_menu" rm WHERE rm."role_id"=r."id" AND rm."menu_id"=m."id");
