package sys

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	collectHistoryPageLimit                = 100
	collectHistoryPagesPerRun              = 20
	collectHistoryPageInterval             = 900 * time.Millisecond
	collectHistoryPendingEventLimitDefault = 80
	collectHistoryBackpressureDelayDefault = 30 * time.Second
)

type collectHistoryPauseError struct {
	delay time.Duration
	err   error
}

func (e *collectHistoryPauseError) Error() string {
	return e.err.Error()
}

func (e *collectHistoryPauseError) Unwrap() error {
	return e.err
}

func (s *sSysPublish) ExecuteCollectHistoryTask(ctx context.Context, taskId int64) error {
	task, err := s.loadCollectHistoryTask(ctx, taskId)
	if err != nil {
		return err
	}
	if task.Status == sysin.CollectHistoryTaskStatusSuccess || task.Status == sysin.CollectHistoryTaskStatusFailed || task.Status == sysin.CollectHistoryTaskStatusCanceled {
		return nil
	}
	version := int64(0)
	if task.UpdatedAt != nil {
		version = task.UpdatedAt.TimestampNano()
	}
	_, err = collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{
		TenantID: task.TenantId, AccountID: task.TgAccountId,
		TaskType: collectorin.AccountTaskTypeHistoryPage,
		TaskKey:  collectHistoryAccountTaskKey(task.Id, task.OffsetId, version),
		Priority: collectorin.EventPriorityNormal, HistoryTaskID: task.Id, MaxAttempts: 5,
	})
	if err != nil {
		return gerror.Wrap(err, "提交历史采集账号任务失败")
	}
	s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "info", "queued", "历史采集已提交账号运行实例", g.Map{"offsetId": task.OffsetId, "tgAccountId": task.TgAccountId})
	return nil
}

func collectHistoryAccountTaskKey(taskID int64, offsetID int, version int64) string {
	return fmt.Sprintf("history:%d:offset:%d:version:%d", taskID, offsetID, version)
}

func (s *sSysPublish) handleCollectHistoryAccountTask(ctx context.Context, client *telegram.Client, taskId int64) error {
	task, err := s.loadCollectHistoryTask(ctx, taskId)
	if err != nil {
		return err
	}
	if task.Status == sysin.CollectHistoryTaskStatusSuccess || task.Status == sysin.CollectHistoryTaskStatusFailed || task.Status == sysin.CollectHistoryTaskStatusCanceled {
		return nil
	}
	err = s.executeCollectHistoryTask(ctx, client, task)
	if canceled, cancelErr := collectHistoryTaskCanceled(ctx, task.Id); cancelErr != nil {
		return cancelErr
	} else if canceled {
		return nil
	}
	var pauseErr *collectHistoryPauseError
	if errors.As(err, &pauseErr) {
		return s.pauseCollectHistoryTask(ctx, task, pauseErr)
	}
	var retryErr *collectMediaRetryError
	if errors.As(err, &retryErr) {
		delay := retryErr.delay
		if delay <= 0 {
			delay = 5 * time.Second
		}
		return s.pauseCollectHistoryTask(ctx, task, &collectHistoryPauseError{delay: delay, err: retryErr})
	}
	if collectHistoryTransientClientError(err) {
		return s.pauseCollectHistoryTask(ctx, task, &collectHistoryPauseError{
			delay: 5 * time.Second,
			err:   gerror.Wrap(err, "TG历史采集连接暂时中断，等待共享连接恢复"),
		})
	}
	if err != nil {
		s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "error", "failed", err.Error(), nil)
		return updateCollectHistoryTask(ctx, task.Id, g.Map{
			"status":        sysin.CollectHistoryTaskStatusFailed,
			"error_message": err.Error(),
			"finished_at":   gtime.Now(),
		})
	}
	return nil
}

func collectHistoryTransientClientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, keyword := range []string{
		"client closed",
		"context canceled",
		"dc is closed",
		"engine forcibly closed",
		"use of closed network connection",
	} {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

func (s *sSysPublish) executeCollectHistoryTask(ctx context.Context, client *telegram.Client, task *sysin.CollectHistoryTaskModel) error {
	if task == nil || task.Id <= 0 {
		return gerror.New("历史采集任务不存在")
	}
	source, err := s.collectHistorySource(ctx, task.SourceId, task.TenantId, task.AccountId)
	if err != nil {
		return err
	}
	cache, channelID, accessHash, err := s.collectHistoryChannel(ctx, source)
	if err != nil {
		return err
	}
	task.SourceChatId = cache.ChannelId
	if err = s.markCollectHistoryRunning(ctx, task); err != nil {
		return err
	}
	if client == nil {
		return gerror.New("TG账号共享连接不可用")
	}
	if _, err = client.Self(ctx); err != nil {
		return err
	}
	return s.scanCollectHistory(ctx, client, task, source, channelID, accessHash)
}

func (s *sSysPublish) scanCollectHistory(ctx context.Context, client *telegram.Client, task *sysin.CollectHistoryTaskModel, source *sysin.CollectSourceModel, channelID int64, accessHash int64) error {
	offsetID := task.OffsetId
	cutoff := collectHistoryCutoff(task)
	for page := 0; page < collectHistoryPagesPerRun; page++ {
		if canceled, cancelErr := collectHistoryTaskCanceled(ctx, task.Id); cancelErr != nil {
			return cancelErr
		} else if canceled {
			return nil
		}
		pendingCount, err := collectHistoryPendingEventCount(ctx, task)
		if err != nil {
			return err
		}
		pendingLimit := collectHistoryPendingEventLimit(ctx)
		if pendingCount >= pendingLimit {
			return &collectHistoryPauseError{
				delay: collectHistoryBackpressureDelay(ctx),
				err:   gerror.Newf("历史采集等待已有资料处理完成，当前待处理:%d，上限:%d", pendingCount, pendingLimit),
			}
		}
		pageLimit := collectHistoryNextPageLimit(pendingCount, pendingLimit)
		messages, err := collectHistoryPage(ctx, client, channelID, accessHash, offsetID, pageLimit)
		if err != nil {
			return err
		}
		if len(messages) == 0 {
			return s.finishCollectHistoryTask(ctx, task, "历史消息已拉取完成")
		}
		stats, nextOffset, stop, err := s.ingestCollectHistoryMessages(ctx, task, source, messages, cutoff)
		if err != nil {
			return err
		}
		if nextOffset > 0 {
			offsetID = nextOffset
		}
		if err = s.updateCollectHistoryProgress(ctx, task, stats, offsetID); err != nil {
			return err
		}
		s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "info", "page", "历史消息分页处理完成", g.Map{
			"page":      page + 1,
			"offsetId":  offsetID,
			"fetched":   len(messages),
			"scanned":   stats.scanned,
			"events":    stats.events,
			"duplicate": stats.duplicates,
			"failed":    stats.failed,
		})
		processPayload := collectProcessQueuePayload{
			SourceId:  task.SourceId,
			TenantId:  task.TenantId,
			AccountId: task.AccountId,
		}
		if processErr := s.enqueueCollectProcess(ctx, processPayload, 0); processErr != nil {
			s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "warn", "process", "历史消息页已落库，资料处理将在下一轮继续", g.Map{"error": processErr.Error()})
		} else {
			s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "info", "process", "历史消息页已投递资料处理队列", g.Map{"offsetId": offsetID})
		}
		if stop || len(messages) < pageLimit {
			return s.finishCollectHistoryTask(ctx, task, "历史消息已拉取完成")
		}
		time.Sleep(collectHistoryPageInterval)
	}
	return s.rescheduleCollectHistoryTask(ctx, task, offsetID)
}

func collectHistoryTaskCanceled(ctx context.Context, taskId int64) (bool, error) {
	if taskId <= 0 {
		return true, nil
	}
	status, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).Where("id", taskId).Fields("status").Value()
	if err != nil {
		return false, err
	}
	return status.String() == sysin.CollectHistoryTaskStatusCanceled, nil
}

func collectHistoryNextOffset(current int, messages []*tg.Message) int {
	next := current
	for _, msg := range messages {
		if msg == nil || msg.ID <= 0 {
			continue
		}
		if next == 0 || msg.ID < next {
			next = msg.ID
		}
	}
	return next
}

func collectHistoryPage(ctx context.Context, client *telegram.Client, channelID int64, accessHash int64, offsetID int, limit int) ([]*tg.Message, error) {
	history := collectorservice.AccountHistory()
	if history == nil {
		return nil, gerror.New("Telegram历史采集执行器未注册")
	}
	messages, err := history.FetchPage(ctx, client, &collectorin.AccountHistoryPageRequest{
		ChannelID: channelID, AccessHash: accessHash, OffsetID: offsetID, Limit: limit,
	})
	if delay, ok := history.RetryDelay(err); ok {
		return nil, &collectHistoryPauseError{delay: delay, err: err}
	}
	return messages, err
}

