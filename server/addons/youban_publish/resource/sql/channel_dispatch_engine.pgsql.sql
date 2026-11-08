-- 频道推送引擎灰度开关。legacy 保持现网行为，unified 启用新调度路径。
ALTER TABLE hg_youban_publish_account_setting
    ADD COLUMN IF NOT EXISTS push_engine varchar(16) NOT NULL DEFAULT 'legacy';

COMMENT ON COLUMN hg_youban_publish_account_setting.push_engine IS
    '频道推送引擎：legacy 旧实现，unified 新实现';

CREATE INDEX IF NOT EXISTS idx_ybp_account_setting_push_engine
    ON hg_youban_publish_account_setting (tenant_id, account_id, push_engine)
    WHERE deleted_at IS NULL;
