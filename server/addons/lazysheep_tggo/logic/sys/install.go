// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/internal/consts"
	"hotgo/internal/library/dbinit"
)

func (s *sLazySheepTGGo) ensureTables(ctx context.Context) error {
	if err := s.ensureAddonsConfigValue(ctx); err != nil {
		return err
	}
	if err := s.ensureUserBotKey(ctx); err != nil {
		return err
	}
	if err := s.ensureUserExpireField(ctx); err != nil {
		return err
	}
	if err := s.ensureBindingPluginSettings(ctx); err != nil {
		return err
	}
	if err := s.ensureBotRoleField(ctx); err != nil {
		return err
	}
	if err := s.ensurePointsLogTable(ctx); err != nil {
		return err
	}
	if err := s.ensureSignLogTable(ctx); err != nil {
		return err
	}
	if err := s.ensureInviteLogTable(ctx); err != nil {
		return err
	}
	if err := s.ensureNoteTables(ctx); err != nil {
		return err
	}
	if err := s.ensureNoteAssetPHashField(ctx); err != nil {
		return err
	}
	if err := s.ensureWebhookLogTable(ctx); err != nil {
		return err
	}
	if err := s.ensureChatMapTable(ctx); err != nil {
		return err
	}
	if err := s.ensurePushQueueTable(ctx); err != nil {
		return err
	}
	if err := s.ensurePushLogTable(ctx); err != nil {
		return err
	}
	if err := s.ensurePushMessageTable(ctx); err != nil {
		return err
	}
	if err := s.ensurePushDedupTable(ctx); err != nil {
		return err
	}
	if err := s.ensureAdminMenus(ctx); err != nil {
		return err
	}
	ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_bot")
	if err != nil {
		return gerror.Wrap(err, "检查懒羊羊TGGo数据表失败")
	}
	if ok {
		return nil
	}
	sqlPath, err := lazySheepSQLPath(ctx)
	if err != nil {
		return err
	}
	if err = dbinit.ImportFile(ctx, sqlPath); err != nil {
		return gerror.Wrap(err, "初始化懒羊羊TGGo数据表失败")
	}
	return s.ensureNoteAssetPHashField(ctx)
}

func (s *sLazySheepTGGo) ensureBotRoleField(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_bot"); err != nil || !ok {
		return err
	}
	hasField, err := tableHasField(ctx, "hg_addon_lazysheep_tggo_bot", "role")
	if err != nil {
		return gerror.Wrap(err, "检查机器人角色字段失败")
	}
	if hasField {
		return nil
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err = g.DB().Exec(ctx, "ALTER TABLE hg_addon_lazysheep_tggo_bot ADD COLUMN IF NOT EXISTS role varchar(32) DEFAULT 'user'")
	case consts.DBMysql, "":
		_, err = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_bot` ADD COLUMN `role` varchar(32) NOT NULL DEFAULT 'user' COMMENT '机器人角色' AFTER `bot_key`")
	default:
		return nil
	}
	if err != nil {
		if isDuplicateColumnError(err) {
			return nil
		}
		return gerror.Wrap(err, "更新机器人角色字段失败")
	}
	return nil
}

func (s *sLazySheepTGGo) ensureNoteTables(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_note"); err != nil || !ok {
		return err
	}
	if err := s.ensureNoteItemLongTextFields(ctx); err != nil {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		if _, err := g.DB().Exec(ctx, "DROP INDEX IF EXISTS hg_addon_lazysheep_tggo_note_content_id"); err != nil {
			return gerror.Wrap(err, "删除旧笔记索引失败")
		}
		if _, err := g.DB().Exec(ctx, "DROP INDEX IF EXISTS hg_addon_lazysheep_tggo_note_bot_content_id"); err != nil {
			return gerror.Wrap(err, "删除旧笔记机器人内容索引失败")
		}
		if _, err := g.DB().Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_note_binding_content_id ON hg_addon_lazysheep_tggo_note (binding_id, content_id)"); err != nil {
			return gerror.Wrap(err, "创建笔记内容索引失败")
		}
		if _, err := g.DB().Exec(ctx, "DROP INDEX IF EXISTS hg_addon_lazysheep_tggo_note_bot_code"); err != nil {
			return gerror.Wrap(err, "删除旧笔记编号索引失败")
		}
		if _, err := g.DB().Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_note_binding_code ON hg_addon_lazysheep_tggo_note (binding_id, code)"); err != nil {
			return gerror.Wrap(err, "创建笔记编号索引失败")
		}
	case consts.DBMysql, "":
		if ok, err := mysqlHasIndex(ctx, "hg_addon_lazysheep_tggo_note", "content_id"); err != nil {
			return gerror.Wrap(err, "检查旧笔记索引失败")
		} else if ok {
			if _, err = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_note` DROP INDEX `content_id`"); err != nil {
				return gerror.Wrap(err, "删除旧笔记索引失败")
			}
		}
		if ok, err := mysqlHasIndex(ctx, "hg_addon_lazysheep_tggo_note", "bot_content_id"); err != nil {
			return gerror.Wrap(err, "检查旧笔记机器人内容索引失败")
		} else if ok {
			if _, err = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_note` DROP INDEX `bot_content_id`"); err != nil {
				return gerror.Wrap(err, "删除旧笔记机器人内容索引失败")
			}
		}
		if ok, err := mysqlHasIndex(ctx, "hg_addon_lazysheep_tggo_note", "bot_code"); err != nil {
			return gerror.Wrap(err, "检查旧笔记机器人编号索引失败")
		} else if ok {
			if _, err = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_note` DROP INDEX `bot_code`"); err != nil {
				return gerror.Wrap(err, "删除旧笔记机器人编号索引失败")
			}
		}
		if ok, err := mysqlHasIndex(ctx, "hg_addon_lazysheep_tggo_note", "binding_content_id"); err != nil {
			return gerror.Wrap(err, "检查笔记内容索引失败")
		} else if !ok {
			if _, err = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_note` ADD UNIQUE KEY `binding_content_id` (`binding_id`,`content_id`)"); err != nil {
				return gerror.Wrap(err, "创建笔记内容索引失败")
			}
		}
		if ok, err := mysqlHasIndex(ctx, "hg_addon_lazysheep_tggo_note", "binding_code"); err != nil {
			return gerror.Wrap(err, "检查笔记编号索引失败")
		} else if !ok {
			if _, err = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_note` ADD UNIQUE KEY `binding_code` (`binding_id`,`code`)"); err != nil {
				return gerror.Wrap(err, "创建笔记编号索引失败")
			}
		}
	default:
		return nil
	}
	sqlPath, err := lazySheepSQLPath(ctx)
	if err != nil {
		return err
	}
	if err = dbinit.ImportFile(ctx, sqlPath); err != nil {
		return gerror.Wrap(err, "初始化笔记资源表失败")
	}
	return nil
}

