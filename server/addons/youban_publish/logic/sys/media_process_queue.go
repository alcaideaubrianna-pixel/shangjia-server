package sys

import (
	"context"
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
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/utility/dbtime"
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
	ready, mediaErr := s.profileMediaReady(ctx, profileId)
	if isProfileMediaValidationError(mediaErr) {
		return s.settleWaitingProfileMediaFailure(ctx, profileId, mediaErr)
	}
	if mediaErr != nil || !ready {
		return mediaErr
	}
	// A Bot user may request publication before asynchronous media processing
	// finishes. In that case submitProfilePublish records only a waiting_media
	// intent and no Telegram job exists yet. Re-submit the same durable intent
	// after every media completion; submitProfilePublish re-checks readiness and
	// creates jobs exactly once when the final media item becomes ready.
	var waiting struct {
		TenantId      int64                   `json:"tenantId" orm:"tenant_id"`
		AccountId     int64                   `json:"accountId" orm:"account_id"`
		OperationNo   string                  `json:"operationNo" orm:"publish_operation_no"`
		PublishStatus sysin.PublishTaskStatus `json:"publishStatus" orm:"publish_status"`
	}
	if err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("tenant_id,account_id,publish_operation_no,publish_task_status AS publish_status").
		Where("profile_id", profileId).WhereNull("deleted_at").Scan(&waiting); err != nil {
		return err
	}
	if waiting.PublishStatus == sysin.PublishTaskStatusWaitingMedia && strings.TrimSpace(waiting.OperationNo) != "" {
		failedCount, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("profile_id", profileId).WhereNull("deleted_at").
			Where("processing_status", mediaProcessingFailed).Count()
		if err != nil {
			return err
		}
		if failedCount > 0 {
			if _, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
				Where("profile_id", profileId).WhereNull("deleted_at").
				Where("publish_task_status", sysin.PublishTaskStatusWaitingMedia.String()).Data(g.Map{
				"publish_task_status":     sysin.PublishTaskStatusFailed.String(),
				"publish_task_updated_at": gtime.Now(),
			}).Update(); err != nil {
				return gerror.Wrap(err, "记录媒体失败导致的上架失败状态失败")
			}
			g.Log().Warningf(ctx, "资料上架因媒体终态失败停止等待 profileId:%d failed_media:%d", profileId, failedCount)
			return nil
		}
		// Media completion resumes the same interactive publish lifecycle. The
		// initial request intentionally leaves the profile private while media
		// is processing, so the wakeup path must perform the same authoritative
		// public transition before creating Telegram jobs. Otherwise the worker
		// sees status=private and keeps requeueing the jobs forever.
		manualLifecycle := strings.TrimSpace(waiting.OperationNo) == "" || isInteractiveProfilePublishOperation(waiting.OperationNo)
		if manualLifecycle {
			// Multiple media workers can finish the same profile concurrently.
			// Claim the durable waiting state before changing visibility or
			// creating Telegram jobs; only the winner may resume publication.
			claim, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
				Where("profile_id", profileId).
				Where("publish_task_status", sysin.PublishTaskStatusWaitingMedia.String()).
				Where("publish_operation_no", waiting.OperationNo).
				Data(g.Map{"publish_task_status": sysin.PublishTaskStatusPublishing.String(), "publish_task_updated_at": gtime.Now()}).Update()
			if err != nil {
				return gerror.Wrap(err, "抢占媒体完成后的资料上架任务失败")
			}
			claimed, err := claim.RowsAffected()
			if err != nil {
				return gerror.Wrap(err, "读取媒体唤醒任务抢占结果失败")
			}
			if claimed == 0 {
				// Another worker already owns this wakeup. It will create the
				// durable channel jobs and enqueue them.
				return nil
			}
			if _, err := s.syncProfilePublishState(ctx, profileId, 1, consts.ContentVisibilityPublic, nil); err != nil {
				_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
					Where("profile_id", profileId).Where("publish_operation_no", waiting.OperationNo).
					Where("publish_task_status", sysin.PublishTaskStatusPublishing.String()).
					Data(g.Map{"publish_task_status": sysin.PublishTaskStatusFailed.String(), "publish_task_updated_at": gtime.Now()}).Update()
				return gerror.Wrap(err, "媒体就绪后切换资料为可发布状态失败")
			}
			if _, err := s.profilePublishSource(ctx, profileId, waiting.TenantId, waiting.AccountId, true); err != nil {
				_, _ = s.syncProfilePublishState(ctx, profileId, 2, consts.ContentVisibilityPrivate, nil)
				_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
					Where("profile_id", profileId).Where("publish_operation_no", waiting.OperationNo).
					Where("publish_task_status", sysin.PublishTaskStatusPublishing.String()).
					Data(g.Map{"publish_task_status": sysin.PublishTaskStatusFailed.String(), "publish_task_updated_at": gtime.Now()}).Update()
				return gerror.Wrap(err, "媒体就绪后校验资料发布状态失败")
			}
		}
		// Prepare the same Telegram jobs directly, then enqueue the committed job IDs.
		jobIds, err := s.prepareProfilePublishJobs(ctx, profileId, waiting.TenantId, waiting.AccountId, waiting.OperationNo, nil, false, telegramProfilePublishMeta{})
		if err != nil {
			message := boundPublishSuccessRecordMessage(err.Error())
			_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
				Where("profile_id", profileId).Where("publish_operation_no", waiting.OperationNo).WhereIn("publish_task_status", []string{sysin.PublishTaskStatusWaitingMedia.String(), sysin.PublishTaskStatusPublishing.String()}).Data(g.Map{
				"publish_task_status": sysin.PublishTaskStatusFailed.String(), "publish_task_updated_at": gtime.Now(),
			}).Update()
			g.Log().Warning(ctx, "媒体就绪后恢复资料上架失败，已结束等待状态", g.Map{"profileId": profileId, "operationNo": waiting.OperationNo, "stage": "media_ready_wakeup", "error": message})
		} else {
			if len(jobIds) == 0 {
				message := "媒体已就绪但未生成任何上架任务"
				_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
					Where("profile_id", profileId).Where("publish_operation_no", waiting.OperationNo).WhereIn("publish_task_status", []string{sysin.PublishTaskStatusWaitingMedia.String(), sysin.PublishTaskStatusPublishing.String()}).Data(g.Map{
					"publish_task_status": sysin.PublishTaskStatusFailed.String(), "publish_task_updated_at": gtime.Now(),
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
	state, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("tenant_id,account_id,expected_display_media_count,expected_verify_media_count,media_intent_status,created_at").
		Where("profile_id", profileId).WhereNull("deleted_at").One()
	if err != nil {
		return false, gerror.Wrap(err, "检查资料预期媒体失败")
	}
	if state.IsEmpty() {
		return false, errTelegramProfileMissing
	}
	// An asynchronous profile write may not have inserted any media yet.
	// Read an optional record instead of Scan(&struct), which treats an empty
	// result as sql.ErrNoRows. Zero counts must still pass the existing bounded
	// write-wait and validation checks below; they never mean media is ready.
	row, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).WhereNull("deleted_at").
		Fields("COALESCE(SUM(CASE WHEN purpose='display' THEN 1 ELSE 0 END),0) AS display_count, " +
			"COALESCE(SUM(CASE WHEN purpose='verify' THEN 1 ELSE 0 END),0) AS verify_count, " +
			"COALESCE(SUM(CASE WHEN processing_status IS NULL OR processing_status NOT IN ('" + strings.Join(terminalMediaProcessingStatuses(), "','") + "') THEN 1 ELSE 0 END),0) AS pending_count, " +
			"COALESCE(SUM(CASE WHEN processing_status='failed' AND COALESCE(file_url,'')='' AND COALESCE(storage_path,'')='' AND COALESCE(tg_file_id,'')='' AND COALESCE(processed_tg_file_id,'')='' THEN 1 ELSE 0 END),0) AS unavailable_count").
		One()
	if err != nil {
		return false, gerror.Wrap(err, "统计资料媒体状态失败")
	}
	counts := profileMediaReadinessCounts{}
	if !row.IsEmpty() {
		counts.Display = row["display_count"].Int()
		counts.Verify = row["verify_count"].Int()
		counts.Pending = row["pending_count"].Int()
		counts.Unavailable = row["unavailable_count"].Int()
	}
	// 数量齐全时不额外查询异步操作，正常路径只需两次 SQL。
	expectedDisplay, expectedVerify := state["expected_display_media_count"].Int(), state["expected_verify_media_count"].Int()
	writing := false
	if counts.Display <= 1 || (expectedVerify > 0 && counts.Verify < 1) {
		op, opErr := g.DB().Model(profileMediaOperationTable).Safe().Ctx(ctx).
			Fields("retry_count,created_at").Where("tenant_id", state["tenant_id"].Int64()).
			Where("account_id", state["account_id"].Int64()).
			Where("profile_no IN (SELECT profile_no FROM "+dao.ContentProfile.Table()+" WHERE id=?)", profileId).
			WhereIn("status", profileMediaOperationPendingStatuses()).OrderAsc("created_at").Limit(1).One()
		if opErr != nil {
			return false, gerror.Wrap(opErr, "检查资料媒体写入状态失败")
		}
		if !op.IsEmpty() {
			if mediaWaitExhausted(op["retry_count"].Int(), op["created_at"].GTime(), time.Now()) {
				return false, gerror.Wrap(errProfileMediaValidation, "媒体写入已达到 3 次尝试或 10 分钟等待上限")
			}
			writing = true
		} else if state["media_intent_status"].String() == "pending" &&
			(counts.Display < expectedDisplay || counts.Verify < expectedVerify) {
			// Bot 创建资料与落库异步操作之间的短窗口。只保护尚未落齐的声明数量。
			created := dbtime.FromTimestamp(state["created_at"].GTime())
			writing = created != nil && time.Since(created.Time) < mediaProcessTaskTimeout
		}
	}
	return validateProfileMediaReadiness(counts, expectedVerify, writing)
}

var errProfileMediaValidation = gerror.New("资料媒体校验不合格")

type profileMediaReadinessCounts struct {
	Display     int `orm:"display_count"`
	Verify      int `orm:"verify_count"`
	Pending     int `orm:"pending_count"`
	Unavailable int `orm:"unavailable_count"`
}

func validateProfileMediaReadiness(counts profileMediaReadinessCounts, expectedVerify int, writing bool) (bool, error) {
	if writing {
		return false, nil
	}
	if counts.Display <= 1 {
		return false, gerror.Wrapf(errProfileMediaValidation, "展示媒体实际 %d 个，要求大于 1 个", counts.Display)
	}
	if expectedVerify > 0 && counts.Verify < 1 {
		return false, gerror.Wrapf(errProfileMediaValidation, "验证媒体实际 %d 个，已声明验证资料，要求至少 1 个", counts.Verify)
	}
	if counts.Unavailable > 0 {
		return false, gerror.Wrapf(errProfileMediaValidation, "失败且无可用来源的媒体实际 %d 个，要求为 0 个", counts.Unavailable)
	}
	return counts.Pending == 0, nil
}

func isProfileMediaValidationError(err error) bool {
	// 错误落库后不再保留 Go 错误链，恢复器还需识别持久化标记。
	return errors.Is(err, errProfileMediaValidation) ||
		(err != nil && strings.Contains(err.Error(), errProfileMediaValidation.Error()))
}

func mediaWaitExhausted(attempts int, startedAt *gtime.Time, now time.Time) bool {
	started := dbtime.FromTimestamp(startedAt)
	return attempts >= mediaProcessMaxAttempts || (started != nil && now.Sub(started.Time) >= mediaProcessTaskTimeout)
}

// 还没有 TG Job 的提交/唤醒入口也必须收口操作投影，不能遗留 waiting_media。
func (s *sSysPublish) settleWaitingProfileMediaFailure(ctx context.Context, profileId int64, cause error) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		state, err := tx.Model(publishProfileStateTable).Safe().Ctx(ctx).
			Where("profile_id", profileId).WhereNull("deleted_at").LockUpdate().One()
		if err != nil || state.IsEmpty() {
			return err
		}
		status := sysin.PublishTaskStatus(state["publish_task_status"].String())
		if status != sysin.PublishTaskStatusWaitingMedia && status != sysin.PublishTaskStatusPending && status != sysin.PublishTaskStatusPublishing {
			return nil
		}
		active, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("profile_id", profileId).WhereIn("status", telegramJobActiveStatuses()).Count()
		if err != nil || active > 0 {
			return err
		}
		if err = s.writeProfilePublishOperationStateTx(ctx, tx, state["tenant_id"].Int64(),
			state["account_id"].Int64(), profileId, state["publish_operation_no"].String(),
			sysin.PublishTaskStatusFailed, true); err != nil {
			return err
		}
		g.Log().Warning(ctx, "资料媒体不合格，已结束上架等待", g.Map{
			"event": "publish_media_validation_failed", "profileId": profileId,
			"operationNo": state["publish_operation_no"].String(), "error": cause.Error(),
		})
		return nil
	})
}

func decideProfileMediaPurposeReadiness(expectedDisplay, expectedVerify, actualDisplay, actualVerify, pending, failed int) (bool, bool) {
	if failed > 0 || pending > 0 || (expectedDisplay > 0 && actualDisplay < 1) || (expectedVerify > 0 && actualVerify < 1) {
		return false, true
	}
	return true, true
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
	// uploaded is a completed upstream upload state.  Older/imported media
	// rows may never pass through the local metadata worker, but they already
	// have a reusable Telegram/file reference and must not block publishing.
	return []string{mediaProcessingUploaded, mediaProcessingReady, mediaProcessingFailed}
}

func (s *sSysPublish) postponeTelegramJobUntilMediaReady(ctx context.Context, job telegramJobRecord) error {
	if !telegramJobLifecycleNeedsRetry(job.Status.String()) || strings.TrimSpace(job.SendPhase.String()) != "" {
		return nil
	}
	if mediaWaitExhausted(job.RetryCount+1, job.CreatedAt, time.Now()) {
		_, err := s.settleStreamFailedMediaJob(ctx, job,
			gerror.Wrap(errProfileMediaValidation, "媒体等待已达到 3 次尝试或入队 10 分钟上限"))
		return err
	}
	nextRetryAt := gtime.Now().Add(telegramMediaReadyRetry)
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", telegramJobRetryableStatuses()).Where("send_phase", "").
		Where("retry_count", job.RetryCount).
		Data(g.Map{
			"retry_count":         job.RetryCount + 1,
			"dispatch_status":     tgDispatchStatusIdle,
			"last_dispatch_error": "媒体仍在异步处理中，媒体就绪后自动唤醒",
			"next_retry_at":       nextRetryAt,
			"updated_at":          gtime.Now(),
		}).Update()
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认媒体等待任务退避结果失败")
	}
	if affected == 0 {
		return nil
	}
	if err = s.rearmTelegramJobOutbox(ctx, job.Id, telegramMediaReadyRetry); err != nil {
		return err
	}
	if wakeErr := s.wakeNextTelegramChannelJob(ctx, job); wakeErr != nil {
		g.Log().Warningf(ctx, "等待媒体时唤醒频道下一条TG任务失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, wakeErr)
	}
	return nil
}
