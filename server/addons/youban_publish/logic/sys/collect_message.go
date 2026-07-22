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
		return s.mergeCollectMessageEvent(ctx, event, message, mediaJSON, rawText, now)
	}
	status := sysin.CollectEventStatusPending
	if strings.TrimSpace(message.SourceGroupedId) != "" {
		status = sysin.CollectEventStatusGroupCollect
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
		eventCols.Status:          status,
		eventCols.ReceivedAt:      receivedAt,
		eventCols.CreatedAt:       now,
		eventCols.UpdatedAt:       now,
	}).InsertAndGetId()
	if err != nil {
		if isCollectEventUniqueConflict(err) {
			event, readErr := eventDao.Ctx(ctx).Where(eventCols.SourceUniqueKey, message.SourceUniqueKey).One()
			if readErr != nil {
				return 0, gerror.Wrap(readErr, "读取冲突采集事件失败")
			}
			if !event.IsEmpty() {
				return s.mergeCollectMessageEvent(ctx, event, message, mediaJSON, rawText, now)
			}
		}
		return 0, gerror.Wrap(err, "创建采集事件失败")
	}
	created, err := eventDao.Ctx(ctx).Where(eventCols.Id, eventId).One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取采集事件失败")
	}
	if err = s.upsertCollectEventMedia(ctx, created, message.Media); err != nil {
		return 0, err
	}
	s.appendCollectEventLogForRecord(ctx, created, "ingest", "created", "采集事件已入库", "")
	_, _ = sourceDao.Ctx(ctx).Where(sourceCols.Id, message.SourceId).Data(g.Map{
		sourceCols.EventTotal:  gdb.Raw(sourceCols.EventTotal + "+1"),
		sourceCols.LastEventAt: now,
		sourceCols.UpdatedAt:   now,
	}).Update()
	return eventId, nil
}

func (s *sSysPublish) mergeCollectMessageEvent(ctx context.Context, event gdb.Record, message *CollectMessage, mediaJSON string, rawText string, now *gtime.Time) (int64, error) {
	eventDao := pdao.YoubanPublishCollectEvent
	eventCols := eventDao.Columns()
	eventId := event[eventCols.Id].Int64()
	shouldMerge := collectExistingEventShouldMerge(event, mediaJSON)
	if collectEventAlreadyMatched(event[eventCols.Status].String()) && !shouldMerge {
		return eventId, nil
	}
	nextText := strings.TrimSpace(event[eventCols.RawText].String())
	if nextText == "" {
		nextText = rawText
	}
	nextMediaJSON, nextMediaCount := mergeCollectMediaJSON(event[eventCols.MediaJson].String(), mediaJSON)
	nextSourceMessageId := event[eventCols.SourceMessageId].Int64()
	if message.SourceMessageId > 0 && (nextSourceMessageId <= 0 || message.SourceMessageId < nextSourceMessageId) {
		nextSourceMessageId = message.SourceMessageId
	}
	_, err := eventDao.Ctx(ctx).Where(eventCols.Id, eventId).Data(g.Map{
		eventCols.RawText:         nextText,
		eventCols.MediaCount:      nextMediaCount,
		eventCols.MediaJson:       nextMediaJSON,
		eventCols.SourceMessageId: nextSourceMessageId,
		eventCols.TextHash:        collectHash(nextText),
		eventCols.DedupeKey:       collectHash(fmt.Sprintf("%s:%s:%d", nextText, nextMediaJSON, nextMediaCount)),
		eventCols.Status:          collectMergedEventStatus(event, message),
		eventCols.ErrorMessage:    "",
		eventCols.UpdatedAt:       now,
	}).Update()
	if err != nil {
		return eventId, gerror.Wrap(err, "更新采集事件失败")
	}
	updated, err := eventDao.Ctx(ctx).Where(eventCols.Id, eventId).One()
	if err != nil {
		return eventId, gerror.Wrap(err, "读取采集事件失败")
	}
	if err = s.upsertCollectEventMedia(ctx, updated, message.Media); err != nil {
		return eventId, err
	}
	s.appendCollectEventLogForRecord(ctx, updated, "ingest", "updated", "采集事件已合并媒体", "")
	return eventId, nil
}

func collectMergedEventStatus(event gdb.Record, message *CollectMessage) string {
	if message != nil && strings.TrimSpace(message.SourceGroupedId) != "" {
		return sysin.CollectEventStatusGroupCollect
	}
	status := strings.TrimSpace(event["status"].String())
	if status == sysin.CollectEventStatusGroupCollect {
		return sysin.CollectEventStatusPending
	}
	return sysin.CollectEventStatusPending
}

func isCollectEventUniqueConflict(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "uk_ybp_collect_event_unique") ||
		strings.Contains(message, "duplicate key value")
}

func collectExistingEventShouldMerge(event gdb.Record, mediaJSON string) bool {
	if strings.TrimSpace(event["source_grouped_id"].String()) == "" || strings.TrimSpace(mediaJSON) == "" || strings.TrimSpace(mediaJSON) == "[]" {
		return false
	}
	nextMediaJSON, nextMediaCount := mergeCollectMediaJSON(event["media_json"].String(), mediaJSON)
	return nextMediaCount > event["media_count"].Int() || collectMediaSignature(nextMediaJSON) != collectMediaSignature(event["media_json"].String())
}

func collectMessageMediaJSON(media []collectMediaItem) (string, int) {
	items := make([]collectMediaItem, 0, len(media))
	for _, item := range media {
		item.Type = strings.TrimSpace(item.Type)
		item.Purpose = strings.TrimSpace(item.Purpose)
		item.FileId = strings.TrimSpace(item.FileId)
		item.FileUrl = strings.TrimSpace(item.FileUrl)
		item.StoragePath = strings.TrimSpace(item.StoragePath)
		item.PosterUrl = strings.TrimSpace(item.PosterUrl)
		item.MetaJson = strings.TrimSpace(item.MetaJson)
		if item.Type == "" || collectMediaSourceKey(item) == "" {
			continue
		}
		items = append(items, item)
	}
	data, _ := json.Marshal(items)
	return string(data), len(items)
}
