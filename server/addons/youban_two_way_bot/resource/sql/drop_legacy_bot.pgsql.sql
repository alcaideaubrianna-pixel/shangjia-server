BEGIN;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM hg_youban_two_way_bot_topic item LEFT JOIN hg_youban_publish_bot bot ON bot.id=item.bot_id WHERE bot.id IS NULL)
    OR EXISTS (SELECT 1 FROM hg_youban_two_way_bot_message item LEFT JOIN hg_youban_publish_bot bot ON bot.id=item.bot_id WHERE bot.id IS NULL)
    OR EXISTS (SELECT 1 FROM hg_youban_bot_relay_config item LEFT JOIN hg_youban_publish_bot bot ON bot.id=item.bot_id WHERE item.source_bot_id>0 AND bot.id IS NULL)
  THEN
    RAISE EXCEPTION '仍存在引用旧双向 Bot ID 的记录，禁止删除旧表';
  END IF;
END $$;

DROP TABLE hg_youban_two_way_bot_bot;

COMMIT;
