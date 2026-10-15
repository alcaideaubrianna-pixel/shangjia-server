package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/internal/library/cache"
)

func (s *sSysPublish) lockTelegramJob(ctx context.Context, jobId int64) (telegramJobRecord, bool, error) {
	var job telegramJobRecord
	transitioned, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId: jobId,
		From:  []string{telegramJobStatusPending, telegramJobStatusRetryWait},
		To:    telegramJobStatusSending,
		Additional: g.Map{
			"error_message": "",
			"sent_at":       nil,
		},
	})
	if err != nil {
		return job, false, gerror.Wrap(err, "锁定TG任务失败")
	}
	if !transitioned {
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
	rowsBySlot := make(map[telegramMediaSlot]*telegramMediaItem, len(records))
	for _, record := range records {
		media := newProfileMediaFromRecord(record)
		asset := media.EffectiveAsset()
		posterUrl := normalizeMediaFileURL(record["poster_url"].String(), record["poster_storage_path"].String())
		posterStoragePath := record["poster_storage_path"].String()
		mediaType := record["media_type"].String()
		assetHash := telegramMediaCacheAssetHash(mediaType, asset.Hash, posterUrl, posterStoragePath, record["tg_thumb_file_id"].String())
		item := &telegramMediaItem{
			Id:                      record["id"].Int64(),
			AttachmentId:            asset.AttachmentId,
			MediaType:               mediaType,
			MustSend:                record["must_send"].Bool(),
			Purpose:                 record["purpose"].String(),
			FileUrl:                 normalizeMediaFileURL(asset.FileUrl, asset.StoragePath),
			PosterUrl:               posterUrl,
			StoragePath:             asset.StoragePath,
			PosterStoragePath:       posterStoragePath,
			TgFileId:                media.ValidTgFileIdForHash(assetHash),
			TgThumbFileId:           "",
			AssetHash:               assetHash,
			SortIndex:               record["sort_index"].Int(),
			CollectedFileIdEligible: strings.TrimSpace(media.EditStatus) != mediaEditStatusEdited,
		}
		if item.TgFileId != "" {
			item.TgThumbFileId = record["tg_thumb_file_id"].String()
		}
		if !telegramMediaItemHasSource(item) {
			continue
		}
		slot := telegramMediaSlot{SortIndex: item.SortIndex, MediaType: item.MediaType}
		existing, ok := rowsBySlot[slot]
		if !ok || telegramMediaItemPriority(item) > telegramMediaItemPriority(existing) {
			rowsBySlot[slot] = item
		}
	}
	rows := make([]*telegramMediaItem, 0, len(rowsBySlot))
	for _, item := range rowsBySlot {
		rows = append(rows, item)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SortIndex != rows[j].SortIndex {
			return rows[i].SortIndex < rows[j].SortIndex
		}
		return rows[i].Id < rows[j].Id
	})
	if len(records) > 0 && len(rows) == 0 {
		return nil, gerror.New("媒体文件地址为空")
	}
	if err = s.applySameBotCollectFileIDs(ctx, job, purpose, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

type sameBotCollectFile struct {
	MediaType    string `orm:"media_type"`
	SourceFileId string `orm:"source_file_id"`
}

func (s *sSysPublish) applySameBotCollectFileIDs(ctx context.Context, job telegramJobRecord, purpose string, media []*telegramMediaItem) error {
	if job.CollectEventId <= 0 || job.BotId <= 0 || len(media) == 0 {
		return nil
	}
	var sources []sameBotCollectFile
	eventTable := pdao.YoubanPublishCollectEvent.Table()
	mediaTable := pdao.YoubanPublishCollectEventMedia.Table()
	mod := g.DB().Model(eventTable+" e").Safe().Ctx(ctx).
		InnerJoin(mediaTable+" em", "em.event_id=e.id").
		Fields("em.media_type,em.source_file_id").
		Where("e.source_type", "bot").Where("e.bot_id", job.BotId).
		Where("e.material_role", purpose).
		WhereNot("em.source_file_id", "").
		OrderAsc("e.id").OrderAsc("em.id")
	if purpose == "verify" {
		mod = mod.Where("e.material_parent_event_id", job.CollectEventId)
	} else {
		mod = mod.Where("e.id", job.CollectEventId)
	}
	if err := mod.Scan(&sources); err != nil {
		return gerror.Wrap(err, "读取同Bot采集媒体File ID失败")
	}
	applySameBotCollectFileIDs(media, sources)
	return nil
}

func applySameBotCollectFileIDs(media []*telegramMediaItem, sources []sameBotCollectFile) {
	byType := make(map[string][]string)
	for _, source := range sources {
		mediaType := listenerTelegramMediaType(source.MediaType)
		fileId := strings.TrimSpace(source.SourceFileId)
		if fileId != "" && telegramFileIDMatchesMediaType(mediaType, fileId) {
			byType[mediaType] = append(byType[mediaType], fileId)
		}
	}
	for _, item := range media {
		if item == nil {
			continue
		}
		mediaType := listenerTelegramMediaType(item.MediaType)
		candidates := byType[mediaType]
		if len(candidates) == 0 {
			continue
		}
		fileId := candidates[0]
		byType[mediaType] = candidates[1:]
		if item.CollectedFileIdEligible && strings.TrimSpace(item.TgFileId) == "" {
			item.TgFileId = fileId
		}
	}
}

type telegramMediaSlot struct {
	SortIndex int
	MediaType string
}

func telegramMediaItemHasSource(item *telegramMediaItem) bool {
	if item == nil {
		return false
	}
	return strings.TrimSpace(item.TgFileId) != "" ||
		strings.TrimSpace(item.FileUrl) != "" ||
		strings.TrimSpace(item.StoragePath) != ""
}

func telegramMediaItemPriority(item *telegramMediaItem) int {
	if item == nil {
		return 0
	}
	priority := 0
	if strings.TrimSpace(item.StoragePath) != "" {
		priority += 10
	}
	if strings.TrimSpace(item.FileUrl) != "" {
		priority += 20
	}
	if strings.TrimSpace(item.TgFileId) != "" {
		priority += 30
	}
	return priority
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
	if err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return saveTelegramSentMessagesTx(ctx, tx, job, messages)
	}); err != nil {
		return err
	}
	s.cacheTelegramSentMessageMetadata(ctx, job, messages)
	return nil
}

