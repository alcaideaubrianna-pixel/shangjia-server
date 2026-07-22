package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

func ensureTenantVipTables(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return ensureTenantVipPgsqlTables(ctx)
	}
	return ensureTenantVipMysqlTables(ctx)
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
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "初始化租户会员表失败")
		}
	}
	return nil
}