func (s *sLazySheepTGGo) ensureNoteItemLongTextFields(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_note_item"); err != nil || !ok {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		if ok, err := pgsqlColumnIsText(ctx, "hg_addon_lazysheep_tggo_note_item", "title"); err != nil {
			return gerror.Wrap(err, "检查笔记项标题字段失败")
		} else if !ok {
			if _, err := g.DB().Exec(ctx, "ALTER TABLE hg_addon_lazysheep_tggo_note_item ALTER COLUMN title TYPE text"); err != nil {
				return gerror.Wrap(err, "更新笔记项标题字段失败")
			}
		}
		if ok, err := pgsqlColumnIsText(ctx, "hg_addon_lazysheep_tggo_note_item", "sub_title"); err != nil {
			return gerror.Wrap(err, "检查笔记项副标题字段失败")
		} else if !ok {
			if _, err := g.DB().Exec(ctx, "ALTER TABLE hg_addon_lazysheep_tggo_note_item ALTER COLUMN sub_title TYPE text"); err != nil {
				return gerror.Wrap(err, "更新笔记项副标题字段失败")
			}
		}
		if ok, err := pgsqlColumnIsText(ctx, "hg_addon_lazysheep_tggo_note_item", "content"); err != nil {
			return gerror.Wrap(err, "检查笔记项内容字段失败")
		} else if !ok {
			if _, err := g.DB().Exec(ctx, "ALTER TABLE hg_addon_lazysheep_tggo_note_item ALTER COLUMN content TYPE text"); err != nil {
				return gerror.Wrap(err, "更新笔记项内容字段失败")
			}
		}
	case consts.DBMysql, "":
		if ok, err := mysqlColumnIsOneOf(ctx, "hg_addon_lazysheep_tggo_note_item", "title", "text", "mediumtext", "longtext"); err != nil {
			return gerror.Wrap(err, "检查笔记项标题字段失败")
		} else if !ok {
			if _, err := g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_note_item` MODIFY COLUMN `title` TEXT COMMENT '标题'"); err != nil {
				return gerror.Wrap(err, "更新笔记项标题字段失败")
			}
		}
		if ok, err := mysqlColumnIsOneOf(ctx, "hg_addon_lazysheep_tggo_note_item", "sub_title", "text", "mediumtext", "longtext"); err != nil {
			return gerror.Wrap(err, "检查笔记项副标题字段失败")
		} else if !ok {
			if _, err := g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_note_item` MODIFY COLUMN `sub_title` TEXT COMMENT '副标题'"); err != nil {
				return gerror.Wrap(err, "更新笔记项副标题字段失败")
			}
		}
		if ok, err := mysqlColumnIsOneOf(ctx, "hg_addon_lazysheep_tggo_note_item", "content", "longtext"); err != nil {
			return gerror.Wrap(err, "检查笔记项内容字段失败")
		} else if !ok {
			if _, err := g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_note_item` MODIFY COLUMN `content` LONGTEXT COMMENT '内容'"); err != nil {
				return gerror.Wrap(err, "更新笔记项内容字段失败")
			}
		}
	default:
		return nil
	}
	return nil
}

func (s *sLazySheepTGGo) ensureWebhookLogTable(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_webhook_log"); err != nil || ok {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err := g.DB().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_webhook_log (
				id BIGSERIAL PRIMARY KEY,
				bot_key VARCHAR(64) NOT NULL DEFAULT '',
				update_id BIGINT NOT NULL DEFAULT 0,
				update_type VARCHAR(32) NOT NULL DEFAULT '',
				chat_id BIGINT NOT NULL DEFAULT 0,
				user_id BIGINT NOT NULL DEFAULT 0,
				username VARCHAR(128) NOT NULL DEFAULT '',
				message_id BIGINT NOT NULL DEFAULT 0,
				summary VARCHAR(512) NOT NULL DEFAULT '',
				payload TEXT,
				created_at TIMESTAMP NULL,
				updated_at TIMESTAMP NULL
			)
		`)
		return err
	case consts.DBMysql, "":
		_, err := g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS `hg_addon_lazysheep_tggo_webhook_log` ("+
			"`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,"+
			"`bot_key` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '机器人标识',"+
			"`update_id` BIGINT NOT NULL DEFAULT 0 COMMENT 'Telegram update id',"+
			"`update_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '更新类型',"+
			"`chat_id` BIGINT NOT NULL DEFAULT 0 COMMENT '聊天ID',"+
			"`user_id` BIGINT NOT NULL DEFAULT 0 COMMENT '用户ID',"+
			"`username` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '用户名',"+
			"`message_id` BIGINT NOT NULL DEFAULT 0 COMMENT '消息ID',"+
			"`summary` VARCHAR(512) NOT NULL DEFAULT '' COMMENT '摘要',"+
			"`payload` LONGTEXT COMMENT '原始内容',"+
			"`created_at` DATETIME DEFAULT NULL,`updated_at` DATETIME DEFAULT NULL,"+
			"PRIMARY KEY (`id`),"+
			"KEY `bot_update_id` (`bot_key`,`update_id`),"+
			"KEY `bot_chat_id` (`bot_key`,`chat_id`),"+
			"KEY `created_at` (`created_at`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='TG webhook原始日志'")
		return err
	default:
		return nil
	}
}

