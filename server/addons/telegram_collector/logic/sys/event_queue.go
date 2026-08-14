package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/do"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

const eventTaskType = "telegram_collector:event"

const (
	eventLeaseDuration     = 2 * time.Minute
	eventRecoveryInterval  = 30 * time.Second
	eventRecoveryStaleTime = 15 * time.Second
	eventTaskUniqueTTL     = 5 * time.Minute
	eventMaxAttempts       = 5
)

func (s *sCollector) StartRuntime(ctx context.Context) {
	if !collectorEnabled(ctx) {
		g.Log().Info(ctx, "Telegram采集插件处于关闭状态，Collector Worker不会消费新事件")
		return
	}
	s.queueMu.Lock()
	if s.queueServer != nil {
		s.queueMu.Unlock()
		return
	}
	runtimeCtx := s.ensureRuntimeContext(ctx)
	server := asynq.NewServer(collectorRedisOption(runtimeCtx), asynq.Config{
		Concurrency: collectorConcurrency(ctx),
		Queues: map[string]int{
			consts.QueueCollectUrgent:   8,
			consts.QueueCollectRealtime: 4,
			consts.QueueCollectHistory:  1,
		},
	})
	if s.queueClient == nil {
		s.queueClient = asynq.NewClient(collectorRedisOption(runtimeCtx))
	}
	s.queueServer = server
	s.queueMu.Unlock()
	mux := asynq.NewServeMux()
	mux.HandleFunc(eventTaskType, s.handleEventTask)
	g.Log().Infof(runtimeCtx, "启动Telegram Collector Worker concurrency:%d", collectorConcurrency(runtimeCtx))
	s.runtimeWG.Add(2)
	go func() {
		defer s.runtimeWG.Done()
		if err := server.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(runtimeCtx, "Telegram Collector Worker停止：%+v", err)
		}
	}()
	go func() {
		defer s.runtimeWG.Done()
		s.runEventRecovery(runtimeCtx)
	}()
}

func (s *sCollector) StopRuntime() {
	s.queueMu.Lock()
	server, deliveryServer, client, runtimeStop := s.queueServer, s.deliveryServer, s.queueClient, s.runtimeStop
	s.queueServer, s.deliveryServer, s.queueClient, s.runtimeCtx, s.runtimeStop = nil, nil, nil, nil, nil
	s.queueMu.Unlock()
	if runtimeStop != nil {
		runtimeStop()
	}
	if server != nil {
		server.Shutdown()
	}
	if deliveryServer != nil {
		deliveryServer.Shutdown()
	}
	if client != nil {
		_ = client.Close()
	}
	s.runtimeWG.Wait()
}

func (s *sCollector) ensureRuntimeContext(ctx context.Context) context.Context {
	if s.runtimeCtx != nil {
		return s.runtimeCtx
	}
	s.runtimeCtx, s.runtimeStop = context.WithCancel(ctx)
	return s.runtimeCtx
}

func (s *sCollector) enqueueEventTask(ctx context.Context, eventID int64, priority int) error {
	if eventID <= 0 {
		return gerror.New("Telegram采集事件任务ID无效")
	}
	queueName := consts.QueueCollectRealtime
	if priority >= sysin.EventPriorityUrgent {
		queueName = consts.QueueCollectUrgent
	}
	s.queueMu.Lock()
	if s.queueClient == nil {
		s.queueClient = asynq.NewClient(collectorRedisOption(ctx))
	}
	client := s.queueClient
	s.queueMu.Unlock()
	_, err := client.EnqueueContext(ctx, asynq.NewTask(eventTaskType, collectorTaskPayload(eventID)),
		asynq.Queue(queueName),
		asynq.MaxRetry(eventMaxAttempts-1),
		asynq.Timeout(2*time.Minute),
		asynq.Unique(eventTaskUniqueTTL),
	)
	if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func (s *sCollector) handleEventTask(ctx context.Context, task *asynq.Task) error {
	eventID, err := parseCollectorTaskID(task.Payload(), "事件")
	if err != nil {
		return err
	}
	startedAt := time.Now()
	if err = s.processEvent(ctx, eventID); err != nil {
		counter, _ := collectorMeter.Int64Counter("telegram_collector_event_total")
		counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "failed")))
		return err
	}
	histogram, _ := collectorMeter.Float64Histogram("telegram_collector_event_duration_seconds")
	histogram.Record(ctx, time.Since(startedAt).Seconds(), metric.WithAttributes(attribute.String("result", "ready")))
	return nil
}

