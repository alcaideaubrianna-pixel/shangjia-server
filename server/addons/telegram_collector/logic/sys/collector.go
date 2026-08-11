package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
)

const gatewayOwner = "telegram_collector"

const mediaPipelineVersion = "v1"

var collectorMeter = otel.Meter("hotgo/addons/telegram_collector")

type sCollector struct {
	queueMu        sync.Mutex
	queueServer    *asynq.Server
	deliveryServer *asynq.Server
	queueClient    *asynq.Client
	runtimeCtx     context.Context
	runtimeStop    context.CancelFunc
	runtimeWG      sync.WaitGroup
}

func init() {
	collectorservice.RegisterCollector(NewCollector())
	gatewayservice.RegisterFeature(&botCollectorFeature{})
}

func NewCollector() *sCollector { return &sCollector{} }

func (s *sCollector) Enabled(ctx context.Context) bool { return collectorEnabled(ctx) }

func (s *sCollector) IngestBotUpdate(ctx context.Context, bot collectorservice.BotContext, update *models.Update) error {
	if update == nil {
		return gerror.New("Telegram更新为空")
	}
	if !collectorEnabled(ctx) {
		return nil
	}
	raw, err := json.Marshal(update)
	if err != nil {
		return gerror.Wrap(err, "序列化Telegram采集事件失败")
	}
	event := sysin.RawUpdateEvent{
		EventID:    updateEventID(bot.Key, bot.Binding.TenantID, update.ID, raw),
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
	eventID, err := s.persistEvent(ctx, &event)
	if err != nil {
		return err
	}
	if err = s.enqueueEventTask(ctx, sysin.EventTask{EventID: eventID, EventKey: event.EventID, Priority: event.Priority}); err != nil {
		return gerror.Wrap(err, "Telegram采集事件入队失败")
	}
	counter, _ := collectorMeter.Int64Counter("telegram_collector_ingest_total")
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("source_type", sysin.SourceTypeBot),
		attribute.String("priority", "urgent"),
	))
	return nil
}

func (s *sCollector) IngestAccountMessage(ctx context.Context, message *sysin.AccountMessageEvent) error {
	if message == nil {
		return gerror.New("Telegram账号采集消息为空")
	}
	if !collectorEnabled(ctx) {
		return nil
	}
	if message.TenantID <= 0 || message.AccountID <= 0 || message.SourceID <= 0 || message.TgAccountID <= 0 {
		return gerror.New("Telegram账号采集消息归属不完整")
	}
	message.SourceChatID = strings.TrimSpace(message.SourceChatID)
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
	event := sysin.RawUpdateEvent{
		EventID:    message.SourceUniqueKey,
		TenantID:   message.TenantID,
		SourceID:   message.SourceID,
		SourceType: sysin.SourceTypeAccount,
		AccountID:  message.TgAccountID,
		ChatID:     parseCollectorChatID(message.SourceChatID),
		MessageID:  message.SourceMessageID,
		Priority:   sysin.EventPriorityRealtime,
		RawUpdate:  raw,
		ReceivedAt: message.ReceivedAt,
		TraceID:    message.TraceID,
	}
	eventID, err := s.persistEvent(ctx, &event)
	if err != nil {
		return err
	}
	if err = s.enqueueEventTask(ctx, sysin.EventTask{EventID: eventID, EventKey: event.EventID, Priority: event.Priority}); err != nil {
		return gerror.Wrap(err, "Telegram账号采集事件入队失败")
	}
	counter, _ := collectorMeter.Int64Counter("telegram_collector_ingest_total")
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("source_type", sysin.SourceTypeAccount),
		attribute.String("priority", "realtime"),
	))
	return nil
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