func (s *sLazySheepTGGo) ensureNoteAssetPHashField(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_note_asset"); err != nil || !ok {
		return err
	}
	hasField, err := tableHasField(ctx, "hg_addon_lazysheep_tggo_note_asset", "media_phash")
	if err != nil {
		return gerror.Wrap(err, "检查笔记资源感知哈希字段失败")
	}
	if hasField {
		return nil
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err = g.DB().Exec(ctx, "ALTER TABLE hg_addon_lazysheep_tggo_note_asset ADD COLUMN IF NOT EXISTS media_phash varchar(32) DEFAULT ''")
		if err == nil {
			_, _ = g.DB().Exec(ctx, "CREATE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_note_asset_media_phash ON hg_addon_lazysheep_tggo_note_asset (media_phash)")
		}
	case consts.DBMysql, "":
		_, err = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_note_asset` ADD COLUMN `media_phash` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '图片感知哈希' AFTER `source_url`")
		if err == nil {
			_, _ = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_note_asset` ADD KEY `media_phash` (`media_phash`)")
		}
	default:
		return nil
	}
	if err != nil {
		if isDuplicateColumnError(err) {
			return nil
		}
		return gerror.Wrap(err, "更新笔记资源感知哈希字段失败")
	}
	return nil
}

func (s *sLazySheepTGGo) ensureChatMapTable(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_chat_map"); err != nil || ok {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err := g.DB().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_chat_map (
				id BIGSERIAL PRIMARY KEY,
				bot_key VARCHAR(64) NOT NULL DEFAULT '',
				chat_id BIGINT NOT NULL DEFAULT 0,
				chat_type VARCHAR(32) NOT NULL DEFAULT '',
				title VARCHAR(255) NOT NULL DEFAULT '',
				username VARCHAR(128) NOT NULL DEFAULT '',
				label VARCHAR(255) NOT NULL DEFAULT '',
				created_at TIMESTAMP NULL,
				updated_at TIMESTAMP NULL,
				UNIQUE (bot_key, chat_id)
			)
		`)
		return err
	case consts.DBMysql, "":
		_, err := g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS `hg_addon_lazysheep_tggo_chat_map` ("+
			"`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,"+
			"`bot_key` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '机器人标识',"+
			"`chat_id` BIGINT NOT NULL DEFAULT 0 COMMENT '聊天ID',"+
			"`chat_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '聊天类型',"+
			"`title` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '聊天标题',"+
			"`username` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '聊天用户名',"+
			"`label` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '显示名称',"+
			"`created_at` DATETIME DEFAULT NULL,`updated_at` DATETIME DEFAULT NULL,"+
			"PRIMARY KEY (`id`),"+
			"UNIQUE KEY `bot_chat_id` (`bot_key`,`chat_id`),"+
			"KEY `updated_at` (`updated_at`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='TG频道显示映射'")
		return err
	default:
		return nil
	}
}

func (s *sLazySheepTGGo) ensurePushQueueTable(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_push_queue"); err != nil || ok {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err := g.DB().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_push_queue (
				id BIGSERIAL PRIMARY KEY,
				bot_key VARCHAR(64) NOT NULL DEFAULT '',
				binding_key VARCHAR(255) NOT NULL DEFAULT '',
				source_url VARCHAR(500) NOT NULL DEFAULT '',
				note_id BIGINT NOT NULL DEFAULT 0,
				content_id BIGINT NOT NULL DEFAULT 0,
				chat_id BIGINT NOT NULL DEFAULT 0,
				status INT NOT NULL DEFAULT 1,
				attempts INT NOT NULL DEFAULT 0,
				max_attempts INT NOT NULL DEFAULT 5,
				last_error TEXT,
				next_retry_at TIMESTAMP NULL,
				started_at TIMESTAMP NULL,
				finished_at TIMESTAMP NULL,
				created_at TIMESTAMP NULL,
				updated_at TIMESTAMP NULL
			)
		`)
		if err != nil {
			return err
		}
		_, _ = g.DB().Exec(ctx, "CREATE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_push_queue_status_next ON hg_addon_lazysheep_tggo_push_queue (status, next_retry_at)")
		_, _ = g.DB().Exec(ctx, "CREATE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_push_queue_binding ON hg_addon_lazysheep_tggo_push_queue (bot_key, binding_key)")
		return nil
	case consts.DBMysql, "":
		_, err := g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS `hg_addon_lazysheep_tggo_push_queue` ("+
			"`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,"+
			"`bot_key` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '机器人标识',"+
			"`binding_key` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '绑定标识',"+
			"`source_url` VARCHAR(500) NOT NULL DEFAULT '' COMMENT 'BangChat链接',"+
			"`note_id` BIGINT NOT NULL DEFAULT 0 COMMENT '笔记ID',"+
			"`content_id` BIGINT NOT NULL DEFAULT 0 COMMENT '内容ID',"+
			"`chat_id` BIGINT NOT NULL DEFAULT 0 COMMENT '目标会话ID',"+
			"`status` TINYINT NOT NULL DEFAULT 1 COMMENT '状态:1待推送 2执行中 3成功 4重试中 5失败 6待确认',"+
			"`attempts` INT NOT NULL DEFAULT 0 COMMENT '已尝试次数',"+
			"`max_attempts` INT NOT NULL DEFAULT 5 COMMENT '最大尝试次数',"+
			"`last_error` TEXT COMMENT '最近错误',"+
			"`next_retry_at` DATETIME DEFAULT NULL COMMENT '下次重试时间',"+
			"`started_at` DATETIME DEFAULT NULL COMMENT '开始时间',"+
			"`finished_at` DATETIME DEFAULT NULL COMMENT '完成时间',"+
			"`created_at` DATETIME DEFAULT NULL,`updated_at` DATETIME DEFAULT NULL,"+
			"PRIMARY KEY (`id`),"+
			"KEY `status_next` (`status`,`next_retry_at`),"+
			"KEY `bot_binding` (`bot_key`,`binding_key`),"+
			"KEY `note_id` (`note_id`),"+
			"KEY `created_at` (`created_at`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='TG采集推送任务'")
		return err
	default:
		return nil
	}
}

