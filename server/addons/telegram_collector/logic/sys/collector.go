package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/do"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/collectobserve"
	"hotgo/internal/library/microbatch"
)

const gatewayOwner = "telegram_collector"

const mediaPipelineVersion = "v1"

var collectorMeter = otel.Meter("hotgo/addons/telegram_collector")

func observeIngestLatency(ctx context.Context, event *sysin.RawUpdateEvent, stage string) {
	if event == nil {
		return
	}
	_, meta := collectorObservationContext(ctx, collectorObservationMeta{})
	if meta.ReceivedAt.IsZero() {
		return
	}
	seconds := time.Since(meta.ReceivedAt).Seconds()
	if seconds < 0 {
		return
	}
	histogram, _ := collectorMeter.Float64Histogram("telegram_collector_ingest_latency_seconds")
	histogram.Record(ctx, seconds, metric.WithAttributes(append(collectorCollectionAttributes(ctx),
		attribute.String("stage", stage),
		attribute.String("source_type", event.SourceType),
		attribute.String("queue_class", collectorQueueClass(event.Priority)),
	)...))
}

func collectorQueueName(ctx context.Context) string {
	queue, _ := asynq.GetQueueName(ctx)
	return queue
}

func collectorQueueClass(priority int) string {
	if priority >= sysin.EventPriorityRealtime {
		return "realtime"
	}
	return "history"
}

type sCollector struct {
	queueMu               sync.Mutex
	queueServer           *asynq.Server
	queueBotServer        *asynq.Server
	queueHistoryServer    *asynq.Server
	deliveryServer        *asynq.Server
	deliveryFreshServer   *asynq.Server
	deliveryHistoryServer *asynq.Server
	historySchedulerStop  context.CancelFunc
	deliveryAccountSlots  deliveryAccountSlotPool
	queueClient           *asynq.Client
	queueInspector        *asynq.Inspector
	historyEventBatch     *microbatch.Batcher[historyEventMutation, entity.TgCollectorEvent]
	historyDeliveryBatch  *microbatch.Batcher[sysin.CollectorDelivery, historyDeliverySaved]
	historyBatchSQLMu     sync.Mutex
	runtimeCtx            context.Context
	runtimeStop           context.CancelFunc
	runtimeWG             sync.WaitGroup
}

func init() {
	collectorservice.RegisterCollector(NewCollector())
	gatewayservice.RegisterFeature(&botCollectorFeature{})
}

func NewCollector() *sCollector { return &sCollector{} }

func (s *sCollector) Enabled(ctx context.Context) bool { return collectorEnabled(ctx) }

func (s *sCollector) IngestBotUpdate(ctx context.Context, bot collectorservice.BotContext, update *models.Update) (err error) {
	ctx, observation := startCollectorStage(ctx, collectorStageIngestBot, collectorObservationMeta{
		SourceType: collectorSourceBot, QueueClass: collectorQueueUrgent,
		TenantID: bot.Binding.TenantID, SourceID: bot.Binding.SourceID,
		Mode: collectorModeRealtime, ReceivedAt: collectorIngressReceivedAt(ctx),
	})
	outcome := collectorResultError // Includes panic paths; never report them as accepted.
	defer func() { endCollectorStage(observation, outcome, err) }()
	if message := collectorUpdateMessage(update); message != nil {
		observation.meta.ChatID = strconv.FormatInt(message.Chat.ID, 10)
		observation.meta.MessageID = int64(message.ID)
	}
	ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
	if update == nil {
		return gerror.New("Telegram更新为空")
	}
	if !collectorEnabled(ctx) {
		outcome = collectorResultSkipped
		return nil
	}
	if message := collectorUpdateMessage(update); message != nil && s.isRelayChannel(ctx, bot.Binding.TenantID, fmt.Sprintf("%d", message.Chat.ID)) {
		outcome = collectorResultSkipped
		return nil
	}
	raw, err := json.Marshal(update)
	if err != nil {
		return gerror.Wrap(err, "序列化Telegram采集事件失败")
	}
	event := sysin.RawUpdateEvent{
		EventID:    updateEventID(bot.Key, bot.Binding.TenantID, bot.Binding.SourceID, update.ID, raw),
		TenantID:   bot.Binding.TenantID,
		SourceID:   bot.Binding.SourceID,
		SourceType: sysin.SourceTypeBot,
		BotKey:     bot.Key,
		RawUpdate:  raw,
		UpdateID:   update.ID,
		Priority:   sysin.EventPriorityUrgent,
		ReceivedAt: time.Now(),
	}
	if message := collectorUpdateMessage(update); message != nil {
		event.ChatID = message.Chat.ID
		event.MessageID = int64(message.ID)
	}
	// Stamp after updateEventID: observation must not change business identity.
	event.RawUpdate, _ = collectobserve.StampPayload(ctx, event.RawUpdate, time.Now())
	if !event.ValidateBoundary() {
		return gerror.New("Telegram采集事件归属不完整")
	}
	notice := collectorSystemNotice(event.SourceType, event.RawUpdate)
	if !notice {
		decision := classifyBotIngress(ctx, event.TenantID, event.SourceID, event.ChatID)
		if decision.Defer() {
			event.Priority = sysin.EventPriorityNormal
			logBotIngressDeferral(ctx, event.TenantID, event.SourceID, 0, event.MessageID, decision)
		}
	}
	var spoolID int64
	if collectorIngestSpoolEnabled(ctx) {
		spoolID, err = persistIngestSpool(ctx, &event)
		if err != nil {
			return err
		}
	}
	eventID, err := s.persistEvent(ctx, &event)
	observation.meta.EventID = eventID
	ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
	if err != nil {
		return err
	}
	if notice {
		if err = completeIngestSpool(ctx, spoolID); err != nil {
			return err
		}
		logCollectorSystemNotice(ctx, event.TenantID, event.SourceID, eventID)
		outcome = collectorResultSkipped
		return nil
	}
	if err = s.enqueueEventTask(ctx, eventID, event.Priority); err != nil {
		return gerror.Wrap(err, "Telegram采集事件入队失败")
	}
	if err = completeIngestSpool(ctx, spoolID); err != nil {
		return err
	}
	observeIngestLatency(ctx, &event, "event_enqueued")
	counter, _ := collectorMeter.Int64Counter("telegram_collector_ingest_total")
	counter.Add(ctx, 1, metric.WithAttributes(append(collectorCollectionAttributes(ctx),
		attribute.String("source_type", sysin.SourceTypeBot),
		attribute.String("priority", "urgent"),
	)...))
	outcome = collectorResultAccepted
	return nil
}

