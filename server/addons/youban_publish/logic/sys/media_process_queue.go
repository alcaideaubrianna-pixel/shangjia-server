package sys

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

// errTelegramProfileMissing marks a durable TG job whose source profile was
// deleted. Such a job must be terminally failed and release its channel head;
// retrying it forever can block every later job in that channel.
var errTelegramProfileMissing = errors.New("telegram profile missing")

const (
	mediaProcessingUploaded   = "uploaded"
	mediaProcessingProcessing = "processing"
	mediaProcessingReady      = "ready"
	mediaProcessingFailed     = "failed"
	mediaProcessingRetry      = "retry"
	mediaProcessMaxAttempts   = 3
	mediaProcessTaskTimeout   = 10 * time.Minute
	telegramMediaReadyRetry   = time.Minute
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
	// A Bot user may request publication before asynchronous media processing
	// finishes. In that case submitProfilePublish records only a waiting_media
	// intent and no Telegram job exists yet. Re-submit the same durable intent
	// after every media completion; submitProfilePublish re-checks readiness and
	// creates jobs exactly once when the final media item becomes ready.
	var waiting struct {
		TenantId      int64  `json:"tenantId" orm:"tenant_id"`
		AccountId     int64  `json:"accountId" orm:"account_id"`
		OperationNo   string `json:"operationNo" orm:"publish_operation_no"`
		PublishStatus string `json:"publishStatus" orm:"publish_status"`
	}
	if err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("tenant_id,account_id,publish_operation_no,publish_task_status AS publish_status").
		Where("profile_id", profileId).WhereNull("deleted_at").Scan(&waiting); err != nil {
		return err
	}
	if waiting.PublishStatus == "waiting_media" && strings.TrimSpace(waiting.OperationNo) != "" {
		failedCount, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("profile_id", profileId).WhereNull("deleted_at").
			Where("processing_status", mediaProcessingFailed).Count()
		if err != nil {
			return err
		}
		if failedCount > 0 {
			if _, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
				Where("profile_id", profileId).WhereNull("deleted_at").
				Where("publish_task_status", "waiting_media").Data(g.Map{
				"publish_task_status":     sysin.PublishTaskStatusFailed,
				"publish_task_updated_at": gtime.Now(),
			}).Update(); err != nil {
				return gerror.Wrap(err, "记录媒体失败导致的上架失败状态失败")
			}
			g.Log().Warningf(ctx, "资料上架因媒体终态失败停止等待 profileId:%d failed_media:%d", profileId, failedCount)
			return nil
		}
		// Media completion is already inside the publish lifecycle. Re-entering
		// submitProfilePublish here can race with the waiting_media projection
		// and make the request look like a second submission. Prepare the same
		// Telegram jobs directly, then enqueue the committed job IDs.
		jobIds, err := s.prepareProfilePublishJobs(ctx, profileId, waiting.TenantId, waiting.AccountId, waiting.OperationNo, nil, false, telegramProfilePublishMeta{})
		if err != nil {
			message := boundPublishSuccessRecordMessage(err.Error())
			_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
				Where("profile_id", profileId).Where("publish_task_status", "waiting_media").Data(g.Map{
				"publish_task_status": sysin.PublishTaskStatusFailed, "publish_task_updated_at": gtime.Now(),
			}).Update()
			g.Log().Warning(ctx, "媒体就绪后恢复资料上架失败，已结束等待状态", g.Map{"profileId": profileId, "operationNo": waiting.OperationNo, "stage": "media_ready_wakeup", "error": message})
		} else {
			if len(jobIds) == 0 {
				message := "媒体已就绪但未生成任何上架任务"
				_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
					Where("profile_id", profileId).Where("publish_task_status", "waiting_media").Data(g.Map{
					"publish_task_status": sysin.PublishTaskStatusFailed, "publish_task_updated_at": gtime.Now(),
				}).Update()
				g.Log().Warning(ctx, "媒体就绪后未生成上架任务，已结束等待状态", g.Map{"profileId": profileId, "operationNo": waiting.OperationNo, "stage": "media_ready_wakeup", "error": message})
			} else {
				s.enqueuePreparedProfileJobs(ctx, jobIds, 0)
			}
		}
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
		if err := s.EnqueueTelegramJob(ctx, job.Id, 0, false); err != nil {
			g.Log().Warningf(ctx, "媒体就绪唤醒TG任务失败 media_profile_id:%d job_id:%d err:%+v", profileId, job.Id, err)
		}
	}
	return nil
}