func saveTelegramSentMessagesTx(ctx context.Context, tx gdb.TX, job telegramJobRecord, messages []*telegramSentMessage) error {
	for _, item := range messages {
		if item == nil || item.MessageId <= 0 {
			continue
		}
		if strings.TrimSpace(item.MediaGroupId) != "" && (item.Purpose == "display" || item.Purpose == "verify") {
			exists, err := tx.Model(publishTgMessageTable).
				Where("job_id", job.Id).Where("media_group_id", item.MediaGroupId).
				WhereNot("purpose", item.Purpose).WhereIn("purpose", []string{"display", "verify"}).Count()
			if err != nil {
				return gerror.Wrap(err, "校验TG媒体组用途失败")
			}
			if exists > 0 {
				observeTelegramMediaPurposeViolation(ctx, "persist")
				return gerror.New("展示资料与验证资料禁止合并发送")
			}
		}
		_, err := tx.Model(publishTgMessageTable).Data(g.Map{
			"job_id":            job.Id,
			"tenant_id":         job.TenantId,
			"account_id":        job.AccountId,
			"profile_id":        job.ProfileId,
			"bot_id":            job.BotId,
			"target_chat_id":    job.TargetChatId,
			"tg_message_id":     item.MessageId,
			"media_group_id":    item.MediaGroupId,
			"media_id":          item.MediaId,
			"purpose":           item.Purpose,
			"tg_file_id":        item.TgFileId,
			"tg_file_unique_id": item.TgFileUniqueId,
			"status":            "sent",
			"sent_at":           gtime.Now(),
			"deleted_at":        nil,
		}).OnConflict("job_id,tg_message_id").OnDuplicate("status,tg_file_id,tg_file_unique_id,sent_at,deleted_at").Save()
		if err != nil {
			return gerror.Wrap(err, "保存TG消息记录失败")
		}
	}
	return nil
}

func (s *sSysPublish) cacheTelegramSentMessageMetadata(ctx context.Context, job telegramJobRecord, messages []*telegramSentMessage) {
	for _, item := range messages {
		if item != nil && item.MessageId > 0 && isProtectedTelegramPublishPurpose(item.Purpose) {
			_ = cache.Instance().Set(ctx, telegramPublishMessageCacheKey(job.TargetChatId, int(item.MessageId)), 1, autoDeletePublishMessageCacheTTL)
		}
	}
	storedHashes := make(map[string]telegramAntiScanHash)
	for _, item := range messages {
		if item == nil || item.ProtectedHashKey == "" || item.ProtectedPHash == 0 && item.ProtectedDHash == 0 {
			continue
		}
		storedHashes[item.ProtectedHashKey] = telegramAntiScanHash{PHash: item.ProtectedPHash, DHash: item.ProtectedDHash}
	}
	for key, hash := range storedHashes {
		appendTelegramAntiScanHashHistory(ctx, key, hash)
	}
	for _, item := range messages {
		if item == nil || item.MediaId <= 0 || item.ProtectedHashKey == "" || item.ProtectedPHash == 0 {
			continue
		}
		if err := s.saveAntiScanSearchFingerprint(ctx, job, item); err != nil {
			g.Log().Warningf(ctx, "保存防扫图搜索指纹失败 jobId:%d mediaId:%d err:%+v", job.Id, item.MediaId, err)
		}
	}
}

// finalizeTelegramMediaDelivery owns the terminal phase transition shared by
// Bot and account transports. Media transports only confirm their own phase.
func (s *sSysPublish) finalizeTelegramMediaDelivery(ctx context.Context, job telegramJobRecord, hasVerify, complete bool) error {
	if !hasVerify {
		if err := s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseCompletedNoVerify); err != nil {
			return err
		}
	}
	if !complete {
		return nil
	}
	return s.completeTelegramJobAndWakeChannel(ctx, job)
}

func (s *sSysPublish) appendTelegramJobLog(ctx context.Context, job telegramJobRecord, action string, status string, message string) {
	_, err := g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
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
	if err != nil {
		observeDurableLogWriteFailure(ctx, "telegram_job")
		g.Log().Warningf(ctx, "TG任务持久日志写入失败 jobId:%d action:%s status:%s err:%+v", job.Id, action, status, err)
	}
}