func (s *sCollector) IngestAccountMessage(ctx context.Context, message *sysin.AccountMessageEvent) (err error) {
	meta := collectorObservationMeta{SourceType: collectorSourceAccount, ReceivedAt: collectorIngressReceivedAt(ctx)}
	if message != nil {
		meta.TenantID, meta.AccountID, meta.SourceID = message.TenantID, message.AccountID, message.SourceID
		meta.ChatID, meta.MessageID = message.SourceChatID, message.SourceMessageID
		meta.QueueClass = collectorObserveQueueClassFromPriority(accountMessagePriority(message))
		meta.Mode = collectorModeFromHistory(message.History)
		meta.TgAccountID = message.TgAccountID
	}
	ctx, observation := startCollectorStage(ctx, collectorStageIngestAccount, meta)
	outcome := collectorResultError
	defer func() { endCollectorStage(observation, outcome, err) }()
	if message == nil {
		return gerror.New("Telegram账号采集消息为空")
	}
	if !collectorEnabled(ctx) {
		outcome = collectorResultSkipped
		return nil
	}
	if !message.ValidateBoundary() || message.TenantID <= 0 || message.AccountID <= 0 || message.SourceID <= 0 || message.TgAccountID <= 0 {
		return gerror.New("Telegram账号采集消息归属不完整")
	}
	message.SourceChatID = strings.TrimSpace(message.SourceChatID)
	if s.isRelayChannel(ctx, message.TenantID, message.SourceChatID) {
		outcome = collectorResultSkipped
		return nil
	}
	message.SourceUniqueKey = strings.TrimSpace(message.SourceUniqueKey)
	if message.SourceChatID == "" || message.SourceMessageID <= 0 || message.SourceUniqueKey == "" {
		return gerror.New("Telegram账号采集消息标识不完整")
	}
	if message.ReceivedAt.IsZero() {
		message.ReceivedAt = time.Now()
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return gerror.Wrap(err, "序列化Telegram账号采集事件失败")
	}
	raw, _ = collectobserve.StampPayload(ctx, raw, time.Now())
	event := sysin.RawUpdateEvent{
		EventID: message.SourceUniqueKey, TenantID: message.TenantID, SourceID: message.SourceID,
		SourceType: sysin.SourceTypeAccount, AccountID: message.TgAccountID,
		ChatID: parseCollectorChatID(message.SourceChatID), MessageID: message.SourceMessageID,
		Priority: accountMessagePriority(message), RawUpdate: raw, ReceivedAt: message.ReceivedAt,
		TraceID: message.TraceID,
	}
	var spoolID int64
	if collectorIngestSpoolEnabled(ctx) {
		spoolID, err = persistIngestSpool(ctx, &event)
		if err != nil {
			return err
		}
	}
	if eventID, repaired, repairErr := s.refreshCorruptedAccountEvent(ctx, message, raw); repairErr != nil {
		return repairErr
	} else if repaired {
		observation.meta.EventID = eventID
		ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
		if err := s.enqueueEventTask(ctx, eventID, accountMessagePriority(message)); err != nil {
			return err
		}
		if err = completeIngestSpool(ctx, spoolID); err != nil {
			return err
		}
		outcome = collectorResultAccepted
		return nil
	}
	eventID, err := s.persistEvent(ctx, &event)
	observation.meta.EventID = eventID
	ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
	if err != nil {
		return err
	}
	if err = s.enqueueEventTask(ctx, eventID, event.Priority); err != nil {
		return gerror.Wrap(err, "Telegram账号采集事件入队失败")
	}
	if err = completeIngestSpool(ctx, spoolID); err != nil {
		return err
	}
	observeIngestLatency(ctx, &event, "event_enqueued")
	counter, _ := collectorMeter.Int64Counter("telegram_collector_ingest_total")
	counter.Add(ctx, 1, metric.WithAttributes(append(collectorCollectionAttributes(ctx),
		attribute.String("source_type", sysin.SourceTypeAccount),
		attribute.String("priority", "realtime"),
	)...))
	outcome = collectorResultAccepted
	return nil
}