func (s *sLazySheepTGGo) ensurePushLogTable(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_push_log"); err != nil || ok {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err := g.DB().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_push_log (
				id BIGSERIAL PRIMARY KEY,
				task_id BIGINT NOT NULL DEFAULT 0,
				bot_key VARCHAR(64) NOT NULL DEFAULT '',
				binding_key VARCHAR(255) NOT NULL DEFAULT '',
				note_id BIGINT NOT NULL DEFAULT 0,
				content_id BIGINT NOT NULL DEFAULT 0,
				chat_id BIGINT NOT NULL DEFAULT 0,
				status INT NOT NULL DEFAULT 0,
				attempt INT NOT NULL DEFAULT 0,
				message_id BIGINT NOT NULL DEFAULT 0,
				elapsed_ms BIGINT NOT NULL DEFAULT 0,
				error TEXT,
				created_at TIMESTAMP NULL
			)
		`)
		if err != nil {
			return err
		}
		_, _ = g.DB().Exec(ctx, "CREATE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_push_log_chat_time ON hg_addon_lazysheep_tggo_push_log (bot_key, chat_id, created_at)")
		return nil
	case consts.DBMysql, "":
		_, err := g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS `hg_addon_lazysheep_tggo_push_log` ("+
			"`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,"+
			"`task_id` BIGINT NOT NULL DEFAULT 0 COMMENT '任务ID',"+
			"`bot_key` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '机器人标识',"+
			"`binding_key` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '绑定标识',"+
			"`note_id` BIGINT NOT NULL DEFAULT 0 COMMENT '笔记ID',"+
			"`content_id` BIGINT NOT NULL DEFAULT 0 COMMENT '内容ID',"+
			"`chat_id` BIGINT NOT NULL DEFAULT 0 COMMENT '频道ID',"+
			"`status` TINYINT NOT NULL DEFAULT 0 COMMENT '状态:1成功 2失败 3跳过',"+
			"`attempt` INT NOT NULL DEFAULT 0 COMMENT '尝试次数',"+
			"`message_id` BIGINT NOT NULL DEFAULT 0 COMMENT 'TG消息ID',"+
			"`elapsed_ms` BIGINT NOT NULL DEFAULT 0 COMMENT '耗时毫秒',"+
			"`error` TEXT COMMENT '错误',"+
			"`created_at` DATETIME DEFAULT NULL,"+
			"PRIMARY KEY (`id`),"+
			"KEY `chat_time` (`bot_key`,`chat_id`,`created_at`),"+
			"KEY `task_id` (`task_id`),"+
			"KEY `note_id` (`note_id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='TG采集推送日志'")
		return err
	default:
		return nil
	}
}

func (s *sLazySheepTGGo) ensurePushMessageTable(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_push_message"); err != nil || ok {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err := g.DB().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_push_message (
				id BIGSERIAL PRIMARY KEY,
				task_id BIGINT NOT NULL DEFAULT 0,
				bot_key VARCHAR(64) NOT NULL DEFAULT '',
				binding_key VARCHAR(255) NOT NULL DEFAULT '',
				note_id BIGINT NOT NULL DEFAULT 0,
				content_id BIGINT NOT NULL DEFAULT 0,
				chat_id BIGINT NOT NULL DEFAULT 0,
				message_id BIGINT NOT NULL DEFAULT 0,
				media_group_id VARCHAR(128) NOT NULL DEFAULT '',
				status INT NOT NULL DEFAULT 1,
				deleted_at TIMESTAMP NULL,
				created_at TIMESTAMP NULL,
				updated_at TIMESTAMP NULL
			)
		`)
		if err != nil {
			return err
		}
		_, _ = g.DB().Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_push_message_scope_msg ON hg_addon_lazysheep_tggo_push_message (bot_key, chat_id, message_id)")
		_, _ = g.DB().Exec(ctx, "CREATE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_push_message_binding ON hg_addon_lazysheep_tggo_push_message (bot_key, binding_key, status)")
		return nil
	case consts.DBMysql, "":
		_, err := g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS `hg_addon_lazysheep_tggo_push_message` ("+
			"`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,"+
			"`task_id` BIGINT NOT NULL DEFAULT 0 COMMENT '任务ID',"+
			"`bot_key` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '机器人标识',"+
			"`binding_key` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '绑定标识',"+
			"`note_id` BIGINT NOT NULL DEFAULT 0 COMMENT '笔记ID',"+
			"`content_id` BIGINT NOT NULL DEFAULT 0 COMMENT '内容ID',"+
			"`chat_id` BIGINT NOT NULL DEFAULT 0 COMMENT '频道ID',"+
			"`message_id` BIGINT NOT NULL DEFAULT 0 COMMENT 'TG消息ID',"+
			"`media_group_id` VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'TG媒体组ID',"+
			"`status` TINYINT NOT NULL DEFAULT 1 COMMENT '状态:1已发送 2已删除 3删除失败',"+
			"`deleted_at` DATETIME DEFAULT NULL COMMENT '删除时间',"+
			"`created_at` DATETIME DEFAULT NULL,`updated_at` DATETIME DEFAULT NULL,"+
			"PRIMARY KEY (`id`),"+
			"UNIQUE KEY `scope_msg` (`bot_key`,`chat_id`,`message_id`),"+
			"KEY `binding_status` (`bot_key`,`binding_key`,`status`),"+
			"KEY `note_id` (`note_id`),"+
			"KEY `chat_id` (`chat_id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='TG采集已发送消息'")
		return err
	default:
		return nil
	}
}

