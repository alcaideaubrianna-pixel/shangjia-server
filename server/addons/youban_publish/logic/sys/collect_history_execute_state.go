package sys

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) loadCollectHistoryTask(ctx context.Context, taskId int64) (*sysin.CollectHistoryTaskModel, error) {
	var task *sysin.CollectHistoryTaskModel
	err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).Where("id", taskId).Scan(&task)
	if err != nil {
		return nil, gerror.Wrap(err, "读取历史采集任务失败")
	}
	if task == nil || task.Id <= 0 {
		return nil, gerror.New("历史采集任务不存在")
	}
	return task, nil
}

func (s *sSysPublish) markCollectHistoryRunning(ctx context.Context, task *sysin.CollectHistoryTaskModel) error {
	data := g.Map{
		"status":        sysin.CollectHistoryTaskStatusRunning,
		"error_message": "",
	}
	if task.StartedAt == nil {
		data["started_at"] = gtime.Now()
	}
	s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "info", "running", "历史采集任务开始执行", g.Map{"offsetId": task.OffsetId})
	return updateCollectHistoryTask(ctx, task.Id, data)
}

func (s *sSysPublish) updateCollectHistoryProgress(ctx context.Context, task *sysin.CollectHistoryTaskModel, stats collectHistoryStats, offsetId int) error {
	task.ScannedCount += stats.scanned
	task.EventCount += stats.events
	task.DuplicateCount += stats.duplicates
	task.FailedCount += stats.failed
	task.OffsetId = offsetId
	return pdao.YoubanPublishCollectHistoryTask.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return updateCollectHistoryTaskTx(ctx, tx, task.Id, g.Map{
			"offset_id":       offsetId,
			"scanned_count":   gdb.Raw("scanned_count+" + strconv.Itoa(stats.scanned)),
			"event_count":     gdb.Raw("event_count+" + strconv.Itoa(stats.events)),
			"duplicate_count": gdb.Raw("duplicate_count+" + strconv.Itoa(stats.duplicates)),
			"failed_count":    gdb.Raw("failed_count+" + strconv.Itoa(stats.failed)),
		})
	})
}

func (s *sSysPublish) finishCollectHistoryTask(ctx context.Context, task *sysin.CollectHistoryTaskModel, message string) error {
	s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "info", "success", message, g.Map{
		"scanned":   task.ScannedCount,
		"events":    task.EventCount,
		"duplicate": task.DuplicateCount,
		"failed":    task.FailedCount,
	})
	return updateCollectHistoryTask(ctx, task.Id, g.Map{
		"status":      sysin.CollectHistoryTaskStatusSuccess,
		"finished_at": gtime.Now(),
	})
}

func (s *sSysPublish) rescheduleCollectHistoryTask(ctx context.Context, task *sysin.CollectHistoryTaskModel, offsetID int) error {
	if err := updateCollectHistoryTask(ctx, task.Id, g.Map{
		"status":    sysin.CollectHistoryTaskStatusPending,
		"offset_id": offsetID,
	}); err != nil {
		return err
	}
	s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "info", "reschedule", "历史采集分片完成，继续排队执行", g.Map{"offsetId": offsetID})
	return s.enqueueCollectHistoryTask(ctx, task.Id, 2*time.Second)
}

func (s *sSysPublish) pauseCollectHistoryTask(ctx context.Context, task *sysin.CollectHistoryTaskModel, pauseErr *collectHistoryPauseError) error {
	delay := pauseErr.delay
	if delay <= 0 {
		delay = time.Minute
	}
	nextRunAt := gtime.NewFromTime(time.Now().Add(delay))
	s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "warn", "limited", "TG历史消息拉取触发限流，任务已暂停等待", g.Map{
		"delaySeconds": int(delay.Seconds()),
		"error":        pauseErr.Error(),
	})
	if err := updateCollectHistoryTask(ctx, task.Id, g.Map{
		"status":        sysin.CollectHistoryTaskStatusPaused,
		"error_message": pauseErr.Error(),
		"next_run_at":   nextRunAt,
	}); err != nil {
		return err
	}
	return s.enqueueCollectHistoryTask(ctx, task.Id, delay)
}

func (s *sSysPublish) collectHistoryChannel(ctx context.Context, source *sysin.CollectSourceModel) (*sysin.ChannelCacheModel, int64, int64, error) {
	cache, err := s.tgChannelCacheByChannelId(ctx, source.TenantId, source.TgAccountId, source.SourceChatId)
	if err != nil {
		return nil, 0, 0, err
	}
	channelID, err := strconv.ParseInt(cache.ChannelId, 10, 64)
	if err != nil {
		return nil, 0, 0, gerror.New("频道ID无效，请刷新频道缓存")
	}
	accessHash, err := strconv.ParseInt(cache.AccessHash, 10, 64)
	if err != nil {
		return nil, 0, 0, gerror.New("频道AccessHash无效，请刷新频道缓存")
	}
	return cache, channelID, accessHash, nil
}

func (s *sSysPublish) collectHistoryTelegram(ctx context.Context, tgAccountId int64) (*model.TelegramConfig, *accountCollectTgAccount, error) {
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, nil, err
	}
	account, err := s.accountCollectTgAccount(ctx, tgAccountId)
	if err != nil {
		return nil, nil, err
	}
	return conf, account, nil
}

func (s *sSysPublish) collectHistoryClient(ctx context.Context, conf *model.TelegramConfig, account *accountCollectTgAccount) (*telegram.Client, error) {
	return s.newAccountCollectClient(ctx, conf, account, tg.NewUpdateDispatcher())
}

func collectHistoryCutoff(task *sysin.CollectHistoryTaskModel) *gtime.Time {
	if task == nil || task.Mode == sysin.CollectHistoryModeAll {
		return nil
	}
	days := task.Days
	if days <= 0 {
		days = 30
	}
	return gtime.NewFromTime(time.Now().Add(-time.Duration(days) * 24 * time.Hour))
}

func collectHistoryIsFloodWait(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "flood_wait") || strings.Contains(message, "too many requests")
}