func (s *sCollector) processEvent(ctx context.Context, eventID int64) error {
	if eventID <= 0 {
		return gerror.New("Telegram采集事件任务无效")
	}
	columns := dao.TgCollectorEvent.Columns()
	now := gtime.Now()
	result, err := dao.TgCollectorEvent.Ctx(ctx).
		Where(columns.Id, eventID).
		WhereIn(columns.Status, []string{sysin.EventStatusReceived, sysin.EventStatusFailedRetry}).
		Data(g.Map{
			columns.Status:       sysin.EventStatusProcessing,
			columns.LeaseOwner:   collectorInstanceID(),
			columns.LeaseUntil:   now.Add(eventLeaseDuration),
			columns.AttemptCount: gdb.Raw(columns.AttemptCount + "+1"),
			columns.UpdatedAt:    now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "领取Telegram采集事件失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	var row entity.TgCollectorEvent
	if err = dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).Scan(&row); err != nil {
		return s.failEvent(ctx, eventID, err)
	}
	if row.RawUpdate == nil {
		return s.failEvent(ctx, eventID, gerror.New("Telegram原始事件为空"))
	}
	raw := []byte(row.RawUpdate.String())
	delivery, err := collectorDeliveryFromEvent(&row, raw)
	if err != nil {
		return s.failEvent(ctx, eventID, err)
	}
	deliveryID, err := s.saveDelivery(ctx, &delivery)
	if err != nil {
		return s.failEvent(ctx, eventID, err)
	}
	delivery.ID = deliveryID
	if err = s.enqueueDeliveryTask(ctx, deliveryID, row.Priority); err != nil {
		return s.failEvent(ctx, eventID, err)
	}
	_, err = dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).Data(g.Map{
		columns.Status:       sysin.EventStatusReady,
		columns.ProcessedAt:  gtime.Now(),
		columns.LeaseOwner:   "",
		columns.LeaseUntil:   nil,
		columns.ErrorMessage: "",
		columns.UpdatedAt:    gtime.Now(),
	}).Update()
	return err
}

func collectorDeliveryFromEvent(row *entity.TgCollectorEvent, raw []byte) (sysin.CollectorDelivery, error) {
	if row == nil {
		return sysin.CollectorDelivery{}, gerror.New("Telegram采集事件为空")
	}
	delivery := sysin.CollectorDelivery{
		DeliveryKey:     fmt.Sprintf("event:%d", row.Id),
		TenantID:        row.TenantId,
		EventID:         row.Id,
		SourceID:        row.SourceId,
		SourceType:      row.SourceType,
		SourceChatID:    row.ChatId,
		SourceMessageID: row.MessageId,
		RawUpdate:       raw,
	}
	switch row.SourceType {
	case sysin.SourceTypeBot:
		var update models.Update
		if err := json.Unmarshal(raw, &update); err != nil {
			return sysin.CollectorDelivery{}, gerror.Wrap(err, "解析Telegram Bot原始事件失败")
		}
		message := collectorUpdateMessage(&update)
		if message != nil {
			delivery.RawText = strings.TrimSpace(message.Text)
			if delivery.RawText == "" {
				delivery.RawText = strings.TrimSpace(message.Caption)
			}
			delivery.SourceChatID = strconv.FormatInt(message.Chat.ID, 10)
			delivery.SourceMessageID = int64(message.ID)
			delivery.SourceGroupedID = strings.TrimSpace(message.MediaGroupID)
			delivery.SourceUniqueKey = collectorBotMessageKey(row.SourceId, delivery.SourceChatID, delivery.SourceMessageID, delivery.SourceGroupedID)
			delivery.Media = collectorBotMediaItems(message)
			if message.Date > 0 {
				delivery.ReceivedAt = time.Unix(int64(message.Date), 0)
			}
		}
	case sysin.SourceTypeAccount:
		var message sysin.AccountMessageEvent
		if err := json.Unmarshal(raw, &message); err != nil {
			return sysin.CollectorDelivery{}, gerror.Wrap(err, "解析Telegram账号原始事件失败")
		}
		delivery.AccountID = message.AccountID
		delivery.TgAccountID = message.TgAccountID
		delivery.SourceChatID = message.SourceChatID
		delivery.SourceMessageID = message.SourceMessageID
		delivery.SourceGroupedID = message.SourceGroupedID
		delivery.SourceUniqueKey = message.SourceUniqueKey
		delivery.RawText = strings.TrimSpace(message.RawText)
		delivery.Media = append([]sysin.CollectorMediaItem(nil), message.Media...)
		delivery.ReceivedAt = message.ReceivedAt
	default:
		return sysin.CollectorDelivery{}, gerror.Newf("不支持的Telegram采集来源类型：%s", row.SourceType)
	}
	return delivery, nil
}

