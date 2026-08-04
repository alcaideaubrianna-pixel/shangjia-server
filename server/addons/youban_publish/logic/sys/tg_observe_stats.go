package sys

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
)

const telegramObserveLastErrorMaxRunes = 512

func (s *sSysPublish) runTelegramObserveStatsRefresher(ctx context.Context) {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.refreshTelegramObserveStats(ctx); err != nil {
			if ctx.Err() == nil && !isExpectedTelegramObserveShutdownError(err) {
				g.Log().Warningf(ctx, "刷新TG推送观测统计失败：%+v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func isExpectedTelegramObserveShutdownError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, sql.ErrTxDone) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "transaction has already been committed or rolled back") ||
		strings.Contains(message, "context canceled")
}

func (s *sSysPublish) refreshTelegramObserveStats(ctx context.Context) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := s.refreshTelegramQueueStats(ctx, tx); err != nil {
			return err
		}
		if err := s.refreshTelegramChannelStats(ctx, tx); err != nil {
			return err
		}
		return s.refreshTelegramBotStats(ctx, tx)
	})
}

func (s *sSysPublish) refreshTelegramQueueStats(ctx context.Context, tx gdb.TX) error {
	rows, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("COALESCE(NULLIF(queue_name,''), 'youban_publish_tg') AS queue_name, priority AS priority_level, status, COUNT(1) AS job_count, MIN(created_at) AS oldest_job_at, MAX(created_at) AS latest_job_at").
		Group("COALESCE(NULLIF(queue_name,''), 'youban_publish_tg'), priority, status").
		All()
	if err != nil {
		return gerror.Wrap(err, "统计TG队列失败")
	}
	if _, err = tx.Ctx(ctx).Exec("DELETE FROM " + publishTgQueueStatTable); err != nil {
		return gerror.Wrap(err, "清理TG队列统计失败")
	}
	now := gtime.Now()
	for _, row := range rows {
		if err = s.upsertTelegramQueueStat(ctx, tx, row, now); err != nil {
			return gerror.Wrap(err, "写入TG队列统计失败")
		}
	}
	return nil
}

func (s *sSysPublish) refreshTelegramChannelStats(ctx context.Context, tx gdb.TX) error {
	fields := `
j.tenant_id,j.account_id,j.channel_id,j.target_chat_id,MAX(c.channel_title) AS channel_title,
SUM(CASE WHEN j.status IN ('pending','failed_retry') AND (j.dispatch_status='' OR j.dispatch_status='idle') THEN 1 ELSE 0 END) AS pending_count,
SUM(CASE WHEN j.dispatch_status='queued' THEN 1 ELSE 0 END) AS queued_count,
SUM(CASE WHEN j.status='sending' THEN 1 ELSE 0 END) AS sending_count,
SUM(CASE WHEN j.status='sent' THEN 1 ELSE 0 END) AS sent_count,
SUM(CASE WHEN j.status='failed' THEN 1 ELSE 0 END) AS failed_count,
SUM(CASE WHEN j.status='failed_retry' THEN 1 ELSE 0 END) AS retry_count,
SUM(CASE WHEN j.error_message LIKE '%限流%' OR j.error_message LIKE '%Too Many Requests%' THEN 1 ELSE 0 END) AS rate_limit_count,
MAX(j.sent_at) AS last_sent_at,
MAX(CASE WHEN j.status IN ('failed','failed_retry') THEN j.updated_at ELSE NULL END) AS last_error_at,
MAX(j.error_message) AS last_error_message`
	rows, err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		LeftJoin(publishChannelTable+" c", "c.id=j.channel_id").
		Fields(fields).
		Group("j.tenant_id,j.account_id,j.channel_id,j.target_chat_id").
		All()
	if err != nil {
		return gerror.Wrap(err, "统计TG频道失败")
	}
	if _, err = tx.Ctx(ctx).Exec("DELETE FROM " + publishTgChannelStatTable); err != nil {
		return gerror.Wrap(err, "清理TG频道统计失败")
	}
	now := gtime.Now()
	for _, row := range rows {
		if err = s.upsertTelegramChannelStat(ctx, tx, row, now); err != nil {
			return gerror.Wrap(err, "写入TG频道统计失败")
		}
	}
	return nil
}

