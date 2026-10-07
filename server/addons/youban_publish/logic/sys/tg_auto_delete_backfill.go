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

const autoDeleteBackfillBatchSize = 500

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
	rows, nextCursor, hasMore, err := s.autoDeleteBackfillMessages(ctx, payload.CursorId, autoDeleteBackfillBatchSize)
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
		if err = s.enqueueTelegramAutoDelete(ctx, autoDeleteQueuePayload{
			BotId: row.ReceivedBotId, TenantId: row.TenantId, ChannelId: row.ChannelId,
			ChatId: row.ChatId, MessageId: row.MessageId, Keyword: keyword,
		}); err != nil {
			return gerror.Wrap(err, "投递历史关键字消息删除任务失败")
		}
		matched++
	}
	if hasMore {
		if err = s.enqueueTelegramAutoDeleteBackfill(ctx, nextCursor, 200*time.Millisecond); err != nil {
			return gerror.Wrap(err, "投递下一批历史关键字消息清理任务失败")
		}
	}
	g.Log().Infof(ctx, "历史关键字消息清理批次完成 cursor:%d scanned:%d matched:%d", payload.CursorId, len(rows), matched)
	return nil
}

type autoDeleteBackfillConfig struct {
	Keywords []string
	Rules    []string
}

func (s *sSysPublish) autoDeleteBackfillMessages(ctx context.Context, cursorId int64, limit int) ([]autoDeleteBackfillMessage, int64, bool, error) {
	if limit <= 0 {
		limit = autoDeleteBackfillBatchSize
	}
	var messages []autoDeleteBackfillMessage
	err := g.DB().Model(telegramBotMessageSourceTable+" m").Safe().Ctx(ctx).
		Fields("m.id,m.received_bot_id,m.chat_id,m.message_id,m.message_text").
		WhereGT("m.id", cursorId).
		Where("m.message_text != ''").
		WhereNull("m.deleted_at").
		OrderAsc("m.id").
		Limit(limit).
		Scan(&messages)
	if err != nil {
		return nil, cursorId, false, gerror.Wrap(err, "读取历史关键字消息失败")
	}
	if len(messages) == 0 {
		return []autoDeleteBackfillMessage{}, cursorId, false, nil
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
		return nil, cursorId, false, gerror.Wrap(err, "读取历史关键字消息频道失败")
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
	return rows, nextCursor, len(messages) == limit, nil
}