func collectorBotMessageKey(sourceID int64, chatID string, messageID int64, groupedID string) string {
	if groupedID = strings.TrimSpace(groupedID); groupedID != "" {
		return fmt.Sprintf("bot:%d:%s:group:%s", sourceID, chatID, groupedID)
	}
	return fmt.Sprintf("bot:%d:%s:message:%d", sourceID, chatID, messageID)
}

func collectorBotMediaItems(message *models.Message) []sysin.CollectorMediaItem {
	items := make([]sysin.CollectorMediaItem, 0, 2)
	if len(message.Photo) > 0 {
		photo := message.Photo[len(message.Photo)-1]
		items = append(items, sysin.CollectorMediaItem{Type: "photo", FileID: photo.FileID})
	}
	if message.Video != nil {
		items = append(items, sysin.CollectorMediaItem{Type: "video", FileID: message.Video.FileID})
	}
	if message.Document != nil {
		items = append(items, sysin.CollectorMediaItem{Type: "document", FileID: message.Document.FileID})
	}
	return items
}

func (s *sCollector) saveDelivery(ctx context.Context, delivery *sysin.CollectorDelivery) (int64, error) {
	columns := dao.TgCollectorDelivery.Columns()
	now := gtime.Now()
	_, err := dao.TgCollectorDelivery.Ctx(ctx).Data(do.TgCollectorDelivery{
		TenantId:    delivery.TenantID,
		EventId:     delivery.EventID,
		DeliveryKey: delivery.DeliveryKey,
		Status:      sysin.DeliveryStatusPending,
		Priority:    deliveryPriority(delivery),
		CreatedAt:   now,
		UpdatedAt:   now,
	}).OnConflict(columns.TenantId + "," + columns.DeliveryKey).
		OnDuplicate(g.Map{columns.DeliveryKey: conflictIncomingColumn(ctx, columns.DeliveryKey)}).
		Save()
	if err != nil {
		return 0, gerror.Wrap(err, "保存Telegram采集交付失败")
	}
	var row entity.TgCollectorDelivery
	if err = dao.TgCollectorDelivery.Ctx(ctx).
		Fields(columns.Id).
		Where(columns.TenantId, delivery.TenantID).
		Where(columns.DeliveryKey, delivery.DeliveryKey).
		Scan(&row); err != nil {
		return 0, gerror.Wrap(err, "读取Telegram采集交付失败")
	}
	if row.Id <= 0 {
		return 0, gerror.New("Telegram采集交付ID无效")
	}
	return row.Id, nil
}

func deliveryPriority(delivery *sysin.CollectorDelivery) int {
	if delivery == nil {
		return sysin.EventPriorityNormal
	}
	if delivery.SourceType == sysin.SourceTypeAccount {
		return sysin.EventPriorityRealtime
	}
	return sysin.EventPriorityUrgent
}

func (s *sCollector) failEvent(ctx context.Context, eventID int64, cause error) error {
	columns := dao.TgCollectorEvent.Columns()
	var row entity.TgCollectorEvent
	if err := dao.TgCollectorEvent.Ctx(ctx).Fields(columns.AttemptCount).WherePri(eventID).Scan(&row); err != nil {
		return gerror.Wrap(err, "读取Telegram采集事件重试次数失败")
	}
	status := sysin.EventStatusFailedRetry
	var nextRunAt any = gtime.Now().Add(30 * time.Second)
	if row.AttemptCount >= eventMaxAttempts {
		status = sysin.EventStatusDead
		nextRunAt = nil
	}
	_, _ = dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).Data(g.Map{
		columns.Status:       status,
		columns.NextRunAt:    nextRunAt,
		columns.LeaseOwner:   "",
		columns.LeaseUntil:   nil,
		columns.ErrorMessage: cause.Error(),
		columns.UpdatedAt:    gtime.Now(),
	}).Update()
	if status == sysin.EventStatusDead {
		return fmt.Errorf("%w: Telegram采集事件超过最大重试次数: %v", asynq.SkipRetry, cause)
	}
	return cause
}

func (s *sCollector) runEventRecovery(ctx context.Context) {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	s.recoverEventTasks(ctx)
	ticker := time.NewTicker(eventRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverEventTasks(ctx)
		}
	}
}

func (s *sCollector) recoverEventTasks(ctx context.Context) {
	if err := s.resetExpiredEventLeases(ctx, collectorRecoveryBatchSize(ctx)); err != nil {
		g.Log().Warningf(ctx, "恢复超时Telegram采集事件失败：%+v", err)
	}
	if err := s.enqueueDueEventTasks(ctx, collectorRecoveryBatchSize(ctx)); err != nil {
		g.Log().Warningf(ctx, "重新投递Telegram采集事件失败：%+v", err)
	}
}

