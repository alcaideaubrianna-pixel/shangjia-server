SET @schema := DATABASE();

SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_sys_serve_log' AND column_name = 'line' AND is_nullable = 'NO' AND column_default IS NULL) > 0,
  'ALTER TABLE `hg_sys_serve_log` MODIFY COLUMN `line` varchar(255) NOT NULL DEFAULT '''' COMMENT ''调用行''',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
