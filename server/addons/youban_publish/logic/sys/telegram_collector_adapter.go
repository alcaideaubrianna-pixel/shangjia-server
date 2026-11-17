package sys

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	publishconsts "hotgo/addons/youban_publish/consts"
	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
	idao "hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
)

type publishCollectorDeliveryHandler struct {
	publish *sSysPublish
}

type publishCollectorAccountTaskHandler struct{ publish *sSysPublish }

// collectorSubmitStage identifies the lifecycle boundary reached by a
// collector delivery. It keeps persistence and process-queue submission
// distinct while preserving the public Handler error API.
type collectorSubmitStage string

const (
	collectorSubmitStagePersisted collectorSubmitStage = "event.persisted"
	collectorSubmitStageQueued    collectorSubmitStage = "process.queued"
	collectorSubmitStageFailed    collectorSubmitStage = "delivery.failed"
)

// collectorSubmitOutcome is private because the adapter remains the
// compatibility boundary for the collector runtime.
type collectorSubmitOutcome string

const (
	collectorSubmitOutcomeQueued collectorSubmitOutcome = "queued"
	collectorSubmitOutcomeFailed collectorSubmitOutcome = "failed"
)

type collectorSubmitResult struct {
	Outcome collectorSubmitOutcome
	Stage   collectorSubmitStage
	Reason  string
	EventID int64
	Err     error
}

// submitCollectorDelivery records the two meaningful write boundaries. An
// event ID with an error means persistence succeeded but queue submission
// failed, which must remain visible to recovery instead of looking successful.
func (s *sSysPublish) submitCollectorDelivery(ctx context.Context, message *CollectMessage, delivery *collectorin.CollectorDelivery) (result collectorSubmitResult) {
	startedAt := time.Now()
	defer func() { observeCollectPipelineSubmit(ctx, startedAt, result, delivery) }()
	eventID, err := s.ingestAndProcessCollectMessage(ctx, message)
	result = collectorSubmitResult{EventID: eventID, Err: err}
	if err != nil {
		result.Outcome = collectorSubmitOutcomeFailed
		if eventID > 0 {
			result.Stage = collectorSubmitStagePersisted
			result.Reason = "process_queue_submit_failed"
		} else {
			result.Stage = collectorSubmitStageFailed
			result.Reason = "event_persist_failed"
		}
		logPublishCollectorDeliveryStage(ctx, string(result.Stage), delivery, err)
		return result
	}
	result.Outcome = collectorSubmitOutcomeQueued
	result.Stage = collectorSubmitStageQueued
	result.Reason = "event_persisted_and_process_queued"
	logPublishCollectorDeliveryStage(ctx, string(collectorSubmitStagePersisted), delivery, nil)
	logPublishCollectorDeliveryStage(ctx, string(result.Stage), delivery, nil)
	return result
}

type publishCollectorAccountMediaProvider struct{ publish *sSysPublish }

// telegramWebhookReconcileMessage 是 webhook 到达的 Telegram 消息短期索引。
// Redis 只用于加速 unknown/reconciling 对账，最终消息记录仍由 PostgreSQL 保存。
type telegramWebhookReconcileMessage struct {
	ChatID       string    `json:"chatId"`
	MessageID    int64     `json:"messageId"`
	MediaGroupID string    `json:"mediaGroupId,omitempty"`
	MessageType  string    `json:"messageType"`
	Status       string    `json:"status"`
	ReceivedAt   time.Time `json:"receivedAt"`
	AttemptID    int64     `json:"attemptId,omitempty"`
	JobID        int64     `json:"jobId,omitempty"`
	Phase        string    `json:"phase,omitempty"`
}

func telegramWebhookMessageType(message *models.Message) string {
	if message == nil {
		return "unknown"
	}
	if strings.TrimSpace(message.MediaGroupID) != "" {
		return "album"
	}
	if message.Video != nil || message.VideoNote != nil {
		return "video"
	}
	if len(message.Photo) > 0 {
		return "photo"
	}
	if message.Document != nil {
		return "document"
	}
	if strings.TrimSpace(message.Text) != "" || strings.TrimSpace(message.Caption) != "" {
		return "text"
	}
	return "unknown"
}

const telegramWebhookReconcileIndexTTL = 24 * time.Hour

func telegramWebhookReconcileMessageKey(chatID string, messageID int64) string {
	return fmt.Sprintf("youban:tg:reconcile:message:%s:%d", strings.TrimSpace(chatID), messageID)
}

func telegramWebhookReconcileGroupKey(chatID, mediaGroupID string) string {
	return fmt.Sprintf("youban:tg:reconcile:group:%s:%s", strings.TrimSpace(chatID), strings.TrimSpace(mediaGroupID))
}

const (
	telegramWebhookRecentMessagesCapacity = 500
	telegramWebhookMessageStatusReceived  = "received"
	telegramWebhookMessageStatusMatched   = "matched"
)

func telegramWebhookRecentKey(chatID string) string {
	return fmt.Sprintf("youban:tg:reconcile:recent:%s", strings.TrimSpace(chatID))
}

func telegramWebhookReconcileTTL(ctx context.Context) time.Duration {
	hours := g.Cfg().MustGet(ctx, "youbanPublish.telegram.webhookReconcileTTLHours", 24).Int()
	if hours <= 0 {
		hours = 24
	}
	return time.Duration(hours) * time.Hour
}

func telegramWebhookRecentCapacity(ctx context.Context) int {
	capacity := g.Cfg().MustGet(ctx, "youbanPublish.telegram.webhookRecentCapacity", telegramWebhookRecentMessagesCapacity).Int()
	if capacity <= 0 {
		capacity = telegramWebhookRecentMessagesCapacity
	}
	return capacity
}

type telegramWebhookMessageState struct {
	MediaGroupID string    `json:"media_group_id,omitempty"`
	MessageType  string    `json:"message_type"`
	Status       string    `json:"status"`
	ReceivedAt   time.Time `json:"received_at"`
	AttemptID    int64     `json:"attempt_id,omitempty"`
	JobID        int64     `json:"job_id,omitempty"`
	Phase        string    `json:"phase,omitempty"`
}

func telegramWebhookMessageStateFrom(message telegramWebhookReconcileMessage) telegramWebhookMessageState {
	return telegramWebhookMessageState{
		MediaGroupID: message.MediaGroupID, MessageType: message.MessageType, Status: message.Status,
		ReceivedAt: message.ReceivedAt, AttemptID: message.AttemptID, JobID: message.JobID, Phase: message.Phase,
	}
}

// reserveTelegramWebhookReconcileMessage atomically claims the channel/message
// fact in Redis. Redis is only a write-through dedupe layer; PostgreSQL unique
// constraints remain the final correctness boundary when Redis is unavailable.
func reserveTelegramWebhookReconcileMessage(ctx context.Context, message telegramWebhookReconcileMessage) (bool, error) {
	if strings.TrimSpace(message.ChatID) == "" || message.MessageID <= 0 || !cache.Initialized() {
		return true, nil
	}
	if message.Status == "" {
		message.Status = telegramWebhookMessageStatusReceived
	}
	state, err := json.Marshal(telegramWebhookMessageStateFrom(message))
	if err != nil {
		return false, err
	}
	return cache.Instance().SetIfNotExist(ctx, telegramWebhookReconcileMessageKey(message.ChatID, message.MessageID), string(state), telegramWebhookReconcileTTL(ctx))
}

func cacheTelegramWebhookReconcileMessage(ctx context.Context, message telegramWebhookReconcileMessage) {
	if strings.TrimSpace(message.ChatID) == "" || message.MessageID <= 0 || !cache.Initialized() {
		return
	}
	if message.Status == "" {
		message.Status = telegramWebhookMessageStatusReceived
	}
	ttl := telegramWebhookReconcileTTL(ctx)
	state, err := json.Marshal(telegramWebhookMessageStateFrom(message))
	if err != nil {
		return
	}
	_ = cache.Instance().Set(ctx, telegramWebhookReconcileMessageKey(message.ChatID, message.MessageID), string(state), ttl)
	if strings.TrimSpace(message.MediaGroupID) != "" {
		_ = cache.Instance().Set(ctx, telegramWebhookReconcileGroupKey(message.ChatID, message.MediaGroupID), message, ttl)
	}
	mutex := lock.NewConfig(5*time.Second, 20*time.Millisecond).WithoutWatchDog().Mutex("youban_publish:tg:webhook:recent:" + strings.TrimSpace(message.ChatID))
	if err := mutex.Lock(ctx); err != nil {
		return
	}
	defer func() { _ = mutex.Unlock(context.Background()) }()
	key := telegramWebhookRecentKey(message.ChatID)
	recent := make([]telegramWebhookReconcileMessage, 0, telegramWebhookRecentMessagesCapacity)
	if value, err := cache.Instance().Get(ctx, key); err == nil && !value.IsNil() {
		_ = json.Unmarshal([]byte(value.String()), &recent)
	}
	filtered := recent[:0]
	for _, item := range recent {
		if item.MessageID != message.MessageID {
			filtered = append(filtered, item)
		}
	}
	recent = append([]telegramWebhookReconcileMessage{message}, filtered...)
	capacity := telegramWebhookRecentCapacity(ctx)
	if len(recent) > capacity {
		recent = recent[:capacity]
	}
	payload, err := json.Marshal(recent)
	if err == nil {
		_ = cache.Instance().Set(ctx, key, string(payload), telegramWebhookReconcileTTL(ctx))
	}
}

type telegramWebhookAttemptIndexEntry struct {
	AttemptID  int64     `json:"attemptId"`
	JobID      int64     `json:"jobId"`
	Phase      string    `json:"phase"`
	CreatedAt  time.Time `json:"createdAt"`
	TargetChat string    `json:"targetChat"`
}

func telegramWebhookAttemptIndexKey(chatID string) string {
	return fmt.Sprintf("youban:tg:reconcile:attempts:%s", strings.TrimSpace(chatID))
}

func indexTelegramDeliveryAttempt(ctx context.Context, attempt telegramDeliveryAttempt) {
	if attempt.Id <= 0 || strings.TrimSpace(attempt.TargetChatId) == "" || !cache.Initialized() {
		return
	}
	mutex := lock.NewConfig(5*time.Second, 20*time.Millisecond).WithoutWatchDog().Mutex("youban_publish:tg:webhook:index:" + strings.TrimSpace(attempt.TargetChatId))
	if err := mutex.Lock(ctx); err != nil {
		return
	}
	defer func() { _ = mutex.Unlock(context.Background()) }()
	key := telegramWebhookAttemptIndexKey(attempt.TargetChatId)
	entries := make([]telegramWebhookAttemptIndexEntry, 0, 8)
	if value, err := cache.Instance().Get(ctx, key); err == nil && !value.IsNil() {
		_ = json.Unmarshal([]byte(value.String()), &entries)
	}
	for _, entry := range entries {
		if entry.AttemptID == attempt.Id {
			return
		}
	}
	createdAt := time.Now()
	if attempt.CreatedAt != nil {
		createdAt = attempt.CreatedAt.Time
	}
	entries = append([]telegramWebhookAttemptIndexEntry{{
		AttemptID: attempt.Id, JobID: attempt.JobId, Phase: attempt.Phase,
		CreatedAt: createdAt, TargetChat: attempt.TargetChatId,
	}}, entries...)
	if len(entries) > 64 {
		entries = entries[:64]
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		return
	}
	_ = cache.Instance().Set(ctx, key, string(payload), telegramWebhookReconcileTTL(ctx))
}

func telegramWebhookAttemptIDs(ctx context.Context, chatID string) []int64 {
	if strings.TrimSpace(chatID) == "" || !cache.Initialized() {
		return nil
	}
	value, err := cache.Instance().Get(ctx, telegramWebhookAttemptIndexKey(chatID))
	if err != nil || value.IsNil() {
		return nil
	}
	var entries []telegramWebhookAttemptIndexEntry
	if err := json.Unmarshal([]byte(value.String()), &entries); err != nil {
		return nil
	}
	ids := make([]int64, 0, len(entries))
	for _, entry := range entries {
		if entry.AttemptID > 0 {
			ids = append(ids, entry.AttemptID)
		}
	}
	return ids
}