func (s *sSysPublish) profileMediaReady(ctx context.Context, profileId int64) (bool, error) {
	if profileId <= 0 {
		return true, nil
	}
	var expected struct {
		Display int `orm:"expected_display_media_count"`
		Verify  int `orm:"expected_verify_media_count"`
	}
	if err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("expected_display_media_count,expected_verify_media_count").Where("profile_id", profileId).WhereNull("deleted_at").Scan(&expected); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, errTelegramProfileMissing
		}
		return false, gerror.Wrap(err, "检查资料预期媒体失败")
	}
	expectedTotal := expected.Display + expected.Verify
	count, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).WhereNull("deleted_at").Count()
	if err != nil {
		return false, err
	}
	if count < expectedTotal {
		return false, nil
	}
	if count == 0 {
		// A newly created profile can be published before the asynchronous
		// media writer inserts its rows. Use the profile declaration to
		// distinguish that state from an intentional text-only profile.
		var declared struct {
			ImageCount           int   `orm:"image_count"`
			VideoCount           int   `orm:"video_count"`
			HasVerificationVideo int   `orm:"has_verification_video"`
			CoverMediaId         int64 `orm:"cover_media_id"`
		}
		if err = g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
			Fields("image_count,video_count,has_verification_video,cover_media_id").Where("id", profileId).WhereNull("deleted_at").Scan(&declared); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, errTelegramProfileMissing
			}
			return false, gerror.Wrap(err, "检查资料声明媒体失败")
		}
		return declared.ImageCount <= 0 && declared.VideoCount <= 0 && declared.HasVerificationVideo <= 0 && declared.CoverMediaId <= 0, nil
	}
	failed, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).WhereNull("deleted_at").Where("processing_status", mediaProcessingFailed).Count()
	if err != nil {
		return false, err
	}
	pending, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).WhereNull("deleted_at").
		Where("processing_status IS NULL OR processing_status = '' OR processing_status NOT IN (?)", terminalMediaProcessingStatuses()).Count()
	if err != nil {
		return false, err
	}
	ready, _ := decideProfileMediaReadiness(expectedTotal, count, pending, failed)
	return ready, nil
}

// decideProfileMediaReadiness is the publish gate's pure state machine. The
// database adapter only supplies counts; every timing combination is tested
// without relying on a live worker or Telegram service.
func decideProfileMediaReadiness(expected, actual, pending, failed int) (ready, decided bool) {
	if failed > 0 || actual < expected || pending > 0 {
		return false, true
	}
	if expected == 0 && actual == 0 {
		return true, true
	}
	return actual >= expected, true
}

func mediaIntentStatusForCounts(displayCount, verifyCount int) string {
	if displayCount > 0 || verifyCount > 0 {
		return "pending"
	}
	return "none"
}

func terminalMediaProcessingStatuses() []string {
	// Metadata processing failure is terminal. The original media remains
	// available and publishing must not wait forever for a retry that will
	// never transition this row back to processing.
	return []string{mediaProcessingReady, mediaProcessingFailed}
}

func (s *sSysPublish) postponeTelegramJobUntilMediaReady(ctx context.Context, job telegramJobRecord) error {
	nextRetryAt := gtime.Now().Add(telegramMediaReadyRetry)
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"last_dispatch_error": "媒体仍在异步处理中，媒体就绪后自动唤醒",
			"next_retry_at":       nextRetryAt,
			"updated_at":          gtime.Now(),
		}).Update()
	if err != nil {
		return err
	}
	if err = s.rearmTelegramJobOutbox(ctx, job.Id, telegramMediaReadyRetry); err != nil {
		return err
	}
	if wakeErr := s.wakeNextTelegramChannelJob(ctx, job); wakeErr != nil {
		g.Log().Warningf(ctx, "等待媒体时唤醒频道下一条TG任务失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, wakeErr)
	}
	return nil
}
