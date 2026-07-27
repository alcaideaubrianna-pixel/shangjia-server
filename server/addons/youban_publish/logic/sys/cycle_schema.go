package sys

import (
	"context"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
)

const publishChannelProfileTable = "hg_youban_publish_channel_profile"

func ensureChannelCycleSchema(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return ensureChannelCyclePgsqlSchema(ctx)
	}
	return ensureChannelCycleMysqlSchema(ctx)
}

func ensureChannelCyclePgsqlSchema(ctx context.Context) error {
	statements := []string{
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_next_run_at" timestamp DEFAULT NULL`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_last_run_at" timestamp DEFAULT NULL`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_active_run_id" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_last_error_message" text`,
		`ALTER TABLE "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "channel_id" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "cursor_id" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "total_count" integer NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "queued_count" integer NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_channel_profile" (
			"id" bigserial PRIMARY KEY,
			"tenant_id" bigint NOT NULL DEFAULT 0,
			"account_id" bigint NOT NULL DEFAULT 0,
			"channel_id" bigint NOT NULL DEFAULT 0,
			"profile_id" bigint NOT NULL DEFAULT 0,
			"task_id" bigint NOT NULL DEFAULT 0,
			"last_job_id" bigint NOT NULL DEFAULT 0,
			"status" varchar(16) NOT NULL DEFAULT 'active',
			"created_at" timestamp DEFAULT NULL,
			"updated_at" timestamp DEFAULT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_channel_profile" ON "hg_youban_publish_channel_profile" ("channel_id", "profile_id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_channel_profile_scan" ON "hg_youban_publish_channel_profile" ("channel_id", "status", "id")`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "初始化频道循环上架结构失败")
		}
	}
	return nil
}

func ensureChannelCycleMysqlSchema(ctx context.Context) error {
	statements := []string{
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_next_run_at` datetime DEFAULT NULL COMMENT '下次循环上架时间'",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_last_run_at` datetime DEFAULT NULL COMMENT '上次循环上架时间'",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_active_run_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '当前循环批次ID'",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_last_error_message` text COMMENT '循环上架最近错误'",
		"ALTER TABLE `hg_youban_publish_cycle_run` ADD COLUMN `cursor_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料索引游标'",
		"ALTER TABLE `hg_youban_publish_cycle_run` ADD COLUMN `total_count` int(11) NOT NULL DEFAULT '0' COMMENT '资料总数'",
		"ALTER TABLE `hg_youban_publish_cycle_run` ADD COLUMN `queued_count` int(11) NOT NULL DEFAULT '0' COMMENT '已生成任务数'",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_channel_profile` (`id` bigint(20) NOT NULL AUTO_INCREMENT,`tenant_id` bigint(20) NOT NULL DEFAULT '0',`account_id` bigint(20) NOT NULL DEFAULT '0',`channel_id` bigint(20) NOT NULL DEFAULT '0',`profile_id` bigint(20) NOT NULL DEFAULT '0',`task_id` bigint(20) NOT NULL DEFAULT '0',`last_job_id` bigint(20) NOT NULL DEFAULT '0',`status` varchar(16) NOT NULL DEFAULT 'active',`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_channel_profile` (`channel_id`,`profile_id`),KEY `idx_ybp_channel_profile_scan` (`channel_id`,`status`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='频道当前上架资料索引'",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil && !isPublishChannelSchemaExistsError(err) {
			return gerror.Wrap(err, "初始化频道循环上架结构失败")
		}
	}
	return nil
}

func (s *sSysPublish) syncChannelCycleAfterSave(ctx context.Context, tenantId int64, channelId int64, enabled int, days int, publishTime string) error {
	if tenantId <= 0 || channelId <= 0 {
		return nil
	}
	if err := ensureChannelCycleSchema(ctx); err != nil {
		return err
	}
	data := g.Map{
		"cycle_active_run_id":      0,
		"cycle_last_error_message": "",
		"updated_at":               gtime.Now(),
	}
	if enabled == 1 {
		data["cycle_next_run_at"] = s.nextChannelCycleRunAt(ctx, days, publishTime, gtime.Now())
	} else {
		data["cycle_next_run_at"] = nil
	}
	_, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("id", channelId).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新频道循环上架时间失败")
	}
	return nil
}

func parseCycleClock(value string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) < 2 {
		return 0, 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}