func mergeTelegramWebhookAttempts(primary, fallback []telegramDeliveryAttempt) []telegramDeliveryAttempt {
	seen := make(map[int64]struct{}, len(primary)+len(fallback))
	merged := make([]telegramDeliveryAttempt, 0, len(primary)+len(fallback))
	for _, attempt := range append(primary, fallback...) {
		if attempt.Id <= 0 {
			continue
		}
		if _, ok := seen[attempt.Id]; ok {
			continue
		}
		seen[attempt.Id] = struct{}{}
		merged = append(merged, attempt)
	}
	return merged
}

type botCollectChannelCandidate struct {
	Id             int64  `json:"id"`
	TelegramChatId string `json:"telegramChatId"`
	Title          string `json:"title"`
}

// botCollectChannelRelation 表示某个采集源与频道的独立订阅状态。
type botCollectChannelRelation struct {
	Id             int64  `json:"id"`
	ChannelId      int64  `json:"channelId"`
	SourceId       int64  `json:"sourceId"`
	CollectEnabled int    `json:"collectEnabled"`
	Status         string `json:"status"`
	ApprovalStatus string `json:"approvalStatus"`
}

func (h *publishCollectorAccountTaskHandler) HandleAccountTaskFailure(ctx context.Context, task *collectorin.AccountTask, cause error) error {
	if h == nil || h.publish == nil || task == nil {
		return nil
	}
	var (
		jobID   int64
		purpose string
	)
	var err error
	switch task.TaskType {
	case collectorin.AccountTaskTypeMessageMediaFallback:
		jobID, purpose, err = parseMessageMediaFallbackTaskKey(task.TaskKey)
	default:
		return nil
	}
	if err != nil {
		return err
	}
	job, err := h.publish.telegramJobById(ctx, jobID)
	if err != nil {
		return err
	}
	err = h.publish.failTelegramFallbackTask(ctx, task, job, purpose, cause)
	if errors.Is(err, errTelegramJobSuperseded) {
		logTelegramFallbackHandoff(ctx, "rejected", job, purpose, task.ID, "obsolete_failure_callback", job.Status, job.Status)
		return nil
	}
	return err
}

func accountFallbackFailureIsObsolete(purpose, sendPhase string) bool {
	return purpose == "display" && telegramSendPhaseHasDisplay(sendPhase)
}

func (h *publishCollectorAccountTaskHandler) HandleAccountTaskCompletion(ctx context.Context, task *collectorin.AccountTask, result *collectorin.AccountMediaDownloadResult) error {
	if h == nil || h.publish == nil || task == nil || result == nil || task.TaskType != collectorin.AccountTaskTypeMediaDownload {
		return nil
	}
	mediaID := parseCollectMediaTaskKey(task.TaskKey)
	if mediaID <= 0 {
		return nil
	}
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	mediaRow, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields(mediaCols.EventId).
		Where(mediaCols.Id, mediaID).
		One()
	if err != nil {
		return gerror.Wrapf(err, "读取异步账号媒体归属事件失败 mediaId:%d", mediaID)
	}
	if mediaRow.IsEmpty() {
		return gerror.Newf("异步账号媒体记录不存在 mediaId:%d", mediaID)
	}
	eventID := mediaRow[mediaCols.EventId].Int64()
	if eventID <= 0 {
		return gerror.Newf("异步账号媒体缺少归属事件 mediaId:%d", mediaID)
	}
	if result.ErrorCode == "source_gone" {
		return h.publish.discardCollectEventGroup(ctx, eventID, firstNonEmpty(result.ErrorMessage, "TG原消息已删除或已无可用媒体"))
	}
	if strings.TrimSpace(result.StoragePath) == "" && strings.TrimSpace(result.FileURL) == "" {
		return gerror.Newf("异步账号媒体下载未返回存储地址 mediaId:%d", mediaID)
	}
	data := g.Map{
		"cache_status": "ready", "file_url": result.FileURL, "storage_path": result.StoragePath,
		"source_file_id": result.Media.FileID, "source_kind": result.Media.SourceKind,
		"source_media_id": result.Media.SourceMediaID, "source_access_hash": result.Media.SourceAccessHash,
		"source_file_reference": result.Media.SourceFileReference, "source_thumb_size": result.Media.SourceThumbSize,
		"source_mime_type": result.Media.SourceMimeType, "source_dc_id": result.Media.SourceDCID,
		"source_size": result.Media.SourceSize, "error_message": "", "updated_at": gtime.Now(),
	}
	if posterURL := strings.TrimSpace(result.PosterURL); posterURL != "" {
		data["poster_url"] = posterURL
	}
	if posterPath := normalizeStoredMediaPath(result.PosterPath); posterPath != "" {
		data["poster_storage_path"] = posterPath
	}
	if _, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where(mediaCols.Id, mediaID).Update(data); err != nil {
		return gerror.Wrapf(err, "异步账号媒体结果回写失败 mediaId:%d", mediaID)
	}
	pendingCount, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Where(mediaCols.EventId, eventID).
		WhereNot(mediaCols.CacheStatus, collectMediaCacheReady).
		Count()
	if err != nil {
		return gerror.Wrapf(err, "检查异步账号媒体组完成状态失败 mediaId:%d eventId:%d", mediaID, eventID)
	}
	if pendingCount > 0 {
		return nil
	}
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(eventID).One()
	if err != nil {
		return gerror.Wrapf(err, "读取异步账号媒体事件失败 mediaId:%d eventId:%d", mediaID, eventID)
	}
	if event.IsEmpty() {
		return gerror.Newf("异步账号媒体事件不存在 mediaId:%d eventId:%d", mediaID, eventID)
	}
	if cache.Initialized() && g.Cfg().MustGet(ctx, "cache.adapter").String() == "redis" {
		key := fmt.Sprintf("%s%d", publishconsts.CollectMediaCompletionKeyPrefix, eventID)
		reserved, reserveErr := cache.Instance().SetIfNotExist(ctx, key, 1, 5*time.Minute)
		if reserveErr != nil {
			g.Log().Warningf(ctx, "采集媒体完成投递去重缓存不可用，放行任务 eventId:%d err:%+v", eventID, reserveErr)
		} else if !reserved {
			return nil
		}
	}
	// The original media-cache task can still hold its Asynq unique key when the
	// final account download finishes. Resume once after the whole group is ready.
	payload := collectMediaQueuePayloadFromEvent(event)
	payload.Reason = "account_completion"
	if _, err = h.publish.enqueueCollectMediaCacheDeferred(ctx, payload, 0); err != nil {
		if cache.Initialized() && g.Cfg().MustGet(ctx, "cache.adapter").String() == "redis" {
			key := fmt.Sprintf("%s%d", publishconsts.CollectMediaCompletionKeyPrefix, eventID)
			_, _ = cache.Instance().Remove(ctx, key)
		}
		return gerror.Wrapf(err, "异步账号媒体完成后重新投递事件失败 mediaId:%d eventId:%d", mediaID, eventID)
	}
	return nil
}

func parseCollectMediaTaskKey(key string) int64 {
	const prefix = "collect-media:"
	if !strings.HasPrefix(key, prefix) {
		return 0
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(key, prefix), 10, 64)
	return id
}

func (h *publishCollectorAccountTaskHandler) HandleAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (*collectorin.AccountMediaDownloadResult, error) {
	if h == nil || h.publish == nil || task == nil {
		return nil, gerror.New("Telegram账号任务处理参数无效")
	}
	switch task.TaskType {
	case collectorin.AccountTaskTypeRelayControl:
		return nil, h.publish.handleRelayControlTask(ctx, client, task)
	case collectorin.AccountTaskTypeRelayPrepare:
		return nil, h.publish.handleRelayPrepareTask(ctx, client, task)
	case collectorin.AccountTaskTypeChannelBotAttach:
		const prefix = "channel-bot-attach:"
		encoded := strings.TrimPrefix(task.TaskKey, prefix)
		if !strings.HasPrefix(task.TaskKey, prefix) {
			return nil, gerror.New("频道Bot管理任务参数无效")
		}
		body, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, gerror.Wrap(err, "解析频道Bot管理任务失败")
		}
		var payload channelBotAttachTaskPayload
		if err = json.Unmarshal(body, &payload); err != nil {
			return nil, gerror.Wrap(err, "解析频道Bot管理参数失败")
		}
		if payload.TenantID != task.TenantID || payload.TGAccountID != task.AccountID || len(payload.BotIDs) == 0 {
			return nil, gerror.New("频道Bot管理任务归属不一致")
		}
		bots, err := h.publish.channelCheckBots(ctx, payload.BotIDs, payload.TenantID)
		if err != nil {
			return nil, err
		}
		return nil, h.publish.attachChannelBotsWithClient(ctx, client, payload.TenantID, payload.TGAccountID, &sysin.ChannelCacheModel{ChannelId: payload.ChannelID, AccessHash: payload.AccessHash}, bots)
	case collectorin.AccountTaskTypeHistoryPage:
		return nil, h.publish.handleCollectHistoryAccountTask(ctx, client, task.HistoryTaskID)
	case collectorin.AccountTaskTypeMaterialImportHistoryPage:
		const prefix = "material-import:"
		parts := strings.Split(strings.TrimPrefix(task.TaskKey, prefix), ":")
		taskID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || taskID <= 0 || !strings.HasPrefix(task.TaskKey, prefix) {
			return nil, gerror.New("资料导入账号任务参数无效")
		}
		importTask, err := h.publish.materialImportTaskByPrimary(ctx, taskID)
		if err != nil {
			return nil, err
		}
		cache, err := h.publish.tgChannelCacheByChannelId(ctx, importTask.TenantId, importTask.TgAccountId, importTask.SourceChatId)
		if err != nil {
			return nil, err
		}
		peer, err := collectInputPeerChannel(cache)
		if err != nil {
			return nil, err
		}
		if _, err = client.Self(ctx); err != nil {
			return nil, err
		}
		return nil, h.publish.pullMaterialImportPages(ctx, client, importTask, peer, cache)
	case collectorin.AccountTaskTypeDialogCacheRefresh:
		return nil, h.publish.handleDialogCacheRefreshAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessagePushInline:
		return nil, h.publish.handleMessagePushInlineAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessageMediaFallback:
		return nil, h.publish.handleMessageMediaFallbackAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessageDeleteFallback:
		return nil, h.publish.handleMessageDeleteFallbackAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeManagedBotUsernameCheck:
		return h.publish.handleManagedBotUsernameCheckAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeManagedBotCreate:
		return h.publish.handleManagedBotCreateAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeChannelMemberSync:
		return nil, h.publish.handleChannelMemberSyncAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeTgAccountRefresh:
		return nil, h.publish.handleTgAccountRefreshTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessageRepair:
		return nil, h.publish.handleMessageRepairAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessageRepairScan:
		return h.publish.handleMessageRepairScanAccountTask(ctx, client, task)
	default:
		return nil, gerror.Newf("不支持的Telegram账号任务类型：%s", task.TaskType)
	}
}

func (s *sSysPublish) handleManagedBotUsernameCheckAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (*collectorin.AccountMediaDownloadResult, error) {
	accountID, username, err := parseManagedBotUsernameCheckTaskKey(task.TaskKey)
	if err != nil {
		return nil, err
	}
	if accountID != task.AccountID {
		return nil, gerror.New("Managed Bot用户名检测任务账号不一致")
	}
	available, err := client.API().BotsCheckUsername(ctx, username)
	if err != nil {
		return nil, gerror.Wrap(err, "检查Managed Bot用户名失败")
	}
	resultCode := managedBotUsernameOccupiedResult
	if available {
		resultCode = managedBotUsernameAvailableResult
	}
	return &collectorin.AccountMediaDownloadResult{ErrorCode: resultCode}, nil
}