func (s *sLazySheepTGGo) ensurePushDedupTable(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_push_dedup"); err != nil || ok {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err := g.DB().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_push_dedup (
				id BIGSERIAL PRIMARY KEY,
				bot_key VARCHAR(64) NOT NULL DEFAULT '',
				binding_key VARCHAR(255) NOT NULL DEFAULT '',
				chat_id BIGINT NOT NULL DEFAULT 0,
				note_id BIGINT NOT NULL DEFAULT 0,
				content_id BIGINT NOT NULL DEFAULT 0,
				fingerprint VARCHAR(128) NOT NULL DEFAULT '',
				task_id BIGINT NOT NULL DEFAULT 0,
				status INT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NULL,
				updated_at TIMESTAMP NULL
			)
		`)
		if err != nil {
			return err
		}
		_, _ = g.DB().Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_push_dedup_scope_fp ON hg_addon_lazysheep_tggo_push_dedup (bot_key, chat_id, fingerprint)")
		return nil
	case consts.DBMysql, "":
		_, err := g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS `hg_addon_lazysheep_tggo_push_dedup` ("+
			"`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,"+
			"`bot_key` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '机器人标识',"+
			"`binding_key` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '绑定标识',"+
			"`chat_id` BIGINT NOT NULL DEFAULT 0 COMMENT '频道ID',"+
			"`note_id` BIGINT NOT NULL DEFAULT 0 COMMENT '笔记ID',"+
			"`content_id` BIGINT NOT NULL DEFAULT 0 COMMENT '内容ID',"+
			"`fingerprint` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '去重指纹',"+
			"`task_id` BIGINT NOT NULL DEFAULT 0 COMMENT '任务ID',"+
			"`status` TINYINT NOT NULL DEFAULT 1 COMMENT '状态:1已入队 2已推送',"+
			"`created_at` DATETIME DEFAULT NULL,`updated_at` DATETIME DEFAULT NULL,"+
			"PRIMARY KEY (`id`),"+
			"UNIQUE KEY `scope_fp` (`bot_key`,`chat_id`,`fingerprint`),"+
			"KEY `chat_id` (`chat_id`),"+
			"KEY `note_id` (`note_id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='TG采集频道推送去重'")
		return err
	default:
		return nil
	}
}

func (s *sLazySheepTGGo) ensureUserBotKey(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_user"); err != nil || !ok {
		return err
	}
	hasField, err := tableHasField(ctx, "hg_addon_lazysheep_tggo_user", "bot_key")
	if err != nil {
		return gerror.Wrap(err, "检查TG用户机器人字段失败")
	}
	if hasField {
		return s.ensureUserUniqueIndex(ctx)
	}
	var sql string
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		sql = "ALTER TABLE hg_addon_lazysheep_tggo_user ADD COLUMN IF NOT EXISTS bot_key varchar(64) DEFAULT ''"
	case consts.DBMysql, "":
		sql = "ALTER TABLE `hg_addon_lazysheep_tggo_user` ADD COLUMN `bot_key` varchar(64) DEFAULT '' COMMENT '机器人标识' AFTER `telegram_id`"
	default:
		return nil
	}
	if _, err := g.DB().Exec(ctx, sql); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return s.ensureUserUniqueIndex(ctx)
		}
		return gerror.Wrap(err, "更新TG用户机器人字段失败")
	}
	return s.ensureUserUniqueIndex(ctx)
}

func (s *sLazySheepTGGo) ensureUserExpireField(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_user"); err != nil || !ok {
		return err
	}
	hasField, err := tableHasField(ctx, "hg_addon_lazysheep_tggo_user", "member_expire_at")
	if err != nil {
		return gerror.Wrap(err, "检查TG用户到期字段失败")
	}
	if hasField {
		return nil
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err = g.DB().Exec(ctx, "ALTER TABLE hg_addon_lazysheep_tggo_user ADD COLUMN IF NOT EXISTS member_expire_at timestamp NULL")
	case consts.DBMysql, "":
		_, err = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_user` ADD COLUMN `member_expire_at` DATETIME DEFAULT NULL COMMENT '会员到期时间' AFTER `points`")
	default:
		return nil
	}
	if err != nil {
		if isDuplicateColumnError(err) {
			return nil
		}
		return gerror.Wrap(err, "更新TG用户到期字段失败")
	}
	return nil
}

func (s *sLazySheepTGGo) ensureBindingPluginSettings(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_binding"); err != nil || !ok {
		return err
	}
	hasField, err := tableHasField(ctx, "hg_addon_lazysheep_tggo_binding", "plugin_settings")
	if err != nil {
		return gerror.Wrap(err, "检查绑定插件状态字段失败")
	}
	if hasField {
		return nil
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err = g.DB().Exec(ctx, "ALTER TABLE hg_addon_lazysheep_tggo_binding ADD COLUMN IF NOT EXISTS plugin_settings text DEFAULT ''")
	case consts.DBMysql, "":
		_, err = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_binding` ADD COLUMN `plugin_settings` LONGTEXT COMMENT '插件状态' AFTER `location_enabled`")
	default:
		return nil
	}
	if err != nil {
		if isDuplicateColumnError(err) {
			return nil
		}
		return gerror.Wrap(err, "更新绑定插件状态字段失败")
	}
	return nil
}

