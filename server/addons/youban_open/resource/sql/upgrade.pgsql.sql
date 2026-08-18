UPDATE "hg_admin_menu"
SET "title" = '开放中心 - CMS 应用',
    "permissions" = '/youban_open/cmsApp/list,/youban_open/cmsApp/save,/youban_open/cmsApp/resetSecret',
    "remark" = '开放中心应用授权管理',
    "updated_at" = NOW()
WHERE "name" = 'youbanPublishCmsApps';

UPDATE "hg_admin_menu" SET "component" = '/addons/youbanopen/index', "updated_at" = NOW() WHERE "name" = 'youbanOpenCmsApps';
UPDATE "hg_admin_menu" m SET "pid" = p."id", "type" = 2, "path" = 'youbanOpen', "component" = '/addons/youbanopen/index', "level" = 2, "updated_at" = NOW() FROM "hg_admin_menu" p WHERE p."name" = 'addons' AND m."name" = 'youbanOpen';
INSERT INTO "hg_admin_role_menu" ("role_id","menu_id") SELECT r."id",m."id" FROM "hg_admin_role" r JOIN "hg_admin_menu" m ON m."name" IN ('youbanOpen','youbanOpenCmsApps') WHERE r."id" IN (1,2) AND NOT EXISTS (SELECT 1 FROM "hg_admin_role_menu" rm WHERE rm."role_id"=r."id" AND rm."menu_id"=m."id");
ALTER TABLE "hg_youban_publish_cms_app" ADD COLUMN IF NOT EXISTS "review_mode" varchar(32) NOT NULL DEFAULT 'review_required';
CREATE TABLE IF NOT EXISTS "hg_youban_open_profile_event" ("id" bigserial PRIMARY KEY,"app_id" varchar(128) NOT NULL,"event_id" varchar(128) NOT NULL,"actor_id" varchar(128) NOT NULL,"profile_id" bigint NOT NULL,"event_type" varchar(32) NOT NULL,"occurred_at" timestamp,"created_at" timestamp);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybo_event_app_event" ON "hg_youban_open_profile_event" ("app_id","event_id");
CREATE INDEX IF NOT EXISTS "idx_ybo_event_app_profile_time" ON "hg_youban_open_profile_event" ("app_id","profile_id","created_at");
CREATE TABLE IF NOT EXISTS "hg_youban_open_profile_metric_daily" ("id" bigserial PRIMARY KEY,"app_id" varchar(128) NOT NULL,"profile_id" bigint NOT NULL,"metric_date" date NOT NULL,"view_count" integer NOT NULL DEFAULT 0,"unique_view_count" integer NOT NULL DEFAULT 0,"favorite_count" integer NOT NULL DEFAULT 0,"created_at" timestamp,"updated_at" timestamp);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybo_metric_app_profile_date" ON "hg_youban_open_profile_metric_daily" ("app_id","profile_id","metric_date");
CREATE INDEX IF NOT EXISTS "idx_ybo_metric_app_date" ON "hg_youban_open_profile_metric_daily" ("app_id","metric_date");
CREATE TABLE IF NOT EXISTS "hg_youban_open_profile_actor_daily" ("id" bigserial PRIMARY KEY,"app_id" varchar(128) NOT NULL,"actor_id" varchar(128) NOT NULL,"profile_id" bigint NOT NULL,"metric_date" date NOT NULL,"created_at" timestamp);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybo_actor_daily" ON "hg_youban_open_profile_actor_daily" ("app_id","actor_id","profile_id","metric_date");
CREATE TABLE IF NOT EXISTS "hg_youban_open_profile_signal" ("id" bigserial PRIMARY KEY,"app_id" varchar(128) NOT NULL,"actor_id" varchar(128) NOT NULL,"profile_id" bigint NOT NULL,"view_count" integer NOT NULL DEFAULT 0,"is_favorite" smallint NOT NULL DEFAULT 0,"last_interaction_at" timestamp,"created_at" timestamp,"updated_at" timestamp);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybo_signal_app_actor_profile" ON "hg_youban_open_profile_signal" ("app_id","actor_id","profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybo_signal_app_actor_time" ON "hg_youban_open_profile_signal" ("app_id","actor_id","last_interaction_at");