func (s *sSysPublish) refreshTelegramBotStats(ctx context.Context, tx gdb.TX) error {
	fields := `
j.tenant_id,j.bot_id,MAX(b.bot_name) AS bot_name,MAX(b.bot_username) AS bot_username,
SUM(CASE WHEN j.status IN ('pending','failed_retry') AND (j.dispatch_status='' OR j.dispatch_status='idle') THEN 1 ELSE 0 END) AS pending_count,
SUM(CASE WHEN j.dispatch_status='queued' THEN 1 ELSE 0 END) AS queued_count,
SUM(CASE WHEN j.status='sending' THEN 1 ELSE 0 END) AS sending_count,
SUM(CASE WHEN j.status='sent' THEN 1 ELSE 0 END) AS sent_count,
SUM(CASE WHEN j.status='failed' THEN 1 ELSE 0 END) AS failed_count,
SUM(CASE WHEN j.status='failed_retry' THEN 1 ELSE 0 END) AS retry_count,
SUM(CASE WHEN j.error_message LIKE '%限流%' OR j.error_message LIKE '%Too Many Requests%' THEN 1 ELSE 0 END) AS rate_limit_count,
MAX(j.sent_at) AS last_sent_at,
MAX(CASE WHEN j.status IN ('failed','failed_retry') THEN j.updated_at ELSE NULL END) AS last_error_at,
MAX(j.error_message) AS last_error_message`
	rows, err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		LeftJoin(publishBotTable+" b", "b.id=j.bot_id").
		Fields(fields).
		Group("j.tenant_id,j.bot_id").
		All()
	if err != nil {
		return gerror.Wrap(err, "统计TG Bot失败")
	}
	if _, err = tx.Ctx(ctx).Exec("DELETE FROM " + publishTgBotStatTable); err != nil {
		return gerror.Wrap(err, "清理TG Bot统计失败")
	}
	now := gtime.Now()
	for _, row := range rows {
		if err = s.upsertTelegramBotStat(ctx, tx, row, now); err != nil {
			return gerror.Wrap(err, "写入TG Bot统计失败")
		}
	}
	return nil
}

func (s *sSysPublish) upsertTelegramQueueStat(ctx context.Context, tx gdb.TX, row gdb.Record, now *gtime.Time) error {
	args := []interface{}{now, row["queue_name"].String(), row["priority_level"].Int(), row["status"].String(), row["job_count"].Int(), row["oldest_job_at"], row["latest_job_at"], now, now}
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		_, err := tx.Ctx(ctx).Exec(`INSERT INTO "`+publishTgQueueStatTable+`" ("stat_time","queue_name","priority_level","status","job_count","oldest_job_at","latest_job_at","created_at","updated_at")
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT ("queue_name","priority_level","status") DO UPDATE SET "stat_time"=EXCLUDED."stat_time","job_count"=EXCLUDED."job_count","oldest_job_at"=EXCLUDED."oldest_job_at","latest_job_at"=EXCLUDED."latest_job_at","updated_at"=EXCLUDED."updated_at"`, args...)
		return err
	}
	_, err := tx.Ctx(ctx).Exec("INSERT INTO `"+publishTgQueueStatTable+"` (`stat_time`,`queue_name`,`priority_level`,`status`,`job_count`,`oldest_job_at`,`latest_job_at`,`created_at`,`updated_at`) VALUES (?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE `stat_time`=VALUES(`stat_time`),`job_count`=VALUES(`job_count`),`oldest_job_at`=VALUES(`oldest_job_at`),`latest_job_at`=VALUES(`latest_job_at`),`updated_at`=VALUES(`updated_at`)", args...)
	return err
}

