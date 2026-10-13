package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// rebuildCollectedProfileMedia restores unavailable persisted media from the
// original Telegram message and deliberately bypasses stale object caches.
func (s *sSysPublish) rebuildCollectedProfileMedia(ctx context.Context, profileID, jobID int64) error {
	var dispatch struct {
		EventID int64 `json:"event_id"`
	}
	if err := g.DB().Model(publishCollectDispatchTable).Safe().Ctx(ctx).
		Fields("event_id").Where("profile_id", profileID).OrderDesc("id").Limit(1).Scan(&dispatch); err != nil {
		return gerror.Wrap(err, "读取采集资料来源失败")
	}
	if dispatch.EventID <= 0 {
		return gerror.New("资料没有可恢复的采集来源")
	}
	display, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(dispatch.EventID).One()
	if err != nil || display.IsEmpty() {
		return gerror.Wrap(err, "读取展示采集事件失败")
	}
	verify, err := s.pairedCollectVerifyEvent(ctx, dispatch.EventID)
	if err != nil {
		return err
	}
	targetPurpose := ""
	if jobID > 0 {
		job, jobErr := s.telegramJobById(ctx, jobID)
		if jobErr != nil {
			return jobErr
		}
		if telegramSendPhaseHasDisplay(job.SendPhase) {
			targetPurpose = "verify"
		}
	}
	media, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileID).WhereNull("deleted_at").OrderAsc("purpose,sort_index,id").All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复资料媒体失败")
	}
	recovered := 0
	for _, target := range media {
		purpose := strings.TrimSpace(target["purpose"].String())
		if targetPurpose != "" && purpose != targetPurpose {
			continue
		}
		eventID := dispatch.EventID
		if purpose == "verify" {
			if verify.IsEmpty() {
				continue
			}
			eventID = verify["id"].Int64()
		} else if purpose != "display" {
			continue
		}
		rows, rowsErr := s.collectEventMediaRows(ctx, eventID)
		if rowsErr != nil {
			return rowsErr
		}
		matching := make([]*collectEventMediaRow, 0, len(rows))
		for _, row := range rows {
			if row != nil && collectMediaTypesEquivalent(row.MediaType, target["media_type"].String()) {
				matching = append(matching, row)
			}
		}
		position := target["sort_index"].Int() - 1
		var source *collectEventMediaRow
		if position >= 0 && position < len(matching) {
			source = matching[position]
		}
		if source == nil || strings.TrimSpace(source.SourceMessageRef) == "" {
			continue
		}
		item := collectorMediaItemFromCollect(collectMediaRowsToItems([]*collectEventMediaRow{source}, purpose)[0])
		item.FileID = source.SourceMessageRef
		item.FileURL, item.StoragePath = "", ""
		item.SourceMediaID, item.SourceAccessHash, item.SourceDCID = 0, 0, 0
		item.SourceFileReference = nil
		task, taskErr := collectorservice.AccountTasks().SubmitAndWait(ctx, &collectorin.AccountTaskSubmit{
			TenantID: display["tenant_id"].Int64(), AccountID: display["tg_account_id"].Int64(),
			MediaOwnerAccountID: display["account_id"].Int64(), TaskType: collectorin.AccountTaskTypeMediaDownload,
			TaskKey:  fmt.Sprintf("profile-media-refresh:%d:%d", target["id"].Int64(), time.Now().UnixNano()),
			Priority: collectorin.EventPriorityUrgent, Media: &item, MaxAttempts: 5,
		}, time.Second)
		if taskErr != nil {
			return gerror.Wrapf(taskErr, "从Telegram恢复采集媒体失败 mediaId:%d", target["id"].Int64())
		}
		if task == nil || task.Status != collectorin.AccountTaskStatusCompleted || task.MediaResult.FileURL == "" {
			return gerror.Newf("从Telegram恢复采集媒体未完成 mediaId:%d", target["id"].Int64())
		}
		result := task.MediaResult
		if _, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).WherePri(source.Id).Data(g.Map{
			"cache_status": "ready", "file_url": result.FileURL, "storage_path": result.StoragePath,
			"poster_url": result.PosterURL, "file_md5": result.Media.FileMD5, "file_phash": result.Media.FilePHash,
			"source_file_id": result.Media.FileID, "source_kind": result.Media.SourceKind,
			"source_media_id": result.Media.SourceMediaID, "source_access_hash": result.Media.SourceAccessHash,
			"source_file_reference": result.Media.SourceFileReference, "source_dc_id": result.Media.SourceDCID,
			"source_size": result.Media.SourceSize, "error_message": "", "updated_at": gtime.Now(),
		}).Update(); err != nil {
			return gerror.Wrap(err, "回写采集事件媒体失败")
		}
		if _, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", target["id"].Int64()).Data(g.Map{
			"attachment_id": result.AttachmentID, "file_url": result.FileURL, "storage_path": result.StoragePath,
			"original_attachment_id": result.AttachmentID, "original_file_url": result.FileURL, "original_storage_path": result.StoragePath,
			"poster_url": result.PosterURL, "md5": result.Media.FileMD5, "size": result.Media.SourceSize,
			"mime_type": result.Media.SourceMimeType, "tg_file_id": "", "tg_cache_asset_hash": "", "tg_cache_status": "",
			"processing_status": mediaProcessingUploaded, "processing_error": "", "updated_at": gtime.Now(),
		}).Update(); err != nil {
			return gerror.Wrap(err, "回写资料媒体失败")
		}
		if err = s.enqueueMediaProcess(ctx, target["id"].Int64(), 0); err != nil {
			return gerror.Wrap(err, "提交恢复媒体处理任务失败")
		}
		recovered++
	}
	if recovered == 0 {
		return gerror.New("资料没有可恢复的Telegram原始媒体")
	}
	return nil
}

func collectMediaTypesEquivalent(source, target string) bool {
	source, target = strings.ToLower(strings.TrimSpace(source)), strings.ToLower(strings.TrimSpace(target))
	if source == target {
		return true
	}
	return (source == "photo" && target == "image") || (source == "image" && target == "photo")
}

func (s *sSysPublish) resumeRepairedTelegramJob(ctx context.Context, jobID int64) error {
	job, err := s.telegramJobById(ctx, jobID)
	if err != nil || job.Id <= 0 {
		return err
	}
	transitioned, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId: job.Id, From: []string{telegramJobStatusFailed}, To: telegramJobStatusPending,
		Additional: g.Map{"retry_count": 0, "next_retry_at": nil, "error_message": "", "last_dispatch_error": ""},
	})
	if err != nil || !transitioned {
		return err
	}
	if err = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPending); err != nil {
		return err
	}
	return s.enqueueTelegramJob(ctx, job.Id, 0)
}
