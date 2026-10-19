package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
)

const (
	mediaProcessingUploaded   = "uploaded"
	mediaProcessingProcessing = "processing"
	mediaProcessingReady      = "ready"
	mediaProcessingFailed     = "failed"
	mediaProcessingRetry      = "retry"
	mediaProcessMaxAttempts   = 3
	mediaProcessTaskTimeout   = 10 * time.Minute
)

func (s *sSysPublish) handleMediaProcessTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeMediaProcessQueuePayload(task)
	if err != nil {
		return err
	}
	media, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("id", payload.MediaId).WhereNull("deleted_at").One()
	if err != nil {
		return err
	}
	if media.IsEmpty() {
		return nil
	}
	status := strings.TrimSpace(media["processing_status"].String())
	if status == mediaProcessingReady && mediaProcessingFingerprintsComplete(media) {
		// A prior attempt may have committed the metadata but failed to index it.
		// Retry the projection and wakeup without downloading the file again.
		return s.completeMediaProcessingProjection(ctx, payload.MediaId, media["profile_id"].Int64())
	}
	retryCount, _ := asynq.GetRetryCount(ctx)
	// Legacy workers wrote failed before every queue retry. Only versioned
	// tasks use failed to mean the processing policy is exhausted.
	if mediaProcessFailureIsTerminal(status, payload) {
		return asynq.SkipRetry
	}
	if retryCount >= mediaProcessMaxAttempts {
		return s.finishMediaProcessFailure(ctx, payload.MediaId, media["profile_id"].Int64(), fmt.Errorf("媒体处理已达到%d次尝试上限", mediaProcessMaxAttempts), retryCount)
	}
	_, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("id", payload.MediaId).
		Data(g.Map{"processing_status": mediaProcessingProcessing, "processing_started_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return err
	}
	fileURL := media["file_url"].String()
	if !isAbsoluteMediaURL(fileURL) && media["attachment_id"].Int64() > 0 {
		fileURL, err = attachmentMediaSource(ctx, media["attachment_id"].Int64())
		if err != nil {
			return err
		}
	}

	metadata, processErr := processMediaAssetMetadata(ctx,
		media["media_type"].String(),
		media["storage_path"].String(),
		fileURL,
		media["poster_url"].String(),
		media["name"].String(),
		media["md5"].String(),
	)
	if processErr != nil {
		return s.finishMediaProcessFailure(ctx, payload.MediaId, media["profile_id"].Int64(), processErr, retryCount)
	}
	data := g.Map{
		"processing_status": mediaProcessingReady,
		"processing_error":  "",
		"perceptual_hash":   metadata.PerceptualHash,
		"md5":               metadata.MD5,
		"updated_at":        gtime.Now(),
	}
	if metadata.PosterURL != "" {
		data["poster_url"] = metadata.PosterURL
	}
	if metadata.PosterStoragePath != "" {
		data["poster_storage_path"] = metadata.PosterStoragePath
	}
	if _, err = s.saveMediaDataAndIndex(ctx, payload.MediaId, data, "保存媒体处理结果失败"); err != nil {
		return err
	}
	return s.completeMediaProcessingProjection(ctx, payload.MediaId, media["profile_id"].Int64())
}

func mediaProcessingFingerprintsComplete(media gdb.Record) bool {
	if strings.TrimSpace(media["md5"].String()) == "" {
		return false
	}
	kind := media["media_type"].String()
	if isTelegramImageMedia(kind) || isTelegramVideoMedia(kind) {
		_, valid := parseUploadPHash(media["perceptual_hash"].String())
		return valid
	}
	return true
}

func (s *sSysPublish) completeMediaProcessingProjection(ctx context.Context, mediaId, profileId int64) error {
	if err := s.syncMediaPHashBucketByMediaId(ctx, mediaId); err != nil {
		return err
	}
	if err := s.refreshProfileFingerprintProjection(ctx, profileId); err != nil {
		return err
	}
	return s.wakeProfileTelegramJobs(ctx, profileId)
}

func mediaProcessFailureIsTerminal(status string, payload mediaProcessQueuePayload) bool {
	return status == mediaProcessingFailed && payload.RetryPolicyVersion > 0
}