func (s *sSysPublish) handleManagedBotCreateAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (*collectorin.AccountMediaDownloadResult, error) {
	payload, err := parseManagedBotCreateTaskKey(task.TaskKey)
	if err != nil {
		return nil, err
	}
	if payload.AccountID != task.AccountID {
		return nil, gerror.New("Managed Bot创建任务账号不一致")
	}
	manager, err := resolveManagedBotUser(ctx, client, payload.ManagerUsername)
	if err != nil {
		return nil, err
	}
	if !manager.BotCanManageBots {
		return nil, gerror.New("官方Bot尚未开启Bot Management Mode")
	}
	available, err := client.API().BotsCheckUsername(ctx, payload.Username)
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, gerror.New("该 Bot 用户名已被占用")
	}
	createdUser, err := client.API().BotsCreateBot(ctx, &tg.BotsCreateBotRequest{
		Name: payload.Name, Username: payload.Username,
		ManagerID: &tg.InputUser{UserID: manager.ID, AccessHash: manager.AccessHash},
	})
	if err != nil {
		return nil, err
	}
	created, ok := createdUser.(*tg.User)
	if !ok || created == nil || created.ID <= 0 || created.AccessHash == 0 {
		return nil, gerror.New("Telegram未返回新Bot的有效身份信息")
	}
	return &collectorin.AccountMediaDownloadResult{
		FileURL: firstNonEmpty(created.Username, payload.Username),
		Media:   collectorin.CollectorMediaItem{SourceMediaID: created.ID, SourceAccessHash: created.AccessHash},
	}, nil
}

// A channel lease wait is admission, not a failed Telegram send. Keep the
// existing busy error as its cause while giving Account runtime an explicit
// no-side-effect contract and a delay bounded by this execution's deadline.
type telegramFallbackAdmissionDeferredError struct {
	cause error
	delay time.Duration
}

func (e *telegramFallbackAdmissionDeferredError) Error() string               { return e.cause.Error() }
func (e *telegramFallbackAdmissionDeferredError) Unwrap() error               { return e.cause }
func (e *telegramFallbackAdmissionDeferredError) AccountTaskNotStarted() bool { return true }
func (e *telegramFallbackAdmissionDeferredError) AccountTaskRetryDelay() time.Duration {
	return e.delay
}

func telegramFallbackAdmissionWait(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		return gerror.New("协议号频道准入缺少持久发送截止时间，拒绝无限等待")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return context.DeadlineExceeded
	}
	delay := telegramStreamExecutionTTL
	if remaining < delay {
		delay = remaining
	}
	return &telegramFallbackAdmissionDeferredError{cause: telegramFallbackLaneBusyError{}, delay: delay}
}

func (s *sSysPublish) handleMessageMediaFallbackAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (err error) {
	startedAt := time.Now()
	stageStartedAt := startedAt
	var jobID int64
	stage := func(name string, fields ...interface{}) {
		duration := time.Since(stageStartedAt)
		g.Log().Infof(ctx, "协议号媒体降级阶段完成 taskId:%d jobId:%d tgAccountId:%d stage:%s stageDuration:%s totalDuration:%s fields:%v", task.ID, jobID, task.AccountID, name, duration.Round(time.Millisecond), time.Since(startedAt).Round(time.Millisecond), fields)
		stageStartedAt = time.Now()
	}
	defer func() {
		if err := ctx.Err(); err != nil {
			g.Log().Warningf(ctx, "协议号媒体降级任务上下文结束 taskId:%d tgAccountId:%d totalDuration:%s err:%+v", task.ID, task.AccountID, time.Since(startedAt).Round(time.Millisecond), err)
		}
	}()
	jobID, purpose, err := parseMessageMediaFallbackTaskKey(task.TaskKey)
	if err != nil {
		return err
	}
	job, err := s.telegramJobById(ctx, jobID)
	if err != nil {
		return err
	}
	ctx = telegramPublishTraceContext(ctx, job.Id, g.Map{
		"source": "account_fallback", "accountTaskId": task.ID, "tgAccountId": task.AccountID,
	})
	ctx = telegramPublishJobTraceContext(ctx, job)
	stage("读取任务与Job", "purpose", purpose, "jobStatus", job.Status)
	if !telegramFallbackTaskIdentity(job, purpose, task) || !telegramFallbackOwnsPhase(job, purpose) {
		logTelegramFallbackHandoff(ctx, "rejected", job, purpose, task.ID, "obsolete_task_identity", job.Status, job.Status)
		return nil
	}
	// Apply the persisted Job deadline before trying a channel slot, so
	// admission retries never create a fresh send budget or wait indefinitely.
	if job.SendDeadlineAt == nil {
		return gerror.New("协议号交接缺少持久发送截止时间，拒绝重新开始发送预算")
	}
	bounded, cancelBudget, budgetErr := s.telegramSendBudgetContext(ctx, job)
	if budgetErr != nil {
		return budgetErr
	}
	defer cancelBudget()
	ctx = bounded
	// The Account runtime gate serializes an account, not a target channel.
	// Use exactly the same channel lease key as Bot publishing before claiming.
	lease, acquired, leaseErr := s.tryTelegramPublishChannelLease(ctx, job)
	if leaseErr != nil {
		return leaseErr
	}
	if !acquired {
		logTelegramFallbackHandoff(ctx, "rejected", job, purpose, task.ID, "channel_lease_busy", job.Status, job.Status)
		return telegramFallbackAdmissionWait(ctx)
	}
	defer s.releaseTelegramChannelLease(ctx, lease)
	ctx, stopExecution := telegramStreamExecutionContext(ctx, lease)
	defer stopExecution()
	earlier, orderErr := s.telegramChannelHasEarlierActiveJob(ctx, job)
	if orderErr != nil {
		return orderErr
	}
	if earlier {
		return telegramFallbackAdmissionWait(ctx)
	}
	job, err = s.claimTelegramFallbackTask(ctx, task, job, purpose)
	if errors.Is(err, errTelegramJobSuperseded) {
		// A legacy key or an obsolete operation cannot authorize new delivery.
		// Recovery creates an operation-bound task for an outstanding marker.
		return nil
	}
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, telegramFallbackTaskContextKey{}, task)
	g.Log().Infof(ctx, "协议号媒体降级任务开始执行 taskId:%d jobId:%d tgAccountId:%d purpose:%s attempt:%d/%d", task.ID, job.Id, task.AccountID, purpose, task.AttemptCount, task.MaxAttempts)
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return err
	}
	if task.AccountID != channel.TgAccountId {
		return gerror.New("媒体降级发送账号任务与目标频道账号不一致")
	}
	stage("读取频道")
	media, err := s.telegramJobMedia(ctx, job, purpose)
	if err != nil {
		return err
	}
	if purpose == "display" {
		media, err = s.selectTelegramDisplayMediaForTenant(ctx, job, media)
		if err != nil {
			return err
		}
	}
	stage("读取并选择媒体", "mediaCount", len(media))
	var verifyMedia []*telegramMediaItem
	if purpose == "display" {
		verifyMedia, err = s.telegramJobMedia(ctx, job, "verify")
		if err != nil {
			return err
		}
	}
	stage("读取验证媒体", "verifyMediaCount", len(verifyMedia))
	caption := ""
	if purpose == "display" {
		caption, err = s.telegramJobCaption(ctx, job)
		if err != nil {
			return err
		}
	}
	// A display fallback owns only the display phase. Verification protection is
	// prepared later by the normal verification send with a fresh task context.
	caption, err = s.applyTelegramJobContentProtection(ctx, job, caption, media)
	if err != nil {
		return err
	}
	stage("媒体内容保护与缩略图准备", "mediaCount", len(media), "verifyMediaCount", len(verifyMedia))
	var messages []*telegramSentMessage
	if !accountFallbackPhaseConfirmed(job.SendPhase.String(), purpose) {
		restored, restoreErr := s.restoreConfirmedTelegramResponse(ctx, job, purpose, media)
		if restoreErr != nil {
			return restoreErr
		}
		if restored {
			job, err = s.telegramJobById(ctx, job.Id)
			if err != nil {
				return err
			}
		}
	}
	phaseConfirmed := accountFallbackPhaseConfirmed(job.SendPhase.String(), purpose)
	if !phaseConfirmed {
		messages, err = s.sendAccountFallbackPhase(ctx, client, channel, job, task.AccountID, purpose, caption, media)
		if err != nil {
			return err
		}
		stage("发送并确认媒体", "purpose", purpose, "messageCount", len(messages))
	} else {
		stage("跳过已确认媒体阶段", "purpose", purpose, "sendPhase", job.SendPhase.String())
	}
	if purpose == "display" {
		if len(verifyMedia) == 0 {
			g.Log().Infof(ctx, "协议号媒体降级任务发送成功 taskId:%d jobId:%d tgAccountId:%d displayMessages:%d verifyMessages:0", task.ID, job.Id, task.AccountID, len(messages))
			if err := s.checkTelegramFallbackExecution(ctx, job, purpose); err != nil {
				return err
			}
			return s.completeConfirmedTelegramFallbackTask(ctx, task, job, purpose, false)
		}
		g.Log().Infof(ctx, "协议号展示媒体降级完成，验证媒体交回正常队列 taskId:%d jobId:%d tgAccountId:%d displayMessages:%d verifyMedia:%d", task.ID, job.Id, task.AccountID, len(messages), len(verifyMedia))
		s.appendTelegramJobLog(ctx, job, "account_fallback", "display_confirmed", "协议号展示媒体已确认，验证媒体交回正常发送队列")
		return s.resumeTelegramVerificationAfterFallback(ctx, job, task)
	}
	g.Log().Infof(ctx, "协议号验证媒体降级任务发送成功 taskId:%d jobId:%d tgAccountId:%d verifyMessages:%d", task.ID, job.Id, task.AccountID, len(messages))
	if err := s.checkTelegramFallbackExecution(ctx, job, purpose); err != nil {
		return err
	}
	return s.completeConfirmedTelegramFallbackTask(ctx, task, job, purpose, true)
}

func accountFallbackPhaseConfirmed(sendPhase, purpose string) bool {
	return purpose == "display" && telegramSendPhaseHasDisplay(sendPhase) ||
		purpose == "verify" && sendPhase == telegramSendPhaseVerifyConfirmed
}