func (s *sSysPublish) upsertTelegramChannelStat(ctx context.Context, tx gdb.TX, row gdb.Record, now *gtime.Time) error {
	args := []interface{}{row["tenant_id"].Int64(), row["account_id"].Int64(), row["channel_id"].Int64(), row["target_chat_id"].String(), row["channel_title"].String(), row["pending_count"].Int(), row["queued_count"].Int(), row["sending_count"].Int(), row["sent_count"].Int(), row["failed_count"].Int(), row["retry_count"].Int(), row["rate_limit_count"].Int(), row["last_sent_at"], row["last_error_at"], truncateTelegramObserveError(row["last_error_message"].String()), now, now}
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		_, err := tx.Ctx(ctx).Exec(`INSERT INTO "`+publishTgChannelStatTable+`" ("tenant_id","account_id","channel_id","target_chat_id","channel_title","pending_count","queued_count","sending_count","sent_count","failed_count","retry_count","rate_limit_count","last_sent_at","last_error_at","last_error_message","created_at","updated_at")
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT ("channel_id","target_chat_id") DO UPDATE SET "tenant_id"=EXCLUDED."tenant_id","account_id"=EXCLUDED."account_id","channel_title"=EXCLUDED."channel_title","pending_count"=EXCLUDED."pending_count","queued_count"=EXCLUDED."queued_count","sending_count"=EXCLUDED."sending_count","sent_count"=EXCLUDED."sent_count","failed_count"=EXCLUDED."failed_count","retry_count"=EXCLUDED."retry_count","rate_limit_count"=EXCLUDED."rate_limit_count","last_sent_at"=EXCLUDED."last_sent_at","last_error_at"=EXCLUDED."last_error_at","last_error_message"=EXCLUDED."last_error_message","updated_at"=EXCLUDED."updated_at"`, args...)
		return err
	}
	_, err := tx.Ctx(ctx).Exec("INSERT INTO `"+publishTgChannelStatTable+"` (`tenant_id`,`account_id`,`channel_id`,`target_chat_id`,`channel_title`,`pending_count`,`queued_count`,`sending_count`,`sent_count`,`failed_count`,`retry_count`,`rate_limit_count`,`last_sent_at`,`last_error_at`,`last_error_message`,`created_at`,`updated_at`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE `tenant_id`=VALUES(`tenant_id`),`account_id`=VALUES(`account_id`),`channel_title`=VALUES(`channel_title`),`pending_count`=VALUES(`pending_count`),`queued_count`=VALUES(`queued_count`),`sending_count`=VALUES(`sending_count`),`sent_count`=VALUES(`sent_count`),`failed_count`=VALUES(`failed_count`),`retry_count`=VALUES(`retry_count`),`rate_limit_count`=VALUES(`rate_limit_count`),`last_sent_at`=VALUES(`last_sent_at`),`last_error_at`=VALUES(`last_error_at`),`last_error_message`=VALUES(`last_error_message`),`updated_at`=VALUES(`updated_at`)", args...)
	return err
}

func (s *sSysPublish) upsertTelegramBotStat(ctx context.Context, tx gdb.TX, row gdb.Record, now *gtime.Time) error {
	args := []interface{}{row["tenant_id"].Int64(), row["bot_id"].Int64(), row["bot_name"].String(), row["bot_username"].String(), row["pending_count"].Int(), row["queued_count"].Int(), row["sending_count"].Int(), row["sent_count"].Int(), row["failed_count"].Int(), row["retry_count"].Int(), row["rate_limit_count"].Int(), row["last_sent_at"], row["last_error_at"], truncateTelegramObserveError(row["last_error_message"].String()), now, now}
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		_, err := tx.Ctx(ctx).Exec(`INSERT INTO "`+publishTgBotStatTable+`" ("tenant_id","bot_id","bot_name","bot_username","pending_count","queued_count","sending_count","sent_count","failed_count","retry_count","rate_limit_count","last_sent_at","last_error_at","last_error_message","created_at","updated_at")
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT ("tenant_id","bot_id") DO UPDATE SET "bot_name"=EXCLUDED."bot_name","bot_username"=EXCLUDED."bot_username","pending_count"=EXCLUDED."pending_count","queued_count"=EXCLUDED."queued_count","sending_count"=EXCLUDED."sending_count","sent_count"=EXCLUDED."sent_count","failed_count"=EXCLUDED."failed_count","retry_count"=EXCLUDED."retry_count","rate_limit_count"=EXCLUDED."rate_limit_count","last_sent_at"=EXCLUDED."last_sent_at","last_error_at"=EXCLUDED."last_error_at","last_error_message"=EXCLUDED."last_error_message","updated_at"=EXCLUDED."updated_at"`, args...)
		return err
	}
	_, err := tx.Ctx(ctx).Exec("INSERT INTO `"+publishTgBotStatTable+"` (`tenant_id`,`bot_id`,`bot_name`,`bot_username`,`pending_count`,`queued_count`,`sending_count`,`sent_count`,`failed_count`,`retry_count`,`rate_limit_count`,`last_sent_at`,`last_error_at`,`last_error_message`,`created_at`,`updated_at`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE `bot_name`=VALUES(`bot_name`),`bot_username`=VALUES(`bot_username`),`pending_count`=VALUES(`pending_count`),`queued_count`=VALUES(`queued_count`),`sending_count`=VALUES(`sending_count`),`sent_count`=VALUES(`sent_count`),`failed_count`=VALUES(`failed_count`),`retry_count`=VALUES(`retry_count`),`rate_limit_count`=VALUES(`rate_limit_count`),`last_sent_at`=VALUES(`last_sent_at`),`last_error_at`=VALUES(`last_error_at`),`last_error_message`=VALUES(`last_error_message`),`updated_at`=VALUES(`updated_at`)", args...)
	return err
}

func truncateTelegramObserveError(message string) string {
	runes := []rune(message)
	if len(runes) <= telegramObserveLastErrorMaxRunes {
		return message
	}
	return string(runes[:telegramObserveLastErrorMaxRunes])
}
