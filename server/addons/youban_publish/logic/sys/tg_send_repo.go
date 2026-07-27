package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) lockTelegramJob(ctx context.Context, jobId int64) (telegramJobRecord, bool, error) {
	var job telegramJobRecord
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Data(g.Map{
			"status":          "sending",
			"dispatch_status": tgDispatchStatusProcessing,
			"error_message":   "",
			"sent_at":         nil,
			"updated_at":      gtime.Now(),
		}).
		Update()
	if err != nil {
		return job, false, gerror.Wrap(err, "锁定TG任务失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return job, false, nil
	}
	if err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		Scan(&job); err != nil {
		return job, false, gerror.Wrap(err, "读取TG任务失败")
	}
	if job.Id <= 0 {
		return job, false, gerror.New("TG任务不存在")
	}
	return job, true, nil
}

func (s *sSysPublish) telegramJobCurrentStatus(ctx context.Context, jobId int64) (string, error) {
	value, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		Fields("status").
		Value()
	if err != nil {
		return "", gerror.Wrap(err, "读取TG任务状态失败")
	}
	return value.String(), nil
}

func (s *sSysPublish) telegramJobStillSending(ctx context.Context, jobId int64) (bool, error) {
	status, err := s.telegramJobCurrentStatus(ctx, jobId)
	if err != nil {
		return false, err
	}
	return status == "sending", nil
}

func (s *sSysPublish) telegramJobMedia(ctx context.Context, job telegramJobRecord, purpose string) ([]*telegramMediaItem, error) {
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", job.ProfileId).
		Where("purpose", purpose).
		WhereNull("deleted_at")
	records, err := mod.
		OrderAsc("sort_index").OrderAsc("id").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG媒体失败")
	}
	rows := make([]*telegramMediaItem, 0, len(records))
	for _, record := range records {
		media := newProfileMediaFromRecord(record)
		asset := media.EffectiveAsset()
		posterUrl := normalizeMediaFileURL(record["poster_url"].String(), record["poster_storage_path"].String())
		posterStoragePath := record["poster_storage_path"].String()
		mediaType := record["media_type"].String()
		assetHash := telegramMediaCacheAssetHash(mediaType, asset.Hash, posterUrl, posterStoragePath, record["tg_thumb_file_id"].String())
		rows = append(rows, &telegramMediaItem{
			Id:                record["id"].Int64(),
			AttachmentId:      asset.AttachmentId,
			MediaType:         mediaType,
			Purpose:           record["purpose"].String(),
			FileUrl:           normalizeMediaFileURL(asset.FileUrl, asset.StoragePath),
			PosterUrl:         posterUrl,
			StoragePath:       asset.StoragePath,
			PosterStoragePath: posterStoragePath,
			TgFileId:          media.ValidTgFileIdForHash(assetHash),
			TgThumbFileId:     "",
			AssetHash:         assetHash,
			SortIndex:         record["sort_index"].Int(),
		})
		if rows[len(rows)-1].TgFileId != "" {
			rows[len(rows)-1].TgThumbFileId = record["tg_thumb_file_id"].String()
		}
	}
	return rows, nil
}

func telegramMediaCacheAssetHash(mediaType string, assetHash string, posterUrl string, posterStoragePath string, tgThumbFileId string) string {
	assetHash = strings.TrimSpace(assetHash)
	if !strings.EqualFold(strings.TrimSpace(mediaType), "video") {
		return assetHash
	}
	var builder strings.Builder
	builder.WriteString("video-meta-v3")
	builder.WriteByte('|')
	builder.WriteString(assetHash)
	builder.WriteByte('|')
	builder.WriteString(strings.TrimSpace(posterUrl))
	builder.WriteByte('|')
	builder.WriteString(strings.TrimSpace(posterStoragePath))
	builder.WriteByte('|')
	builder.WriteString(strings.TrimSpace(tgThumbFileId))
	sum := sha256.Sum256([]byte(builder.String()))
	return "video-thumb:" + hex.EncodeToString(sum[:])
}

func (s *sSysPublish) saveTelegramSentMessages(ctx context.Context, job telegramJobRecord, messages []*telegramSentMessage) error {
	now := gtime.Now()
	for _, item := range messages {
		if item == nil || item.MessageId <= 0 {
			continue
		}
		_, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).Data(g.Map{
			"job_id":         job.Id,
			"tenant_id":      job.TenantId,
			"account_id":     job.AccountId,
			"profile_id":     job.ProfileId,
			"bot_id":         job.BotId,
			"target_chat_id": job.TargetChatId,
			"tg_message_id":  item.MessageId,
			"media_group_id": item.MediaGroupId,
			"media_id":       item.MediaId,
			"purpose":        item.Purpose,
			"tg_file_id":     item.TgFileId,
			"status":         "sent",
			"sent_at":        now,
			"created_at":     now,
			"updated_at":     now,
		}).Insert()
		if err != nil {
			return gerror.Wrap(err, "保存TG消息记录失败")
		}
	}
	return nil
}

func (s *sSysPublish) appendTelegramJobLog(ctx context.Context, job telegramJobRecord, action string, status string, message string) {
	_, _ = g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":     job.Id,
		"tenant_id":  job.TenantId,
		"account_id": job.AccountId,
		"profile_id": job.ProfileId,
		"bot_id":     job.BotId,
		"action":     action,
		"status":     status,
		"message":    message,
		"created_at": gtime.Now(),
	}).Insert()
}
