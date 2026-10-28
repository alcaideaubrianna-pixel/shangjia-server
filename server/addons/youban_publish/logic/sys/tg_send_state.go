package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) updateTelegramMediaFileIds(ctx context.Context, messages []*telegramSentMessage) error {
	now := gtime.Now()
	for _, item := range messages {
		if item == nil {
			g.Log().Warning(ctx, "跳过TG媒体file_id回写：消息为空")
			continue
		}
		if item.MediaId <= 0 || item.TgFileId == "" {
			continue
		}
		if strings.TrimSpace(item.ProcessedAssetHash) != "" {
			_, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
				Where("id", item.MediaId).
				Data(g.Map{"processed_tg_file_id": item.TgFileId, "processed_asset_hash": item.ProcessedAssetHash, "processed_status": "ready", "updated_at": now}).Update()
			if err != nil {
				return gerror.Wrap(err, "更新处理后TG媒体file_id失败")
			}
			continue
		}
		if strings.HasPrefix(item.AssetHash, "anti-scan:") {
			continue
		}
		startedAt := time.Now()
		mediaType, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("id", item.MediaId).
			Fields("media_type").
			Value()
		if err != nil {
			g.Log().Warning(ctx, "读取TG媒体类型失败，file_id回写中止", g.Map{
				"mediaId": item.MediaId, "fileIdLength": len(item.TgFileId), "elapsedMs": time.Since(startedAt).Milliseconds(), "error": err.Error(),
			})
			return gerror.Wrap(err, "读取TG媒体类型失败")
		}
		if !telegramFileIDMatchesMediaType(mediaType.String(), item.TgFileId) {
			g.Log().Warning(ctx, "拒绝回填类型不匹配的TG媒体file_id", g.Map{
				"mediaId": item.MediaId, "mediaType": mediaType.String(), "fileIdLength": len(item.TgFileId), "elapsedMs": time.Since(startedAt).Milliseconds(),
			})
			continue
		}
		result, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("id", item.MediaId).
			Data(g.Map{
				"tg_file_id":          item.TgFileId,
				"tg_cache_asset_hash": item.AssetHash,
				"tg_cache_status":     tgCacheStatusValid,
				"updated_at":          now,
			}).
			Update()
		if err != nil {
			g.Log().Warning(ctx, "更新TG媒体file_id失败", g.Map{
				"mediaId": item.MediaId, "mediaType": mediaType.String(), "fileIdLength": len(item.TgFileId), "elapsedMs": time.Since(startedAt).Milliseconds(), "error": err.Error(),
			})
			return gerror.Wrap(err, "更新TG媒体file_id失败")
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			g.Log().Warning(ctx, "读取TG媒体file_id回写影响行数失败", g.Map{
				"mediaId": item.MediaId, "mediaType": mediaType.String(), "elapsedMs": time.Since(startedAt).Milliseconds(), "error": affectedErr.Error(),
			})
			continue
		}
		if affected == 0 {
			g.Log().Warning(ctx, "TG媒体file_id回写未更新任何记录", g.Map{
				"mediaId": item.MediaId, "mediaType": mediaType.String(), "elapsedMs": time.Since(startedAt).Milliseconds(),
			})
			continue
		}
		g.Log().Debugf(ctx, "TG媒体file_id回写完成 mediaId:%d mediaType:%s fileIdLength:%d elapsedMs:%d", item.MediaId, mediaType.String(), len(item.TgFileId), time.Since(startedAt).Milliseconds())
	}
	return nil
}

func (s *sSysPublish) incrementDailyPublishStat(ctx context.Context, job telegramJobRecord) error {
	today := gtime.Now().Format("Y-m-d")
	now := gtime.Now()
	result, err := g.DB().Model(publishDailyStatTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("account_id", job.AccountId).
		Where("stat_date", today).
		Increment("published_count", 1)
	if err != nil {
		return gerror.Wrap(err, "更新每日上架统计失败")
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return nil
	}
	_, err = g.DB().Model(publishDailyStatTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":         job.TenantId,
		"account_id":        job.AccountId,
		"stat_date":         today,
		"new_profile_count": 1,
		"published_count":   1,
		"failed_count":      0,
		"down_count":        0,
		"created_at":        now,
		"updated_at":        now,
	}).Insert()
	if err != nil {
		return gerror.Wrap(err, "创建每日上架统计失败")
	}
	return nil
}
