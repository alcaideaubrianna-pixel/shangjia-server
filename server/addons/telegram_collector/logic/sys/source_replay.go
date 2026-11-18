package sys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	candidates, _, err := s.loadBotUpdateReplayCandidates(ctx, tenantID, eventIDs)
	return candidates, err
}

func (s *sCollector) loadBotUpdateReplayCandidates(ctx context.Context, tenantID int64, eventIDs []int64) ([]*collectorservice.BotUpdateReplayCandidate, map[int64]*entity.TgCollectorEvent, error) {
	ids, err := normalizeBotReplayEventIDs(eventIDs)
	if err != nil {
		return nil, nil, err
	}
	if tenantID <= 0 {
		return nil, nil, gerror.New("租户ID不能为空")
	}
	columns := dao.TgCollectorEvent.Columns()
	var rows []*entity.TgCollectorEvent
	if err = dao.TgCollectorEvent.Ctx(ctx).
		Where(columns.TenantId, tenantID).
		WhereIn(columns.Id, ids).
		Where(columns.SourceType, sysin.SourceTypeBot).
		Scan(&rows); err != nil {
		return nil, nil, gerror.Wrap(err, "读取待重放Bot原始事件失败")
	}
	if len(rows) != len(ids) {
		return nil, nil, gerror.Newf("原始事件校验失败：请求%d条，找到%d条Bot事件", len(ids), len(rows))
	}
	loaded := make(map[int64]*entity.TgCollectorEvent, len(rows))
	byID := make(map[int64]*collectorservice.BotUpdateReplayCandidate, len(rows))
	for _, row := range rows {
		if row == nil || row.Id <= 0 || row.SourceId <= 0 || strings.TrimSpace(row.BotKey) == "" || row.RawUpdate == nil {
			return nil, nil, gerror.New("原始Bot事件缺少来源或机器人标识")
		}
		var update models.Update
		if err := json.Unmarshal([]byte(row.RawUpdate.String()), &update); err != nil {
			return nil, nil, gerror.Wrapf(err, "解析原始Bot事件%d失败", row.Id)
		}
		message := collectorUpdateMessage(&update)
		if message == nil || row.UpdateId > 0 && row.UpdateId != update.ID {
			return nil, nil, gerror.Newf("原始Bot事件%d不是有效消息更新", row.Id)
		}
		candidate := &collectorservice.BotUpdateReplayCandidate{
			ID: row.Id, TenantID: row.TenantId, SourceID: row.SourceId, BotKey: row.BotKey,
			UpdateID: update.ID, ChatID: strconv.FormatInt(message.Chat.ID, 10), MessageID: int64(message.ID), EventStatus: row.Status,
		}
		if row.ReceivedAt != nil {
			candidate.ReceivedAt = row.ReceivedAt.Time
		}
		byID[row.Id] = candidate
		loaded[row.Id] = row
	}
	candidates := make([]*collectorservice.BotUpdateReplayCandidate, 0, len(ids))
	for _, id := range ids {
		candidate := byID[id]
		if candidate == nil {
			return nil, nil, gerror.Newf("原始Bot事件%d不存在", id)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, loaded, nil
}

func (s *sCollector) ReplayBotUpdatesForSource(ctx context.Context, tenantID, sourceID int64, eventIDs []int64) (*collectorservice.BotUpdateReplayResult, error) {
	if tenantID <= 0 || sourceID <= 0 {
		return nil, gerror.New("目标采集源归属不能为空")
	}
	candidates, rows, err := s.loadBotUpdateReplayCandidates(ctx, tenantID, eventIDs)
	if err != nil {
		return nil, err
	}
	result := &collectorservice.BotUpdateReplayResult{Failed: make([]int64, 0)}
	for _, candidate := range candidates {
		queued, replayErr := s.replayLoadedBotUpdateForSource(ctx, candidate, rows[candidate.ID], sourceID)
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
	return s.replayLoadedBotUpdateForSource(ctx, candidate, &original, sourceID)
}

func (s *sCollector) replayLoadedBotUpdateForSource(ctx context.Context, candidate *collectorservice.BotUpdateReplayCandidate, original *entity.TgCollectorEvent, sourceID int64) (bool, error) {
	if candidate == nil || candidate.ID <= 0 || sourceID <= 0 || original == nil ||
		original.Id != candidate.ID || original.TenantId != candidate.TenantID || original.SourceId != candidate.SourceID ||
		original.SourceType != sysin.SourceTypeBot || original.RawUpdate == nil || original.BotKey != candidate.BotKey {
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
	event := botSourceReplayEvent(original, sourceID, update.ID, raw, message, receivedAt)
	// Only reuse the persisted identity when every scope and message field agrees.
	// Legacy keys or changed sources must still go through the scoped upsert.
	if original.SourceId == sourceID && original.EventKey == event.EventID &&
		original.AccountId == 0 && original.UpdateId == event.UpdateID && candidate.UpdateID == event.UpdateID &&
		original.ChatId == candidate.ChatID && original.MessageId == event.MessageID {
		return s.enqueueReplayLoadedEvent(ctx, original)
	}
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
	var row entity.TgCollectorEvent
	if err := dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).Scan(&row); err != nil {
		return false, gerror.Wrap(err, "读取source重放事件状态失败")
	}
	if row.Id <= 0 {
		return false, gerror.New("source重放事件不存在")
	}
	return s.enqueueReplayLoadedEvent(ctx, &row)
}

func (s *sCollector) enqueueReplayLoadedEvent(ctx context.Context, row *entity.TgCollectorEvent) (bool, error) {
	if row == nil || row.Id <= 0 {
		return false, gerror.New("source重放事件不存在")
	}
	// System notices remain terminal even during an explicit history repair.
	if row.Status == sysin.EventStatusIgnored && row.RawUpdate != nil && collectorSystemNotice(row.SourceType, []byte(row.RawUpdate.String())) {
		return false, nil
	}
	columns := dao.TgCollectorEvent.Columns()
	eventID := row.Id
	ctx = withCollectorSourceScope(ctx, row.TenantId, row.SourceId)
	// The row is already loaded for replay; register before any state mutation.
	if err := markCollectorRecoveryScope(ctx, collectorRecoveryScope{row.TenantId, row.AccountId, row.SourceId}); err != nil {
		return false, gerror.Wrap(err, "登记source重放恢复 scope 失败")
	}

	switch row.Status {
	case sysin.EventStatusReceived, sysin.EventStatusFailedRetry:
		return true, s.enqueueSourceReplayEventTask(ctx, eventID)
	case sysin.EventStatusProcessing:
		return false, nil
	case sysin.EventStatusDead, sysin.EventStatusIgnored:
		result, err := dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).
			WhereIn(columns.Status, []string{sysin.EventStatusDead, sysin.EventStatusIgnored}).
			Where(fmt.Sprintf("(%s IS NULL OR %s<=?)", columns.LeaseUntil, columns.LeaseUntil), gtime.Now()).
			Data(g.Map{columns.Status: sysin.EventStatusReceived, columns.AttemptCount: 0, columns.NextRunAt: nil,
				columns.LeaseOwner: "", columns.LeaseUntil: nil, columns.ErrorMessage: "", columns.UpdatedAt: gtime.Now()}).Update()
		if err != nil {
			return false, gerror.Wrap(err, "恢复source重放事件失败")
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return false, nil
		}
		return true, s.enqueueSourceReplayEventTask(ctx, eventID)
	case sysin.EventStatusReady:
		return s.enqueueReplayDelivery(ctx, row)
	default:
		return false, gerror.Newf("source重放事件状态不支持：%s", row.Status)
	}
}

// Explicit, bounded repairs use the isolated history budget. Keep the original
// raw event unchanged: atomic claims prevent an old urgent copy from repeating
// an event that this repair has already handed off.
func (s *sCollector) enqueueSourceReplayEventTask(ctx context.Context, eventID int64) error {
	return s.enqueueEventTask(ctx, eventID, sysin.EventPriorityNormal, eventOriginReplay)
}

func (s *sCollector) enqueueReplayDelivery(ctx context.Context, event *entity.TgCollectorEvent) (bool, error) {
	if event == nil || event.Id <= 0 || event.TenantId <= 0 || event.SourceId <= 0 || event.RawUpdate == nil {
		return false, gerror.New("source重放交付缺少原始事件身份")
	}
	columns := dao.TgCollectorDelivery.Columns()
	eventID := event.Id
	raw := []byte(event.RawUpdate.String())
	baseKey := collectorDeliveryKey(event, raw)
	var row entity.TgCollectorDelivery
	var identityErr error
	// All probes use the existing (tenant_id, delivery_key) unique index.
	// Never fall back to event_id: it is neither indexed nor an update identity.
	for _, key := range []string{baseKey, collectorDeliveryCollisionKey(baseKey, eventID), fmt.Sprintf("event:%d", eventID)} {
		var candidate struct {
			entity.TgCollectorDelivery
			SourceId  int64
			AccountId int64
		}
		if err := dao.TgCollectorDelivery.Ctx(ctx).
			Where(columns.TenantId, event.TenantId).
			Where(columns.DeliveryKey, key).Scan(&candidate); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, gerror.Wrap(err, "读取source重放交付状态失败")
		}
		if candidate.Id <= 0 {
			continue
		}
		// Legacy keys need both the event lifetime fence and explicit routing
		// scope; a restored/reused event ID must never revive another source.
		if candidate.SourceId != event.SourceId || candidate.AccountId != event.AccountId ||
			validateCollectorDeliveryEvent(&candidate.TgCollectorDelivery, event, raw) != nil {
			if key != fmt.Sprintf("event:%d", eventID) && candidate.EventId == eventID {
				identityErr = gerror.New("source重放交付身份冲突，禁止交付到其他来源")
			}
			continue
		}
		row = candidate.TgCollectorDelivery
		break
	}
	if row.Id <= 0 {
		if identityErr != nil {
			return false, identityErr
		}
		eventColumns := dao.TgCollectorEvent.Columns()
		result, err := dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).
			Where(eventColumns.TenantId, event.TenantId).
			Where(eventColumns.SourceId, event.SourceId).
			Where(eventColumns.EventKey, event.EventKey).
			Where(eventColumns.Status, sysin.EventStatusReady).
			Where(fmt.Sprintf("(%s IS NULL OR %s<=?)", eventColumns.LeaseUntil, eventColumns.LeaseUntil), gtime.Now()).
			Data(do.TgCollectorEvent{Status: sysin.EventStatusReceived, AttemptCount: 0, ProcessedAt: nil, ErrorMessage: "", UpdatedAt: gtime.Now()}).Update()
		if err != nil {
			return false, gerror.Wrap(err, "恢复缺失source交付的事件失败")
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			return true, s.enqueueSourceReplayEventTask(ctx, eventID)
		}
		return false, nil
	}
	switch row.Status {
	case sysin.DeliveryStatusCompleted, sysin.DeliveryStatusProcessing, sysin.DeliveryStatusQuarantined:
		return false, nil
	case sysin.DeliveryStatusPending:
		result, err := dao.TgCollectorDelivery.Ctx(ctx).WherePri(row.Id).
			Where(columns.Status, sysin.DeliveryStatusPending).
			Data(g.Map{columns.Priority: sysin.EventPriorityNormal, "scheduler_pool": "history"}).Update()
		if err != nil {
			return false, gerror.Wrap(err, "保存source重放历史链路失败")
		}
		if affected, err := result.RowsAffected(); err != nil || affected == 0 {
			return false, err
		}
		return true, s.enqueueDeliveryTaskWithOptions(ctx, row.Id, sysin.EventPriorityNormal, false)
	case sysin.DeliveryStatusFailedRetry, sysin.DeliveryStatusDead:
		result, err := dao.TgCollectorDelivery.Ctx(ctx).WherePri(row.Id).
			WhereIn(columns.Status, []string{sysin.DeliveryStatusFailedRetry, sysin.DeliveryStatusDead}).
			Where(fmt.Sprintf("(%s IS NULL OR %s<=?)", columns.LeaseUntil, columns.LeaseUntil), gtime.Now()).
			Data(g.Map{columns.Status: sysin.DeliveryStatusPending, columns.AttemptCount: 0, columns.NextRunAt: nil,
				columns.Priority: sysin.EventPriorityNormal, "scheduler_pool": "history",
				columns.LeaseOwner: "", columns.LeaseUntil: nil, columns.ErrorMessage: "", columns.UpdatedAt: gtime.Now()}).Update()
		if err != nil {
			return false, gerror.Wrap(err, "恢复source重放交付失败")
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return false, nil
		}
		return true, s.enqueueDeliveryTaskWithOptions(ctx, row.Id, sysin.EventPriorityNormal, false)
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
