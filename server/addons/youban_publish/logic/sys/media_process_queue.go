package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
)

const (
	mediaProcessingUploaded   = "uploaded"
	mediaProcessingProcessing = "processing"
	mediaProcessingReady      = "ready"
	mediaProcessingFailed     = "failed"
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
	if status == mediaProcessingReady {
		return nil
	}
	_, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("id", payload.MediaId).
		Data(g.Map{"processing_status": mediaProcessingProcessing, "processing_started_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return err
	}

	metadata, processErr := processMediaAssetMetadata(ctx,
		media["media_type"].String(),
		media["storage_path"].String(),
		media["file_url"].String(),
		media["poster_url"].String(),
		media["name"].String(),
	)
	if processErr != nil {
		_, _ = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", payload.MediaId).Data(g.Map{
			"processing_status": mediaProcessingFailed,
			"processing_error":  processErr.Error(),
			"updated_at":        gtime.Now(),
		}).Update()
		return processErr
	}
	data := g.Map{
		"processing_status": mediaProcessingReady,
		"processing_error":  "",
		"perceptual_hash":   metadata.PerceptualHash,
		"updated_at":        gtime.Now(),
	}
	if metadata.PosterURL != "" {
		data["poster_url"] = metadata.PosterURL
	}
	if metadata.PosterStoragePath != "" {
		data["poster_storage_path"] = metadata.PosterStoragePath
	}
	if _, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", payload.MediaId).Data(data).Update(); err != nil {
		return err
	}
	if err = s.syncMediaPHashBucketByMediaId(ctx, payload.MediaId); err != nil {
		return err
	}
	return s.wakeProfileTelegramJobs(ctx, media["profile_id"].Int64())
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
		if err := s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, true); err != nil {
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
	if err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		WhereIn("processing_status", []string{mediaProcessingUploaded, mediaProcessingProcessing}).
		WhereLT("updated_at", cutoff).
		WhereNull("deleted_at").Fields("id").Limit(200).Scan(&rows); err != nil {
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
		Where("processing_status IS NULL OR processing_status = '' OR processing_status NOT IN (?)", []string{mediaProcessingReady}).Count()
	return pending == 0, err
}

func (s *sSysPublish) postponeTelegramJobUntilMediaReady(ctx context.Context, jobId int64) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"last_dispatch_error": "媒体仍在异步处理中，媒体就绪后自动唤醒",
			"next_retry_at":       nil,
			"updated_at":          gtime.Now(),
		}).Update()
	return err
}