// Relay channels are an internal publishing transport and must never be fed
// back into collection, regardless of which collector bot/account receives it.
func (s *sCollector) isRelayChannel(ctx context.Context, tenantID int64, chatID string) bool {
	chatID = strings.TrimSpace(chatID)
	if tenantID <= 0 || chatID == "" {
		return false
	}
	count, err := g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).
		Where("tenant_id", tenantID).Where("target_chat_id", chatID).
		Where("deleted_at IS NULL").Count()
	return err == nil && count > 0
}

func losslessRawUpdate(raw []byte) *gjson.Json {
	return gjson.NewWithOptions(raw, gjson.Options{StrNumber: true})
}

func (s *sCollector) refreshCorruptedAccountEvent(ctx context.Context, message *sysin.AccountMessageEvent, raw []byte) (int64, bool, error) {
	columns := dao.TgCollectorEvent.Columns()
	existing, err := dao.TgCollectorEvent.Ctx(ctx).
		Fields(columns.Id, columns.RawUpdate).
		Where(columns.TenantId, message.TenantID).
		Where(columns.EventKey, message.SourceUniqueKey).
		One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取Telegram账号采集事件失败")
	}
	if existing.IsEmpty() {
		return 0, false, nil
	}
	eventID := existing[columns.Id].Int64()
	rawUpdate := existing[columns.RawUpdate].String()
	if eventID <= 0 || rawUpdate == "" {
		return 0, false, nil
	}
	var stored sysin.AccountMessageEvent
	if err := json.Unmarshal([]byte(rawUpdate), &stored); err != nil {
		return 0, false, gerror.Wrap(err, "解析Telegram账号历史采集事件失败")
	}
	if !accountMediaIdentityChanged(stored.Media, message.Media) {
		return eventID, false, nil
	}
	// This repair bypasses persistEvent. Register from the ingress metadata
	// before reopening the stored event so a crash cannot hide its recovery lane.
	if err := markCollectorRecoveryScope(ctx, collectorRecoveryScope{message.TenantID, message.TgAccountID, message.SourceID}); err != nil {
		return 0, false, gerror.Wrap(err, "登记账号事件修复恢复 scope 失败")
	}

	now := gtime.Now()
	if _, err := dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).Data(do.TgCollectorEvent{
		RawUpdate:    losslessRawUpdate(raw),
		Status:       sysin.EventStatusReceived,
		AttemptCount: 0,
		NextRunAt:    nil,
		LeaseOwner:   "",
		LeaseUntil:   nil,
		ProcessedAt:  nil,
		ErrorMessage: "",
		UpdatedAt:    now,
	}).Update(); err != nil {
		return 0, false, gerror.Wrap(err, "刷新Telegram账号采集事件媒体身份失败")
	}
	deliveryColumns := dao.TgCollectorDelivery.Columns()
	if _, err := dao.TgCollectorDelivery.Ctx(ctx).
		Where(deliveryColumns.EventId, eventID).
		Data(do.TgCollectorDelivery{
			Status:       sysin.DeliveryStatusPending,
			AttemptCount: 0,
			NextRunAt:    nil,
			LeaseOwner:   "",
			LeaseUntil:   nil,
			ErrorMessage: "",
			UpdatedAt:    now,
		}).Update(); err != nil {
		return 0, false, gerror.Wrap(err, "重置Telegram账号采集交付失败")
	}
	return eventID, true, nil
}

func accountMediaIdentityChanged(stored, incoming []sysin.CollectorMediaItem) bool {
	if len(stored) != len(incoming) {
		return false
	}
	for index := range stored {
		if stored[index].FileID != incoming[index].FileID {
			return false
		}
		if stored[index].SourceMediaID != incoming[index].SourceMediaID ||
			stored[index].SourceAccessHash != incoming[index].SourceAccessHash {
			return true
		}
	}
	return false
}