func (s *sLazySheepTGGo) ensureAdminMenus(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_admin_menu"); err != nil || !ok {
		return err
	}
	now := gtime.Now()
	addonsId, err := adminMenuIdByName(ctx, "addons")
	if err != nil {
		return err
	}
	if addonsId <= 0 {
		return gerror.New("插件应用菜单不存在，请先初始化系统菜单")
	}
	root := g.Map{
		"pid": addonsId, "level": 2, "tree": fmt.Sprintf("tr_%d ", addonsId), "title": "懒羊羊TGGo", "name": "addons_lazysheep_tggo", "path": "lazysheep_tggo", "icon": "icon-park-outline:robot-one", "type": 1,
		"redirect": "/addons/lazysheep_tggo/bot", "permissions": "", "permission_name": "", "component": "ParentLayout", "always_show": 1, "active_menu": "",
		"is_root": 0, "is_frame": 0, "frame_src": "", "keep_alive": 0, "hidden": 0, "affix": 0, "sort": 36, "remark": "懒羊羊TGGo插件后台", "status": 1,
		"updated_at": now, "created_at": now,
	}
	rootId, err := upsertAdminMenu(ctx, root)
	if err != nil {
		return err
	}
	childTree := fmt.Sprintf("tr_%d tr_%d ", addonsId, rootId)
	botPermissions := strings.Join([]string{
		"/lazysheep_tggo/config/get",
		"/lazysheep_tggo/config/update",
		"/lazysheep_tggo/config/upsertBot",
		"/lazysheep_tggo/config/bots",
		"/lazysheep_tggo/config/inspectBot",
		"/lazysheep_tggo/config/deleteBot",
		"/lazysheep_tggo/config/startBot",
		"/lazysheep_tggo/config/botUsers",
		"/lazysheep_tggo/config/updateBotUser",
		"/lazysheep_tggo/config/testTelegramProxy",
	}, ",")
	monitorPermissions := strings.Join([]string{
		"/lazysheep_tggo/config/pullMonitor",
		"/lazysheep_tggo/config/pullMonitorOverview",
		"/lazysheep_tggo/config/pullMonitorBindings",
		"/lazysheep_tggo/config/pullMonitorRecent",
		"/lazysheep_tggo/config/pushQueueMonitor",
		"/lazysheep_tggo/config/pushQueueControl",
		"/lazysheep_tggo/config/bindingAutoPullControl",
	}, ",")
	children := []g.Map{
		{"pid": rootId, "level": 3, "tree": childTree, "title": "机器人管理", "name": "addons_lazysheep_tggo_bot", "path": "bot", "icon": "", "type": 2, "redirect": "", "permissions": botPermissions, "permission_name": "", "component": "/addons/lazysheep_tggo/bot/index", "always_show": 0, "active_menu": "", "is_root": 0, "is_frame": 0, "frame_src": "", "keep_alive": 0, "hidden": 0, "affix": 0, "sort": 10, "remark": "懒羊羊机器人管理", "status": 1, "updated_at": now, "created_at": now},
		{"pid": rootId, "level": 3, "tree": childTree, "title": "插件配置", "name": "addons_lazysheep_tggo_config", "path": "config", "icon": "", "type": 2, "redirect": "", "permissions": "/lazysheep_tggo/config/get,/lazysheep_tggo/config/update", "permission_name": "", "component": "/addons/lazysheep_tggo/config/system", "always_show": 0, "active_menu": "", "is_root": 0, "is_frame": 0, "frame_src": "", "keep_alive": 0, "hidden": 0, "affix": 0, "sort": 20, "remark": "懒羊羊插件配置", "status": 1, "updated_at": now, "created_at": now},
		{"pid": rootId, "level": 3, "tree": childTree, "title": "全局配置", "name": "addons_lazysheep_tggo_global", "path": "global", "icon": "", "type": 2, "redirect": "", "permissions": "/lazysheep_tggo/config/get,/lazysheep_tggo/config/update", "permission_name": "", "component": "/addons/lazysheep_tggo/global/index", "always_show": 0, "active_menu": "", "is_root": 0, "is_frame": 0, "frame_src": "", "keep_alive": 0, "hidden": 0, "affix": 0, "sort": 30, "remark": "懒羊羊全局配置", "status": 1, "updated_at": now, "created_at": now},
		{"pid": rootId, "level": 3, "tree": childTree, "title": "频道列表", "name": "addons_lazysheep_tggo_channel_list", "path": "channel-list", "icon": "", "type": 2, "redirect": "", "permissions": "/lazysheep_tggo/config/channelList", "permission_name": "", "component": "/addons/lazysheep_tggo/config/channel-list", "always_show": 0, "active_menu": "", "is_root": 0, "is_frame": 0, "frame_src": "", "keep_alive": 0, "hidden": 0, "affix": 0, "sort": 35, "remark": "懒羊羊频道列表", "status": 1, "updated_at": now, "created_at": now},
		{"pid": rootId, "level": 3, "tree": childTree, "title": "拉取监控", "name": "addons_lazysheep_tggo_pull_monitor", "path": "pull-monitor", "icon": "", "type": 2, "redirect": "", "permissions": monitorPermissions, "permission_name": "", "component": "/addons/lazysheep_tggo/config/pull-monitor", "always_show": 0, "active_menu": "", "is_root": 0, "is_frame": 0, "frame_src": "", "keep_alive": 0, "hidden": 0, "affix": 0, "sort": 40, "remark": "懒羊羊拉取监控", "status": 1, "updated_at": now, "created_at": now},
	}
	menuNames := []string{"addons_lazysheep_tggo"}
	for _, menu := range children {
		if _, err := upsertAdminMenu(ctx, menu); err != nil {
			return err
		}
		menuNames = append(menuNames, fmt.Sprint(menu["name"]))
	}
	return grantAdminMenus(ctx, menuNames)
}

