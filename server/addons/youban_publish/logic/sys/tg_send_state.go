package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) updateTelegramMediaFileIds(ctx context.Context, messages []*telegramSentMessage) error {
	now := gtime.Now()
	for _, item := range messages {
		if item == nil || item.MediaId <= 0 || item.TgFileId == "" || strings.HasPrefix(item.AssetHash, "anti-scan:") {
			continue
		}
		_, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("id", item.MediaId).
			Data(g.Map{
				"tg_file_id":          item.TgFileId,
				"tg_cache_asset_hash": item.AssetHash,
				"tg_cache_status":     tgCacheStatusValid,
				"updated_at":          now,
			}).
			Update()
		if err != nil {
			return gerror.Wrap(err, "更新TG媒体file_id失败")
		}
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