func (s *sSysPublish) sendAccountFallbackPhase(ctx context.Context, client *telegram.Client, channel *messagePushChannel, job telegramJobRecord, accountID int64, purpose, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if err := s.checkTelegramFallbackExecution(ctx, job, purpose); err != nil {
		return nil, err
	}
	attempt, messages, err := s.prepareAccountFallbackAttempt(ctx, client, channel, job, purpose, len(media))
	if err != nil {
		return nil, err
	}
	stopAttemptHeartbeat := s.startTelegramAttemptHeartbeat(attempt.Id)
	defer stopAttemptHeartbeat()
	if len(messages) == 0 {
		if err := s.checkTelegramFallbackExecution(ctx, job, purpose); err != nil {
			return nil, err
		}
		peer, peerErr := messagePushInputPeer(channel)
		if peerErr != nil {
			return nil, peerErr
		}
		caption = telegramCaptionWithJobMarker(caption, job.Id, purpose) + telegramAttemptMarker(attempt.AttemptToken)
		sendStartedAt := time.Now()
		logTelegramProtocolSend(ctx, job, attempt, purpose, len(media), nil, "start", 0, nil)
		messages, err = s.sendMessageTemplateWithTgClient(ctx, client, peer, caption, media, nil, accountID, "")
		logTelegramProtocolSend(ctx, job, attempt, purpose, len(media), messages, "result", time.Since(sendStartedAt), err)
		if err != nil {
			if canceled, cancelErr := recordCanceledSendOutcome(ctx, job, attempt, messages, err); cancelErr != nil {
				return nil, cancelErr
			} else if canceled {
				return nil, errTelegramJobSuperseded
			}
		}
		if err != nil {
			if !telegramFallbackBotFailureKnown(err) {
				waitCtx, cancel := telegramFinalizeContext(ctx, "account_fallback.await_facts")
				defer cancel()
				if persistErr := s.recordTelegramAttemptMessages(waitCtx, attempt, messages, "response"); persistErr != nil {
					return nil, persistErr
				}
				if waitErr := s.waitTelegramDeliveryWebhook(waitCtx, attempt, err); waitErr != nil {
					return nil, waitErr
				}
				return nil, errTelegramAwaitingWebhook
			}
			if cleanupErr := s.cleanupPartialAccountMediaSend(ctx, client, job, channel, purpose, messages, err); cleanupErr != nil {
				return nil, cleanupErr
			}
			if len(messages) > 0 {
				// Cleanup evidence must be reconciled before another token
				// can represent the partially delivered album.
				return nil, errTelegramAttemptNeedsReconcile
			}
			if persistErr := s.recordTelegramFallbackAccountRejection(ctx, job, attempt.Id, purpose, err); persistErr != nil {
				return nil, persistErr
			}
			return nil, gerror.Wrap(err, "协议号媒体降级发送失败")
		}
	}
	bindAccountFallbackMessagesToMedia(messages, media)
	if err = validateTelegramMediaSendResult(purpose, media, messages); err != nil {
		s.appendTelegramJobLog(ctx, job, "account", "incomplete", err.Error())
		return nil, err
	}
	// Preserve the API response even if ownership expired while sending. A
	// stale owner may record facts but must not advance the current operation.
	responseCtx, cancelResponse := telegramFinalizeContext(ctx, "account_fallback.response")
	defer cancelResponse()
	if err = persistTelegramAttemptResponseDurably(responseCtx, attempt, messages); err != nil {
		return nil, err
	}
	if err = s.checkTelegramFallbackExecution(responseCtx, job, purpose); err != nil {
		return nil, err
	}
	if err = s.confirmTelegramAPISuccess(ctx, job, attempt, messages); err != nil {
		g.Log().Warningf(ctx, "协议号媒体发送成功但本地确认失败 jobId:%d attemptId:%d purpose:%s messageCount:%d err:%+v", job.Id, attempt.Id, purpose, len(messages), err)
		return nil, telegramDeliveryUncertainError(err)
	}
	if err = s.updateTelegramMediaFileIds(ctx, messages); err != nil {
		g.Log().Warningf(ctx, "协议号媒体file_id更新失败 jobId:%d purpose:%s err:%+v", job.Id, purpose, err)
	}
	s.appendTelegramJobLog(ctx, job, "account", "confirmed", fmt.Sprintf("媒体阶段确认完成 purpose:%s expected:%d confirmed:%d", purpose, len(media), len(messages)))
	return messages, nil
}

func (s *sSysPublish) prepareAccountFallbackAttempt(ctx context.Context, client *telegram.Client, channel *messagePushChannel, job telegramJobRecord, purpose string, expectedCount int) (telegramDeliveryAttempt, []*telegramSentMessage, error) {
	attempt, err := s.activeTelegramDeliveryAttempt(ctx, job.Id, purpose)
	if err != nil {
		return attempt, nil, err
	}
	pendingCleanup, err := g.DB().Model(publishTgDeliveryStepTable).Ctx(ctx).Where("job_id", job.Id).
		Where("step_key", fmt.Sprintf("%s%d", telegramPartialCleanupCheckpoint, attempt.Id)).Where("status", telegramDeliveryStepRunning).Count()
	if err != nil {
		return attempt, nil, err
	}
	if pendingCleanup > 0 {
		task, _ := ctx.Value(telegramFallbackTaskContextKey{}).(*collectorin.AccountTask)
		if task == nil {
			return attempt, nil, errTelegramJobSuperseded
		}
		checkpoint, err := g.DB().Model(publishTgDeliveryStepTable).Ctx(ctx).
			Where("job_id", job.Id).Where("step_key", telegramFallbackTransportStep(purpose)).
			Where("attempt_id", attempt.Id).Where("error_message", task.TaskKey).One()
		if err != nil {
			return attempt, nil, err
		}
		if checkpoint.IsEmpty() {
			return attempt, nil, errTelegramAttemptNeedsReconcile
		}
		return s.resumePartialAccountAlbum(ctx, client, channel, job, attempt, expectedCount)
	}
	// Protocol-account fallback is a send path only. Do not pull channel history
	// to deduplicate or reconcile an ambiguous Bot result; Webhook/Redis is the
	// only remote fact source. Existing local messages are handled by the
	// partial-cleanup checkpoint above.
	attempt, err = s.accountFallbackDeliveryAttempt(ctx, job, purpose, expectedCount)
	return attempt, nil, err
}

const telegramPartialCleanupCheckpoint = "partial_album_cleanup:"

func (s *sSysPublish) resumePartialAccountAlbum(ctx context.Context, client *telegram.Client, channel *messagePushChannel, job telegramJobRecord, attempt telegramDeliveryAttempt, expected int) (telegramDeliveryAttempt, []*telegramSentMessage, error) {
	var messages []*telegramSentMessage
	if err := g.DB().Model(publishTgMessageTable).Ctx(ctx).Fields("tg_message_id AS message_id,purpose,media_group_id").
		Where("job_id", job.Id).Where("purpose", attempt.Phase).WhereIn("status", []string{"sent", "undeletable"}).Scan(&messages); err != nil {
		return attempt, nil, gerror.Wrap(err, "读取待清理半组消息失败")
	}
	if err := s.cleanupPartialAccountMediaSend(ctx, client, job, channel, attempt.Phase, messages, gerror.New("恢复半组相册")); err != nil {
		return attempt, nil, err
	}
	// Delete acknowledgment and the local deleted projection must both succeed
	// before a different token can be created. Keep old evidence for late posts.
	var next telegramDeliveryAttempt
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var err error
		next, err = replaceCleanedPartialAttemptTx(ctx, tx, job.Id, attempt.Id, expected)
		return err
	})
	return next, nil, err
}

func (s *sSysPublish) accountFallbackDeliveryAttempt(ctx context.Context, job telegramJobRecord, purpose string, expectedCount int) (telegramDeliveryAttempt, error) {
	var attempt telegramDeliveryAttempt
	if err := s.prepareTelegramFallbackTransport(ctx, job, purpose); err != nil {
		return attempt, err
	}
	attempt, err := s.prepareTelegramDeliveryAttempt(ctx, job, purpose, expectedCount, true)
	if err != nil {
		return attempt, err
	}
	return attempt, s.markTelegramFallbackTransport(ctx, job, purpose, attempt.Id)
}

func bindAccountFallbackMessagesToMedia(messages []*telegramSentMessage, media []*telegramMediaItem) {
	for index, message := range messages {
		if message == nil || message.MediaId > 0 || index >= len(media) || media[index] == nil {
			continue
		}
		message.MediaId = media[index].Id
	}
}

func parseMessageMediaFallbackTaskKey(taskKey string) (int64, string, error) {
	const prefix = "message-media-fallback:"
	parts := strings.Split(strings.TrimPrefix(taskKey, prefix), ":")
	if !strings.HasPrefix(taskKey, prefix) || (len(parts) != 2 && len(parts) != 4) {
		return 0, "", gerror.New("媒体降级发送账号任务参数无效")
	}
	jobID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || jobID <= 0 || (parts[1] != "display" && parts[1] != "verify") {
		return 0, "", gerror.New("媒体降级发送账号任务参数无效")
	}
	return jobID, parts[1], nil
}

func telegramMediaPurposeLabel(purpose string) string {
	if purpose == "verify" {
		return "验证视频"
	}
	return "展示资料"
}

func validateTelegramMediaSendResult(purpose string, media []*telegramMediaItem, messages []*telegramSentMessage) error {
	if len(media) == 0 {
		return nil
	}
	mediaIDs := make(map[int64]struct{}, len(media))
	for _, item := range media {
		if item != nil && item.Id > 0 {
			mediaIDs[item.Id] = struct{}{}
		}
	}
	sentIDs := make(map[int64]struct{}, len(messages))
	for _, item := range messages {
		if item != nil && item.MessageId > 0 && item.Purpose != purpose {
			return gerror.Newf("Telegram媒体发送结果用途不一致：期望%s，实际%s，消息%d", purpose, item.Purpose, item.MessageId)
		}
		if item != nil && item.MessageId > 0 && item.MediaId > 0 {
			sentIDs[item.MediaId] = struct{}{}
		}
	}
	for mediaID := range mediaIDs {
		if _, ok := sentIDs[mediaID]; !ok {
			return gerror.Newf("Telegram%s媒体发送结果不完整：期望%d个媒体，确认%d个，缺少mediaId=%d", purpose, len(mediaIDs), len(sentIDs), mediaID)
		}
	}
	if len(mediaIDs) == 0 && len(messages) < len(media) {
		return gerror.Newf("Telegram%s媒体发送结果不完整：期望%d个媒体，确认%d条消息", purpose, len(media), len(messages))
	}
	return nil
}

func mediaFallbackTaskCanSkip(jobStatus string, sentMessageCount int) bool {
	return jobStatus == "sent" && sentMessageCount > 0
}

func (s *sSysPublish) telegramJobSentMessageCount(ctx context.Context, jobID int64, purpose string) (int, error) {
	count, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("job_id", jobID).
		Where("purpose", purpose).
		Where("status", "sent").
		Count()
	if err != nil {
		return 0, gerror.Wrap(err, "读取协议号媒体降级消息数量失败")
	}
	return count, nil
}

func (s *sSysPublish) handleMessagePushInlineAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (err error) {
	startedAt := time.Now()
	jobId, mode, err := parseMessagePushAccountTaskKey(task.TaskKey)
	if err != nil {
		return err
	}
	accountOnly := mode == "account"
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil || !accountTaskFinalAttempt(task) {
			return
		}
		message := "Telegram消息推送连续失败，账号任务已停止：" + err.Error()
		s.failMessagePushJob(ctx, job, message)
		g.Log().Errorf(ctx, "消息推送账号任务最终失败 taskId:%d jobId:%d tgAccountId:%d attempt:%d/%d err:%+v", task.ID, job.Id, task.AccountID, task.AttemptCount, task.MaxAttempts, err)
	}()
	if job.Status == sysin.MessagePushStatusSent {
		return nil
	}
	if accountOnly {
		s.appendTelegramJobLog(ctx, job, "account_send", "started", fmt.Sprintf("开始协议号发送 accountTaskId:%d tgAccountId:%d", task.ID, task.AccountID))
	} else {
		s.appendTelegramJobLog(ctx, job, "inline_send", "started", fmt.Sprintf("开始Inline发布 accountTaskId:%d tgAccountId:%d", task.ID, task.AccountID))
	}
	templateId, err := messagePushTemplateIdFromOperationNo(job.OperationNo)
	if err != nil {
		return err
	}
	template, err := s.messageTemplate(ctx, templateId, job.TenantId)
	if err != nil {
		return err
	}
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return err
	}
	if task.AccountID != channel.TgAccountId {
		return gerror.New("Inline账号任务与目标频道账号不一致")
	}
	if accountOnly {
		s.appendTelegramJobLog(ctx, job, "account_send", "started", fmt.Sprintf("开始执行协议号降级 accountTaskId:%d tgAccountId:%d", task.ID, task.AccountID))
		return s.sendMessageTemplateByAccountClient(ctx, client, channel, job, template)
	}
	if validationErr := validateInlinePublishTemplate(template); validationErr != nil {
		g.Log().Errorf(ctx, "Inline发布协议校验失败，进入Bot与协议号降级链路 jobId:%d serial:%s err:%+v", job.Id, template.SerialNo, validationErr)
		s.appendTelegramJobLog(ctx, job, "inline_send", "skipped", "Inline协议校验失败，进入降级链路："+validationErr.Error())
		return s.fallbackMessagePushInline(ctx, client, channel, job, template, validationErr)
	}
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return err
	}
	botUsername, err := inlineBotUsername(ctx)
	if err != nil {
		return err
	}
	messages, err := sendInlineTemplateWithClient(ctx, client, peer, botUsername, template.SerialNo)
	if err != nil {
		inlineErr := gerror.Wrapf(err, "Inline推送失败 serial:%s", template.SerialNo)
		s.appendTelegramJobLog(ctx, job, "inline_send", "failed", fmt.Sprintf("Inline请求失败 duration:%s err:%v", time.Since(startedAt), inlineErr))
		return s.fallbackMessagePushInline(ctx, client, channel, job, template, inlineErr)
	}
	if len(messages) == 0 {
		return s.fallbackMessagePushInline(ctx, client, channel, job, template, gerror.New("Inline推送未返回Telegram消息记录"))
	}
	if err = s.completeMessagePushJob(ctx, job, messages, "更新Inline消息推送任务状态失败"); err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "inline_send", sysin.MessagePushStatusSent, fmt.Sprintf("账号服务Inline机器人消息模板推送成功 duration:%s", time.Since(startedAt)))
	return nil
}

