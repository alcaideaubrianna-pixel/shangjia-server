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
SELECT DISTINCT ON (old.tenant_id, BTRIM(old.bot_token))
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

CREATE TEMP TABLE ybtwb_bot_map ON COMMIT DROP AS
SELECT old.id AS old_id, current.id AS new_id
FROM hg_youban_two_way_bot_bot old
JOIN LATERAL (
  SELECT candidate.id
  FROM hg_youban_publish_bot candidate
  WHERE candidate.deleted_at IS NULL
    AND BTRIM(candidate.bot_token) = BTRIM(old.bot_token)
    AND candidate.tenant_id = old.tenant_id
  ORDER BY (candidate.account_id = old.account_id) DESC,
           (candidate.bot_type = 'two_way') DESC,
           candidate.id ASC
  LIMIT 1
) current ON TRUE
WHERE BTRIM(old.bot_token) <> '';

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM hg_youban_two_way_bot_bot old
    WHERE BTRIM(old.bot_token) <> ''
      AND NOT EXISTS (SELECT 1 FROM ybtwb_bot_map map WHERE map.old_id = old.id)
  ) THEN
    RAISE EXCEPTION '存在无法映射到 hg_youban_publish_bot 的双向机器人';
  END IF;
END $$;

WITH selected_map AS (
  SELECT DISTINCT ON (map.new_id) map.new_id,map.old_id
  FROM ybtwb_bot_map map
  JOIN hg_youban_two_way_bot_bot old ON old.id=map.old_id
  ORDER BY map.new_id,(old.deleted_at IS NULL) DESC,old.status DESC,old.id DESC
)
UPDATE hg_youban_publish_bot current
SET account_id = CASE WHEN current.account_id > 0 THEN current.account_id ELSE old.account_id END,
    created_by = CASE WHEN current.created_by > 0 THEN current.created_by ELSE old.account_id END,
    bot_type = 'two_way', tg_account_id = old.tg_account_id,
    bot_user_id = old.bot_user_id, supergroup_id = old.supergroup_id,
    supergroup_access_hash = old.supergroup_access_hash,
    supergroup_title = old.supergroup_title, invite_link = old.invite_link,
    setup_status = old.setup_status, webhook_status = old.webhook_status,
    error_message = old.error_message, last_setup_at = old.last_setup_at,
    last_webhook_at = old.last_webhook_at, welcome_message = old.welcome_message,
    updated_at = NOW()
FROM selected_map map
JOIN hg_youban_two_way_bot_bot old ON old.id = map.old_id
WHERE current.id = map.new_id;

DELETE FROM hg_youban_two_way_bot_topic item
USING ybtwb_bot_map item_map, hg_youban_two_way_bot_topic keeper, ybtwb_bot_map keeper_map
WHERE item.bot_id=item_map.old_id AND keeper.bot_id=keeper_map.old_id
  AND item_map.new_id=keeper_map.new_id
  AND item.tenant_id=keeper.tenant_id
  AND (item.telegram_user_id=keeper.telegram_user_id OR item.thread_id=keeper.thread_id)
  AND item.id<keeper.id;
UPDATE hg_youban_two_way_bot_topic item SET bot_id = map.new_id FROM ybtwb_bot_map map WHERE item.bot_id = map.old_id;
UPDATE hg_youban_two_way_bot_message item SET bot_id = map.new_id FROM ybtwb_bot_map map WHERE item.bot_id = map.old_id;
UPDATE hg_youban_two_way_bot_cooperation_config item SET two_way_bot_id = map.new_id FROM ybtwb_bot_map map WHERE item.two_way_bot_id = map.old_id;

CREATE TEMP TABLE ybtwb_relay_map ON COMMIT DROP AS
SELECT bot_id AS old_id,source_bot_id AS new_id
FROM hg_youban_bot_relay_config
WHERE source_bot_id>0 AND bot_id<>source_bot_id;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM ybtwb_relay_map map
    JOIN hg_youban_bot_relay_config target ON target.bot_id=map.new_id
    WHERE target.source_bot_id<>map.new_id
  ) THEN
    RAISE EXCEPTION '目标 Bot ID 已被其他 relay 配置占用，禁止自动覆盖';
  END IF;
END $$;