func (s *sLazySheepTGGo) ensurePointsLogTable(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_points_log"); err != nil || ok {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err := g.DB().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_points_log (
				id BIGSERIAL PRIMARY KEY,
				bot_key VARCHAR(64) NOT NULL DEFAULT '',
				telegram_id BIGINT NOT NULL DEFAULT 0,
				change_num NUMERIC(18,4) NOT NULL DEFAULT 0,
				before_num NUMERIC(18,4) NOT NULL DEFAULT 0,
				after_num NUMERIC(18,4) NOT NULL DEFAULT 0,
				action VARCHAR(64) NOT NULL DEFAULT '',
				remark VARCHAR(255) NOT NULL DEFAULT '',
				status INT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NULL,
				updated_at TIMESTAMP NULL
			)
		`)
		return err
	case consts.DBMysql, "":
		_, err := g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS `hg_addon_lazysheep_tggo_points_log` ("+
			"`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,"+
			"`bot_key` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '机器人标识',"+
			"`telegram_id` BIGINT NOT NULL DEFAULT 0 COMMENT 'Telegram 用户ID',"+
			"`change_num` DECIMAL(18,4) NOT NULL DEFAULT 0 COMMENT '变动值',"+
			"`before_num` DECIMAL(18,4) NOT NULL DEFAULT 0 COMMENT '变动前',"+
			"`after_num` DECIMAL(18,4) NOT NULL DEFAULT 0 COMMENT '变动后',"+
			"`action` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '变动动作',"+
			"`remark` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '备注',"+
			"`status` TINYINT NOT NULL DEFAULT 1 COMMENT '状态',"+
			"`created_at` DATETIME DEFAULT NULL,`updated_at` DATETIME DEFAULT NULL,"+
			"PRIMARY KEY (`id`),"+
			"KEY `bot_telegram_id` (`bot_key`,`telegram_id`),"+
			"KEY `bot_action` (`bot_key`,`action`),"+
			"KEY `created_at` (`created_at`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='TG积分流水'")
		return err
	default:
		return nil
	}
}

func (s *sLazySheepTGGo) ensureSignLogTable(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_sign_log"); err != nil || ok {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err := g.DB().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_sign_log (
				id BIGSERIAL PRIMARY KEY,
				bot_key VARCHAR(64) NOT NULL DEFAULT '',
				telegram_id BIGINT NOT NULL DEFAULT 0,
				sign_day VARCHAR(10) NOT NULL DEFAULT '',
				channel_total INT NOT NULL DEFAULT 0,
				verified_total INT NOT NULL DEFAULT 0,
				points_reward NUMERIC(18,4) NOT NULL DEFAULT 0,
				remark VARCHAR(255) NOT NULL DEFAULT '',
				status INT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NULL,
				updated_at TIMESTAMP NULL,
				UNIQUE (bot_key, telegram_id, sign_day)
			)
		`)
		return err
	case consts.DBMysql, "":
		_, err := g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS `hg_addon_lazysheep_tggo_sign_log` ("+
			"`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,"+
			"`bot_key` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '机器人标识',"+
			"`telegram_id` BIGINT NOT NULL DEFAULT 0 COMMENT 'Telegram 用户ID',"+
			"`sign_day` VARCHAR(10) NOT NULL DEFAULT '' COMMENT '签到日期',"+
			"`channel_total` INT NOT NULL DEFAULT 0 COMMENT '关注频道数',"+
			"`verified_total` INT NOT NULL DEFAULT 0 COMMENT '已验证频道数',"+
			"`points_reward` DECIMAL(18,4) NOT NULL DEFAULT 0 COMMENT '积分奖励',"+
			"`remark` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '备注',"+
			"`status` TINYINT NOT NULL DEFAULT 1 COMMENT '状态',"+
			"`created_at` DATETIME DEFAULT NULL,`updated_at` DATETIME DEFAULT NULL,"+
			"PRIMARY KEY (`id`),"+
			"UNIQUE KEY `bot_telegram_day` (`bot_key`,`telegram_id`,`sign_day`),"+
			"KEY `bot_day` (`bot_key`,`sign_day`),"+
			"KEY `created_at` (`created_at`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='TG签到记录'")
		return err
	default:
		return nil
	}
}

func (s *sLazySheepTGGo) ensureInviteLogTable(ctx context.Context) error {
	if ok, err := dbinit.HasTable(ctx, "hg_addon_lazysheep_tggo_invite_log"); err != nil || ok {
		return err
	}
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, err := g.DB().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_invite_log (
				id BIGSERIAL PRIMARY KEY,
				bot_key VARCHAR(64) NOT NULL DEFAULT '',
				inviter_telegram_id BIGINT NOT NULL DEFAULT 0,
				invitee_telegram_id BIGINT NOT NULL DEFAULT 0,
				payload VARCHAR(64) NOT NULL DEFAULT '',
				reward_points NUMERIC(18,4) NOT NULL DEFAULT 0,
				status INT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NULL,
				updated_at TIMESTAMP NULL,
				UNIQUE (bot_key, invitee_telegram_id)
			)
		`)
		return err
	case consts.DBMysql, "":
		_, err := g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS `hg_addon_lazysheep_tggo_invite_log` ("+
			"`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,"+
			"`bot_key` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '机器人标识',"+
			"`inviter_telegram_id` BIGINT NOT NULL DEFAULT 0 COMMENT '邀请人 Telegram 用户ID',"+
			"`invitee_telegram_id` BIGINT NOT NULL DEFAULT 0 COMMENT '被邀请 Telegram 用户ID',"+
			"`payload` VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'start 参数',"+
			"`reward_points` DECIMAL(18,4) NOT NULL DEFAULT 0 COMMENT '奖励积分',"+
			"`status` TINYINT NOT NULL DEFAULT 1 COMMENT '状态',"+
			"`created_at` DATETIME DEFAULT NULL,`updated_at` DATETIME DEFAULT NULL,"+
			"PRIMARY KEY (`id`),"+
			"UNIQUE KEY `bot_invitee` (`bot_key`,`invitee_telegram_id`),"+
			"KEY `bot_inviter` (`bot_key`,`inviter_telegram_id`),"+
			"KEY `created_at` (`created_at`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='TG邀请记录'")
		return err
	default:
		return nil
	}
}