func (s *sCollector) EventExists(ctx context.Context, tenantID int64, eventKey string) (bool, error) {
	eventKey = strings.TrimSpace(eventKey)
	if tenantID <= 0 || eventKey == "" {
		return false, nil
	}
	columns := dao.TgCollectorEvent.Columns()
	count, err := dao.TgCollectorEvent.Ctx(ctx).
		Where(columns.TenantId, tenantID).
		Where(columns.EventKey, eventKey).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查Telegram采集事件失败")
	}
	return count > 0, nil
}

func parseCollectorChatID(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var chatID int64
	_, _ = fmt.Sscan(value, &chatID)
	return chatID
}

func (s *sCollector) persistEvent(ctx context.Context, event *sysin.RawUpdateEvent) (eventID int64, err error) {
	meta := collectorObservationMeta{}
	if event != nil {
		meta.TenantID, meta.SourceID = event.TenantID, event.SourceID
		meta.SourceType = collectorObserveSourceType(event.SourceType)
		meta.Mode = collectorModeFromRaw(event.SourceType, event.RawUpdate)
		meta.ChatID, meta.MessageID = strconv.FormatInt(event.ChatID, 10), event.MessageID
		meta.QueueClass = collectorObserveQueueClassFromPriority(event.Priority)
		if event.SourceType == sysin.SourceTypeAccount {
			meta.TgAccountID = event.AccountID
		}
	}
	ctx, observation := startCollectorStage(ctx, collectorStageEventPersist, meta)
	outcome := collectorResultError
	defer func() {
		observation.meta.EventID = eventID
		endCollectorStage(observation, outcome, err)
	}()
	if event == nil {
		return 0, gerror.New("Telegram采集事件为空")
	}
	// Register before persistence: a crash after the database commit must still
	// leave the raw Bot/account lane discoverable without a global DB scan.
	notice := collectorSystemNotice(event.SourceType, event.RawUpdate)
	if !notice {
		if err := markCollectorRecoveryScope(ctx, collectorRecoveryScope{event.TenantID, event.AccountID, event.SourceID}); err != nil {
			return 0, gerror.Wrap(err, "持久化前注册原始采集恢复 scope 失败")
		}

	}
	columns := dao.TgCollectorEvent.Columns()
	now := gtime.Now()
	data := do.TgCollectorEvent{
		TenantId:   event.TenantID,
		SourceId:   event.SourceID,
		SourceType: event.SourceType,
		BotKey:     event.BotKey,
		AccountId:  event.AccountID,
		ChatId:     fmt.Sprintf("%d", event.ChatID),
		MessageId:  event.MessageID,
		UpdateId:   event.UpdateID,
		EventKey:   event.EventID,
		RawUpdate:  losslessRawUpdate(event.RawUpdate),
		Priority:   event.Priority,
		Status:     sysin.EventStatusReceived,
		ReceivedAt: gtime.New(event.ReceivedAt),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if notice {
		data.Status = sysin.EventStatusIgnored
		data.ProcessedAt = now
		data.ErrorMessage = collectorSystemNoticeReason
	}
	_, err = dao.TgCollectorEvent.Ctx(ctx).Data(data).OnConflict(columns.TenantId + "," + columns.EventKey).
		OnDuplicate(g.Map{columns.EventKey: conflictIncomingColumn(ctx, columns.EventKey)}).
		Save()
	if err != nil {
		return 0, gerror.Wrap(err, "保存Telegram采集事件失败")
	}
	var row entity.TgCollectorEvent
	if err = dao.TgCollectorEvent.Ctx(ctx).
		Fields(columns.Id, columns.Status).
		Where(columns.TenantId, event.TenantID).
		Where(columns.EventKey, event.EventID).
		Scan(&row); err != nil {
		return 0, gerror.Wrap(err, "读取Telegram采集事件失败")
	}
	if row.Id <= 0 {
		return 0, gerror.New("Telegram采集事件ID无效")
	}
	if notice && row.Status == sysin.EventStatusReceived {
		_, err = dao.TgCollectorEvent.Ctx(ctx).WherePri(row.Id).Where(columns.TenantId, event.TenantID).Where(columns.SourceId, event.SourceID).
			Where(columns.Status, sysin.EventStatusReceived).
			Data(do.TgCollectorEvent{Status: sysin.EventStatusIgnored, ProcessedAt: now, ErrorMessage: collectorSystemNoticeReason, LeaseOwner: "", LeaseUntil: nil, NextRunAt: nil}).Update()
		if err != nil {
			return 0, gerror.Wrap(err, "收口重复接收的BOT系统回执失败")
		}
	}
	outcome = collectorResultAccepted // Upsert accepted; not proof of a newly inserted event.
	return row.Id, nil
}

func collectorUpdateMessage(update *models.Update) *models.Message {
	if update == nil {
		return nil
	}
	switch {
	case update.ChannelPost != nil:
		return update.ChannelPost
	case update.EditedChannelPost != nil:
		return update.EditedChannelPost
	case update.Message != nil:
		return update.Message
	case update.EditedMessage != nil:
		return update.EditedMessage
	default:
		return nil
	}
}

func (s *sCollector) MediaCache(ctx context.Context, fingerprint string) (*sysin.MediaCacheEntry, bool, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return nil, false, nil
	}
	if cache.Initialized() {
		value, err := cache.Instance().Get(ctx, consts.MediaCacheKey(fingerprint))
		if err == nil && value != nil && !value.IsNil() && strings.TrimSpace(value.String()) != "" {
			var entry sysin.MediaCacheEntry
			if err = json.Unmarshal([]byte(value.String()), &entry); err == nil && entry.Status == sysin.MediaStatusReady {
				return &entry, true, nil
			}
		}
	}
	columns := dao.TgCollectorMedia.Columns()
	var rows []*entity.TgCollectorMedia
	if err := dao.TgCollectorMedia.Ctx(ctx).
		Where(columns.TenantId, 0).
		Where(columns.Fingerprint, fingerprint).
		Where(columns.PipelineVersion, mediaPipelineVersion).
		Limit(1).
		Scan(&rows); err != nil {
		return nil, false, gerror.Wrap(err, "读取Telegram媒体索引失败")
	}
	if len(rows) == 0 || rows[0] == nil || rows[0].Id <= 0 {
		return nil, false, nil
	}
	row := rows[0]
	entry := &sysin.MediaCacheEntry{
		Fingerprint:       row.Fingerprint,
		FileURL:           row.FileURL,
		StoragePath:       row.StoragePath,
		PosterURL:         row.PosterURL,
		PosterStoragePath: row.PosterStoragePath,
		PHash:             row.Phash,
		DHash:             row.Dhash,
		Kind:              row.Kind,
		MimeType:          row.MimeType,
		Size:              row.Size,
		PipelineVersion:   row.PipelineVersion,
		Status:            row.Status,
	}
	if row.UpdatedAt != nil {
		entry.UpdatedAt = row.UpdatedAt.Time
	}
	// Only completed results may be cached: a delayed processing read must not
	// overwrite SaveMediaReady's cache entry with a long-lived stale state.
	if cache.Initialized() && entry.Status == sysin.MediaStatusReady {
		if payload, marshalErr := json.Marshal(entry); marshalErr == nil {
			_ = cache.Instance().Set(ctx, consts.MediaCacheKey(fingerprint), string(payload), time.Duration(consts.MediaCacheDefaultHours)*time.Hour)
		}
	}
	return entry, entry.Status == sysin.MediaStatusReady, nil
}