func (s *sSysPublish) finishMediaProcessFailure(ctx context.Context, mediaId, profileId int64, processErr error, retryCount int) error {
	// Persist the final state even when the worker's request deadline expired.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	nextStatus := mediaProcessFailureStatus(cleanupCtx, processErr, retryCount)
	if _, err := g.DB().Model(publishMediaTable).Safe().Ctx(cleanupCtx).Where("id", mediaId).Data(g.Map{
		"processing_status": nextStatus, "processing_error": processErr.Error(), "updated_at": gtime.Now(),
	}).Update(); err != nil {
		return err
	}
	g.Log().Warningf(cleanupCtx, "媒体处理失败 mediaId:%d profileId:%d attempt:%d/%d status:%s err:%+v", mediaId, profileId, retryCount+1, mediaProcessMaxAttempts, nextStatus, processErr)
	if nextStatus == mediaProcessingFailed {
		if wakeErr := s.wakeProfileTelegramJobs(cleanupCtx, profileId); wakeErr != nil {
			g.Log().Warningf(cleanupCtx, "媒体处理终止后唤醒推送失败 profileId:%d err:%+v", profileId, wakeErr)
		}
		return errors.Join(processErr, asynq.SkipRetry)
	}
	return processErr
}

func mediaProcessFailureStatus(ctx context.Context, err error, retryCount int) string {
	if retryCount+1 < mediaProcessMaxAttempts && isRetryableMediaFileCacheError(ctx, err) {
		return mediaProcessingRetry
	}
	return mediaProcessingFailed
}

func (s *sSysPublish) wakeProfileTelegramJobs(ctx context.Context, profileId int64) error {
	if profileId <= 0 {
		return nil
	}
	var jobs []struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Where("(dispatch_status = ? OR dispatch_status = '')", tgDispatchStatusIdle).
		Fields("id").Scan(&jobs); err != nil {
		return err
	}
	for _, job := range jobs {
		if err := s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false); err != nil {
			g.Log().Warningf(ctx, "媒体就绪唤醒TG任务失败 media_profile_id:%d job_id:%d err:%+v", profileId, job.Id, err)
		}
	}
	return nil
}

func (s *sSysPublish) recoverMediaProcessTasks(ctx context.Context) {
	var rows []struct {
		Id int64 `json:"id"`
	}
	cutoff := gtime.Now().Add(-time.Minute)
	processingCutoff := gtime.Now().Add(-mediaProcessTaskTimeout - time.Minute)
	if err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("((processing_status = ? AND updated_at < ?) OR (processing_status = ? AND updated_at < ?))", mediaProcessingUploaded, cutoff, mediaProcessingProcessing, processingCutoff).
		WhereNull("deleted_at").OrderAsc("updated_at").Fields("id").Limit(200).Scan(&rows); err != nil {
		g.Log().Warningf(ctx, "恢复媒体异步处理任务查询失败 err:%+v", err)
		return
	}
	for _, row := range rows {
		if err := s.enqueueMediaProcess(ctx, row.Id, 0); err != nil {
			g.Log().Warningf(ctx, "恢复媒体异步处理任务入队失败 media_id:%d err:%+v", row.Id, err)
		}
	}
}

func (s *sSysPublish) profileMediaReady(ctx context.Context, profileId int64) (bool, error) {
	if profileId <= 0 {
		return true, nil
	}
	count, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).WhereNull("deleted_at").Count()
	if err != nil || count == 0 {
		return err == nil, err
	}
	pending, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).WhereNull("deleted_at").
		Where("processing_status IS NULL OR processing_status = '' OR processing_status NOT IN (?)", terminalMediaProcessingStatuses()).Count()
	return pending == 0, err
}

func terminalMediaProcessingStatuses() []string {
	// Metadata processing failure is terminal. The original media remains
	// available and publishing must not wait forever for a retry that will
	// never transition this row back to processing.
	return []string{mediaProcessingReady, mediaProcessingFailed}
}

func (s *sSysPublish) postponeTelegramJobUntilMediaReady(ctx context.Context, job telegramJobRecord) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"last_dispatch_error": "媒体仍在异步处理中，媒体就绪后自动唤醒",
			"next_retry_at":       gtime.Now().Add(telegramPublishTaskTimeout),
			"updated_at":          gtime.Now(),
		}).Update()
	if err != nil {
		return err
	}
	if wakeErr := s.wakeNextTelegramChannelJob(ctx, job); wakeErr != nil {
		g.Log().Warningf(ctx, "等待媒体时唤醒频道下一条TG任务失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, wakeErr)
	}
	return nil
}
