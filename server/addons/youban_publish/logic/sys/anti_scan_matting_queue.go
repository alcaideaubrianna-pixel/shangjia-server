package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/hibiken/asynq"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cache"
	lock "hotgo/internal/library/hgrds/lock"
)

const (
	antiScanMattingStatusProcessing = "processing"
	antiScanMattingStatusCompleted  = "completed"
	antiScanMattingStatusFailed     = "failed"
	antiScanMattingTaskTTL          = 30 * time.Minute
	antiScanMattingFailedTaskTTL    = 30 * time.Second
)

type antiScanMattingQueuePayload struct {
	TaskId    string `json:"taskId"`
	TenantId  int64  `json:"tenantId"`
	AccountId int64  `json:"accountId"`
	MediaId   int64  `json:"mediaId"`
	ImageHash string `json:"imageHash"`
	Provider  string `json:"provider"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	// SourceURL/StoragePath are captured by the API node so the Worker can
	// fetch the already-resolved object directly instead of waiting on the
	// Telegram media cache again.
	SourceURL   string `json:"sourceUrl,omitempty"`
	StoragePath string `json:"storagePath,omitempty"`
}

func antiScanMattingTaskCacheKey(mediaId int64, provider string) string {
	return fmt.Sprintf("%s%d:%s", publishconsts.AntiScanMattingTaskKeyPrefix, mediaId, provider)
}

func loadAntiScanMattingTaskState(ctx context.Context, mediaId int64, provider string) (*sysin.AntiScanSegmentModel, bool) {
	value, err := cache.Instance().Get(ctx, antiScanMattingTaskCacheKey(mediaId, provider))
	if err != nil || value.IsNil() {
		return nil, false
	}
	var state sysin.AntiScanSegmentModel
	if err = value.Scan(&state); err != nil || state.Status == "" {
		return nil, false
	}
	return &state, true
}

func saveAntiScanMattingTaskState(ctx context.Context, mediaId int64, provider string, state *sysin.AntiScanSegmentModel) error {
	ttl := antiScanMattingTaskTTL
	if state != nil && state.Status == antiScanMattingStatusFailed {
		ttl = antiScanMattingFailedTaskTTL
	}
	return cache.Instance().Set(ctx, antiScanMattingTaskCacheKey(mediaId, provider), state, ttl)
}

func (s *sSysPublish) enqueueAntiScanMattingTask(ctx context.Context, payload antiScanMattingQueuePayload) (*sysin.AntiScanSegmentModel, error) {
	lockKey := fmt.Sprintf("%s%d:%s", publishconsts.AntiScanMattingTaskLockKeyPrefix, payload.MediaId, payload.Provider)
	mutex := lock.NewConfig(10*time.Second, 50*time.Millisecond).Mutex(lockKey)
	if err := mutex.Lock(ctx); err != nil {
		return nil, gerror.Wrap(err, "创建人像分割任务失败")
	}
	defer func() { _ = mutex.Unlock(context.Background()) }()
	if state, ok := loadAntiScanMattingTaskState(ctx, payload.MediaId, payload.Provider); ok {
		return state, nil
	}
	payload.TaskId = guid.S()
	state := &sysin.AntiScanSegmentModel{
		ImageHash: payload.ImageHash, Width: payload.Width, Height: payload.Height,
		Status: antiScanMattingStatusProcessing, TaskId: payload.TaskId, RetryAfterMs: 300,
	}
	if err := saveAntiScanMattingTaskState(ctx, payload.MediaId, payload.Provider, state); err != nil {
		return nil, gerror.Wrap(err, "保存人像分割任务状态失败")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, gerror.Wrap(err, "编码人像分割任务失败")
	}
	client, err := s.telegramQueueClient(ctx)
	if err == nil {
		_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeMatting, body),
			asynq.Queue(tgQueueNameMatting), asynq.MaxRetry(3), asynq.Timeout(2*time.Minute))
	}
	if err != nil {
		_, _ = cache.Instance().Remove(ctx, antiScanMattingTaskCacheKey(payload.MediaId, payload.Provider))
		return nil, gerror.Wrap(err, "提交人像分割任务失败")
	}
	return state, nil
}

func (s *sSysPublish) handleAntiScanMattingTask(ctx context.Context, task *asynq.Task) (err error) {
	startedAt := time.Now()
	var payload antiScanMattingQueuePayload
	if err = json.Unmarshal(task.Payload(), &payload); err != nil {
		return gerror.Wrap(err, "解析人像分割任务失败")
	}
	retryCount, _ := asynq.GetRetryCount(ctx)
	g.Log().Warningf(ctx, "防扫图任务开始 taskId:%s mediaId:%d provider:%s retry:%d", payload.TaskId, payload.MediaId, payload.Provider, retryCount)
	defer func() {
		observeAntiScanMattingTask(ctx, payload.Provider, startedAt, err)
		if err == nil {
			return
		}
		state := &sysin.AntiScanSegmentModel{
			ImageHash: payload.ImageHash, Width: payload.Width, Height: payload.Height,
			Status: antiScanMattingStatusProcessing, TaskId: payload.TaskId, RetryAfterMs: 300,
			Error: "人像分割遇到临时错误，系统正在自动重试",
		}
		retryCount, _ := asynq.GetRetryCount(ctx)
		maxRetry, _ := asynq.GetMaxRetry(ctx)
		if retryCount >= maxRetry {
			state.Status = antiScanMattingStatusFailed
			state.RetryAfterMs = 0
			state.Error = antiScanMattingErrorMessage
		}
		stateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = saveAntiScanMattingTaskState(stateCtx, payload.MediaId, payload.Provider, state)
		g.Log().Warning(ctx, "人像分割任务失败", g.Map{"taskId": payload.TaskId, "mediaId": payload.MediaId, "imageHash": payload.ImageHash, "err": err})
	}()
	if payload.TaskId == "" || payload.TenantId <= 0 || payload.AccountId <= 0 || payload.MediaId <= 0 || payload.ImageHash == "" {
		return gerror.New("人像分割任务参数不完整")
	}
	account := &sysin.AccountModel{Id: payload.AccountId, TenantId: payload.TenantId}
	stageStartedAt := time.Now()
	media, err := s.antiScanSegmentMedia(ctx, payload.MediaId, account)
	g.Log().Warningf(ctx, "防扫图任务阶段完成 stage:media_lookup taskId:%s durationMs:%d", payload.TaskId, time.Since(stageStartedAt).Milliseconds())
	if err != nil {
		return err
	}
	stageStartedAt = time.Now()
	conf, err := service.SysConfig().GetCloudResource(ctx)
	g.Log().Warningf(ctx, "防扫图任务阶段完成 stage:cloud_config taskId:%s durationMs:%d", payload.TaskId, time.Since(stageStartedAt).Milliseconds())
	if err != nil {
		return err
	}
	if provider := antiScanMattingProvider(conf); provider != payload.Provider {
		return gerror.New("人像分割服务配置已更新，请重新提交")
	}
	stageStartedAt = time.Now()
	if strings.TrimSpace(payload.SourceURL) != "" || strings.TrimSpace(payload.StoragePath) != "" {
		media = &telegramMediaItem{
			Id: media.Id, AttachmentId: 0, MediaType: media.MediaType,
			FileUrl: payload.SourceURL, StoragePath: payload.StoragePath, AssetHash: media.AssetHash,
		}
		g.Log().Warningf(ctx, "防扫图任务使用已解析媒体来源 taskId:%s sourceUrl:%t storagePath:%t", payload.TaskId, strings.TrimSpace(payload.SourceURL) != "", strings.TrimSpace(payload.StoragePath) != "")
	}
	path, _, err := cachedTelegramMediaFile(ctx, media)
	if err != nil {
		return gerror.Wrap(err, "读取媒体图片失败")
	}
	g.Log().Warningf(ctx, "防扫图任务阶段完成 stage:media_cache taskId:%s durationMs:%d", payload.TaskId, time.Since(stageStartedAt).Milliseconds())
	stageStartedAt = time.Now()
	imageBytes, err := os.ReadFile(path)
	if err != nil {
		return gerror.Wrap(err, "读取媒体图片失败")
	}
	g.Log().Warningf(ctx, "防扫图任务阶段完成 stage:read_file taskId:%s bytes:%d durationMs:%d", payload.TaskId, len(imageBytes), time.Since(stageStartedAt).Milliseconds())
	stageStartedAt = time.Now()
	imageHash, err := antiScanImageHash(imageBytes)
	if err != nil {
		return err
	}
	g.Log().Warningf(ctx, "防扫图任务阶段完成 stage:hash taskId:%s durationMs:%d", payload.TaskId, time.Since(stageStartedAt).Milliseconds())
	if !strings.EqualFold(imageHash, payload.ImageHash) {
		return gerror.New("媒体图片已更新，请重新提交")
	}
	if err = s.ensureImageQuotaAvailable(ctx, payload.TenantId); err != nil {
		return err
	}
	stageStartedAt = time.Now()
	segmentRaw, created, err := s.getOrCreateAntiScanMatting(ctx, imageHash, imageBytes, conf, cloudResourceUsageOwner{TenantId: payload.TenantId, AccountId: payload.AccountId})
	g.Log().Warningf(ctx, "防扫图任务阶段完成 stage:matting taskId:%s durationMs:%d created:%t", payload.TaskId, time.Since(stageStartedAt).Milliseconds(), created)
	if err != nil {
		return err
	}
	segmentURL := antiScanSegmentURL(segmentRaw)
	if segmentURL == "" {
		return gerror.New("云端抠图能力未启用")
	}
	if created {
		reference := antiScanMattingQuotaReference(payload.TenantId, payload.Provider, imageHash, time.Now())
		if err = s.consumeImageQuota(ctx, payload.TenantId, payload.AccountId, reference, "manual_background_replace"); err != nil {
			return err
		}
	}
	state := &sysin.AntiScanSegmentModel{
		ImageHash: imageHash, SegmentUrl: antiScanSegmentPresentationURL(segmentURL),
		Width: payload.Width, Height: payload.Height, Status: antiScanMattingStatusCompleted, TaskId: payload.TaskId,
	}
	stageStartedAt = time.Now()
	if err = s.saveAntiScanMediaSegmentCache(ctx, payload.MediaId, state, segmentRaw, payload.Provider); err != nil {
		return err
	}
	g.Log().Warningf(ctx, "防扫图任务阶段完成 stage:media_cache_save taskId:%s durationMs:%d", payload.TaskId, time.Since(stageStartedAt).Milliseconds())
	if err = saveAntiScanMattingTaskState(ctx, payload.MediaId, payload.Provider, state); err != nil {
		return gerror.Wrap(err, "保存人像分割完成状态失败")
	}
	g.Log().Info(ctx, "人像分割任务完成", g.Map{"taskId": payload.TaskId, "mediaId": payload.MediaId, "imageHash": imageHash})
	return nil
}