func (s *sCollector) ClaimMediaProcessing(ctx context.Context, fingerprint string, ttl time.Duration) (bool, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return false, gerror.New("媒体指纹为空")
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	columns := dao.TgCollectorMedia.Columns()
	now := gtime.Now()
	_, err := dao.TgCollectorMedia.Ctx(ctx).Data(do.TgCollectorMedia{
		TenantId:        0,
		Fingerprint:     fingerprint,
		PipelineVersion: mediaPipelineVersion,
		Status:          sysin.MediaStatusProcessing,
		CreatedAt:       now,
		UpdatedAt:       now,
	}).OnConflict(columns.TenantId + "," + columns.Fingerprint + "," + columns.PipelineVersion).
		OnDuplicate(g.Map{columns.Fingerprint: conflictIncomingColumn(ctx, columns.Fingerprint)}).
		Save()
	if err != nil {
		return false, gerror.Wrap(err, "初始化Telegram媒体处理索引失败")
	}
	condition := fmt.Sprintf("%s<>? AND (%s IS NULL OR %s<=?)", columns.Status, columns.LeaseUntil, columns.LeaseUntil)
	result, err := dao.TgCollectorMedia.Ctx(ctx).
		Where(columns.TenantId, 0).
		Where(columns.Fingerprint, fingerprint).
		Where(columns.PipelineVersion, mediaPipelineVersion).
		Where(condition, sysin.MediaStatusReady, now).
		Data(g.Map{
			columns.Status:       sysin.MediaStatusProcessing,
			columns.LeaseOwner:   collectorInstanceID(),
			columns.LeaseUntil:   now.Add(ttl),
			columns.AttemptCount: gdb.Raw(columns.AttemptCount + "+1"),
			columns.ErrorMessage: "",
			columns.UpdatedAt:    now,
		}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "领取Telegram媒体处理任务失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	if cache.Initialized() {
		payload, marshalErr := json.Marshal(sysin.MediaCacheEntry{Fingerprint: fingerprint, PipelineVersion: mediaPipelineVersion, Status: sysin.MediaStatusProcessing, UpdatedAt: time.Now()})
		if marshalErr == nil {
			_ = cache.Instance().Set(ctx, consts.MediaLockKey(fingerprint), string(payload), ttl)
		}
	}
	return true, nil
}

