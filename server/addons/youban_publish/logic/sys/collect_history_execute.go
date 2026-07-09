package sys

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	collectHistoryPageLimit    = 100
	collectHistoryPagesPerRun  = 20
	collectHistoryPageInterval = 900 * time.Millisecond
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
	if task.Status == sysin.CollectHistoryTaskStatusSuccess || task.Status == sysin.CollectHistoryTaskStatusFailed {
		return nil
	}
	runCtx, cancel := context.WithTimeout(ctx, 25*time.Minute)
	defer cancel()
	err = s.executeCollectHistoryTask(runCtx, task)
	if pauseErr, ok := err.(*collectHistoryPauseError); ok {
		return s.pauseCollectHistoryTask(ctx, task, pauseErr)
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

func (s *sSysPublish) executeCollectHistoryTask(ctx context.Context, task *sysin.CollectHistoryTaskModel) error {
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
	conf, tgAccount, err := s.collectHistoryTelegram(ctx, task.TgAccountId)
	if err != nil {
		return err
	}
	task.SourceChatId = cache.ChannelId
	if err = s.markCollectHistoryRunning(ctx, task); err != nil {
		return err
	}
	client, err := s.collectHistoryClient(ctx, conf, tgAccount)
	if err != nil {
		return err
	}
	return client.Run(ctx, func(runCtx context.Context) error {
		if _, err = client.Self(runCtx); err != nil {
			return err
		}
		return s.scanCollectHistory(runCtx, client, task, source, channelID, accessHash)
	})
}

func (s *sSysPublish) scanCollectHistory(ctx context.Context, client *telegram.Client, task *sysin.CollectHistoryTaskModel, source *sysin.CollectSourceModel, channelID int64, accessHash int64) error {
	offsetID := task.OffsetId
	cutoff := collectHistoryCutoff(task)
	pages := make([]*tg.Message, 0, collectHistoryPageLimit)
	fetchedPages := 0
	shouldFinish := false
	for page := 0; page < collectHistoryPagesPerRun; page++ {
		messages, err := collectHistoryPage(ctx, client, channelID, accessHash, offsetID)
		if err != nil {
			return err
		}
		if len(messages) == 0 {
			shouldFinish = true
			break
		}
		fetchedPages++
		pages = append(pages, messages...)
		offsetID = collectHistoryNextOffset(offsetID, messages)
		s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "info", "page", "历史消息分页处理完成", g.Map{
			"page":     page + 1,
			"offsetId": offsetID,
			"fetched":  len(messages),
		})
		if len(messages) < collectHistoryPageLimit {
			shouldFinish = true
			break
		}
		time.Sleep(collectHistoryPageInterval)
	}
	if len(pages) == 0 {
		return s.finishCollectHistoryTask(ctx, task, "历史消息已拉取完成")
	}
	stats, nextOffset, stop, err := s.ingestCollectHistoryMessages(ctx, task, source, pages, cutoff)
	if err != nil {
		return err
	}
	if nextOffset > 0 {
		offsetID = nextOffset
	}
	if err = s.updateCollectHistoryProgress(ctx, task, stats, offsetID); err != nil {
		return err
	}
	s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "info", "batch", "历史消息批次处理完成", g.Map{
		"pages":     fetchedPages,
		"offsetId":  offsetID,
		"scanned":   stats.scanned,
		"events":    stats.events,
		"duplicate": stats.duplicates,
		"failed":    stats.failed,
	})
	if stop || shouldFinish {
		return s.finishCollectHistoryTask(ctx, task, "历史消息已拉取完成")
	}
	return s.rescheduleCollectHistoryTask(ctx, task, offsetID)
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

func collectHistoryPage(ctx context.Context, client *telegram.Client, channelID int64, accessHash int64, offsetID int) ([]*tg.Message, error) {
	var res tg.MessagesMessagesClass
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		res, err = client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     &tg.InputPeerChannel{ChannelID: channelID, AccessHash: accessHash},
			OffsetID: offsetID,
			Limit:    collectHistoryPageLimit,
		})
		if err == nil {
			return tgHistoryMessages(res), nil
		}
		delay := tgRepairBackoffDelay(attempt, err)
		if collectHistoryIsFloodWait(err) {
			return nil, &collectHistoryPauseError{delay: delay, err: err}
		}
		if !isTgRepairRetryableErr(err) || attempt == 2 {
			return nil, gerror.Wrap(err, "拉取历史消息失败")
		}
		time.Sleep(delay)
	}
	return tgHistoryMessages(res), nil
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
		exists, err := collectEventExists(ctx, message.SourceUniqueKey)
		if err != nil {
			return stats, nextOffset, stop, err
		}
		eventId, err := s.ingestCollectMessage(ctx, message)
		if err != nil {
			stats.failed++
			s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "warn", "event", "历史消息写入事件失败", g.Map{"messageId": msg.ID, "error": err.Error()})
			continue
		}
		if exists {
			stats.duplicates++
		} else {
			stats.events++
		}
		err = s.processCollectEvent(ctx, eventId, source.TenantId, source.AccountId)
		if err == nil && !exists {
			s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "info", "process", "历史采集事件已处理", g.Map{"eventId": eventId, "messageId": msg.ID})
		}
		if err != nil {
			stats.failed++
			s.appendCollectHistoryLog(ctx, task.Id, task.TenantId, task.AccountId, "warn", "event", "历史采集事件处理失败", g.Map{"eventId": eventId, "messageId": msg.ID, "error": err.Error()})
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

func collectEventExists(ctx context.Context, uniqueKey string) (bool, error) {
	count, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("source_unique_key", strings.TrimSpace(uniqueKey)).
		Count()
	return count > 0, gerror.Wrap(err, "检查采集事件重复失败")
}