func (s *sCollector) persistEvent(ctx context.Context, event *sysin.RawUpdateEvent) (int64, error) {
	if event == nil {
		return 0, gerror.New("Telegram采集事件为空")
	}
	columns := dao.TgCollectorEvent.Columns()
	now := gtime.Now()
	_, err := dao.TgCollectorEvent.Ctx(ctx).Data(do.TgCollectorEvent{
		TenantId:   event.TenantID,
		SourceId:   event.SourceID,
		SourceType: event.SourceType,
		BotKey:     event.BotKey,
		AccountId:  event.AccountID,
		ChatId:     fmt.Sprintf("%d", event.ChatID),
		MessageId:  event.MessageID,
		UpdateId:   event.UpdateID,
		EventKey:   event.EventID,
		RawUpdate:  gjson.New(event.RawUpdate),
		Priority:   event.Priority,
		Status:     sysin.EventStatusReceived,
		ReceivedAt: gtime.New(event.ReceivedAt),
		CreatedAt:  now,
		UpdatedAt:  now,
	}).OnConflict(columns.TenantId + "," + columns.EventKey).
		OnDuplicate(g.Map{columns.EventKey: conflictIncomingColumn(ctx, columns.EventKey)}).
		Save()
	if err != nil {
		return 0, gerror.Wrap(err, "保存Telegram采集事件失败")
	}
	var row entity.TgCollectorEvent
	if err = dao.TgCollectorEvent.Ctx(ctx).
		Fields(columns.Id).
		Where(columns.TenantId, event.TenantID).
		Where(columns.EventKey, event.EventID).
		Scan(&row); err != nil {
		return 0, gerror.Wrap(err, "读取Telegram采集事件失败")
	}
	if row.Id <= 0 {
		return 0, gerror.New("Telegram采集事件ID无效")
	}
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
			if err = json.Unmarshal([]byte(value.String()), &entry); err == nil {
				return &entry, entry.Status == sysin.MediaStatusReady, nil
			}
		}
	}
	columns := dao.TgCollectorMedia.Columns()
	var row entity.TgCollectorMedia
	if err := dao.TgCollectorMedia.Ctx(ctx).
		Where(columns.TenantId, 0).
		Where(columns.Fingerprint, fingerprint).
		Where(columns.PipelineVersion, mediaPipelineVersion).
		Scan(&row); err != nil {
		return nil, false, gerror.Wrap(err, "读取Telegram媒体索引失败")
	}
	if row.Id <= 0 {
		return nil, false, nil
	}
	entry := &sysin.MediaCacheEntry{
		Fingerprint:       row.Fingerprint,
		StoragePath:       row.StoragePath,
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
	if cache.Initialized() {
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
		StoragePath:       entry.StoragePath,
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
			columns.StoragePath:       entry.StoragePath,
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

func updateEventID(botKey string, tenantID, updateID int64, raw []byte) string {
	if updateID > 0 {
		return fmt.Sprintf("bot:%s:tenant:%d:update:%d", botKey, tenantID, updateID)
	}
	hash := sha256.Sum256(raw)
	return "bot:" + botKey + ":tenant:" + fmt.Sprintf("%d", tenantID) + ":hash:" + hex.EncodeToString(hash[:])
}

type botCollectorFeature struct{}

func (f *botCollectorFeature) Key() string { return gatewayOwner }

func (f *botCollectorFeature) Priority() int { return 10 }

func (f *botCollectorFeature) Menus(context.Context, gatewayservice.BotContext) (gatewayservice.FeatureMenus, error) {
	return gatewayservice.FeatureMenus{}, nil
}

func (f *botCollectorFeature) HandleUpdate(ctx context.Context, bot gatewayservice.BotContext, update *models.Update) (bool, error) {
	if !collectorEnabled(ctx) {
		return false, nil
	}
	for _, binding := range bot.Bindings {
		if binding.Owner != gatewayOwner && binding.Owner != "youban_publish" {
			continue
		}
		if err := collectorservice.Collector().IngestBotUpdate(ctx, collectorservice.BotContext{
			Key:   bot.Key,
			Token: bot.Token,
			Binding: collectorservice.BotBinding{
				TenantID:  binding.TenantID,
				SourceID:  binding.ReferenceID,
				Reference: fmt.Sprintf("bot:%d", binding.ReferenceID),
			},
		}, update); err != nil {
			return false, err
		}
	}
	// Collector is an additional gateway consumer. Returning false allows the
	// existing provider features to process the same update as before.
	return false, nil
}