func (s *sCollector) SaveMediaReady(ctx context.Context, entry *sysin.MediaCacheEntry, ttl time.Duration) error {
	if entry == nil || strings.TrimSpace(entry.Fingerprint) == "" {
		return gerror.New("媒体缓存结果无效")
	}
	entry.Status = sysin.MediaStatusReady
	if strings.TrimSpace(entry.PipelineVersion) == "" {
		entry.PipelineVersion = mediaPipelineVersion
	}
	entry.UpdatedAt = time.Now()
	if ttl <= 0 {
		ttl = time.Duration(consts.MediaCacheDefaultHours) * time.Hour
	}
	columns := dao.TgCollectorMedia.Columns()
	now := gtime.Now()
	_, err := dao.TgCollectorMedia.Ctx(ctx).Data(do.TgCollectorMedia{
		TenantId:          0,
		Fingerprint:       entry.Fingerprint,
		Kind:              entry.Kind,
		MimeType:          entry.MimeType,
		Size:              entry.Size,
		PipelineVersion:   entry.PipelineVersion,
		Status:            sysin.MediaStatusReady,
		FileURL:           entry.FileURL,
		StoragePath:       entry.StoragePath,
		PosterURL:         entry.PosterURL,
		PosterStoragePath: entry.PosterStoragePath,
		Phash:             entry.PHash,
		Dhash:             entry.DHash,
		LeaseOwner:        "",
		LeaseUntil:        nil,
		ErrorMessage:      "",
		CreatedAt:         now,
		UpdatedAt:         now,
	}).OnConflict(columns.TenantId+","+columns.Fingerprint+","+columns.PipelineVersion).
		OnDuplicateEx(columns.Id, columns.TenantId, columns.Fingerprint, columns.PipelineVersion, columns.CreatedAt).
		Save()
	if err != nil {
		return gerror.Wrap(err, "保存Telegram媒体索引失败")
	}
	_, err = dao.TgCollectorMedia.Ctx(ctx).
		Where(columns.TenantId, 0).
		Where(columns.Fingerprint, entry.Fingerprint).
		Where(columns.PipelineVersion, entry.PipelineVersion).
		Data(g.Map{
			columns.Kind:              entry.Kind,
			columns.MimeType:          entry.MimeType,
			columns.Size:              entry.Size,
			columns.Status:            sysin.MediaStatusReady,
			columns.FileURL:           entry.FileURL,
			columns.StoragePath:       entry.StoragePath,
			columns.PosterURL:         entry.PosterURL,
			columns.PosterStoragePath: entry.PosterStoragePath,
			columns.Phash:             entry.PHash,
			columns.Dhash:             entry.DHash,
			columns.LeaseOwner:        "",
			columns.LeaseUntil:        nil,
			columns.ErrorMessage:      "",
			columns.UpdatedAt:         now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新Telegram媒体索引失败")
	}
	if cache.Initialized() {
		payload, marshalErr := json.Marshal(entry)
		if marshalErr == nil {
			if cacheErr := cache.Instance().Set(ctx, consts.MediaCacheKey(entry.Fingerprint), string(payload), ttl); cacheErr != nil {
				g.Log().Warningf(ctx, "刷新Telegram媒体缓存失败 fingerprint:%s err:%+v", entry.Fingerprint, cacheErr)
			}
		}
		_, _ = cache.Instance().Remove(ctx, consts.MediaLockKey(entry.Fingerprint))
	}
	return nil
}

func (s *sCollector) ReleaseMediaProcessing(ctx context.Context, fingerprint string, cause error) error {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return nil
	}
	columns := dao.TgCollectorMedia.Columns()
	errorMessage := "媒体处理已释放，等待重试"
	if cause != nil {
		errorMessage = cause.Error()
	}
	_, err := dao.TgCollectorMedia.Ctx(ctx).
		Where(columns.TenantId, 0).
		Where(columns.Fingerprint, fingerprint).
		Where(columns.PipelineVersion, mediaPipelineVersion).
		Where(columns.Status, sysin.MediaStatusProcessing).
		Data(g.Map{
			columns.Status:       sysin.MediaStatusFailed,
			columns.NextRunAt:    gtime.Now().Add(15 * time.Second),
			columns.LeaseOwner:   "",
			columns.LeaseUntil:   nil,
			columns.ErrorMessage: errorMessage,
			columns.UpdatedAt:    gtime.Now(),
		}).Update()
	if cache.Initialized() {
		_, _ = cache.Instance().Remove(ctx, consts.MediaLockKey(fingerprint), consts.MediaCacheKey(fingerprint))
	}
	return err
}

func collectorEnabled(ctx context.Context) bool {
	return g.Cfg().MustGet(ctx, "telegramCollector.enabled", true).Bool()
}

