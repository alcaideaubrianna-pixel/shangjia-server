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
	"github.com/gogf/gf/v2/net/gtrace"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/hibiken/asynq"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/collectobserve"
	"hotgo/internal/library/hgrds/lock"
	"hotgo/utility/sourcequeue"
)

const eventTaskType = "telegram_collector:event"

const (
	eventLeaseDuration     = 2 * time.Minute
	eventRecoveryStaleTime = 15 * time.Second
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
	q, err := sourcequeue.New(collectorEventSourceNamespace)
	if err != nil {
		s.queueMu.Unlock()
		g.Log().Warning(runtimeCtx, "来源采集队列初始化失败，Worker未启动", g.Map{"event": "collector.event.source_start_failed", "queueAdapter": "cache.Instance/sourcequeue", "error": err.Error()})
		return
	}
	probeCtx, stopProbe := context.WithTimeout(runtimeCtx, 5*time.Second)
	probeErr := q.Recover(probeCtx, sourcequeue.Realtime, 1)
	if probeErr == nil {
		probeErr = q.Recover(probeCtx, sourcequeue.History, 1)
	}
	stopProbe()
	if probeErr != nil {
		s.queueMu.Unlock()
		g.Log().Warning(runtimeCtx, "来源队列不可用，Worker未启动", g.Map{"event": "collector.event.source_start_failed", "queueAdapter": "cache.Instance/sourcequeue", "error": probeErr.Error()})
		return
	}
	// The legacy reader only migrates existing tasks; it has no business slots.
	server := asynq.NewServer(collectorRedisOption(runtimeCtx), asynq.Config{
		Concurrency: 1,
		Queues: map[string]int{consts.QueueCollectFresh: 1, consts.QueueCollectRealtime: 1,
			consts.QueueCollectHistory: 1, consts.QueueCollectUrgent: 1, consts.QueueCollectUrgentLive: 1},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(eventTaskType, s.handleEventTask)
	if collectorEventHistoryConcurrency(runtimeCtx) > 0 {
		if err := server.Start(mux); err != nil {
			s.queueMu.Unlock()
			g.Log().Warning(runtimeCtx, "采集兼容迁移消费者启动失败，Worker未标记启动", g.Map{"event": "collector.event.source_start_failed", "error": err.Error()})
			return
		}
	}
	s.queueServer = server
	s.runtimeWG.Add(1)
	go func() { defer s.runtimeWG.Done(); s.runSourceEventRecovery(runtimeCtx, q) }()

	for _, pool := range []struct {
		lane sourcequeue.Lane
		size int
	}{
		{sourcequeue.Realtime, collectorConcurrency(runtimeCtx)},
		{sourcequeue.History, collectorEventHistoryConcurrency(runtimeCtx)},
	} {
		for i := 0; i < pool.size; i++ {
			s.runtimeWG.Add(1)
			go func(lane sourcequeue.Lane) {
				defer s.runtimeWG.Done()
				s.runSourceEventWorker(runtimeCtx, q, lane)
			}(pool.lane)
		}
	}
	g.Log().Info(runtimeCtx, "启动来源隔离Collector Worker", g.Map{
		"event": "collector_worker_started", "queueAdapter": "cache.Instance/sourcequeue",
		"realtimeConcurrency": collectorConcurrency(runtimeCtx),
		"historyConcurrency":  collectorEventHistoryConcurrency(runtimeCtx),
		"legacyMode":          "history_migration_only", "eventHandoffMode": "queued",
	})
	s.runtimeWG.Add(1)
	go func() {
		defer s.runtimeWG.Done()
		s.runEventRecovery(runtimeCtx)
	}()
	s.queueMu.Unlock()
}

func (s *sCollector) StopRuntime() {
	s.queueMu.Lock()
	freshDeliveryServer := s.deliveryFreshServer
	s.deliveryFreshServer = nil
	botServer := s.queueBotServer
	s.queueBotServer = nil
	eventHistoryServer := s.queueHistoryServer
	s.queueHistoryServer = nil
	historyServer := s.deliveryHistoryServer
	s.deliveryHistoryServer = nil
	server, deliveryServer, client, runtimeStop := s.queueServer, s.deliveryServer, s.queueClient, s.runtimeStop
	s.queueServer, s.deliveryServer, s.queueClient, s.runtimeCtx, s.runtimeStop = nil, nil, nil, nil, nil
	s.queueMu.Unlock()
	if runtimeStop != nil {
		runtimeStop()
	}
	if server != nil {
		server.Shutdown()
	}
	if botServer != nil {
		botServer.Shutdown()
	}
	if eventHistoryServer != nil {
		eventHistoryServer.Shutdown()
	}
	if deliveryServer != nil {
		deliveryServer.Shutdown()
	}
	if freshDeliveryServer != nil {
		freshDeliveryServer.Shutdown()
	}
	if historyServer != nil {
		historyServer.Shutdown()
	}
	if client != nil {
		_ = client.Close()
	}
	s.runtimeWG.Wait()
	s.queueMu.Lock()
	inspector := s.queueInspector
	s.queueInspector = nil
	batch := s.historyEventBatch
	s.historyEventBatch = nil
	realtimeBatch := s.realtimeEventBatch
	s.realtimeEventBatch = nil
	deliveryBatch := s.historyDeliveryBatch
	s.historyDeliveryBatch = nil
	deliveryReadyBatch := s.realtimeDeliveryReadyBatch
	s.realtimeDeliveryReadyBatch = nil
	s.queueMu.Unlock()
	if inspector != nil {
		_ = inspector.Close()
	}
	if batch != nil {
		batch.Close()
	}
	if realtimeBatch != nil {
		realtimeBatch.Close()
	}
	if deliveryReadyBatch != nil {
		deliveryReadyBatch.Close()
	}
	if deliveryBatch != nil {
		deliveryBatch.Close()
	}
}

// Zero/invalid Bot budget selects the default. ENV is read here because this
// change intentionally does not alter the shared bootstrap ENV mapping.
func collectorBotRealtimeBudget(ctx context.Context) int {
	if raw := strings.TrimSpace(os.Getenv("YOUBAN_COLLECTOR_BOT_REALTIME_CONCURRENCY")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}
	return g.Cfg().MustGet(ctx, "telegramCollector.worker.botRealtimeConcurrency", 0).Int()
}

// Account is first, Bot second. A single total slot cannot be reserved twice:
// preserve weighted consumption of both realtime queues in that case.
func collectorRealtimeServerConfigs(total, botBudget int) []asynq.Config {
	if total < 1 {
		total = 1
	}
	if total == 1 {
		return []asynq.Config{{Concurrency: 1, Queues: map[string]int{
			consts.QueueCollectFresh: 8, consts.QueueCollectRealtime: 4,
		}}}
	}
	if botBudget <= 0 {
		botBudget = total - total/3
	}
	if botBudget >= total {
		botBudget = total - 1
	}
	return []asynq.Config{
		{Concurrency: total - botBudget, Queues: map[string]int{consts.QueueCollectRealtime: 1}},
		{Concurrency: botBudget, Queues: map[string]int{consts.QueueCollectFresh: 1}},
	}
}

func (s *sCollector) ensureRuntimeContext(ctx context.Context) context.Context {
	if s.runtimeCtx != nil {
		return s.runtimeCtx
	}
	s.runtimeCtx, s.runtimeStop = context.WithCancel(ctx)
	return s.runtimeCtx
}

func (s *sCollector) enqueueEventTask(ctx context.Context, eventID int64, priority int, logOrigins ...eventEnqueueOrigin) error {
	if eventID <= 0 {
		return gerror.New("Telegram采集事件任务ID无效")
	}

	origin := eventOriginIngress
	if len(logOrigins) > 0 {
		origin = logOrigins[0]
	}
	if origin != eventOriginIngress && origin != eventOriginRecovery && origin != eventOriginReplay {
		return gerror.New("采集事件入队职责无效")
	}
	return s.enqueueSourceEvent(ctx, eventID, priority, origin)
}

func (s *sCollector) handleEventTask(ctx context.Context, task *asynq.Task) error {
	taskReceivedAt := time.Now()
	ctx, eventID, decodeErr := collectobserve.DecodeTask(ctx, task.Payload(), taskReceivedAt)
	_, parseObservation := startCollectorStage(ctx, collectorStageEventTaskParse, collectorObservationMeta{})
	defer parseObservation.Finish(collectorResultError, nil)
	err := decodeErr
	if err != nil {
		err = gerror.Wrap(err, "Telegram采集事件任务ID无效")
	}
	parseObservation.meta.EventID = eventID
	parseObservation.Finish(collectorResultOK, err)
	if err != nil {
		return err
	}
	// Existing shared live tasks are backlog on upgrade. Migration must
	// succeed before Asynq ACK; it never consumes the realtime business pool.
	return s.migrateLegacySourceEvent(ctx, eventID)
}

func (s *sCollector) processEvent(ctx context.Context, eventID int64) (err error) {
	if eventID <= 0 {
		return gerror.New("Telegram采集事件任务无效")
	}
	ctx, observation := startCollectorStage(ctx, collectorStageEventConsume, collectorObservationMeta{
		EventID: eventID, QueueClass: collectorObserveQueueClassFromClass("mixed"),
	})
	observeResult := collectorResultError
	defer func() { observation.Finish(observeResult, err) }()
	pipelineStarted := time.Now()
	defer func() {
		if wait := collectorQueueWaitFromContext(ctx); wait != nil && wait.stage == collectorStageEventConsume {
			wait.Finish(observation.meta)
		}
	}()
	var row entity.TgCollectorEvent
	row.Id = eventID
	logCollectorPipelineStage(ctx, &row, "event.claim.started", pipelineStarted, nil, nil)
	executionOwner := guid.S()
	claimStarted := time.Now()
	var affected int64
	scope, sourceRuntime := collectorSourceScopeFromContext(ctx)
	batched := historyEventBatchEnabled(ctx) && !sourceRuntime
	if sourceRuntime {
		row, err = claimSourceCollectorEvent(ctx, eventID, executionOwner, scope)
		if err == nil && row.Id > 0 {
			affected = 1
		}
	} else if batched {
		row, err = s.historyEventWriter(ctx).Do(ctx, historyEventMutation{Kind: historyEventClaim, ID: eventID, Owner: executionOwner})
		if err == nil && row.Id > 0 {
			affected = 1
		}
	} else {
		var claimed []entity.TgCollectorEvent
		claimed, err = executeHistoryEventMutation(ctx, historyEventClaim, []historyEventMutation{{Kind: historyEventClaim, ID: eventID, Owner: executionOwner}})
		if err == nil && len(claimed) == 1 {
			row = claimed[0]
			affected = 1
		}
	}
	claimDuration := time.Since(claimStarted).Milliseconds()
	if err != nil {
		recordCollectorStage(ctx, collectorStageEventClaim, observation.meta, collectorResultError, time.Since(claimStarted), err)
		logCollectorPipelineStage(ctx, &row, "event.claim.failed", pipelineStarted, nil, err,
			g.Map{"claimDurationMs": claimDuration})
		return gerror.Wrap(err, "领取Telegram采集事件失败")
	}
	if affected == 0 {
		row.Id = eventID
		recordCollectorStage(ctx, collectorStageEventClaim, observation.meta, collectorResultSkipped, time.Since(claimStarted), nil)
		observeResult = collectorResultSkipped
		logCollectorPipelineStage(ctx, &row, "event.claim.skipped", pipelineStarted, nil, nil,
			g.Map{"claimDurationMs": claimDuration, "rowsAffected": affected})
		return nil
	}
	recordCollectorStage(ctx, collectorStageEventClaim, observation.meta, collectorResultOK, time.Since(claimStarted), nil)
	logCollectorPipelineStage(ctx, &row, "event.claimed", pipelineStarted, nil, nil,
		g.Map{"claimDurationMs": claimDuration, "rowsAffected": affected})
	readStarted := time.Now()
	if row.LeaseOwner != executionOwner || row.Status != sysin.EventStatusProcessing {
		err = gerror.New("采集执行归属已变化，停止旧执行")
		recordCollectorStage(ctx, collectorStageEventRead, observation.meta, collectorResultError, time.Since(readStarted), err)
		return err
	}
	if sourceRuntime && (row.TenantId != scope.TenantID || row.SourceId != scope.SourceID) {
		return gerror.New("采集事件持久化来源与队列归属不一致")
	}
	observation.meta.TenantID, observation.meta.SourceID = row.TenantId, row.SourceId
	if row.SourceType == sysin.SourceTypeAccount {
		observation.meta.TgAccountID = row.AccountId
	}
	observation.meta.ChatID, observation.meta.MessageID = row.ChatId, row.MessageId
	observation.meta.SourceType = collectorObserveSourceType(row.SourceType)
	observation.meta.QueueClass = collectorObserveQueueClassFromPriority(row.Priority)
	ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
	recordCollectorStage(ctx, collectorStageEventRead, observation.meta, collectorResultOK, time.Since(readStarted), nil)
	logCollectorPipelineStage(ctx, &row, "event.loaded", pipelineStarted, nil, nil,
		g.Map{"readDurationMs": time.Since(readStarted).Milliseconds(), "claimDurationMs": claimDuration, "readFromClaim": true})
	if row.RawUpdate == nil {
		return s.failEvent(ctx, eventID, executionOwner, gerror.New("Telegram原始事件为空"), &row)
	}
	if collectorSystemNotice(row.SourceType, []byte(row.RawUpdate.String())) {
		if err = ignoreClaimedSystemNotice(ctx, &row); err != nil {
			return s.failEvent(ctx, eventID, executionOwner, err, &row)
		}
		observeResult = collectorResultSkipped
		return nil
	}
	parseStarted := time.Now()
	raw := []byte(row.RawUpdate.String())
	ctx = collectobserve.RestorePayloadClock(ctx, raw, time.Now())
	observation.meta.ReceivedAt = collectobserve.From(ctx).ReceivedAt
	delivery, err := collectorDeliveryFromEvent(&row, raw)
	if err == nil {
		observation.meta.Mode = collectorModeFromRaw(row.SourceType, raw)
		observation.meta.AccountID, observation.meta.TgAccountID = delivery.AccountID, delivery.TgAccountID
		ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
	}
	recordCollectorStage(ctx, collectorStageEventParse, observation.meta, collectorResultOK, time.Since(parseStarted), err)
	if err != nil {
		return s.failEvent(ctx, eventID, executionOwner, err, &row)
	}
	// Reclassify known rejected backlog without running the slow handler inline.
	deferred := false
	if row.SourceType == sysin.SourceTypeBot && !eventQueueIsHistory(collectorQueueName(ctx)) {
		if chatID, parseErr := strconv.ParseInt(delivery.SourceChatID, 10, 64); parseErr == nil {
			decision := classifyBotIngress(ctx, row.TenantId, row.SourceId, chatID)
			deferred = decision.Defer()
			if deferred {
				logBotIngressDeferral(ctx, row.TenantId, row.SourceId, row.Id, delivery.SourceMessageID, decision)
			}
		}
	}
	// Execution class is explicit, never inferred from message age.
	if lane, ok := collectorSourceLaneFromContext(ctx); deferred || (ok && lane == sourcequeue.History) || eventQueueIsHistory(collectorQueueName(ctx)) {
		delivery.History = true
		observation.meta.Mode = collectorModeHistory
		observation.meta.QueueClass = collectorObserveQueueClassFromPriority(sysin.EventPriorityNormal)
		ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
	}
	logCollectorPipelineStage(ctx, &row, "delivery.parsed", pipelineStarted, &delivery, nil)
	dispatchCtx, dispatchObservation := startCollectorStage(ctx, collectorStageEventDispatch, observation.meta)
	defer dispatchObservation.Finish(collectorResultError, nil)
	err = s.handoffCollectorEvent(dispatchCtx, &row, &delivery)
	dispatchObservation.Finish(collectorResultOK, err)
	if err != nil {
		return s.failEvent(ctx, eventID, executionOwner, err, &row)
	}
	observeResult = collectorResultOK
	return nil
}

// logCollectorPipelineStage 输出采集到分发的统一链路字段，便于按事件/账户/来源追踪耗时。
func logCollectorPipelineStage(ctx context.Context, row *entity.TgCollectorEvent, stage string, started time.Time, delivery *sysin.CollectorDelivery, err error, details ...g.Map) {
	if row == nil {
		return
	}
	fields := g.Map{
		"log_type": "telegram_collect_pipeline", "event": "collect_pipeline_stage", "stage": stage,
		"status": "ok", "traceId": gtrace.GetTraceID(ctx), "collectEventId": row.Id,
		"eventId": row.Id, "tenantId": row.TenantId, "accountId": row.AccountId,
		"sourceId": row.SourceId, "sourceType": row.SourceType, "sourceChatId": row.ChatId,
		"sourceMessageId": row.MessageId, "durationMs": time.Since(started).Milliseconds(),
		"observedAtUnixMs": time.Now().UnixMilli(), "startedAtUnixMs": started.UnixMilli(),
		"queue": collectorQueueName(ctx),
	}
	fields["ageTimeBasis"] = "db_wall_clock_unverified"
	collectorObservationLogFields(ctx, fields)
	fields["eventAgeAvailable"] = false
	if row.CreatedAt != nil {
		fields["eventCreatedAtRaw"] = row.CreatedAt.Time.Format("2006-01-02 15:04:05.999999")
	}
	if row.ReceivedAt != nil {
		fields["eventReceivedAtRaw"] = row.ReceivedAt.Time.Format("2006-01-02 15:04:05.999999")
	}
	for _, detail := range details {
		for key, value := range detail {
			fields[key] = value
		}
	}
	if delivery != nil {
		fields["history"] = delivery.History
		fields["deliveryId"] = delivery.ID
		fields["deliverySourceChatId"] = delivery.SourceChatID
		fields["deliverySourceMessageId"] = delivery.SourceMessageID
		fields["mediaCount"] = len(delivery.Media)
		fields["textLength"] = len([]rune(delivery.RawText))
	}
	if err != nil {
		fields["status"] = "error"
		fields["errorType"] = fmt.Sprintf("%T", err)
		fields["error"] = err.Error()
	}
	if err != nil {
		g.Log().Warning(ctx, "Telegram采集链路阶段", fields)
		return
	}
	g.Log().Info(ctx, "Telegram采集链路阶段", fields)
}

func collectorDeliveryFromEvent(row *entity.TgCollectorEvent, raw []byte) (sysin.CollectorDelivery, error) {
	if row == nil {
		return sysin.CollectorDelivery{}, gerror.New("Telegram采集事件为空")
	}
	delivery := sysin.CollectorDelivery{
		DeliveryKey:     collectorDeliveryKey(row, raw),
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
		delivery.History = message.History
		// Legacy events have no history flag. Classify against persistence time,
		// never worker time, so queue delay cannot demote realtime messages.
		var legacy struct {
			History *bool `json:"history"`
		}
		_ = json.Unmarshal(raw, &legacy)
		if legacy.History == nil && row.CreatedAt != nil && !message.ReceivedAt.IsZero() {
			receivedAt := message.ReceivedAt
			// Both database timestamps use the same wall-clock convention. Do not
			// mix timestamp-without-time-zone scans with RFC3339 offsets in raw JSON.
			if row.ReceivedAt != nil {
				receivedAt = row.ReceivedAt.Time
			}
			delivery.History = receivedAt.Before(row.CreatedAt.Time.Add(-5 * time.Minute))
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
	seen := make(map[string]struct{}, 4)
	appendItem := func(mediaType, fileID, fileUniqueID string, size int64, mime string) {
		fileID = strings.TrimSpace(fileID)
		fileUniqueID = strings.TrimSpace(fileUniqueID)
		if fileID == "" {
			return
		}
		if _, ok := seen["id:"+fileID]; ok {
			return
		}
		if fileUniqueID != "" {
			if _, ok := seen["unique:"+fileUniqueID]; ok {
				return
			}
		}
		seen["id:"+fileID] = struct{}{}
		if fileUniqueID != "" {
			seen["unique:"+fileUniqueID] = struct{}{}
		}
		items = append(items, sysin.CollectorMediaItem{Type: mediaType, FileID: fileID, FileUniqueID: fileUniqueID, SourceSize: size, SourceMimeType: mime})
	}
	if len(message.Photo) > 0 {
		photo := message.Photo[len(message.Photo)-1]
		appendItem("photo", photo.FileID, photo.FileUniqueID, int64(photo.FileSize), "image/jpeg")
	}
	if message.Video != nil {
		appendItem("video", message.Video.FileID, message.Video.FileUniqueID, int64(message.Video.FileSize), message.Video.MimeType)
	}
	if message.Animation != nil {
		appendItem("video", message.Animation.FileID, message.Animation.FileUniqueID, int64(message.Animation.FileSize), message.Animation.MimeType)
	}
	if message.Document != nil {
		mediaType := "document"
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(message.Document.MimeType)), "video/") {
			mediaType = "video"
		}
		appendItem(mediaType, message.Document.FileID, message.Document.FileUniqueID, int64(message.Document.FileSize), message.Document.MimeType)
	}
	return items
}

func (s *sCollector) saveDelivery(ctx context.Context, delivery *sysin.CollectorDelivery) (int64, error) {
	if delivery == nil {
		return 0, gerror.New("采集交付不能为空")
	}
	if err := markCollectorRecoveryScope(ctx, collectorRecoveryScope{delivery.TenantID, delivery.AccountID, delivery.SourceID}); err != nil {
		return 0, gerror.Wrap(err, "注册采集交付恢复 scope 失败")
	}
	columns := dao.TgCollectorDelivery.Columns()
	baseKey := delivery.DeliveryKey
	// One bounded fallback handles a stable identity left pointing at an old
	// database row after a restore. Never modify the old completed delivery.
	for attempt := 0; attempt < 2; attempt++ {
		var row entity.TgCollectorDelivery
		if delivery.History && historyEventBatchEnabled(ctx) {
			saved, err := s.historyDeliveryWriter(ctx).Do(ctx, *delivery)
			if err != nil {
				return 0, err
			}
			row.Id, row.EventId = saved.ID, saved.EventID
		} else {
			now := gtime.Now()
			data := g.Map{
				columns.TenantId: delivery.TenantID, columns.EventId: delivery.EventID,
				columns.DeliveryKey: delivery.DeliveryKey, columns.Status: sysin.DeliveryStatusPending,
				columns.Priority: deliveryPriority(delivery), columns.CreatedAt: now, columns.UpdatedAt: now,
				"source_id": delivery.SourceID, "account_id": delivery.AccountID,
				"scheduler_pool": deliverySchedulerPool(delivery),
			}
			_, err := dao.TgCollectorDelivery.Ctx(ctx).Data(data).OnConflict(columns.TenantId + "," + columns.DeliveryKey).
				OnDuplicate(g.Map{columns.DeliveryKey: conflictIncomingColumn(ctx, columns.DeliveryKey)}).Save()
			if err != nil {
				return 0, gerror.Wrap(err, "保存Telegram采集交付失败")
			}
			if err = dao.TgCollectorDelivery.Ctx(ctx).
				Fields(columns.Id, columns.EventId).
				Where(columns.TenantId, delivery.TenantID).
				Where(columns.DeliveryKey, delivery.DeliveryKey).Scan(&row); err != nil {
				return 0, gerror.Wrap(err, "读取Telegram采集交付失败")
			}
		}
		if row.Id <= 0 {
			return 0, gerror.New("Telegram采集交付ID无效")
		}
		if row.EventId == delivery.EventID {
			return row.Id, nil
		}
		delivery.DeliveryKey = collectorDeliveryCollisionKey(baseKey, delivery.EventID)
		g.Log().Warning(ctx, "采集交付身份冲突，使用独立恢复交付", g.Map{
			"tenantId": delivery.TenantID, "sourceId": delivery.SourceID,
			"eventId": delivery.EventID, "conflictingDeliveryId": row.Id,
		})
	}
	return 0, gerror.New("恢复交付尚未保存成功，等待重试")
}

func deliverySchedulerPool(delivery *sysin.CollectorDelivery) string {
	if delivery != nil && delivery.History {
		return "history"
	}
	return "realtime"
}

func deliveryPriority(delivery *sysin.CollectorDelivery) int {
	if delivery == nil {
		return sysin.EventPriorityNormal
	}
	if delivery.History {
		return sysin.EventPriorityNormal
	}
	if delivery.SourceType == sysin.SourceTypeAccount {
		return sysin.EventPriorityRealtime
	}
	return sysin.EventPriorityUrgent
}

func (s *sCollector) failEvent(ctx context.Context, eventID int64, executionOwner string, cause error, known ...*entity.TgCollectorEvent) error {
	if strings.TrimSpace(executionOwner) == "" {
		return gerror.New("采集失败更新必须指定本次执行归属")
	}
	columns := dao.TgCollectorEvent.Columns()
	var row entity.TgCollectorEvent
	if len(known) > 0 && known[0] != nil && known[0].Id == eventID && known[0].LeaseOwner == executionOwner {
		row = *known[0]
	} else if err := dao.TgCollectorEvent.Ctx(ctx).Fields(columns.AttemptCount, columns.LeaseOwner).WherePri(eventID).
		Where(columns.Status, sysin.EventStatusProcessing).Where(columns.LeaseOwner, executionOwner).Scan(&row); err != nil {
		return gerror.Wrap(err, "读取Telegram采集事件重试次数失败")
	}
	if row.LeaseOwner != executionOwner {
		return gerror.New("采集执行归属已变化，未写入失败状态")
	}
	status := sysin.EventStatusFailedRetry
	var nextRunAt any = gtime.Now().Add(30 * time.Second)
	if row.AttemptCount >= eventMaxAttempts {
		status = sysin.EventStatusDead
		nextRunAt = nil
	}
	result, err := dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).
		Where(columns.Status, sysin.EventStatusProcessing).
		Where(columns.LeaseOwner, executionOwner).Data(g.Map{
		columns.Status:       status,
		columns.NextRunAt:    nextRunAt,
		columns.LeaseOwner:   "",
		columns.LeaseUntil:   nil,
		columns.ErrorMessage: cause.Error(),
		columns.UpdatedAt:    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "保存采集失败状态失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认采集失败状态更新结果失败")
	}
	if affected != 1 {
		return gerror.New("采集执行归属已变化，未确认失败或终止状态")
	}
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
	ticker := time.NewTicker(collectorRecoveryInterval(ctx))
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
	if collectorIngestSpoolEnabled(ctx) {
		spoolCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		scopes, err := collectorRecoveryScopes(spoolCtx, collectorRecoverySpool, 8)
		if err != nil {
			g.Log().Warningf(ctx, "读取 Spool 恢复 scope 失败：%+v", err)
		} else {
			for _, scope := range scopes {
				laneCtx, cancelLane := context.WithTimeout(spoolCtx, 500*time.Millisecond)
				if err := s.recoverIngestSpool(laneCtx, 5, scope); err != nil {
					g.Log().Warningf(ctx, "恢复来源 Spool 失败 scope:%s err:%+v", scope.key(), err)
				}
				cancelLane()
			}
		}
		cancel()
	}
	if err := s.resetExpiredEventLeases(ctx, collectorRecoveryBatchSize(ctx)); err != nil {
		g.Log().Warningf(ctx, "恢复超时Telegram采集事件失败：%+v", err)
	}
	if err := s.enqueueDueEventTasks(ctx, collectorRecoveryBatchSize(ctx)); err != nil {
		g.Log().Warningf(ctx, "重新投递Telegram采集事件失败：%+v", err)
	}
}

func (s *sCollector) resetExpiredEventLeases(ctx context.Context, limit int, scopes ...collectorRecoveryScope) error {
	if len(scopes) == 0 {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		lanes, err := collectorRecoveryScopes(ctx, collectorRecoveryEventLease, 8)
		if err != nil {
			return gerror.Wrap(err, "读取原始事件租约恢复 scope 失败，禁止全局扫描")
		}
		var result error
		for _, scope := range lanes {
			laneCtx, cancelLane := context.WithTimeout(ctx, 500*time.Millisecond)
			result = errors.Join(result, s.resetExpiredEventLeases(laneCtx, min(16, limit), scope))
			cancelLane()
		}
		return result
	}
	if len(scopes) != 1 || !scopes[0].valid() {
		return gerror.New("原始事件恢复需要有效单 scope")
	}
	scope := scopes[0]
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	columns := dao.TgCollectorEvent.Columns()
	now := gtime.Now()
	rows, err := dao.TgCollectorEvent.Ctx(ctx).
		Fields(columns.Id, columns.AttemptCount).
		Where(columns.TenantId, scope.TenantID).Where(columns.AccountId, scope.AccountID).Where(columns.SourceId, scope.SourceID).
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
			Where(columns.TenantId, scope.TenantID).Where(columns.AccountId, scope.AccountID).Where(columns.SourceId, scope.SourceID).
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

func (s *sCollector) enqueueDueEventTasks(ctx context.Context, limit int) (recoveryErr error) {
	ctx, recoveryObservation := startCollectorStage(ctx, collectorStageEventRecovery, collectorObservationMeta{QueueClass: collectorQueueMixed})
	defer func() { recoveryObservation.Finish(collectorResultOK, recoveryErr) }()
	scopeCtx, cancel := context.WithTimeout(ctx, time.Second)
	scopes, err := collectorRecoveryScopes(scopeCtx, collectorRecoveryEventEnqueue, 8)
	cancel()
	if err != nil {
		return gerror.Wrap(err, "读取原始事件恢复 scope 失败，禁止全局扫描")
	}
	if len(scopes) == 0 {
		g.Log().Info(ctx, "原始事件恢复跳过", g.Map{"reason": "redis_scope_unavailable"})
		return nil
	}
	receivedLimit, retryLimit := eventRecoveryBranchLimits(min(limit, 50))
	var result error
	branches := collectorRecoveryBranchOrder([]string{sysin.EventStatusReceived, sysin.EventStatusFailedRetry})
	for _, scope := range scopes {
		for _, status := range branches {
			branchLimit := receivedLimit
			if status == sysin.EventStatusFailedRetry {
				branchLimit = retryLimit
			}
			branchCtx, cancelBranch := context.WithTimeout(ctx, 500*time.Millisecond)
			branchErr := s.enqueueDueEventBranch(branchCtx, status, branchLimit, scope)
			cancelBranch()
			result = errors.Join(result, branchErr)
			if errors.Is(branchErr, errCollectorRecoveryRedisUnavailable) {
				return result
			}
		}
	}
	return result
}

const (
	eventRecoveryCursorTTL = 24 * time.Hour
	eventRecoveryTimeField = "recovery_cursor_time"
)

// Timestamp strings retain the database's wall-clock value. Converting them to
// Unix and back here would reinterpret historical timestamp-without-time-zone data.
type eventRecoveryCursor struct {
	AfterTime string `json:"afterTime"`
	AfterID   int64  `json:"afterId"`
	UntilTime string `json:"untilTime"`
}

func eventRecoveryBranchLimits(limit int) (received, retry int) {
	// A minimum of two gives both states a slot even with recoveryBatchSize=1.
	if limit < 2 {
		limit = 2
	}
	if limit > 1000 {
		limit = 1000
	}
	return (limit + 1) / 2, limit / 2
}

func newEventRecoveryCursor(ctx context.Context, status string) (eventRecoveryCursor, error) {
	var staleSeconds int64
	if status == sysin.EventStatusReceived {
		staleSeconds = int64(eventRecoveryStaleTime / time.Second)
	}
	// Business timestamp fields follow global.Init's Asia/Shanghai wall clock.
	// Pin that zone explicitly: LOCALTIMESTAMP would follow the DB session zone
	// and could delay recovery by eight hours on a UTC connection.
	value, err := g.DB().GetValue(ctx,
		"SELECT to_char((CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai') - ? * INTERVAL '1 second', 'YYYY-MM-DD HH24:MI:SS.US')",
		staleSeconds)
	if err != nil {
		return eventRecoveryCursor{}, gerror.Wrap(err, "读取采集事件恢复扫描时间上界失败")
	}
	if value == nil || value.String() == "" {
		return eventRecoveryCursor{}, gerror.New("采集事件恢复扫描时间上界为空")
	}
	return eventRecoveryCursor{UntilTime: value.String()}, nil
}

func eventRecoveryPage(ctx context.Context, status string, cursor eventRecoveryCursor, limit int, scopes ...collectorRecoveryScope) (gdb.Result, error) {
	if len(scopes) != 1 || !scopes[0].valid() {
		return nil, gerror.New("原始事件恢复分页必须提供单一 scope，禁止全局扫描")
	}
	scope := scopes[0]
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	columns := dao.TgCollectorEvent.Columns()
	model := dao.TgCollectorEvent.Ctx(ctx).Where(columns.TenantId, scope.TenantID).Where(columns.AccountId, scope.AccountID).Where(columns.SourceId, scope.SourceID)
	var timeColumn string
	switch status {
	case sysin.EventStatusReceived:
		// Literal predicates also match the partial index with generic prepared plans.
		model = model.Where(fmt.Sprintf("%s='received'", columns.Status))
		timeColumn = columns.UpdatedAt
		// Received is waiting for its first handoff, not an expired retry. No
		// lower age bound or attempt limit may silently remove this backlog.
	case sysin.EventStatusFailedRetry:
		model = model.Where(fmt.Sprintf("%s='failed_retry'", columns.Status)).
			WhereLT(columns.AttemptCount, eventMaxAttempts)
		timeColumn = columns.NextRunAt
	default:
		return nil, gerror.New("采集事件恢复状态不支持")
	}
	model = model.WhereLTE(timeColumn, cursor.UntilTime)
	if cursor.AfterID > 0 {
		model = model.Where(fmt.Sprintf("(%s,%s) > (?,?)", timeColumn, columns.Id), cursor.AfterTime, cursor.AfterID)
	}
	return model.Fields(columns.Id, columns.Priority,
		fmt.Sprintf("to_char(%s, 'YYYY-MM-DD HH24:MI:SS.US') AS %s", timeColumn, eventRecoveryTimeField)).
		OrderAsc(timeColumn).OrderAsc(columns.Id).Limit(limit).All()
}

func advanceEventRecoveryCursor(cursor eventRecoveryCursor, status string, row gdb.Record) eventRecoveryCursor {
	// The query returns text directly so ORM timezone interpretation cannot
	// shift either side of a timestamp-without-time-zone keyset comparison.
	cursor.AfterTime = row[eventRecoveryTimeField].String()
	cursor.AfterID = row[dao.TgCollectorEvent.Columns().Id].Int64()
	return cursor
}

func enqueueEventRecoveryPage(rows gdb.Result, status string, cursor eventRecoveryCursor, limit int, enqueue func(int64, int) error) (eventRecoveryCursor, error) {
	columns := dao.TgCollectorEvent.Columns()
	for _, row := range rows {
		if err := enqueue(row[columns.Id].Int64(), row[columns.Priority].Int()); err != nil {
			// Keep the last successful position so this row remains retryable.
			return cursor, gerror.Wrap(err, "投递待恢复Telegram采集事件失败")
		}
		// Asynq conflicts return nil too; they must not pin the next scan.
		cursor = advanceEventRecoveryCursor(cursor, status, row)
	}
	if len(rows) < limit {
		// End of the fixed window: revisit earlier rows in the next sweep,
		// including any whose previously enqueued Redis task disappeared.
		cursor = eventRecoveryCursor{}
	}
	return cursor, nil
}

func (s *sCollector) enqueueDueEventBranch(ctx context.Context, status string, limit int, scope collectorRecoveryScope) error {
	if !scope.valid() {
		return gerror.New("原始事件恢复 scope 无效")
	}
	recoveryLock := lock.NewConfig(10*time.Second, time.Second).WithoutWatchDog().Mutex(consts.EventRecoveryLockKey(scope.key(), status))
	if err := recoveryLock.TryLock(ctx); err != nil {
		if errors.Is(err, lock.ErrLockFailed) {
			return nil
		}
		return errors.Join(errCollectorRecoveryRedisUnavailable, err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Millisecond)
		defer cancel()
		_ = recoveryLock.Unlock(unlockCtx)
	}()
	cursorKey := consts.EventRecoveryCursorKey(scope.key(), status)
	value, err := cache.Instance().Get(ctx, cursorKey)
	if err != nil {
		return errors.Join(errCollectorRecoveryRedisUnavailable, gerror.Wrap(err, "读取采集事件恢复游标失败"))
	}
	var cursor eventRecoveryCursor
	if value != nil && !value.IsNil() {
		if err = value.Scan(&cursor); err != nil {
			return gerror.Wrap(err, "解析采集事件恢复游标失败")
		}
	}
	if cursor.UntilTime == "" {
		cursor, err = newEventRecoveryCursor(ctx, status)
		if err != nil {
			return err
		}
	}
	queryStarted := time.Now()
	scanMeta := collectorObservationMeta{
		QueueClass: collectorQueueMixed,
		TenantID:   scope.TenantID, AccountID: scope.AccountID, SourceID: scope.SourceID,
	}
	scanCtx, observation := startCollectorStage(ctx, collectorStageEventScan, scanMeta)
	rows, err := eventRecoveryPage(scanCtx, status, cursor, limit, scope)
	observation.Finish(collectorResultOK, err)
	if err == nil {
		observeCollectorRecoveryCandidates(scanCtx, collectorStageEventScan, scanMeta, len(rows))
	}
	logCollectorPipelineStage(ctx, &entity.TgCollectorEvent{}, "event.recovery.queried", queryStarted, nil, err,
		g.Map{"queryDurationMs": time.Since(queryStarted).Milliseconds(), "resultCount": len(rows),
			"limit": limit, "workerInstanceId": collectorInstanceID(), "recoveryStatus": status,
			"cursorAfterId": cursor.AfterID, "cursorAfterTime": cursor.AfterTime, "scanUntilTime": cursor.UntilTime,
			"scope": scope.key(), "tenantId": scope.TenantID, "accountId": scope.AccountID, "sourceId": scope.SourceID})
	if err != nil {
		return gerror.Wrap(err, "读取待恢复Telegram采集事件失败")
	}
	cursor, enqueueErr := enqueueEventRecoveryPage(rows, status, cursor, limit, func(eventID int64, priority int) error {
		meta := scanMeta
		meta.QueueClass = collectorObserveQueueClassFromPriority(priority)
		meta.EventID = eventID
		enqueueCtx, observation := startCollectorStage(ctx, collectorStageEventReenqueue, meta)
		enqueueErr := s.enqueueEventTask(withCollectorSourceScope(enqueueCtx, scope.TenantID, scope.SourceID), eventID, priority, "recovery")
		observation.Finish(collectorResultOK, enqueueErr)
		if enqueueErr != nil && !errors.Is(enqueueErr, asynq.ErrDuplicateTask) && !errors.Is(enqueueErr, asynq.ErrTaskIDConflict) {
			return errors.Join(errCollectorRecoveryRedisUnavailable, enqueueErr)
		}
		return enqueueErr
	})
	// A branch deadline must not discard already confirmed enqueue progress.
	// Persist only while this execution still owns the cursor lease.
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Millisecond)
	defer cancelSave()
	owned, refreshErr := recoveryLock.Refresh(saveCtx)
	if refreshErr != nil {
		return errors.Join(enqueueErr, errCollectorRecoveryRedisUnavailable, refreshErr)
	}
	if !owned {
		return errors.Join(enqueueErr, gerror.New("采集事件恢复游标租约已失效"))
	}
	if saveErr := cache.Instance().Set(saveCtx, cursorKey, cursor, eventRecoveryCursorTTL); saveErr != nil {
		return errors.Join(enqueueErr, errCollectorRecoveryRedisUnavailable, gerror.Wrap(saveErr, "保存采集事件恢复游标失败"))
	}
	return enqueueErr
}

func collectorRedisOption(ctx context.Context) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     g.Cfg().MustGet(ctx, "redis.default.address", "127.0.0.1:6379").String(),
		Password: g.Cfg().MustGet(ctx, "redis.default.pass", "").String(),
		DB:       g.Cfg().MustGet(ctx, "redis.default.db", 0).Int(),
	}
}

func parseCollectorTaskID(payload []byte, taskName string) (int64, error) {
	_, taskID, err := collectobserve.DecodeTask(context.Background(), payload, time.Time{})
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

func collectorRecoveryInterval(ctx context.Context) time.Duration {
	value := g.Cfg().MustGet(ctx, "telegramCollector.worker.recoveryIntervalSeconds", 60).Int()
	if value < 30 {
		value = 30
	}
	return time.Duration(value) * time.Second
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

func accountMessagePriority(message *sysin.AccountMessageEvent) int {
	if message != nil && message.History {
		return sysin.EventPriorityNormal
	}
	return sysin.EventPriorityRealtime
}