func collectHistoryNextPageLimit(pendingCount int, pendingLimit int) int {
	remaining := pendingLimit - pendingCount
	if remaining <= 0 {
		return 0
	}
	if remaining < collectHistoryPageLimit {
		return remaining
	}
	return collectHistoryPageLimit
}

func collectHistoryPendingEventLimit(ctx context.Context) int {
	limit := g.Cfg().MustGet(ctx, "youbanPublish.queue.historyPendingEventLimit", collectHistoryPendingEventLimitDefault).Int()
	if limit < 1 {
		return collectHistoryPendingEventLimitDefault
	}
	return limit
}

func collectHistoryBackpressureDelay(ctx context.Context) time.Duration {
	seconds := g.Cfg().MustGet(ctx, "youbanPublish.queue.historyBackpressureDelaySeconds", int(collectHistoryBackpressureDelayDefault/time.Second)).Int()
	if seconds < 1 {
		return collectHistoryBackpressureDelayDefault
	}
	return time.Duration(seconds) * time.Second
}

func collectHistoryQueueConcurrency(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.queue.historyConcurrency", 1).Int()
	if value < 1 {
		return 1
	}
	if value > 4 {
		return 4
	}
	return value
}

func collectHistoryPendingEventCount(ctx context.Context, task *sysin.CollectHistoryTaskModel) (int, error) {
	if task == nil || task.SourceId <= 0 || task.TenantId <= 0 {
		return 0, nil
	}
	cols := pdao.YoubanPublishCollectEvent.Columns()
	count, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where(cols.TenantId, task.TenantId).
		Where(cols.SourceId, task.SourceId).
		WhereIn(cols.Status, []string{
			sysin.CollectEventStatusPending,
			sysin.CollectEventStatusGroupCollect,
			sysin.CollectEventStatusWaitingOrder,
			sysin.CollectEventStatusPrechecked,
			sysin.CollectEventStatusMediaPending,
			sysin.CollectEventStatusMediaReady,
		}).
		Count()
	if err != nil {
		return 0, gerror.Wrap(err, "统计历史采集待处理资料失败")
	}
	return count, nil
}

type collectHistoryStats struct {
	scanned    int
	events     int
	duplicates int
	failed     int
}

func (s *sSysPublish) ingestCollectHistoryMessages(ctx context.Context, task *sysin.CollectHistoryTaskModel, source *sysin.CollectSourceModel, messages []*tg.Message, cutoff *gtime.Time) (collectHistoryStats, int, bool, error) {
	stats := collectHistoryStats{}
	nextOffset := task.OffsetId
	stop := false
	runtimeSource := accountCollectSourceRuntime{
		Id:           source.Id,
		TenantId:     source.TenantId,
		AccountId:    source.AccountId,
		TgAccountId:  source.TgAccountId,
		SourceChatId: source.SourceChatId,
	}
	messages = collectHistoryMessagesInSendOrder(messages)
	for _, msg := range messages {
		if msg == nil || msg.ID <= 0 {
			continue
		}
		if nextOffset == 0 || msg.ID < nextOffset {
			nextOffset = msg.ID
		}
		receivedAt := gtime.NewFromTime(time.Unix(int64(msg.Date), 0))
		if cutoff != nil && receivedAt.Before(cutoff) {
			stop = true
			continue
		}
		stats.scanned++
		message := gotdCollectMessage(source.TgAccountId, runtimeSource, msg, source.SourceChatId)
		event := accountCollectorEvent(message)
		exists, err := collectorservice.Collector().EventExists(ctx, event.TenantID, event.SourceUniqueKey)
		if err != nil {
			return stats, nextOffset, stop, err
		}
		err = collectorservice.Collector().IngestAccountMessage(ctx, event)
		if err != nil {
			stats.failed++
			s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "warn", "event", "历史消息提交采集插件失败", g.Map{"messageId": msg.ID, "error": err.Error()})
			continue
		}
		if exists {
			stats.duplicates++
		} else {
			stats.events++
		}
	}
	return stats, nextOffset, stop, nil
}

func collectHistoryMessagesInSendOrder(messages []*tg.Message) []*tg.Message {
	ordered := make([]*tg.Message, 0, len(messages))
	for _, msg := range messages {
		if msg != nil && msg.ID > 0 {
			ordered = append(ordered, msg)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}