func conflictIncomingColumn(ctx context.Context, column string) gdb.Raw {
	if strings.EqualFold(strings.TrimSpace(g.DB().GetConfig().Type), "pgsql") {
		return gdb.Raw(`EXCLUDED."` + column + `"`)
	}
	return gdb.Raw("VALUES(`" + column + "`)")
}

func BuildMediaFingerprint(md5 string, size int64, kind, mimeType string) string {
	return collectorservice.BuildMediaFingerprint(md5, size, kind, mimeType)
}

func BuildMediaIdentityFingerprint(fileUniqueID string, mediaID int64, dcID int, kind, variant string) string {
	return collectorservice.BuildMediaIdentityFingerprint(fileUniqueID, mediaID, dcID, kind, variant)
}

func updateEventID(botKey string, tenantID, sourceID, updateID int64, raw []byte) string {
	scope := fmt.Sprintf("source:%d", sourceID)
	if updateID > 0 {
		return fmt.Sprintf("bot:%s:tenant:%d:%s:update:%d", botKey, tenantID, scope, updateID)
	}
	hash := sha256.Sum256(raw)
	return "bot:" + botKey + ":tenant:" + fmt.Sprintf("%d", tenantID) + ":" + scope + ":hash:" + hex.EncodeToString(hash[:])
}

type botCollectorFeature struct{}

type botCollectionSource struct {
	ID       int64
	TenantID int64
}

func matchBotCollectionSources(sources []botCollectionSource, tenantID, botID int64) []botCollectionSource {
	matched := make([]botCollectionSource, 0, len(sources))
	seen := make(map[int64]struct{}, len(sources))
	for _, source := range sources {
		// Gateway 可能只知道 BOT 身份而没有租户上下文；同一个 BOT
		// 可以被多个租户绑定，此时按来源自身租户拆分，不能用 tenant=0 过滤掉全部规则。
		if (tenantID > 0 && source.TenantID != tenantID) || source.TenantID <= 0 || source.ID <= 0 || botID <= 0 {
			continue
		}
		if _, exists := seen[source.ID]; exists {
			continue
		}
		seen[source.ID] = struct{}{}
		matched = append(matched, source)
	}
	return matched
}

func ingestBotCollectionSources(ctx context.Context, sources []botCollectionSource, ingest func(botCollectionSource) error) error {
	if len(sources) == 0 {
		return ctx.Err()
	}
	// Bound workers, not just concurrent callbacks: a Bot with many sources
	// must not create a goroutine per source. Each input index has one owner.
	const concurrency = 4
	workers := concurrency
	if len(sources) < workers {
		workers = len(sources)
	}
	var (
		mu           sync.Mutex
		wg           sync.WaitGroup
		next         int
		ingestErrors = make([]error, len(sources))
	)
	invoke := func(source botCollectionSource) (ingestErr error) {
		returned := false
		defer func() {
			if !returned {
				value := recover()
				if panicErr, ok := value.(error); ok {
					ingestErr = fmt.Errorf("采集来源处理发生panic: %w", panicErr)
				} else {
					// Avoid copying arbitrary panic payloads into logs/errors.
					ingestErr = fmt.Errorf("采集来源处理发生panic，类型: %T", value)
				}
			}
		}()
		ingestErr = ingest(source)
		returned = true
		return ingestErr
	}
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				index := next
				if index >= len(sources) {
					mu.Unlock()
					return
				}
				next++
				mu.Unlock()
				source := sources[index]
				// Only shared caller cancellation stops unstarted sources.
				// A per-source error or recovered panic never cancels siblings.
				ingestErr := ctx.Err()
				if ingestErr == nil {
					ingestErr = invoke(source)
				}
				if ingestErr != nil {
					ingestErrors[index] = fmt.Errorf("采集source %d接收Telegram更新失败: %w", source.ID, ingestErr)
				}
			}
		}()
	}
	wg.Wait()
	// Callback completion order is intentionally independent; error ordering
	// remains deterministic in the original source order.
	combined := errors.Join(ingestErrors...)
	if cancelErr := ctx.Err(); cancelErr != nil && !errors.Is(combined, cancelErr) {
		combined = errors.Join(combined, cancelErr)
	}
	return combined
}

// Only accept sources already matched within their own gateway binding.
// Dedupe across bindings by tenant/source without widening authorization.
func appendMatchedBotCollectionSources(targets, matched []botCollectionSource, seen map[[2]int64]struct{}) []botCollectionSource {
	for _, source := range matched {
		key := [2]int64{source.TenantID, source.ID}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, source)
	}
	return targets
}

func (f *botCollectorFeature) Key() string { return gatewayOwner }

func (f *botCollectorFeature) Priority() int { return 10 }

func (f *botCollectorFeature) Menus(context.Context, gatewayservice.BotContext) (gatewayservice.FeatureMenus, error) {
	return gatewayservice.FeatureMenus{}, nil
}

