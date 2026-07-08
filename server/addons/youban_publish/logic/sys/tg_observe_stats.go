package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) runTelegramObserveStatsRefresher(ctx context.Context) {
	time.Sleep(5 * time.Second)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.refreshTelegramObserveStats(ctx); err != nil {
			g.Log().Warningf(ctx, "刷新TG推送观测统计失败：%+v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
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
		_, err = tx.Ctx(ctx).Model(publishTgQueueStatTable).Data(g.Map{
			"stat_time":      now,
			"queue_name":     row["queue_name"].String(),
			"priority_level": row["priority_level"].Int(),
			"status":         row["status"].String(),
			"job_count":      row["job_count"].Int(),
			"oldest_job_at":  row["oldest_job_at"],
			"latest_job_at":  row["latest_job_at"],
			"created_at":     now,
			"updated_at":     now,
		}).Insert()
		if err != nil {
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
		_, err = tx.Ctx(ctx).Model(publishTgChannelStatTable).Data(g.Map{
			"tenant_id":          row["tenant_id"].Int64(),
			"account_id":         row["account_id"].Int64(),
			"channel_id":         row["channel_id"].Int64(),
			"target_chat_id":     row["target_chat_id"].String(),
			"channel_title":      row["channel_title"].String(),
			"pending_count":      row["pending_count"].Int(),
			"queued_count":       row["queued_count"].Int(),
			"sending_count":      row["sending_count"].Int(),
			"sent_count":         row["sent_count"].Int(),
			"failed_count":       row["failed_count"].Int(),
			"retry_count":        row["retry_count"].Int(),
			"rate_limit_count":   row["rate_limit_count"].Int(),
			"last_sent_at":       row["last_sent_at"],
			"last_error_at":      row["last_error_at"],
			"last_error_message": row["last_error_message"].String(),
			"created_at":         now,
			"updated_at":         now,
		}).Insert()
		if err != nil {
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
		_, err = tx.Ctx(ctx).Model(publishTgBotStatTable).Data(g.Map{
			"tenant_id":          row["tenant_id"].Int64(),
			"bot_id":             row["bot_id"].Int64(),
			"bot_name":           row["bot_name"].String(),
			"bot_username":       row["bot_username"].String(),
			"pending_count":      row["pending_count"].Int(),
			"queued_count":       row["queued_count"].Int(),
			"sending_count":      row["sending_count"].Int(),
			"sent_count":         row["sent_count"].Int(),
			"failed_count":       row["failed_count"].Int(),
			"retry_count":        row["retry_count"].Int(),
			"rate_limit_count":   row["rate_limit_count"].Int(),
			"last_sent_at":       row["last_sent_at"],
			"last_error_at":      row["last_error_at"],
			"last_error_message": row["last_error_message"].String(),
			"created_at":         now,
			"updated_at":         now,
		}).Insert()
		if err != nil {
			return gerror.Wrap(err, "写入TG Bot统计失败")
		}
	}
	return nil
}