func (s *sCollector) resetExpiredEventLeases(ctx context.Context, limit int) error {
	columns := dao.TgCollectorEvent.Columns()
	now := gtime.Now()
	rows, err := dao.TgCollectorEvent.Ctx(ctx).
		Fields(columns.Id, columns.AttemptCount).
		Where(columns.Status, sysin.EventStatusProcessing).
		WhereNotNull(columns.LeaseUntil).
		WhereLTE(columns.LeaseUntil, now).
		OrderAsc(columns.LeaseUntil).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取超时Telegram采集事件失败")
	}
	for _, row := range rows {
		eventID := row[columns.Id].Int64()
		if eventID <= 0 {
			continue
		}
		status := sysin.EventStatusFailedRetry
		var nextRunAt any = now
		if row[columns.AttemptCount].Int() >= eventMaxAttempts {
			status = sysin.EventStatusDead
			nextRunAt = nil
		}
		_, updateErr := dao.TgCollectorEvent.Ctx(ctx).
			WherePri(eventID).
			Where(columns.Status, sysin.EventStatusProcessing).
			WhereLTE(columns.LeaseUntil, now).
			Data(g.Map{
				columns.Status:       status,
				columns.NextRunAt:    nextRunAt,
				columns.LeaseOwner:   "",
				columns.LeaseUntil:   nil,
				columns.ErrorMessage: "处理租约超时，已自动恢复",
				columns.UpdatedAt:    now,
			}).Update()
		if updateErr != nil {
			return gerror.Wrap(updateErr, "重置超时Telegram采集事件失败")
		}
	}
	return nil
}

func (s *sCollector) enqueueDueEventTasks(ctx context.Context, limit int) error {
	columns := dao.TgCollectorEvent.Columns()
	now := gtime.Now()
	staleBefore := now.Add(-eventRecoveryStaleTime)
	condition := fmt.Sprintf(
		"(%s=? AND %s<=?) OR (%s=? AND %s IS NOT NULL AND %s<=?)",
		columns.Status,
		columns.UpdatedAt,
		columns.Status,
		columns.NextRunAt,
		columns.NextRunAt,
	)
	rows, err := dao.TgCollectorEvent.Ctx(ctx).
		Fields(columns.Id, columns.EventKey, columns.Priority, columns.Status, columns.UpdatedAt, columns.NextRunAt).
		Where(condition,
			sysin.EventStatusReceived, staleBefore,
			sysin.EventStatusFailedRetry, now,
		).
		WhereLT(columns.AttemptCount, eventMaxAttempts).
		OrderDesc(columns.Priority).
		OrderAsc(columns.UpdatedAt).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复Telegram采集事件失败")
	}
	for _, row := range rows {
		eventID := row[columns.Id].Int64()
		status := row[columns.Status].String()
		if eventID <= 0 || status == "" {
			continue
		}
		if updateErr := s.enqueueEventTask(ctx, eventID, row[columns.Priority].Int()); updateErr != nil {
			return gerror.Wrap(updateErr, "投递待恢复Telegram采集事件失败")
		}
	}
	return nil
}

func collectorRedisOption(ctx context.Context) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     g.Cfg().MustGet(ctx, "redis.default.address", "127.0.0.1:6379").String(),
		Password: g.Cfg().MustGet(ctx, "redis.default.pass", "").String(),
		DB:       g.Cfg().MustGet(ctx, "redis.default.db", 0).Int(),
	}
}

func parseCollectorTaskID(payload []byte, taskName string) (int64, error) {
	taskID, err := strconv.ParseInt(strings.TrimSpace(string(payload)), 10, 64)
	if err != nil || taskID <= 0 {
		return 0, gerror.Newf("Telegram采集%s任务ID无效", taskName)
	}
	return taskID, nil
}

func collectorTaskPayload(taskID int64) []byte {
	return []byte(strconv.FormatInt(taskID, 10))
}

func collectorConcurrency(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "telegramCollector.worker.concurrency", 12).Int()
	if value < 1 {
		return 1
	}
	return value
}

func collectorRecoveryBatchSize(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "telegramCollector.worker.recoveryBatchSize", 200).Int()
	if value < 1 {
		return 1
	}
	if value > 1000 {
		return 1000
	}
	return value
}

func collectorInstanceID() string {
	for _, key := range []string{"RAILWAY_REPLICA_ID", "RAILWAY_DEPLOYMENT_ID", "HOSTNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	value, err := os.Hostname()
	if err == nil && strings.TrimSpace(value) != "" {
		return value
	}
	return "local"
}
