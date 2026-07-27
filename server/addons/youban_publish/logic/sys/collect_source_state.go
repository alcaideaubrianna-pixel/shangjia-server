package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
)

const collectSourceDisabledMessage = "采集源已关闭"

func (s *sSysPublish) collectSourcePushEnabled(ctx context.Context, sourceId int64, tenantId int64, accountId int64) (bool, error) {
	if sourceId <= 0 {
		return true, nil
	}
	mod := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", sourceId).
		Where("collect_enabled", 1).
		Where("status", 1).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	count, err := mod.Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查采集源状态失败")
	}
	return count > 0, nil
}

func (s *sSysPublish) stopDisabledCollectSourcePipeline(ctx context.Context, sourceId int64, tenantId int64, accountId int64) error {
	profileIds, err := s.collectSourceProfileIds(ctx, sourceId, tenantId, accountId)
	if err != nil {
		return err
	}
	if err = s.supersedeCollectSourcePendingJobs(ctx, profileIds, tenantId); err != nil {
		return err
	}
	now := gtime.Now()
	if len(profileIds) > 0 {
		for _, profileId := range profileIds {
			if _, err = s.syncProfilePublishState(ctx, profileId, 0, consts.ContentVisibilityPrivate, nil); err != nil {
				return err
			}
		}
		if err = s.deactivateChannelProfiles(ctx, tenantId, profileIds); err != nil {
			return err
		}
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusSkipped,
			"error_message": collectSourceDisabledMessage,
			"finished_at":   now,
			"updated_at":    now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "取消采集源待分发任务失败")
	}
	_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("status", []string{
			sysin.CollectEventStatusPending,
			sysin.CollectEventStatusGroupCollect,
			sysin.CollectEventStatusWaitingOrder,
			sysin.CollectEventStatusPrechecked,
			sysin.CollectEventStatusMediaPending,
			sysin.CollectEventStatusMediaReady,
		}).
		Data(g.Map{
			"status":        sysin.CollectEventStatusIgnored,
			"error_message": collectSourceDisabledMessage,
			"processed_at":  now,
			"updated_at":    now,
		}).
		Update()
	return gerror.Wrap(err, "停止采集源待处理事件失败")
}
