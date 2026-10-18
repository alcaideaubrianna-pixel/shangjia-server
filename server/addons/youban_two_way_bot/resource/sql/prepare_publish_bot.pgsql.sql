BEGIN;

ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS account_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS bot_type varchar(32) NOT NULL DEFAULT 'normal';
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS tg_account_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS bot_user_id varchar(64) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS supergroup_id varchar(64) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS supergroup_access_hash varchar(128) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS supergroup_title varchar(128) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS invite_link varchar(512) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS setup_status varchar(32) NOT NULL DEFAULT 'pending';
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS webhook_status varchar(32) NOT NULL DEFAULT 'pending';
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS error_message varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS last_setup_at timestamp;
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS last_webhook_at timestamp;
ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS welcome_message varchar(1000) NOT NULL DEFAULT '';

INSERT INTO hg_youban_publish_bot (
  tenant_id,account_id,bot_type,bot_name,bot_username,bot_token,status,
  created_by,updated_by,created_at,updated_at
)
SELECT DISTINCT ON (old.tenant_id,BTRIM(old.bot_token))
  old.tenant_id,old.account_id,'two_way',old.name,old.bot_username,BTRIM(old.bot_token),
  CASE WHEN old.status=1 THEN 1 ELSE 2 END,old.account_id,old.account_id,
  COALESCE(old.created_at,NOW()),COALESCE(old.updated_at,NOW())
FROM hg_youban_two_way_bot_bot old
WHERE BTRIM(old.bot_token)<>'' AND NOT EXISTS (
  SELECT 1 FROM hg_youban_publish_bot current
  WHERE current.tenant_id=old.tenant_id AND current.deleted_at IS NULL
    AND BTRIM(current.bot_token)=BTRIM(old.bot_token)
)
ORDER BY old.tenant_id,BTRIM(old.bot_token),old.status DESC,old.id DESC;

WITH selected_old AS (
  SELECT DISTINCT ON (old.tenant_id,BTRIM(old.bot_token)) old.*
  FROM hg_youban_two_way_bot_bot old
  WHERE BTRIM(old.bot_token)<>''
  ORDER BY old.tenant_id,BTRIM(old.bot_token),(old.deleted_at IS NULL) DESC,old.status DESC,old.id DESC
)
UPDATE hg_youban_publish_bot current
SET account_id=CASE WHEN current.account_id>0 THEN current.account_id ELSE old.account_id END,
    created_by=CASE WHEN current.created_by>0 THEN current.created_by ELSE old.account_id END,
    bot_type='two_way',tg_account_id=old.tg_account_id,bot_user_id=old.bot_user_id,
    supergroup_id=old.supergroup_id,supergroup_access_hash=old.supergroup_access_hash,
    supergroup_title=old.supergroup_title,invite_link=old.invite_link,
    setup_status=old.setup_status,webhook_status=old.webhook_status,
    error_message=old.error_message,last_setup_at=old.last_setup_at,
    last_webhook_at=old.last_webhook_at,welcome_message=old.welcome_message,updated_at=NOW()
FROM selected_old old
WHERE current.tenant_id=old.tenant_id AND current.deleted_at IS NULL
  AND BTRIM(current.bot_token)=BTRIM(old.bot_token);

COMMIT;
