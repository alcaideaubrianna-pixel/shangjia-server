package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type CollectMessage struct {
	TenantId        int64
	AccountId       int64
	SourceId        int64
	SourceType      string
	BotId           int64
	TgAccountId     int64
	SourceChatId    string
	SourceMessageId int64
	SourceGroupedId string
	SourceUniqueKey string
	RawText         string
	Media           []collectMediaItem
	ReceivedAt      *gtime.Time
}

func (s *sSysPublish) ingestCollectMessage(ctx context.Context, message *CollectMessage) (int64, error) {
	if message == nil {
		return 0, gerror.New("采集消息不能为空")
	}
	if message.TenantId <= 0 || message.AccountId <= 0 || message.SourceId <= 0 {
		return 0, gerror.New("采集消息归属不完整")
	}
	message.SourceType = strings.TrimSpace(message.SourceType)
	if message.SourceType == "" {
		return 0, gerror.New("采集消息来源类型不能为空")
	}
	message.SourceUniqueKey = strings.TrimSpace(message.SourceUniqueKey)
	if message.SourceUniqueKey == "" {
		return 0, gerror.New("采集消息唯一键不能为空")
	}
	mediaJSON, mediaCount := collectMessageMediaJSON(message.Media)
	rawText := strings.TrimSpace(message.RawText)
	dedupeKey := collectHash(fmt.Sprintf("%s:%s:%d", rawText, mediaJSON, mediaCount))
	now := gtime.Now()
	receivedAt := message.ReceivedAt
	if receivedAt == nil {
		receivedAt = now
	}
	eventDao := pdao.YoubanPublishCollectEvent
	eventCols := eventDao.Columns()
	sourceDao := pdao.YoubanPublishCollectSource
	sourceCols := sourceDao.Columns()
	event, err := eventDao.Ctx(ctx).Where(eventCols.SourceUniqueKey, message.SourceUniqueKey).One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取采集事件失败")
	}
	if !event.IsEmpty() {
		if event[eventCols.Status].String() == sysin.CollectEventStatusProcessed {
			return event[eventCols.Id].Int64(), nil
		}
		_, err = eventDao.Ctx(ctx).Where(eventCols.Id, event[eventCols.Id].Int64()).Data(g.Map{
			eventCols.RawText:      rawText,
			eventCols.MediaCount:   mediaCount,
			eventCols.MediaJson:    mediaJSON,
			eventCols.TextHash:     collectHash(rawText),
			eventCols.DedupeKey:    dedupeKey,
			eventCols.ErrorMessage: "",
			eventCols.UpdatedAt:    now,
		}).Update()
		return event[eventCols.Id].Int64(), gerror.Wrap(err, "更新采集事件失败")
	}
	eventId, err := eventDao.Ctx(ctx).Data(g.Map{
		eventCols.TenantId:        message.TenantId,
		eventCols.AccountId:       message.AccountId,
		eventCols.SourceId:        message.SourceId,
		eventCols.SourceType:      message.SourceType,
		eventCols.BotId:           message.BotId,
		eventCols.TgAccountId:     message.TgAccountId,
		eventCols.SourceChatId:    message.SourceChatId,
		eventCols.SourceMessageId: message.SourceMessageId,
		eventCols.SourceGroupedId: message.SourceGroupedId,
		eventCols.SourceUniqueKey: message.SourceUniqueKey,
		eventCols.RawText:         rawText,
		eventCols.MediaCount:      mediaCount,
		eventCols.MediaJson:       mediaJSON,
		eventCols.TextHash:        collectHash(rawText),
		eventCols.DedupeKey:       dedupeKey,
		eventCols.Status:          sysin.CollectEventStatusPending,
		eventCols.ReceivedAt:      receivedAt,
		eventCols.CreatedAt:       now,
		eventCols.UpdatedAt:       now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建采集事件失败")
	}
	_, _ = sourceDao.Ctx(ctx).Where(sourceCols.Id, message.SourceId).Data(g.Map{
		sourceCols.EventTotal:  gdb.Raw(sourceCols.EventTotal + "+1"),
		sourceCols.LastEventAt: now,
		sourceCols.UpdatedAt:   now,
	}).Update()
	return eventId, nil
}

func collectMessageMediaJSON(media []collectMediaItem) (string, int) {
	items := make([]collectMediaItem, 0, len(media))
	for _, item := range media {
		item.Type = strings.TrimSpace(item.Type)
		item.FileId = strings.TrimSpace(item.FileId)
		item.FileUrl = strings.TrimSpace(item.FileUrl)
		item.StoragePath = strings.TrimSpace(item.StoragePath)
		item.PosterUrl = strings.TrimSpace(item.PosterUrl)
		if item.Type == "" || collectMediaSourceKey(item) == "" {
			continue
		}
		items = append(items, item)
	}
	data, _ := json.Marshal(items)
	return string(data), len(items)
}