func parseMessagePushAccountTaskKey(taskKey string) (jobId int64, mode string, err error) {
	const prefix = "message-push-inline:"
	if !strings.HasPrefix(taskKey, prefix) {
		return 0, "", gerror.New("消息推送账号任务参数无效")
	}
	parts := strings.Split(strings.TrimPrefix(taskKey, prefix), ":")
	if len(parts) == 0 || len(parts) > 2 {
		return 0, "", gerror.New("消息推送账号任务参数无效")
	}
	jobId, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || jobId <= 0 {
		return 0, "", gerror.New("消息推送账号任务Job ID无效")
	}
	mode = "inline"
	if len(parts) == 2 {
		mode = parts[1]
	}
	if mode != "inline" && mode != "account" {
		return 0, "", gerror.New("消息推送账号任务模式无效")
	}
	return jobId, mode, nil
}

func accountTaskFinalAttempt(task *collectorin.AccountTask) bool {
	return task != nil && task.MaxAttempts > 0 && task.AttemptCount >= task.MaxAttempts
}

func validateInlinePublishTemplate(template *sysin.MessageTemplateModel) error {
	if template == nil {
		return gerror.New("Inline模板为空")
	}
	if strings.TrimSpace(template.SerialNo) == "" {
		return gerror.New("Inline模板编号为空")
	}
	media := inlineDisplayMedia(template)
	if len(media) > 1 {
		return gerror.New("仅支持单个媒体，当前媒体数量超过1")
	}
	if len(media) == 1 {
		item := media[0]
		if item == nil || !strings.EqualFold(strings.TrimSpace(item.MediaType), "image") {
			return gerror.New("仅支持单张图片，视频或媒体组改用协议号")
		}
		if strings.TrimSpace(item.TgFileId) == "" && !isInlineHTTPSURL(firstNonEmpty(item.FileUrl, item.StoragePath)) {
			return gerror.New("图片缺少有效Telegram文件ID或公网HTTPS地址")
		}
		if len([]rune(template.Text)) > 1024 {
			return gerror.New("图片文案超过 Telegram BOT 限制，已自动使用协议号发送")
		}
	} else if len([]rune(template.Text)) > 4096 {
		return gerror.New("Inline文案超过Telegram允许的4096字符")
	}
	if strings.TrimSpace(template.ButtonConfig) != "" {
		var config sysin.MessageTemplateButtonConfig
		if err := json.Unmarshal([]byte(template.ButtonConfig), &config); err != nil {
			return gerror.Wrap(err, "Inline按钮配置不是有效JSON")
		}
		if config.Mode != "inline" {
			return gerror.New("Inline按钮配置模式必须是inline")
		}
		for rowIndex, row := range config.Rows {
			for buttonIndex, button := range row {
				if strings.TrimSpace(button.Text) == "" {
					return gerror.Newf("Inline按钮文本为空，位置：%d-%d", rowIndex+1, buttonIndex+1)
				}
				if !isInlineHTTPSURL(inlinePublishButtonURL(button.URL)) {
					return gerror.Newf("Inline按钮链接不是有效HTTPS地址，位置：%d-%d，原值：%s", rowIndex+1, buttonIndex+1, button.URL)
				}
			}
		}
	}
	return nil
}

func isInlineHTTPSURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func inlinePublishButtonURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "@") && len(raw) > 1 {
		return "https://t.me/" + strings.TrimPrefix(raw, "@")
	}
	return raw
}

func (s *sSysPublish) fallbackMessagePushInline(ctx context.Context, client *telegram.Client, channel *messagePushChannel, job telegramJobRecord, template *sysin.MessageTemplateModel, inlineErr error) error {
	s.appendTelegramJobLog(ctx, job, "inline_send", "fallback", "Inline推送失败，改用官方Bot上传："+inlineErr.Error())
	if !s.botCanAccessChat(ctx, job.TargetChatId) {
		s.appendTelegramJobLog(ctx, job, "bot_upload", "skipped", "官方Bot未加入目标群，跳过Bot发送，改用协议号发送")
		return s.sendMessageTemplateByAccountClient(ctx, client, channel, job, template)
	}
	fallbackMessages, fallbackErr := s.sendMessageTemplateByBot(ctx, job, template, messageTemplateTelegramMedia(template))
	if fallbackErr == nil {
		if completeErr := s.completeMessagePushJob(ctx, job, fallbackMessages, "更新Bot降级推送任务状态失败"); completeErr != nil {
			return completeErr
		}
		s.appendTelegramJobLog(ctx, job, "bot_upload", sysin.MessagePushStatusSent, "Inline推送超时，已由官方Bot降级发送成功")
		return nil
	}
	s.appendTelegramJobLog(ctx, job, "bot_upload", "failed", "官方Bot发送失败，改用协议号发送："+fallbackErr.Error())
	return s.sendMessageTemplateByAccountClient(ctx, client, channel, job, template)
}

func (s *sSysPublish) botCanAccessChat(ctx context.Context, chatID string) bool {
	cacheKey := officialBotChatUnavailableCacheKey(chatID)
	if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
		return false
	}
	token, err := botService.SysBot().OfficialBotToken(ctx)
	if err != nil {
		return false
	}
	bot, err := s.telegramBot(ctx, token)
	if err != nil {
		return false
	}
	me, err := bot.GetMe(ctx)
	if err != nil {
		return false
	}
	member, err := bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{ChatID: normalizeTelegramChannelChatID(chatID), UserID: me.ID})
	if err != nil || member == nil {
		return false
	}
	canSend := member.Type != models.ChatMemberTypeLeft && member.Type != models.ChatMemberTypeBanned && telegramBotCanSendMessage(member)
	if !canSend {
		_ = cache.Instance().Set(ctx, cacheKey, 1, 24*time.Hour)
	}
	return canSend
}

func officialBotChatUnavailableCacheKey(chatID string) string {
	return "youban_publish:official_bot_chat_unavailable:" + normalizeTelegramChannelChatID(chatID)
}

func (s *sSysPublish) sendMessageTemplateByAccountClient(ctx context.Context, client *telegram.Client, channel *messagePushChannel, job telegramJobRecord, template *sysin.MessageTemplateModel) error {
	if client == nil || channel == nil {
		return gerror.New("协议号降级发送客户端或目标无效")
	}
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return err
	}
	media := messageTemplateTelegramMedia(template)
	templateHash := messageTemplateHash(template)
	var source *messageTemplateForwardSource
	if !messageTemplateRequiresSanitizedUpload(media) {
		source, err = s.messageTemplateForwardSource(ctx, channel.TgAccountId, templateHash)
		if err != nil {
			g.Log().Warningf(ctx, "读取消息模板复用源失败 jobId:%d tgAccountId:%d templateHash:%s err:%+v", job.Id, channel.TgAccountId, templateHash, err)
			source = nil
		}
	}
	sent, err := s.sendMessageTemplateWithTgClient(ctx, client, peer, telegramRichTextHTML(template.Text), media, source, channel.TgAccountId, templateHash)
	if err != nil {
		return gerror.Wrap(err, "协议号降级发送失败")
	}
	if err = s.completeMessagePushJob(ctx, job, sent, "更新协议号降级任务状态失败"); err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "account_send", sysin.MessagePushStatusSent, "Inline和Bot均不可用，已由协议号发送成功")
	return nil
}

func (p *publishCollectorAccountMediaProvider) ResolvePeer(ctx context.Context, tenantID, accountID int64, chatID string, client *telegram.Client) (tg.InputPeerClass, error) {
	if p == nil || p.publish == nil {
		return nil, gerror.New("Telegram账号媒体 Peer 解析器未初始化")
	}
	return p.publish.collectMediaInputPeer(ctx, tenantID, accountID, client, chatID)
}

func (p *publishCollectorAccountMediaProvider) StoreMedia(ctx context.Context, task *collectorin.AccountTask, localPath string) (*collectorin.AccountMediaDownloadResult, error) {
	if p == nil || p.publish == nil || task == nil {
		return nil, gerror.New("Telegram账号媒体存储参数无效")
	}
	ctx = collectMediaRuntimeContext(ctx, task.MediaOwnerAccountID)
	md5Value, mediaSize, mimeType, err := calculateTelegramMediaFingerprint(localPath, task.Media.Type, task.Media.SourceMimeType)
	if err != nil {
		return nil, err
	}
	forceRefresh := strings.HasPrefix(task.TaskKey, "profile-media-refresh:")
	if cached, ok, cacheErr := lookupTelegramCollectorMediaCache(ctx, task.Media.Type, md5Value, mediaSize, mimeType); !forceRefresh && cacheErr != nil {
		return nil, cacheErr
	} else if !forceRefresh && ok {
		task.Media.FileURL = telegramCollectorMediaCacheURL(cached)
		task.Media.StoragePath = cached.StoragePath
		task.Media.FileMD5 = md5Value
		task.Media.SourceSize = mediaSize
		task.Media.FilePHash = cached.PHash
		task.Media.PosterURL = cached.PosterURL
		return &collectorin.AccountMediaDownloadResult{FileURL: task.Media.FileURL, StoragePath: task.Media.StoragePath, PosterURL: cached.PosterURL, PosterPath: cached.PosterStoragePath, Media: task.Media}, nil
	}
	attachment, err := p.publish.uploadCollectMediaToStorage(ctx, task.Media.Type, localPath)
	if err != nil {
		return nil, err
	}
	item := task.Media
	item.FileURL = strings.TrimSpace(attachment.FileUrl)
	item.StoragePath = normalizeStoredMediaPath(attachment.Path)
	item.FileMD5 = md5Value
	item.SourceSize = mediaSize
	item.SourceMimeType = mimeType
	identityKey := strings.TrimSpace(item.FileUniqueID)
	if identityKey == "" && item.SourceMediaID > 0 {
		identityKey = fmt.Sprintf("media:%d", item.SourceMediaID)
	}
	if identityKey != "" {
		identityFingerprint := collectorservice.BuildMediaIdentityFingerprint(identityKey, 0, item.SourceDCID, telegramCollectorMediaKind(item.Type), item.SourceThumbSize)
		if saveErr := collectorservice.Collector().SaveMediaReady(ctx, &collectorin.MediaCacheEntry{
			Fingerprint: identityFingerprint, FileURL: item.FileURL, StoragePath: item.StoragePath,
			Kind: telegramCollectorMediaKind(item.Type), MimeType: mimeType, Size: mediaSize,
		}, 0); saveErr != nil {
			return nil, gerror.Wrap(saveErr, "保存Telegram账号媒体身份复用索引失败")
		}
	}
	return &collectorin.AccountMediaDownloadResult{
		AttachmentID: attachment.Id, FileURL: item.FileURL, StoragePath: item.StoragePath, Media: item,
	}, nil
}

func (p *publishCollectorAccountMediaProvider) StoreMediaPoster(ctx context.Context, task *collectorin.AccountTask, localPath string) (string, string, error) {
	if p == nil || p.publish == nil || task == nil {
		return "", "", gerror.New("Telegram账号媒体封面存储参数无效")
	}
	ctx = collectMediaRuntimeContext(ctx, task.MediaOwnerAccountID)
	md5Value, mediaSize, mimeType, err := calculateTelegramMediaFingerprint(localPath, "image", "image/jpeg")
	if err != nil {
		return "", "", err
	}
	if cached, ok, cacheErr := lookupTelegramCollectorMediaCache(ctx, "image", md5Value, mediaSize, mimeType); cacheErr != nil {
		return "", "", cacheErr
	} else if ok {
		return telegramCollectorMediaCacheURL(cached), normalizeStoredMediaPath(cached.StoragePath), nil
	}
	attachment, err := p.publish.uploadCollectMediaToStorage(ctx, "image", localPath)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(attachment.FileUrl), normalizeStoredMediaPath(attachment.Path), nil
}

