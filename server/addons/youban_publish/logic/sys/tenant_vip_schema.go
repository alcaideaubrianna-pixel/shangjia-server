package sys

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

var tenantVipSchemaReady atomic.Bool
var tenantVipSchemaMu sync.Mutex

func ensureTenantVipTables(ctx context.Context) error {
	if tenantVipSchemaReady.Load() {
		return nil
	}
	tenantVipSchemaMu.Lock()
	defer tenantVipSchemaMu.Unlock()
	if tenantVipSchemaReady.Load() {
		return nil
	}
	var err error
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		err = ensureTenantVipPgsqlTables(ctx)
	} else {
		err = ensureTenantVipMysqlTables(ctx)
	}
	if err == nil {
		tenantVipSchemaReady.Store(true)
	}
	return err
}

func ensureTenantVipPgsqlTables(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_vip" (
			"id" BIGSERIAL PRIMARY KEY,
			"tenant_id" bigint NOT NULL DEFAULT 0,
			"level" integer NOT NULL DEFAULT 0,
			"status" integer NOT NULL DEFAULT 2,
			"opened_at" timestamp DEFAULT NULL,
			"expired_at" timestamp DEFAULT NULL,
			"remark" text NOT NULL DEFAULT '',
			"created_at" timestamp DEFAULT NULL,
			"updated_at" timestamp DEFAULT NULL,
			"deleted_at" timestamp DEFAULT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tenant_vip_tenant" ON "hg_youban_publish_tenant_vip" ("tenant_id") WHERE "deleted_at" IS NULL`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_vip_log" (
			"id" BIGSERIAL PRIMARY KEY,
			"tenant_id" bigint NOT NULL DEFAULT 0,
			"operator_id" bigint NOT NULL DEFAULT 0,
			"source" varchar(32) NOT NULL DEFAULT '',
			"action" varchar(32) NOT NULL DEFAULT '',
			"before_status" integer NOT NULL DEFAULT 0,
			"before_level" integer NOT NULL DEFAULT 0,
			"before_expired_at" timestamp DEFAULT NULL,
			"after_status" integer NOT NULL DEFAULT 0,
			"after_level" integer NOT NULL DEFAULT 0,
			"after_expired_at" timestamp DEFAULT NULL,
			"remark" text NOT NULL DEFAULT '',
			"created_at" timestamp DEFAULT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_tenant_vip_log_tenant" ON "hg_youban_publish_tenant_vip_log" ("tenant_id", "id")`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_vip_coupon" (
			"id" BIGSERIAL PRIMARY KEY,
			"code" varchar(64) NOT NULL DEFAULT '',
			"use_type" varchar(16) NOT NULL DEFAULT 'single',
			"amount" numeric(10,2) NOT NULL DEFAULT 0,
			"total_count" integer NOT NULL DEFAULT 1,
			"used_count" integer NOT NULL DEFAULT 0,
			"status" integer NOT NULL DEFAULT 1,
			"remark" text NOT NULL DEFAULT '',
			"expired_at" timestamp DEFAULT NULL,
			"created_at" timestamp DEFAULT NULL,
			"updated_at" timestamp DEFAULT NULL,
			"deleted_at" timestamp DEFAULT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tenant_vip_coupon_code" ON "hg_youban_publish_tenant_vip_coupon" ("code") WHERE "deleted_at" IS NULL`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_vip_event" (
			"id" BIGSERIAL PRIMARY KEY,
			"event_key" varchar(160) NOT NULL DEFAULT '',
			"event_type" varchar(48) NOT NULL DEFAULT '',
			"activity_code" varchar(64) NOT NULL DEFAULT '',
			"activity_generation" integer NOT NULL DEFAULT 1,
			"tenant_id" bigint NOT NULL DEFAULT 0,
			"account_id" bigint NOT NULL DEFAULT 0,
			"trigger_tenant_id" bigint NOT NULL DEFAULT 0,
			"trigger_account_id" bigint NOT NULL DEFAULT 0,
			"reference_type" varchar(32) NOT NULL DEFAULT '',
			"reference_id" varchar(64) NOT NULL DEFAULT '',
			"change_days" integer NOT NULL DEFAULT 0,
			"before_expired_at" timestamp DEFAULT NULL,
			"after_expired_at" timestamp DEFAULT NULL,
			"notify_status" varchar(16) NOT NULL DEFAULT 'pending',
			"notify_retry_count" integer NOT NULL DEFAULT 0,
			"notify_next_retry_at" timestamp DEFAULT NULL,
			"notified_at" timestamp DEFAULT NULL,
			"error_message" text NOT NULL DEFAULT '',
			"remark" text NOT NULL DEFAULT '',
			"created_at" timestamp DEFAULT NULL,
			"updated_at" timestamp DEFAULT NULL
		)`,
		`ALTER TABLE "hg_youban_publish_tenant_vip_event" ADD COLUMN IF NOT EXISTS "activity_code" varchar(64) NOT NULL DEFAULT ''`,
		`ALTER TABLE "hg_youban_publish_tenant_vip_event" ADD COLUMN IF NOT EXISTS "activity_generation" integer NOT NULL DEFAULT 1`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_activity_generation" (
			"id" BIGSERIAL PRIMARY KEY,
			"activity_code" varchar(64) NOT NULL DEFAULT '',
			"tenant_id" bigint NOT NULL DEFAULT 0,
			"generation" integer NOT NULL DEFAULT 1,
			"reset_reason" text NOT NULL DEFAULT '',
			"updated_by" bigint NOT NULL DEFAULT 0,
			"created_at" timestamp DEFAULT NULL,
			"updated_at" timestamp DEFAULT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_activity_generation" ON "hg_youban_publish_activity_generation" ("activity_code", "tenant_id")`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_vip_event_key" ON "hg_youban_publish_tenant_vip_event" ("event_key")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_vip_event_activity" ON "hg_youban_publish_tenant_vip_event" ("activity_code", "activity_generation", "id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_vip_event_tenant" ON "hg_youban_publish_tenant_vip_event" ("tenant_id", "event_type", "id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_vip_event_notify" ON "hg_youban_publish_tenant_vip_event" ("notify_status", "notify_next_retry_at", "id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_vip_expired" ON "hg_youban_publish_tenant_vip" ("status", "expired_at", "id")`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "初始化租户会员表失败")
		}
	}
	return nil
}

func ensureTenantVipMysqlTables(ctx context.Context) error {
	statements := []string{
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT 0,`level` int NOT NULL DEFAULT 0,`status` int NOT NULL DEFAULT 2,`opened_at` datetime DEFAULT NULL,`expired_at` datetime DEFAULT NULL,`remark` text NOT NULL,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,`deleted_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_tenant_vip_tenant` (`tenant_id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip_log` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT 0,`operator_id` bigint NOT NULL DEFAULT 0,`source` varchar(32) NOT NULL DEFAULT '',`action` varchar(32) NOT NULL DEFAULT '',`before_status` int NOT NULL DEFAULT 0,`before_level` int NOT NULL DEFAULT 0,`before_expired_at` datetime DEFAULT NULL,`after_status` int NOT NULL DEFAULT 0,`after_level` int NOT NULL DEFAULT 0,`after_expired_at` datetime DEFAULT NULL,`remark` text NOT NULL,`created_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),KEY `idx_ybp_tenant_vip_log_tenant` (`tenant_id`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip_coupon` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`code` varchar(64) NOT NULL DEFAULT '',`use_type` varchar(16) NOT NULL DEFAULT 'single',`amount` decimal(10,2) NOT NULL DEFAULT 0,`total_count` int NOT NULL DEFAULT 1,`used_count` int NOT NULL DEFAULT 0,`status` int NOT NULL DEFAULT 1,`remark` text NOT NULL,`expired_at` datetime DEFAULT NULL,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,`deleted_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_tenant_vip_coupon_code` (`code`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip_event` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`event_key` varchar(160) NOT NULL DEFAULT '',`event_type` varchar(48) NOT NULL DEFAULT '',`activity_code` varchar(64) NOT NULL DEFAULT '',`activity_generation` int NOT NULL DEFAULT 1,`tenant_id` bigint NOT NULL DEFAULT 0,`account_id` bigint NOT NULL DEFAULT 0,`trigger_tenant_id` bigint NOT NULL DEFAULT 0,`trigger_account_id` bigint NOT NULL DEFAULT 0,`reference_type` varchar(32) NOT NULL DEFAULT '',`reference_id` varchar(64) NOT NULL DEFAULT '',`change_days` int NOT NULL DEFAULT 0,`before_expired_at` datetime DEFAULT NULL,`after_expired_at` datetime DEFAULT NULL,`notify_status` varchar(16) NOT NULL DEFAULT 'pending',`notify_retry_count` int NOT NULL DEFAULT 0,`notify_next_retry_at` datetime DEFAULT NULL,`notified_at` datetime DEFAULT NULL,`error_message` text NOT NULL,`remark` text NOT NULL,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_vip_event_key` (`event_key`),KEY `idx_ybp_vip_event_tenant` (`tenant_id`,`event_type`,`id`),KEY `idx_ybp_vip_event_notify` (`notify_status`,`notify_next_retry_at`,`id`),KEY `idx_ybp_vip_event_activity` (`activity_code`,`activity_generation`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"ALTER TABLE `hg_youban_publish_tenant_vip_event` ADD COLUMN `activity_code` varchar(64) NOT NULL DEFAULT '' AFTER `event_type`",
		"ALTER TABLE `hg_youban_publish_tenant_vip_event` ADD COLUMN `activity_generation` int NOT NULL DEFAULT 1 AFTER `activity_code`",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_activity_generation` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`activity_code` varchar(64) NOT NULL DEFAULT '',`tenant_id` bigint NOT NULL DEFAULT 0,`generation` int NOT NULL DEFAULT 1,`reset_reason` text NOT NULL,`updated_by` bigint NOT NULL DEFAULT 0,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_activity_generation` (`activity_code`,`tenant_id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE INDEX `idx_ybp_vip_expired` ON `hg_youban_publish_tenant_vip` (`status`,`expired_at`,`id`)",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			message := strings.ToLower(err.Error())
			if strings.Contains(message, "duplicate key name") || strings.Contains(message, "duplicate column") {
				continue
			}
			return gerror.Wrap(err, "初始化租户会员表失败")
		}
	}
	return nil
}
