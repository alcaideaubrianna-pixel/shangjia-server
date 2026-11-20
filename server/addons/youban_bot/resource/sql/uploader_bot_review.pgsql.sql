CREATE TABLE IF NOT EXISTS hg_youban_uploader_bot_review (
    id bigserial PRIMARY KEY,
    tenant_id bigint NOT NULL,
    bot_id bigint NOT NULL,
    owner_account_id bigint NOT NULL,
    telegram_user_id bigint NOT NULL,
    telegram_username varchar(64) NOT NULL DEFAULT '',
    status varchar(16) NOT NULL DEFAULT 'pending',
    account_id bigint NOT NULL DEFAULT 0,
    reviewed_by bigint NOT NULL DEFAULT 0,
    reviewed_at timestamp,
    created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, bot_id, telegram_user_id)
);
CREATE INDEX IF NOT EXISTS idx_uploader_bot_review_account ON hg_youban_uploader_bot_review (tenant_id, account_id);
ALTER TABLE hg_youban_uploader_bot_review ADD COLUMN IF NOT EXISTS telegram_display_name varchar(256) NOT NULL DEFAULT '';