func collectorMediaItemFromCollect(item collectMediaItem) collectorin.CollectorMediaItem {
	return collectorin.CollectorMediaItem{
		Type: item.Type, Purpose: item.Purpose, FileID: item.FileId, FileUniqueID: item.FileUniqueId, FileURL: item.FileUrl,
		StoragePath: item.StoragePath, PosterURL: item.PosterUrl, FileMD5: item.FileMd5,
		FilePHash: item.FilePhash, SourceKind: item.SourceKind, SourceMediaID: item.SourceMediaId,
		SourceAccessHash: item.SourceAccessHash, SourceFileReference: append([]byte(nil), item.SourceFileReference...),
		SourceThumbSize: item.SourceThumbSize, SourceMimeType: item.SourceMimeType,
		SourceDCID: item.SourceDCId, SourceSize: item.SourceSize, DebugMetaJSON: item.DebugMetaJson,
	}
}

func collectMediaItemFromCollector(item collectorin.CollectorMediaItem) collectMediaItem {
	return collectMediaItem{
		Type: item.Type, Purpose: item.Purpose, FileId: item.FileID, FileUniqueId: item.FileUniqueID, FileUrl: item.FileURL,
		StoragePath: item.StoragePath, PosterUrl: item.PosterURL, FileMd5: item.FileMD5,
		FilePhash: item.FilePHash, SourceKind: item.SourceKind, SourceMediaId: item.SourceMediaID,
		SourceAccessHash: item.SourceAccessHash, SourceFileReference: append([]byte(nil), item.SourceFileReference...),
		SourceThumbSize: item.SourceThumbSize, SourceMimeType: item.SourceMimeType,
		SourceDCId: item.SourceDCID, SourceSize: item.SourceSize, DebugMetaJson: item.DebugMetaJSON,
	}
}

func (h *publishCollectorDeliveryHandler) HandleCollectorDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	if h == nil || h.publish == nil || delivery == nil {
		err := gerror.New("Telegram采集交付处理参数无效")
		logPublishCollectorDeliveryStage(ctx, "delivery.handler.invalid", delivery, err)
		return err
	}
	logPublishCollectorDeliveryStage(ctx, "delivery.handler.received", delivery, nil)
	var err error
	switch delivery.SourceType {
	case collectorin.SourceTypeBot:
		err = h.publish.ingestCollectorBotDelivery(ctx, delivery)
	case collectorin.SourceTypeAccount:
		err = h.publish.ingestCollectorAccountDelivery(ctx, delivery)
	default:
		err = gerror.Newf("暂不支持的Telegram采集来源类型：%s", delivery.SourceType)
	}
	if err != nil {
		logPublishCollectorDeliveryStage(ctx, "delivery.handler.failed", delivery, err)
		return err
	}
	logPublishCollectorDeliveryStage(ctx, "delivery.handler.completed", delivery, nil)
	return nil
}

func logPublishCollectorDeliveryStage(ctx context.Context, stage string, delivery *collectorin.CollectorDelivery, err error) {
	fields := g.Map{
		"log_type":        "telegram_collect_delivery",
		"event":           "collect_delivery_stage",
		"stage":           stage,
		"status":          "ok",
		"deliveryId":      0,
		"collectEventId":  0,
		"eventId":         0,
		"tenantId":        0,
		"accountId":       0,
		"sourceId":        0,
		"sourceType":      "",
		"sourceChatId":    "",
		"sourceMessageId": 0,
		"workerRole":      "publish-delivery-handler",
	}
	if delivery != nil {
		fields["deliveryId"] = delivery.ID
		fields["collectEventId"] = delivery.EventID
		fields["eventId"] = delivery.EventID
		fields["tenantId"] = delivery.TenantID
		fields["accountId"] = delivery.AccountID
		fields["sourceId"] = delivery.SourceID
		fields["sourceType"] = delivery.SourceType
		fields["sourceChatId"] = delivery.SourceChatID
		fields["sourceMessageId"] = delivery.SourceMessageID
	}
	if err != nil {
		fields["status"] = "error"
		fields["errorType"] = fmt.Sprintf("%T", err)
		fields["error"] = err.Error()
		g.Log().Warning(ctx, "Telegram采集交付发布阶段", fields)
		return
	}
	g.Log().Info(ctx, "Telegram采集交付发布阶段", fields)
}

func (s *sSysPublish) ingestCollectorBotDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	if !s.collectGlobalEnabled(ctx) {
		logCollectorDeliverySkipped(ctx, delivery, "global_disabled")
		return nil
	}
	source, err := g.DB().Model(publishCollectSourceTable+" s").Safe().Ctx(ctx).
		InnerJoin("hg_youban_publish_tenant_vip vip", "vip.tenant_id=s.tenant_id AND vip.status=1 AND vip.level>0 AND vip.deleted_at IS NULL").
		Fields("s.id,s.tenant_id,s.account_id,s.bot_id,s.bot_collect_scope").
		Where("s.id", delivery.SourceID).
		Where("s.source_type", sysin.CollectSourceTypeBot).
		Where("s.collect_enabled", 1).
		Where("s.status", 1).
		Where("(vip.expired_at IS NULL OR vip.expired_at>?)", gtime.Now()).
		WhereNull("s.deleted_at").
		One()
	if err != nil {
		return gerror.Wrap(err, "读取Bot采集源失败")
	}
	if source.IsEmpty() {
		logCollectorDeliverySkipped(ctx, delivery, "source_unavailable")
		return nil
	}
	// Collection owns persistence and process-queue handoff. Telegram delivery
	// reconciliation is maintained by the publish worker and must not be part
	// of the collector handoff path.
	// Reject publish targets and denied subscriptions before metadata writes.
	// Accepted messages synchronize their current title before rule matching.
	blocked, err := s.collectMessageFromAccountPublishChannel(ctx, source["tenant_id"].Int64(), 0, delivery.SourceChatID, delivery.SourceMessageID)
	if err != nil {
		return gerror.Wrap(err, "检查Bot采集上架频道过滤失败")
	}
	if blocked {
		logCollectorDeliverySkipped(ctx, delivery, "self_loop_publish_target")
		g.Log().Warning(ctx, "账户采集跳过自循环上架频道", g.Map{
			"reason":   "source_chat_is_publish_target",
			"tenantId": delivery.TenantID, "sourceId": delivery.SourceID,
			"accountId": delivery.AccountID, "tgAccountId": delivery.TgAccountID,
			"sourceChatId": delivery.SourceChatID, "sourceMessageId": delivery.SourceMessageID,
		})
		logPublishCollectorDeliveryStage(ctx, "delivery.skipped.self_loop", delivery, nil)
		return nil
	}
	allowed, err := s.ensureBotCollectChannelApproval(ctx, source, delivery)
	if err != nil {
		return err
	}
	if !allowed {
		logCollectorDeliverySkipped(ctx, delivery, "approval_blocked")
		return nil
	}
	s.syncBotChannelMetadataFromUpdate(ctx, source["tenant_id"].Int64(), source["bot_id"].Int64(), delivery.RawUpdate)
	message := collectorDeliveryMessage(delivery, source["tenant_id"].Int64(), source["account_id"].Int64(), source["id"].Int64(), sysin.CollectSourceTypeBot)
	message.BotId = source["bot_id"].Int64()
	if collectorDeliveryIsEdit(delivery.RawUpdate) {
		return s.updateCollectedBotMessage(ctx, message)
	}
	return s.submitCollectorDelivery(ctx, message, delivery).Err
}

func collectorDeliveryIsEdit(raw []byte) bool {
	var update struct {
		EditedChannelPost json.RawMessage `json:"edited_channel_post"`
		EditedMessage     json.RawMessage `json:"edited_message"`
	}
	if json.Unmarshal(raw, &update) != nil {
		return false
	}
	return (len(update.EditedChannelPost) > 0 && string(update.EditedChannelPost) != "null") ||
		(len(update.EditedMessage) > 0 && string(update.EditedMessage) != "null")
}