func (s *sLazySheepTGGo) ensureUserUniqueIndex(ctx context.Context) error {
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		_, _ = g.DB().Exec(ctx, "DROP INDEX IF EXISTS hg_addon_lazysheep_tggo_user_telegram_id")
		_, err := g.DB().Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_user_bot_telegram_id ON hg_addon_lazysheep_tggo_user (bot_key, telegram_id)")
		return err
	case consts.DBMysql, "":
		if ok, err := mysqlHasIndex(ctx, "hg_addon_lazysheep_tggo_user", "bot_telegram_id"); err != nil {
			return gerror.Wrap(err, "检查TG用户联合索引失败")
		} else if ok {
			return nil
		}
		if ok, err := mysqlHasIndex(ctx, "hg_addon_lazysheep_tggo_user", "telegram_id"); err != nil {
			return gerror.Wrap(err, "检查TG用户旧索引失败")
		} else if ok {
			if _, err = g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_user` DROP INDEX `telegram_id`"); err != nil {
				return gerror.Wrap(err, "删除TG用户旧索引失败")
			}
		}
		_, err := g.DB().Exec(ctx, "ALTER TABLE `hg_addon_lazysheep_tggo_user` ADD UNIQUE KEY `bot_telegram_id` (`bot_key`,`telegram_id`)")
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
			return nil
		}
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate entry") {
			return nil
		}
		return err
	default:
		return nil
	}
}

func tableHasField(ctx context.Context, table, field string) (bool, error) {
	fields, err := g.DB().TableFields(ctx, table)
	if err != nil {
		return false, err
	}
	for name := range fields {
		if strings.EqualFold(name, field) {
			return true, nil
		}
	}
	return false, nil
}

func adminMenuIdByName(ctx context.Context, name string) (int64, error) {
	value, err := g.DB().Model("hg_admin_menu").Where("name", name).Fields("id").Value()
	if err != nil || value.IsNil() {
		return 0, err
	}
	return value.Int64(), nil
}

func upsertAdminMenu(ctx context.Context, data g.Map) (int64, error) {
	name := strings.TrimSpace(fmt.Sprint(data["name"]))
	if name == "" {
		return 0, nil
	}
	id, err := adminMenuIdByName(ctx, name)
	if err != nil {
		return 0, gerror.Wrap(err, "检查插件菜单失败")
	}
	if id > 0 {
		update := g.Map{}
		for k, v := range data {
			if k == "id" || k == "name" || k == "created_at" {
				continue
			}
			update[k] = v
		}
		if _, err = g.DB().Model("hg_admin_menu").Where("id", id).Data(update).Update(); err != nil {
			return 0, gerror.Wrap(err, "更新插件菜单失败")
		}
		return id, nil
	}
	delete(data, "id")
	id, err = g.DB().Model("hg_admin_menu").Data(data).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建插件菜单失败")
	}
	return id, nil
}

func grantAdminMenus(ctx context.Context, names []string) error {
	for _, name := range names {
		menuId, err := adminMenuIdByName(ctx, name)
		if err != nil {
			return gerror.Wrap(err, "查询插件菜单授权失败")
		}
		if menuId <= 0 {
			continue
		}
		for _, roleId := range []int{1, 2} {
			count, err := g.DB().Model("hg_admin_role_menu").Where("role_id", roleId).Where("menu_id", menuId).Count()
			if err != nil {
				return gerror.Wrap(err, "检查插件菜单授权失败")
			}
			if count > 0 {
				continue
			}
			if _, err = g.DB().Model("hg_admin_role_menu").Data(g.Map{"role_id": roleId, "menu_id": menuId}).Insert(); err != nil {
				return gerror.Wrap(err, "创建插件菜单授权失败")
			}
		}
	}
	return nil
}

func mysqlHasIndex(ctx context.Context, table, index string) (bool, error) {
	count, err := g.DB().GetCount(ctx, `
		SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?
	`, table, index)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func mysqlColumnIsOneOf(ctx context.Context, table, column string, types ...string) (bool, error) {
	if len(types) == 0 {
		return false, nil
	}
	value, err := g.DB().GetValue(ctx, `
		SELECT DATA_TYPE FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?
		LIMIT 1
	`, table, column)
	if err != nil || value.IsNil() {
		return false, err
	}
	current := strings.ToLower(strings.TrimSpace(value.String()))
	for _, item := range types {
		if current == strings.ToLower(strings.TrimSpace(item)) {
			return true, nil
		}
	}
	return false, nil
}

func pgsqlColumnIsText(ctx context.Context, table, column string) (bool, error) {
	value, err := g.DB().GetValue(ctx, `
		SELECT data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?
		LIMIT 1
	`, table, column)
	if err != nil || value.IsNil() {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(value.String()), "text"), nil
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "duplicate column") || strings.Contains(text, "1060")
}

func (s *sLazySheepTGGo) ensureAddonsConfigValue(ctx context.Context) error {
	var sql string
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		sql = "ALTER TABLE hg_sys_addons_config ALTER COLUMN value TYPE TEXT"
	case consts.DBMysql, "":
		sql = "ALTER TABLE `hg_sys_addons_config` MODIFY COLUMN `value` LONGTEXT COMMENT '参数键值'"
	default:
		return nil
	}
	if _, err := g.DB().Exec(ctx, sql); err != nil {
		return gerror.Wrap(err, "更新插件配置字段失败")
	}
	return nil
}

func lazySheepSQLPath(ctx context.Context) (string, error) {
	switch g.DB().GetConfig().Type {
	case consts.DBPgsql:
		return filepath.Join("storage", "data", "generate", "addons", "lazysheep_tggo_pgsql.sql"), nil
	case consts.DBMysql, "":
		return filepath.Join("storage", "data", "generate", "addons", "lazysheep_tggo_mysql.sql"), nil
	default:
		return "", gerror.Newf("懒羊羊TGGo暂不支持当前数据库类型自动初始化：%s", g.DB().GetConfig().Type)
	}
}
