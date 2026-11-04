package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/do"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

const botUpdateReplayLimit = 1000

func (s *sCollector) BotUpdateReplayCandidates(ctx context.Context, tenantID int64, eventIDs []int64) ([]*collectorservice.BotUpdateReplayCandidate, error) {
	ids, err := normalizeBotReplayEventIDs(eventIDs)
	if err != nil {
		return nil, err
	}
	if tenantID <= 0 {
		return nil, gerror.New("租户ID不能为空")
	}
	columns := dao.TgCollectorEvent.Columns()
	var rows []*entity.TgCollectorEvent
	if err = dao.TgCollectorEvent.Ctx(ctx).
		Fields(columns.Id, columns.TenantId, columns.SourceId, columns.BotKey, columns.UpdateId, columns.ChatId, columns.MessageId, columns.Status, columns.ReceivedAt, columns.RawUpdate).
		Where(columns.TenantId, tenantID).
		WhereIn(columns.Id, ids).
		Where(columns.SourceType, sysin.SourceTypeBot).
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取待重放Bot原始事件失败")
	}
	if len(rows) != len(ids) {
		return nil, gerror.Newf("原始事件校验失败：请求%d条，找到%d条Bot事件", len(ids), len(rows))
	}
	byID := make(map[int64]*collectorservice.BotUpdateReplayCandidate, len(rows))
	for _, row := range rows {
		if row == nil || row.Id <= 0 || row.SourceId <= 0 || strings.TrimSpace(row.BotKey) == "" || row.RawUpdate == nil {
			return nil, gerror.New("原始Bot事件缺少来源或机器人标识")
		}
		var update models.Update
		if err := json.Unmarshal([]byte(row.RawUpdate.String()), &update); err != nil {
			return nil, gerror.Wrapf(err, "解析原始Bot事件%d失败", row.Id)
		}
		message := collectorUpdateMessage(&update)
		if message == nil || row.UpdateId > 0 && row.UpdateId != update.ID {
			return nil, gerror.Newf("原始Bot事件%d不是有效消息更新", row.Id)
		}
		candidate := &collectorservice.BotUpdateReplayCandidate{
			ID: row.Id, TenantID: row.TenantId, SourceID: row.SourceId, BotKey: row.BotKey,
			UpdateID: update.ID, ChatID: strconv.FormatInt(message.Chat.ID, 10), MessageID: int64(message.ID), EventStatus: row.Status,
		}
		if row.ReceivedAt != nil {
			candidate.ReceivedAt = row.ReceivedAt.Time
		}
		byID[row.Id] = candidate
	}
	candidates := make([]*collectorservice.BotUpdateReplayCandidate, 0, len(ids))
	for _, id := range ids {
		candidate := byID[id]
		if candidate == nil {
			return nil, gerror.Newf("原始Bot事件%d不存在", id)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func (s *sCollector) ReplayBotUpdatesForSource(ctx context.Context, tenantID, sourceID int64, eventIDs []int64) (*collectorservice.BotUpdateReplayResult, error) {
	if tenantID <= 0 || sourceID <= 0 {
		return nil, gerror.New("目标采集源归属不能为空")
	}
	candidates, err := s.BotUpdateReplayCandidates(ctx, tenantID, eventIDs)
	if err != nil {
		return nil, err
	}
	result := &collectorservice.BotUpdateReplayResult{Failed: make([]int64, 0)}
	for _, candidate := range candidates {
		queued, replayErr := s.replayBotUpdateForSource(ctx, candidate, sourceID)
		if replayErr != nil {
			result.Failed = append(result.Failed, candidate.ID)
			g.Log().Warningf(ctx, "Bot原始事件按source重放失败 sourceId:%d rawEventId:%d err:%+v", sourceID, candidate.ID, replayErr)
			continue
		}
		if queued {
			result.Queued++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}

func (s *sCollector) replayBotUpdateForSource(ctx context.Context, candidate *collectorservice.BotUpdateReplayCandidate, sourceID int64) (bool, error) {
	if candidate == nil || candidate.ID <= 0 {
		return false, gerror.New("待重放Bot原始事件无效")
	}
	columns := dao.TgCollectorEvent.Columns()
	var original entity.TgCollectorEvent
	if err := dao.TgCollectorEvent.Ctx(ctx).Where(columns.Id, candidate.ID).
		Where(columns.TenantId, candidate.TenantID).
		Where(columns.SourceType, sysin.SourceTypeBot).Scan(&original); err != nil {
		return false, gerror.Wrap(err, "读取Bot原始更新失败")
	}
	if original.Id <= 0 || original.RawUpdate == nil || original.BotKey != candidate.BotKey {
		return false, gerror.New("Bot原始更新已变化或不存在")
	}
	raw := []byte(original.RawUpdate.String())
	var update models.Update
	if err := json.Unmarshal(raw, &update); err != nil {
		return false, gerror.Wrap(err, "解析待重放Telegram Bot更新失败")
	}
	message := collectorUpdateMessage(&update)
	if message == nil || candidate.UpdateID > 0 && candidate.UpdateID != update.ID ||
		strconv.FormatInt(message.Chat.ID, 10) != candidate.ChatID || int64(message.ID) != candidate.MessageID {
		return false, gerror.New("原始Bot事件不是有效消息更新")
	}
	receivedAt := time.Now()
	if original.ReceivedAt != nil && !original.ReceivedAt.IsZero() {
		receivedAt = original.ReceivedAt.Time
	}
	event := botSourceReplayEvent(&original, sourceID, update.ID, raw, message, receivedAt)
	eventID, err := s.persistEvent(ctx, event)
	if err != nil {
		return false, err
	}
	return s.enqueueReplayEvent(ctx, eventID)
}

func botSourceReplayEvent(original *entity.TgCollectorEvent, sourceID, updateID int64, raw []byte, message *models.Message, receivedAt time.Time) *sysin.RawUpdateEvent {
	return &sysin.RawUpdateEvent{
		EventID:  updateEventID(original.BotKey, original.TenantId, sourceID, updateID, raw),
		TenantID: original.TenantId, SourceID: sourceID, SourceType: sysin.SourceTypeBot,
		BotKey: original.BotKey, ChatID: message.Chat.ID, MessageID: int64(message.ID),
		UpdateID: updateID, Priority: sysin.EventPriorityUrgent, RawUpdate: raw, ReceivedAt: receivedAt,
	}
}

func (s *sCollector) enqueueReplayEvent(ctx context.Context, eventID int64) (bool, error) {
	columns := dao.TgCollectorEvent.Columns()
	var row entity.TgCollectorEvent
	if err := dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).Scan(&row); err != nil {
		return false, gerror.Wrap(err, "读取source重放事件状态失败")
	}
	if row.Id <= 0 {
		return false, gerror.New("source重放事件不存在")
	}
	switch row.Status {
	case sysin.EventStatusReceived, sysin.EventStatusFailedRetry:
		return true, s.enqueueEventTask(ctx, eventID, row.Priority)
	case sysin.EventStatusProcessing:
		return false, nil
	case sysin.EventStatusDead:
		result, err := dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).
			Where(columns.Status, sysin.EventStatusDead).
			Where(fmt.Sprintf("(%s IS NULL OR %s<=?)", columns.LeaseUntil, columns.LeaseUntil), gtime.Now()).
			Data(g.Map{columns.Status: sysin.EventStatusReceived, columns.AttemptCount: 0, columns.NextRunAt: nil,
				columns.LeaseOwner: "", columns.LeaseUntil: nil, columns.ErrorMessage: "", columns.UpdatedAt: gtime.Now()}).Update()
		if err != nil {
			return false, gerror.Wrap(err, "恢复source重放事件失败")
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return false, nil
		}
		return true, s.enqueueEventTask(ctx, eventID, row.Priority)
	case sysin.EventStatusReady:
		return s.enqueueReplayDelivery(ctx, eventID)
	default:
		return false, gerror.Newf("source重放事件状态不支持：%s", row.Status)
	}
}

func (s *sCollector) enqueueReplayDelivery(ctx context.Context, eventID int64) (bool, error) {
	columns := dao.TgCollectorDelivery.Columns()
	var row entity.TgCollectorDelivery
	if err := dao.TgCollectorDelivery.Ctx(ctx).Where(columns.EventId, eventID).Scan(&row); err != nil {
		return false, gerror.Wrap(err, "读取source重放交付状态失败")
	}
	if row.Id <= 0 {
		eventColumns := dao.TgCollectorEvent.Columns()
		result, err := dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).
			Where(eventColumns.Status, sysin.EventStatusReady).
			Data(do.TgCollectorEvent{Status: sysin.EventStatusReceived, AttemptCount: 0, ProcessedAt: nil, ErrorMessage: "", UpdatedAt: gtime.Now()}).Update()
		if err != nil {
			return false, gerror.Wrap(err, "恢复缺失source交付的事件失败")
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			return true, s.enqueueEventTask(ctx, eventID, sysin.EventPriorityUrgent)
		}
		return false, nil
	}
	switch row.Status {
	case sysin.DeliveryStatusCompleted, sysin.DeliveryStatusProcessing, sysin.DeliveryStatusQuarantined:
		return false, nil
	case sysin.DeliveryStatusPending:
		return true, s.enqueueDeliveryTask(ctx, row.Id, row.Priority)
	case sysin.DeliveryStatusFailedRetry, sysin.DeliveryStatusDead:
		result, err := dao.TgCollectorDelivery.Ctx(ctx).WherePri(row.Id).
			WhereIn(columns.Status, []string{sysin.DeliveryStatusFailedRetry, sysin.DeliveryStatusDead}).
			Where(fmt.Sprintf("(%s IS NULL OR %s<=?)", columns.LeaseUntil, columns.LeaseUntil), gtime.Now()).
			Data(g.Map{columns.Status: sysin.DeliveryStatusPending, columns.AttemptCount: 0, columns.NextRunAt: nil,
				columns.LeaseOwner: "", columns.LeaseUntil: nil, columns.ErrorMessage: "", columns.UpdatedAt: gtime.Now()}).Update()
		if err != nil {
			return false, gerror.Wrap(err, "恢复source重放交付失败")
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return false, nil
		}
		return true, s.enqueueDeliveryTask(ctx, row.Id, row.Priority)
	default:
		return false, gerror.Newf("source重放交付状态不支持：%s", row.Status)
	}
}

func normalizeBotReplayEventIDs(eventIDs []int64) ([]int64, error) {
	ids := make([]int64, 0, len(eventIDs))
	seen := make(map[int64]struct{}, len(eventIDs))
	for _, id := range eventIDs {
		if id <= 0 {
			return nil, gerror.New("collector event ID必须为正整数")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(ids) > botUpdateReplayLimit {
		return nil, gerror.Newf("原始事件数量必须为1到%d条", botUpdateReplayLimit)
	}
	return ids, nil
}