// Edits are updates, never new collection input. In particular, an edited
// member of a historical album must not replace its complete media set.
func (s *sSysPublish) updateCollectedBotMessage(ctx context.Context, message *CollectMessage) error {
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", message.TenantId).Where("account_id", message.AccountId).
		Where("source_id", message.SourceId).Where("source_unique_key", message.SourceUniqueKey).One()
	if err != nil {
		return gerror.Wrap(err, "查找编辑消息对应采集事件失败")
	}
	if event.IsEmpty() {
		g.Log().Infof(ctx, "忽略无来源资料的编辑消息 sourceId:%d chat:%s messageId:%d", message.SourceId, message.SourceChatId, message.SourceMessageId)
		return nil
	}
	dispatches, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("event_id", event["id"].Int64()).WhereGT("profile_id", 0).All()
	if err != nil {
		return gerror.Wrap(err, "查找编辑消息对应资料失败")
	}
	if len(dispatches) == 0 {
		g.Log().Infof(ctx, "忽略未生成资料的编辑消息 eventId:%d sourceId:%d messageId:%d", event["id"].Int64(), message.SourceId, message.SourceMessageId)
		return nil
	}
	// Refresh identity only; do not replay pairing, reset its waiting window,
	// replace album members, or enqueue a new publication.
	if err = s.upsertCollectEventMedia(ctx, event, normalizeCollectMediaItems(message.Media)); err != nil {
		return err
	}
	if strings.TrimSpace(message.RawText) == "" {
		return nil
	}
	ruleIDs := make([]int64, 0, len(dispatches))
	for _, dispatch := range dispatches {
		ruleIDs = append(ruleIDs, dispatch["rule_id"].Int64())
	}
	rulesByID, err := s.collectHistoricalRules(ctx, event, message.TenantId, message.AccountId, ruleIDs)
	if err != nil {
		return gerror.Wrap(err, "读取编辑消息的完整采集规则失败")
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, dispatch := range dispatches {
			rule := rulesByID[dispatch["rule_id"].Int64()]
			if rule.IsEmpty() {
				g.Log().Warningf(ctx, "跳过已删除规则的来源消息编辑 eventId:%d dispatchId:%d ruleId:%d", event["id"].Int64(), dispatch["id"].Int64(), dispatch["rule_id"].Int64())
				continue
			}
			cols := idao.ContentProfile.Columns()
			if _, updateErr := tx.Model(idao.ContentProfile.Table()).
				Where(cols.Id, dispatch["profile_id"].Int64()).
				Where("EXISTS (SELECT 1 FROM "+publishProfileStateTable+" ps WHERE ps.profile_id=? AND ps.tenant_id=? AND ps.account_id=? AND ps.deleted_at IS NULL)", dispatch["profile_id"].Int64(), message.TenantId, message.AccountId).
				Data(g.Map{cols.PlainText: normalizeCollectedBotEditText(message.RawText, rule)}).Update(); updateErr != nil {
				return updateErr
			}
		}
		_, updateErr := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Where("id", event["id"].Int64()).
			Data(g.Map{"raw_text": message.RawText, "text_hash": collectHash(message.RawText)}).Update()
		return updateErr
	})
	if err != nil {
		return err
	}
	for _, dispatch := range dispatches {
		if err := s.syncProfileNoteIndex(ctx, dispatch["profile_id"].Int64()); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCollectedBotEditText(rawText string, rule gdb.Record) string {
	text := normalizeCollectRuleBody(rawText, rule)
	if suffix := strings.TrimSpace(rule["intro_fee_suffix"].String()); suffix != "" {
		amountText := normalizeCollectRuleBodyWithoutTruncate(rawText, rule)
		text = applyCollectIntroFeeSuffix(text, amountText, suffix)
	}
	return strings.TrimSpace(text)
}

func (s *sSysPublish) ensureBotCollectChannelApproval(ctx context.Context, source gdb.Record, delivery *collectorin.CollectorDelivery) (allowedResult bool, resultErr error) {
	tenantID, accountID, botID := source["tenant_id"].Int64(), source["account_id"].Int64(), source["bot_id"].Int64()
	chatID := normalizeTelegramChannelChatID(delivery.SourceChatID)
	if tenantID <= 0 || accountID <= 0 || source["id"].Int64() <= 0 || botID <= 0 || chatID == "" {
		return false, nil
	}
	// Cache only explicit rejection; an allow decision always checks the database.
	rejectedKey := s.collectApprovalRejectedKey(ctx, tenantID, accountID, source["id"].Int64(), botID, chatID)
	cacheRejected := false
	if rejectedKey != "" {
		marker, cacheErr := cache.Instance().Get(ctx, rejectedKey)
		if cacheErr == nil && marker != nil && marker.String() == "rejected" {
			logCollectApprovalCache(ctx, delivery, accountID, collectApprovalCacheHit)
			return false, nil
		}
		if cacheErr != nil {
			logCollectApprovalCache(ctx, delivery, accountID, collectApprovalCacheReadFailed)
		} else {
			logCollectApprovalCache(ctx, delivery, accountID, collectApprovalCacheMiss)
		}
	}
	defer func() {
		if rejectedKey != "" && cacheRejected && resultErr == nil {
			if err := cache.Instance().Set(ctx, rejectedKey, "rejected", collectApprovalRejectedTTL); err != nil {
				logCollectApprovalCache(ctx, delivery, accountID, collectApprovalCacheWriteFailed)
			} else {
				logCollectApprovalCache(ctx, delivery, accountID, collectApprovalCacheStored)
			}
		}
	}()
	// The marker only avoids repeating discovery writes. Always read the current
	// source-scoped decision so a manual disable takes effect immediately.
	boundKey := publishconsts.CollectChannelBoundKeyPrefix + collectChannelAccessRelationKey(tenantID, accountID, source["id"].Int64(), chatID) + ":bot:" + strconv.FormatInt(botID, 10)
	if marker, cacheErr := cache.Instance().Get(ctx, boundKey); cacheErr == nil && marker != nil && marker.Bool() {
		access, accessErr := resolveCollectEventChannelAccess(ctx, gdb.Record{
			"tenant_id": source["tenant_id"], "source_id": source["id"], "source_chat_id": g.NewVar(chatID),
		}, accountID)
		if accessErr != nil {
			return false, accessErr
		}
		if access.Relation.Id > 0 {
			cacheRejected = !access.Allowed
			return access.Allowed, nil
		}
		// A deleted relation must be rediscovered; Redis is not authoritative.
		_, _ = cache.Instance().Remove(ctx, boundKey)
	}
	defer func() {
		if resultErr == nil && allowedResult {
			if cacheErr := cache.Instance().Set(ctx, boundKey, true, 5*time.Minute); cacheErr != nil {
				g.Log().Warning(ctx, "写入采集频道绑定标记失败", g.Map{"tenantId": tenantID, "accountId": accountID, "sourceId": source["id"].Int64(), "chatId": chatID, "error": cacheErr.Error()})
			}
		}
	}()
	now := gtime.Now()
	var channels []botCollectChannelCandidate
	if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields("id,telegram_chat_id,title").Where("tenant_id", tenantID).WhereIn("telegram_chat_id", tgChannelCacheLookupIds(chatID)).Scan(&channels); !collectOptionalRecordScanSucceeded(err) {
		return false, err
	}
	channelIDs := make([]int64, 0, len(channels))
	for _, candidate := range channels {
		if candidate.Id > 0 {
			channelIDs = append(channelIDs, candidate.Id)
		}
	}
	var relations []botCollectChannelRelation
	if len(channelIDs) > 0 {
		if err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).
			Fields("id,channel_id,source_id,collect_enabled,status,approval_status").
			Where("tenant_id", tenantID).Where("account_id", accountID).
			Where("source_id", source["id"].Int64()).
			WhereIn("channel_id", channelIDs).Scan(&relations); !collectOptionalRecordScanSucceeded(err) {
			return false, gerror.Wrap(err, "读取BOT采集频道审批状态失败")
		}
	}
	channel := selectBotCollectChannelForAccount(channels, relations, chatID)
	relation := selectBotCollectChannelRelation(relations, channel.Id)
	if relation.Id > 0 {
		decision := decideCollectChannelAccess(collectChannelAccessPolicyInput{
			TenantID: tenantID, AccountID: accountID, SourceID: source["id"].Int64(),
			CanonicalChat: chatID, BotCollectScope: source["bot_collect_scope"].String(),
			RelationExists: true, CollectEnabled: relation.CollectEnabled,
			Status: relation.Status, ApprovalStatus: relation.ApprovalStatus,
		})
		allowed := decision.Allowed
		if !allowed {
			cacheRejected = true
			g.Log().Warning(ctx, "BOT采集频道审批未通过，跳过采集交付", g.Map{
				"tenantId": tenantID, "accountId": accountID, "sourceId": source["id"].Int64(),
				"botId": botID, "chatId": chatID, "channelId": channel.Id, "relationId": relation.Id,
				"collectEnabled": relation.CollectEnabled, "status": relation.Status, "approvalStatus": relation.ApprovalStatus,
				"sourceMessageId": delivery.SourceMessageID,
			})
		}
		if shouldQueueCollectDiscoveryNotification(allowed, s.collectDiscoveryNotificationSent(ctx, tenantID, source["id"].Int64(), chatID)) {
			if err := s.enqueueCollectNotification(ctx, collectNotificationPayload{
				ID:    fmt.Sprintf("discovery:%d:%d:%d:%s", tenantID, accountID, source["id"].Int64(), chatID),
				BotID: botID, TenantID: tenantID, AccountID: accountID, SourceID: source["id"].Int64(), ChannelID: chatID,
			}); err != nil {
				return false, err
			}
		}
		if !allowed {
			return false, nil
		}
	}
	if channel.Id <= 0 {
		if err := s.upsertCollectChannel(ctx, tenantID, chatID, "channel", chatID, "", 1, 0, delivery.SourceMessageID, now, now); err != nil {
			return false, gerror.Wrap(err, "记录BOT采集频道失败")
		}
		if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields("id,telegram_chat_id,title").Where("tenant_id", tenantID).Where("telegram_chat_id", chatID).Scan(&channel); err != nil || channel.Id <= 0 {
			return false, err
		}
	} else {
		if err := s.touchCollectChannelProgress(ctx, tenantID, channel.Id, delivery.SourceMessageID); err != nil {
			g.Log().Warning(ctx, "写入采集频道进度失败", g.Map{"tenantId": tenantID, "channelId": channel.Id, "sourceId": delivery.SourceID, "error": err.Error()})
		}
	}
	_ = s.touchCollectBotChannel(ctx, tenantID, botID, chatID, now)
	if relation.Id > 0 {
		return true, nil
	}
	// Queue before creating the discovery relation: a crash or send failure
	// must not permanently lose the first-channel notification.
	decision := decideCollectChannelAccess(collectChannelAccessPolicyInput{
		TenantID: tenantID, AccountID: accountID, SourceID: source["id"].Int64(),
		CanonicalChat: chatID, BotCollectScope: source["bot_collect_scope"].String(),
	})
	if !decision.Discover {
		return false, nil
	}
	_, err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Data(pdo.YoubanPublishCollectAccountChannel{
		TenantId: tenantID, AccountId: accountID, ChannelId: channel.Id, SourceId: source["id"].Int64(),
		CollectEnabled: 1, Status: sysin.CollectAccountChannelEnabled, ApprovalStatus: "approved", ApprovedAt: now,
	}).InsertAndGetId()
	if err != nil {
		// A concurrent delivery may have created the relation; respect its decision.
		var current botCollectChannelRelation
		lookupErr := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Fields("id,channel_id,source_id,collect_enabled,status,approval_status").Where("tenant_id", tenantID).Where("account_id", accountID).Where("source_id", source["id"].Int64()).Where("channel_id", channel.Id).Scan(&current)
		if lookupErr == nil && current.Id > 0 {
			return decideCollectChannelAccess(collectChannelAccessPolicyInput{
				TenantID: tenantID, AccountID: accountID, SourceID: source["id"].Int64(),
				CanonicalChat: chatID, BotCollectScope: source["bot_collect_scope"].String(),
				RelationExists: true, CollectEnabled: current.CollectEnabled,
				Status: current.Status, ApprovalStatus: current.ApprovalStatus,
			}).Allowed, nil
		}
		return false, gerror.Wrap(err, "创建BOT采集频道审批记录失败")
	}
	if err := s.enqueueCollectNotification(ctx, collectNotificationPayload{
		ID:    fmt.Sprintf("discovery:%d:%d:%d:%s", tenantID, accountID, source["id"].Int64(), chatID),
		BotID: botID, TenantID: tenantID, AccountID: accountID, SourceID: source["id"].Int64(), ChannelID: chatID,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func selectBotCollectChannel(candidates []botCollectChannelCandidate, chatID string) botCollectChannelCandidate {
	exact := normalizeTelegramChannelChatID(chatID)
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.TelegramChatId) == exact {
			return candidate
		}
	}
	for _, candidate := range candidates {
		if normalizeTelegramChannelChatID(candidate.TelegramChatId) == exact {
			return candidate
		}
	}
	return botCollectChannelCandidate{}
}

func selectBotCollectChannelForAccount(candidates []botCollectChannelCandidate, relations []botCollectChannelRelation, chatID string) botCollectChannelCandidate {
	var enabled []botCollectChannelCandidate
	var related []botCollectChannelCandidate
	for _, candidate := range candidates {
		relation := selectBotCollectChannelRelation(relations, candidate.Id)
		if relation.Id == 0 {
			continue
		}
		related = append(related, candidate)
		if botCollectChannelRelationEnabled(relation.CollectEnabled, relation.Status, relation.ApprovalStatus) {
			enabled = append(enabled, candidate)
		}
	}
	if len(enabled) > 0 {
		return selectBotCollectChannel(enabled, chatID)
	}
	if len(related) > 0 {
		return selectBotCollectChannel(related, chatID)
	}
	return selectBotCollectChannel(candidates, chatID)
}

func selectBotCollectChannelRelation(relations []botCollectChannelRelation, channelID int64) botCollectChannelRelation {
	for _, relation := range relations {
		if relation.ChannelId == channelID {
			return relation
		}
	}
	return botCollectChannelRelation{}
}

func (s *sSysPublish) sendBotCollectChannelDiscovery(ctx context.Context, source gdb.Record, chatID string, relationID int64, notificationID string) error {
	tenantID, botID := source["tenant_id"].Int64(), source["bot_id"].Int64()
	rules, ruleErr := botCollectChannelDiscoveryRules(ctx, source)
	if ruleErr != nil {
		return gerror.Wrap(ruleErr, "读取新频道默认后缀失败")
	}
	title := firstNonEmpty(s.collectChannelDisplayName(ctx, tenantID, chatID), chatID)
	linkLine := collectChannelLinkLine(s.collectChannelDisplayLink(ctx, tenantID, chatID))
	if len(rules) == 0 {
		text := "发现新的 BOT 采集频道\n频道：" + html.EscapeString(title) + "\n频道 ID：" + html.EscapeString(chatID) + linkLine + "\n采集状态：默认开启"
		buttons := [][]botsysin.NotifyButton{{{Label: "允许采集", Data: fmt.Sprintf("cm:channelapprove:%d", relationID)}, {Label: "禁用", Data: fmt.Sprintf("cm:channelreject:%d", relationID)}}}
		if notifyErr := botService.SysBot().NotifyCollectEvent(ctx, botID, "", notificationID+":plain", text, buttons); notifyErr != nil {
			return notifyErr
		}
	}
	for _, rule := range rules {
		suffix := strings.TrimSpace(rule["intro_fee_suffix"].String())
		if configured, found, suffixErr := s.resolveIntroFeeChannelSuffix(ctx, gdb.Record{"tenant_id": source["tenant_id"], "account_id": source["account_id"], "bot_id": source["bot_id"], "source_chat_id": gvar.New(chatID)}, rule); suffixErr != nil {
			return suffixErr
		} else if found {
			suffix = configured
		}
		if suffix == "" {
			suffix = "未设置"
		}
		text := fmt.Sprintf("发现新的代理采集频道\n频道：%s\n频道 ID：%s%s\n采集规则：%s\n频道后缀：%s\n采集状态：默认开启\n后续新采集资料将使用上述后缀，可通过下方按钮禁用采集或修改后缀。", html.EscapeString(title), html.EscapeString(chatID), linkLine, html.EscapeString(rule["name"].String()), html.EscapeString(suffix))
		buttons := [][]botsysin.NotifyButton{{{Label: "禁用", Data: fmt.Sprintf("cm:channelreject:%d", relationID)}, {Label: "修改后缀", Data: fmt.Sprintf("cm:channeledit:%d:%s:0", rule["id"].Int64(), chatID)}}}
		if notifyErr := botService.SysBot().NotifyCollectEvent(ctx, botID, "", fmt.Sprintf("%s:rule:%d", notificationID, rule["id"].Int64()), text, buttons); notifyErr != nil {
			return notifyErr
		}
	}
	return nil
}

func botCollectChannelDiscoveryRules(ctx context.Context, source gdb.Record) ([]gdb.Record, error) {
	cols := pdao.YoubanPublishCollectRule.Columns()
	bindings := pdao.YoubanPublishCollectSourceRule.Columns()
	rules, err := pdao.YoubanPublishCollectRule.Ctx(ctx).As("r").
		InnerJoin(pdao.YoubanPublishCollectSourceRule.Table()+" sr", "sr."+bindings.RuleId+"=r."+cols.Id+" AND sr."+bindings.TenantId+"=r."+cols.TenantId).
		Fields("r."+cols.Id, "r."+cols.Name).
		Where("sr."+bindings.SourceId, source["id"].Int64()).
		Where("r."+cols.TenantId, source["tenant_id"].Int64()).
		Where("r."+cols.AccountId, source["account_id"].Int64()).
		WhereNull("r." + cols.DeletedAt).All()
	if err != nil {
		return nil, err
	}
	if err = attachCollectRuleItems(ctx, rules); err != nil {
		return nil, err
	}
	// Suffix configuration lives in rule items, not legacy columns on the rule.
	result := make([]gdb.Record, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule["intro_fee_suffix"].String()) != "" {
			result = append(result, rule)
		}
	}
	return result, nil
}

