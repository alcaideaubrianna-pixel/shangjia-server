CREATE TABLE IF NOT EXISTS hg_youban_bot_role_registry (
  telegram_bot_id bigint NOT NULL PRIMARY KEY,
  tenant_id bigint NOT NULL,
  role varchar(16) NOT NULL,
  require_registration smallint NOT NULL DEFAULT 0
);
