-- Rerunnable MySQL 5.7+/8.0 migration; legacy rows are not rebound.
SET @cleanup_ddl = IF((SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='hg_youban_publish_tg_outbox' AND column_name='operation_no')=0,
 'ALTER TABLE `hg_youban_publish_tg_outbox` ADD COLUMN `operation_no` varchar(128) NOT NULL DEFAULT ''''', 'SELECT 1');
PREPARE cleanup_stmt FROM @cleanup_ddl;
EXECUTE cleanup_stmt;
DEALLOCATE PREPARE cleanup_stmt;
SET @cleanup_ddl = IF((SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='hg_youban_publish_tg_outbox' AND column_name='payload')=0,
 'ALTER TABLE `hg_youban_publish_tg_outbox` ADD COLUMN `payload` text NULL', 'SELECT 1');
PREPARE cleanup_stmt FROM @cleanup_ddl;
EXECUTE cleanup_stmt;
DEALLOCATE PREPARE cleanup_stmt;
-- MySQL TEXT has no portable empty default; NULL is legacy empty context.