func collectOptionalRecordScanSucceeded(err error) bool {
	return err == nil || errors.Is(err, sql.ErrNoRows)
}

func botCollectChannelRelationEnabled(collectEnabled int, status, approvalStatus string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	approvalStatus = strings.ToLower(strings.TrimSpace(approvalStatus))
	if status == sysin.CollectAccountChannelDisabled || approvalStatus == "rejected" {
		return false
	}
	// Missing and legacy pending relations follow the default-enabled policy.
	if status == "" || status == sysin.CollectAccountChannelPending {
		return true
	}
	return collectEnabled == 1 && (status == sysin.CollectAccountChannelEnabled || status == "approved")
}

func (s *sSysPublish) ingestCollectorAccountDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	if !s.collectGlobalEnabled(ctx) {
		logCollectorDeliverySkipped(ctx, delivery, "global_disabled")
		return nil
	}
	allowed, err := s.ensureAccountCollectChannelRelation(ctx, delivery)
	if err != nil {
		return err
	}
	if !allowed {
		logCollectorDeliverySkipped(ctx, delivery, "channel_access_blocked")
		return nil
	}
	blocked, err := s.collectMessageFromAccountPublishChannel(ctx, delivery.TenantID, delivery.TgAccountID, delivery.SourceChatID, delivery.SourceMessageID)
	if err != nil {
		return gerror.Wrap(err, "检查协议号采集上架频道过滤失败")
	}
	if blocked {
		logCollectorDeliverySkipped(ctx, delivery, "self_loop_publish_target")
		g.Log().Info(ctx, "协议号采集跳过上架目标频道消息", g.Map{
			"sourceType": "account", "reason": "source_chat_is_publish_target",
			"tenantId": delivery.TenantID, "accountId": delivery.AccountID,
			"tgAccountId": delivery.TgAccountID, "sourceId": delivery.SourceID,
			"sourceChatId": delivery.SourceChatID, "sourceMessageId": delivery.SourceMessageID,
		})
		return nil
	}
	message := collectorDeliveryMessage(delivery, delivery.TenantID, delivery.AccountID, delivery.SourceID, sysin.CollectSourceTypeAccount)
	message.TgAccountId = delivery.TgAccountID
	if groupedID := strings.TrimSpace(delivery.SourceGroupedID); groupedID != "" {
		message.SourceUniqueKey = accountCollectMaterialGroupKey(delivery, groupedID)
	}
	return s.submitCollectorDelivery(ctx, message, delivery).Err
}

func (s *sSysPublish) ensureAccountCollectChannelRelation(ctx context.Context, delivery *collectorin.CollectorDelivery) (bool, error) {
	if delivery == nil || delivery.SourceID <= 0 || delivery.TenantID <= 0 || delivery.AccountID <= 0 {
		return false, nil
	}
	chatID := normalizeTelegramChannelChatID(delivery.SourceChatID)
	if chatID == "" {
		return false, nil
	}
	var source struct {
		Id             int64  `json:"id"`
		Title          string `json:"title"`
		SourceUsername string `json:"sourceUsername"`
		CollectEnabled int    `json:"collectEnabled"`
		Status         int    `json:"status"`
	}
	if err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields("id,title,source_username,collect_enabled,status").
		Where("id", delivery.SourceID).Where("tenant_id", delivery.TenantID).
		Where("account_id", delivery.AccountID).Scan(&source); err != nil {
		return false, gerror.Wrap(err, "读取账户采集源频道信息失败")
	}
	if source.Id <= 0 {
		return false, nil
	}
	now := gtime.Now()
	if err := s.upsertCollectChannel(ctx, delivery.TenantID, chatID, "channel", strings.TrimSpace(source.Title), strings.TrimSpace(source.SourceUsername), 0, 0, delivery.SourceMessageID, now, now); err != nil {
		return false, gerror.Wrap(err, "同步账户采集频道信息失败")
	}
	var channel struct {
		Id int64 `json:"id"`
	}
	channelCols := pdao.YoubanPublishCollectChannel.Columns()
	if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields(channelCols.Id).
		Where(channelCols.TenantId, delivery.TenantID).
		Where(channelCols.TelegramChatId, chatID).Scan(&channel); err != nil {
		return false, gerror.Wrap(err, "读取账户采集频道关系失败")
	}
	if channel.Id <= 0 {
		return false, gerror.New("账户采集频道关系创建失败")
	}
	sourceCols := pdao.YoubanPublishCollectSource.Columns()
	if _, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where(sourceCols.Id, source.Id).Where(sourceCols.ChannelId, 0).
		Data(pdo.YoubanPublishCollectSource{ChannelId: channel.Id}).Update(); err != nil {
		return false, gerror.Wrap(err, "回写账户采集频道关系失败")
	}
	var current botCollectChannelRelation
	if err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).
		Fields("id,collect_enabled,status,approval_status").
		Where("tenant_id", delivery.TenantID).Where("account_id", delivery.AccountID).
		Where("source_id", source.Id).Where("channel_id", channel.Id).Scan(&current); err != nil {
		return false, gerror.Wrap(err, "读取账户采集频道授权状态失败")
	}
	if current.Id > 0 {
		return decideCollectChannelAccess(collectChannelAccessPolicyInput{
			TenantID: delivery.TenantID, AccountID: delivery.AccountID, SourceID: source.Id,
			CanonicalChat: chatID, RelationExists: true, CollectEnabled: current.CollectEnabled,
			Status: current.Status, ApprovalStatus: current.ApprovalStatus,
		}).Allowed, nil
	}
	enabled := source.CollectEnabled == 1 && source.Status == 1
	relation := pdo.YoubanPublishCollectAccountChannel{
		TenantId: delivery.TenantID, AccountId: delivery.AccountID, ChannelId: channel.Id, SourceId: source.Id,
		CollectEnabled: introFeeBoolToInt(enabled), Status: sysin.CollectAccountChannelDisabled, ApprovalStatus: "approved",
	}
	if enabled {
		relation.Status = sysin.CollectAccountChannelEnabled
		relation.ApprovedAt = now
	} else {
		relation.DisabledAt = now
	}
	if _, err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Data(relation).InsertAndGetId(); err != nil {
		return false, gerror.Wrap(err, "同步账户采集频道状态失败")
	}
	return enabled, nil
}

func logCollectorDeliverySkipped(ctx context.Context, delivery *collectorin.CollectorDelivery, reason string) {
	fields := g.Map{
		"reason":     strings.TrimSpace(reason),
		"outcome":    "skipped",
		"workerRole": "publish-delivery-handler",
	}
	if delivery != nil {
		fields["deliveryId"] = delivery.ID
		fields["collectEventId"] = delivery.EventID
		fields["tenantId"] = delivery.TenantID
		fields["accountId"] = delivery.AccountID
		fields["sourceId"] = delivery.SourceID
		fields["sourceType"] = delivery.SourceType
		fields["sourceChatId"] = delivery.SourceChatID
		fields["sourceMessageId"] = delivery.SourceMessageID
	}
	g.Log().Info(ctx, "Telegram采集交付显式跳过", fields)
}

func collectorDeliveryMessage(delivery *collectorin.CollectorDelivery, tenantID, accountID, sourceID int64, sourceType string) *CollectMessage {
	message := &CollectMessage{
		History:  delivery.History || (sourceType == sysin.CollectSourceTypeBot && !delivery.ReceivedAt.IsZero() && time.Since(delivery.ReceivedAt) > 5*time.Minute),
		TenantId: tenantID, AccountId: accountID, SourceId: sourceID, SourceType: sourceType,
		SourceChatId: delivery.SourceChatID, SourceMessageId: delivery.SourceMessageID,
		SourceGroupedId: delivery.SourceGroupedID, SourceUniqueKey: delivery.SourceUniqueKey, RawText: delivery.RawText,
	}
	if !delivery.ReceivedAt.IsZero() {
		message.ReceivedAt = gtime.New(delivery.ReceivedAt)
	}
	message.Media = make([]collectMediaItem, 0, len(delivery.Media))
	for _, item := range delivery.Media {
		message.Media = append(message.Media, collectMediaItem{
			Type: item.Type, Purpose: item.Purpose, FileId: item.FileID, FileUniqueId: item.FileUniqueID, FileUrl: item.FileURL,
			StoragePath: item.StoragePath, PosterUrl: item.PosterURL, FileMd5: item.FileMD5,
			FilePhash: item.FilePHash, SourceKind: item.SourceKind, SourceMediaId: item.SourceMediaID,
			SourceAccessHash: item.SourceAccessHash, SourceFileReference: append([]byte(nil), item.SourceFileReference...),
			SourceThumbSize: item.SourceThumbSize, SourceMimeType: item.SourceMimeType,
			SourceDCId: item.SourceDCID, SourceSize: item.SourceSize, DebugMetaJson: item.DebugMetaJSON,
		})
	}
	return message
}

func accountCollectMaterialGroupKey(delivery *collectorin.CollectorDelivery, groupedID string) string {
	if delivery == nil {
		return ""
	}
	groupedID = strings.TrimSpace(groupedID)
	if groupedID == "" {
		return strings.TrimSpace(delivery.SourceUniqueKey)
	}
	return fmt.Sprintf(
		"account:%d:source:%d:%s:group:%s",
		delivery.TgAccountID,
		delivery.SourceID,
		strings.TrimSpace(delivery.SourceChatID),
		groupedID,
	)
}
