ALTER TABLE `hg_youban_publish_collect_rule`
  ADD COLUMN `full_match_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '全量匹配' AFTER `dedupe_days`;

ALTER TABLE `hg_youban_publish_collect_rule`
  ADD COLUMN `delete_text_json` text COMMENT '删除文本JSON' AFTER `replace_json`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD COLUMN `history_collect_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '账号历史采集开关' AFTER `collect_enabled`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD COLUMN `history_collect_mode` varchar(32) NOT NULL DEFAULT 'recent_days' COMMENT '账号历史采集模式' AFTER `history_collect_enabled`;

ALTER TABLE `hg_youban_publish_collect_source`
  ADD COLUMN `history_collect_days` int(11) NOT NULL DEFAULT '30' COMMENT '账号历史采集天数' AFTER `history_collect_mode`;