func (f *botCollectorFeature) HandleUpdate(ctx context.Context, bot gatewayservice.BotContext, update *models.Update) (bool, error) {
	// Channel membership and other non-message updates are maintained by the
	// channel discovery feature. They are not profile collection input.
	if collectorUpdateMessage(update) == nil {
		return false, nil
	}
	if !collectorEnabled(ctx) {
		return false, nil
	}
	var ingestErrors []error
	var targets []botCollectionSource
	seenTargets := make(map[[2]int64]struct{})
	// Observation only: retain the same first matched binding as target dedupe.
	// Gateway ReferenceID is the publish Bot PK; collector Reference is a source.
	targetBotIDs := make(map[[2]int64]int64)
	for _, binding := range bot.Bindings {
		// 采集只消费发布模块提供的规范 Bot 绑定，避免混用入口 Bot 表的 ID。
		if binding.Owner != "youban_publish" {
			continue
		}
		// Gateway bindings identify the BOT. Resolve all enabled collection
		// sources for that BOT here, so private chats, groups, and channels use
		// the same source routing and never confuse bot_id with source_id.
		var sources []botCollectionSource
		// publish provider 的 ReferenceID 已经是统一主表 hg_youban_publish_bot.id。
		publishBotID := binding.ReferenceID
		botIDs := []int64{publishBotID}
		model := g.DB().Model("hg_youban_publish_collect_source").Safe().Ctx(ctx).
			Fields("id,tenant_id").WhereIn("bot_id", botIDs).Where("source_type", "bot").
			Where("collect_enabled", 1).Where("status", 1).WhereNull("deleted_at")
		if err := model.Scan(&sources); err != nil {
			ingestErrors = append(ingestErrors, fmt.Errorf("读取TG Bot采集源失败 tenant:%d bot:%d: %w", binding.TenantID, binding.ReferenceID, err))
			continue
		}
		// 部分历史配置把入口 BOT 写在来源频道绑定中，bot_id 可能为空或指向旧 Bot。
		// 当按 BOT 身份没有命中时，再按消息所在频道补偿匹配，避免消息已落原始表却完全丢失采集分发。
		if len(sources) == 0 {
			if message := collectorUpdateMessage(update); message != nil && message.Chat.ID != 0 {
				chatID := fmt.Sprintf("%d", message.Chat.ID)
				chatVariants := []string{chatID}
				if strings.HasPrefix(chatID, "-100") {
					chatVariants = append(chatVariants, strings.TrimPrefix(chatID, "-100"))
				} else if strings.HasPrefix(chatID, "-") {
					chatVariants = append(chatVariants, "-100"+strings.TrimPrefix(chatID, "-"))
				}
				fallbackModel := g.DB().Model("hg_youban_publish_collect_source").Safe().Ctx(ctx).
					Fields("id,tenant_id").Where("source_type", "bot").Where("collect_enabled", 1).
					Where("status", 1).WhereIn("source_chat_id", chatVariants).WhereNull("deleted_at")
				if err := fallbackModel.Scan(&sources); err != nil {
					ingestErrors = append(ingestErrors, fmt.Errorf("按来源频道读取TG Bot采集源失败 chat:%s: %w", chatID, err))
					continue
				}
			}
		}
		matchedSources := matchBotCollectionSources(sources, binding.TenantID, publishBotID)
		if len(matchedSources) == 0 {
			g.Log().Warningf(ctx, "TG Bot采集更新未匹配到采集源 tenantId:%d publishBotId:%d gatewayBotId:%d sourceCount:%d", binding.TenantID, publishBotID, binding.ReferenceID, len(sources))
		}
		for _, source := range matchedSources {
			key := [2]int64{source.TenantID, source.ID}
			if _, exists := targetBotIDs[key]; !exists {
				targetBotIDs[key] = publishBotID
			}
		}
		targets = appendMatchedBotCollectionSources(targets, matchedSources, seenTargets)
	}
	// Schedule all authorized bindings together: a slow source from the first
	// tenant must not prevent later tenants from reaching an available worker.
	if err := ingestBotCollectionSources(ctx, targets, func(source botCollectionSource) error {
		// All writes finish before bounded workers start; callbacks only read.
		ingestCtx, _ := collectorObservationContext(ctx, collectorObservationMeta{
			TenantID: source.TenantID, SourceID: source.ID,
			BotID: targetBotIDs[[2]int64{source.TenantID, source.ID}],
		})
		return collectorservice.Collector().IngestBotUpdate(ingestCtx, collectorservice.BotContext{
			Key: bot.Key, Token: bot.Token,
			Binding: collectorservice.BotBinding{TenantID: source.TenantID, SourceID: source.ID, Reference: fmt.Sprintf("source:%d", source.ID)},
		}, update)
	}); err != nil {
		ingestErrors = append(ingestErrors, err)
	}
	// Collector is an additional gateway consumer. Returning false allows the
	// existing provider features to process the same update as before.
	return false, errors.Join(ingestErrors...)
}
