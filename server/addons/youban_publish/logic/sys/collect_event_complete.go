package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) recoverProcessedCollectEvents(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 200
	}
	rows, err := g.DB().Model(pdao.YoubanPublishCollectDispatch.Table()+" d").Safe().Ctx(ctx).
		InnerJoin(pdao.YoubanPublishCollectEvent.Table()+" e", "e.id=d.event_id").
		Fields("d.event_id,e.source_id").
		Where("d.status", sysin.CollectDispatchStatusSent).
		Where("e.status", sysin.CollectEventStatusDispatched).
		WhereGT("d.event_id", 0).
		Group("d.event_id,e.source_id").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取已发送采集事件失败")
	}
	now := gtime.Now()
	dirtySourceIds := make(map[int64]struct{})
	for _, row := range rows {
		eventId := row["event_id"].Int64()
		if eventId <= 0 {
			continue
		}
		result, updateErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Where("id", eventId).
			Where("status", sysin.CollectEventStatusDispatched).
			Data(g.Map{
				"status":        sysin.CollectEventStatusProcessed,
				"error_message": "",
				"processed_at":  now,
				"updated_at":    now,
			}).
			Update()
		if updateErr != nil {
			return gerror.Wrap(updateErr, "恢复采集事件完成状态失败")
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return gerror.Wrap(affectedErr, "确认采集事件完成状态失败")
		}
		if affected == 0 {
			continue
		}
		if sourceId := row["source_id"].Int64(); sourceId > 0 {
			dirtySourceIds[sourceId] = struct{}{}
		}
	}
	for sourceId := range dirtySourceIds {
		if statsErr := s.syncCollectSourceStats(ctx, sourceId); statsErr != nil {
			g.Log().Warningf(ctx, "同步采集源统计失败 sourceId:%d err:%+v", sourceId, statsErr)
		}
	}
	return nil
}
