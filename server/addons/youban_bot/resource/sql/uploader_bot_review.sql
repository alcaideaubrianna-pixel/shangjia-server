CREATE TABLE IF NOT EXISTS hg_youban_uploader_bot_review (
    id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
    tenant_id bigint NOT NULL,
    bot_id bigint NOT NULL,
    owner_account_id bigint NOT NULL,
    telegram_user_id bigint NOT NULL,
    telegram_username varchar(64) NOT NULL DEFAULT '',
    status varchar(16) NOT NULL DEFAULT 'pending',
    account_id bigint NOT NULL DEFAULT 0,
    reviewed_by bigint NOT NULL DEFAULT 0,
    reviewed_at datetime,
    created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_uploader_bot_review_user (tenant_id, bot_id, telegram_user_id),
    KEY idx_uploader_bot_review_account (tenant_id, account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE hg_youban_uploader_bot_review ADD COLUMN telegram_display_name varchar(256) NOT NULL DEFAULT '';