CREATE TEMP TABLE ybtwb_binding_map ON COMMIT DROP AS
SELECT old_binding.id AS old_id,target_binding.id AS new_id
FROM hg_youban_bot_relay_binding old_binding
JOIN ybtwb_relay_map map ON map.old_id=old_binding.bot_id
JOIN hg_youban_bot_relay_binding target_binding
  ON target_binding.bot_id=map.new_id AND target_binding.pair_key=old_binding.pair_key;
UPDATE hg_youban_bot_relay_endpoint item SET binding_id=map.new_id FROM ybtwb_binding_map map WHERE item.binding_id=map.old_id;
UPDATE hg_youban_bot_relay_delivery item SET binding_id=map.new_id FROM ybtwb_binding_map map WHERE item.binding_id=map.old_id;
DELETE FROM hg_youban_bot_relay_binding item USING ybtwb_binding_map map WHERE item.id=map.old_id;

DELETE FROM hg_youban_bot_relay_admin item USING ybtwb_relay_map map
WHERE item.bot_id=map.old_id AND EXISTS (SELECT 1 FROM hg_youban_bot_relay_admin target WHERE target.bot_id=map.new_id AND target.username=item.username);
DELETE FROM hg_youban_bot_relay_audit_topic item USING ybtwb_relay_map map
WHERE item.bot_id=map.old_id AND EXISTS (SELECT 1 FROM hg_youban_bot_relay_audit_topic target WHERE target.bot_id=map.new_id AND target.group_id=item.group_id AND target.pair_key=item.pair_key);
DELETE FROM hg_youban_bot_relay_endpoint item USING ybtwb_relay_map map
WHERE item.bot_id=map.old_id AND EXISTS (SELECT 1 FROM hg_youban_bot_relay_endpoint target WHERE target.bot_id=map.new_id AND target.chat_id=item.chat_id);
DELETE FROM hg_youban_bot_relay_rule item USING ybtwb_relay_map map
WHERE item.bot_id=map.old_id AND EXISTS (SELECT 1 FROM hg_youban_bot_relay_rule target WHERE target.bot_id=map.new_id AND target.kind=item.kind AND target.value=item.value);
DELETE FROM hg_youban_bot_relay_delivery item USING ybtwb_relay_map map
WHERE item.bot_id=map.old_id AND EXISTS (
  SELECT 1 FROM hg_youban_bot_relay_delivery target
  WHERE target.bot_id=map.new_id AND target.source_chat_id=item.source_chat_id
    AND target.source_message_id=item.source_message_id AND target.kind=item.kind
    AND target.target_chat_id=item.target_chat_id
);

UPDATE hg_youban_bot_relay_admin item SET bot_id=map.new_id FROM ybtwb_relay_map map WHERE item.bot_id=map.old_id;
UPDATE hg_youban_bot_relay_audit_topic item SET bot_id=map.new_id FROM ybtwb_relay_map map WHERE item.bot_id=map.old_id;
UPDATE hg_youban_bot_relay_binding item SET bot_id=map.new_id FROM ybtwb_relay_map map WHERE item.bot_id=map.old_id;
UPDATE hg_youban_bot_relay_delivery item SET bot_id=map.new_id FROM ybtwb_relay_map map WHERE item.bot_id=map.old_id;
UPDATE hg_youban_bot_relay_endpoint item SET bot_id=map.new_id FROM ybtwb_relay_map map WHERE item.bot_id=map.old_id;
UPDATE hg_youban_bot_relay_rule item SET bot_id=map.new_id FROM ybtwb_relay_map map WHERE item.bot_id=map.old_id;
UPDATE hg_youban_bot_relay_config item SET bot_id=map.new_id FROM ybtwb_relay_map map WHERE item.bot_id=map.old_id;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM hg_youban_two_way_bot_topic item LEFT JOIN hg_youban_publish_bot bot ON bot.id=item.bot_id WHERE bot.id IS NULL)
    OR EXISTS (SELECT 1 FROM hg_youban_two_way_bot_message item LEFT JOIN hg_youban_publish_bot bot ON bot.id=item.bot_id WHERE bot.id IS NULL)
    OR EXISTS (SELECT 1 FROM hg_youban_bot_relay_config item LEFT JOIN hg_youban_publish_bot bot ON bot.id=item.bot_id WHERE item.source_bot_id>0 AND bot.id IS NULL)
  THEN
    RAISE EXCEPTION '统一 Bot 主表后仍存在孤儿记录';
  END IF;
END $$;

COMMIT;
