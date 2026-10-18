package sys

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/service"
)

const (
	autoDeleteBackfillBatchSize     = 20
	autoDeleteBackfillMaxMessages   = 10000
	autoDeleteBackfillMaxWindow     = 3 * time.Hour
	autoDeleteBackfillBatchInterval = 30 * time.Second
	autoDeleteBackfillDeleteSpacing = 2 * time.Second
)

type autoDeleteBackfillMessage struct {
	Id            int64  `json:"id"`
	TenantId      int64  `json:"tenantId"`
	ChannelId     int64  `json:"channelId"`
	ReceivedBotId int64  `json:"receivedBotId"`
	ChatId        string `json:"chatId"`
	MessageId     int    `json:"messageId"`
	MessageText   string `json:"messageText"`
}

func (s *sSysPublish) handleTelegramAutoDeleteBackfillTask(ctx context.Context, task *asynq.Task) error {
	var payload autoDeleteBackfillQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return gerror.Wrap(err, "解析历史关键字消息清理任务失败")
	}
	startAt, endAt, err := validateAutoDeleteBackfillPayload(payload)
	if err != nil {
		return err
	}
	remaining := payload.MaxMessages - payload.Scanned
	if remaining <= 0 {
		return nil
	}
	limit := autoDeleteBackfillBatchSize
	if remaining < limit {
		limit = remaining
	}
	rows, nextCursor, scanned, hasMore, err := s.autoDeleteBackfillMessages(ctx, payload.CursorId, startAt, endAt, limit)
	if err != nil {
		return err
	}
	configs := make(map[int64]autoDeleteBackfillConfig)
	matched := 0
	for _, row := range rows {
		conf, ok := configs[row.TenantId]
		if !ok {
			view, loadErr := service.SysConfig().AutoDeleteConfigForTenant(ctx, row.TenantId)
			if loadErr != nil {
				return gerror.Wrapf(loadErr, "读取历史关键字消息清理配置失败 tenant:%d", row.TenantId)
			}
			if view != nil && view.AutoDeleteConfig != nil {
				conf.Keywords = view.Keywords
				conf.Rules = view.Rules
			}
			configs[row.TenantId] = conf
		}
		keyword := matchedAutoDeleteKeyword(row.MessageText, conf.Keywords)
		if keyword == "" {
			keyword = matchedAutoDeleteRule(row.MessageText, conf.Rules)
		}
		if keyword == "" {
			continue
		}
		if err = s.enqueueTelegramAutoDeleteBackfillDelete(ctx, autoDeleteQueuePayload{
			BotId: row.ReceivedBotId, TenantId: row.TenantId, ChannelId: row.ChannelId,
			ChatId: row.ChatId, MessageId: row.MessageId, Keyword: keyword,
		}, time.Duration(matched)*autoDeleteBackfillDeleteSpacing); err != nil {
			return gerror.Wrap(err, "投递历史关键字消息删除任务失败")
		}
		matched++
	}
	payload.CursorId = nextCursor
	payload.Scanned += scanned
	if hasMore && payload.Scanned < payload.MaxMessages {
		if err = s.enqueueTelegramAutoDeleteBackfill(ctx, payload, autoDeleteBackfillBatchInterval); err != nil {
			return gerror.Wrap(err, "投递下一批历史关键字消息清理任务失败")
		}
	}
	g.Log().Infof(ctx, "历史关键字消息清理批次完成 cursor:%d scanned:%d totalScanned:%d matched:%d", payload.CursorId, scanned, payload.Scanned, matched)
	return nil
}

func validateAutoDeleteBackfillPayload(payload autoDeleteBackfillQueuePayload) (time.Time, time.Time, error) {
	startAt, err := time.Parse(time.RFC3339, payload.StartAt)
	if err != nil {
		return time.Time{}, time.Time{}, gerror.New("历史关键字消息清理缺少合法开始时间")
	}
	endAt, err := time.Parse(time.RFC3339, payload.EndAt)
	if err != nil || !endAt.After(startAt) || endAt.Sub(startAt) > autoDeleteBackfillMaxWindow {
		return time.Time{}, time.Time{}, gerror.New("历史关键字消息清理时间区间不合法")
	}
	if payload.MaxMessages <= 0 || payload.MaxMessages > autoDeleteBackfillMaxMessages {
		return time.Time{}, time.Time{}, gerror.New("历史关键字消息清理扫描上限不合法")
	}
	if payload.Scanned < 0 || payload.Scanned > payload.MaxMessages {
		return time.Time{}, time.Time{}, gerror.New("历史关键字消息清理进度不合法")
	}
	return startAt, endAt, nil
}

type autoDeleteBackfillConfig struct {
	Keywords []string
	Rules    []string
}

func (s *sSysPublish) autoDeleteBackfillMessages(ctx context.Context, cursorId int64, startAt, endAt time.Time, limit int) ([]autoDeleteBackfillMessage, int64, int, bool, error) {
	if limit <= 0 {
		limit = autoDeleteBackfillBatchSize
	}
	var messages []autoDeleteBackfillMessage
	err := g.DB().Model(telegramBotMessageSourceTable+" m").Safe().Ctx(ctx).
		Fields("m.id,m.received_bot_id,m.chat_id,m.message_id,m.message_text").
		WhereGT("m.id", cursorId).
		WhereGTE("m.received_at", startAt).
		WhereLT("m.received_at", endAt).
		Where("m.message_text != ''").
		WhereNull("m.deleted_at").
		OrderAsc("m.id").
		Limit(limit).
		Scan(&messages)
	if err != nil {
		return nil, cursorId, 0, false, gerror.Wrap(err, "读取历史关键字消息失败")
	}
	if len(messages) == 0 {
		return []autoDeleteBackfillMessage{}, cursorId, 0, false, nil
	}
	nextCursor := messages[len(messages)-1].Id
	chatIds := make([]string, 0, len(messages)*2)
	for i := range messages {
		messages[i].ChatId = normalizeTelegramChannelChatID(messages[i].ChatId)
		chatIds = append(chatIds, messages[i].ChatId, strings.TrimPrefix(messages[i].ChatId, "-100"))
	}
	type channelRow struct {
		Id           int64  `json:"id"`
		TenantId     int64  `json:"tenantId"`
		TargetChatId string `json:"targetChatId"`
	}
	var channels []channelRow
	if err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,target_chat_id").
		Where("auto_delete_enabled", 1).
		WhereIn("target_chat_id", uniqueStrings(chatIds)).
		WhereNull("deleted_at").
		Scan(&channels); err != nil {
		return nil, cursorId, 0, false, gerror.Wrap(err, "读取历史关键字消息频道失败")
	}
	channelMap := make(map[string][]channelRow, len(channels))
	for _, channel := range channels {
		key := normalizeTelegramChannelChatID(channel.TargetChatId)
		channelMap[key] = append(channelMap[key], channel)
	}
	rows := make([]autoDeleteBackfillMessage, 0, len(messages))
	for _, message := range messages {
		for _, channel := range channelMap[message.ChatId] {
			item := message
			item.TenantId = channel.TenantId
			item.ChannelId = channel.Id
			rows = append(rows, item)
		}
	}
	return rows, nextCursor, len(messages), len(messages) == limit, nil
}
