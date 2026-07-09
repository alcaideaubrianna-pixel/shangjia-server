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
		LeftJoin(pdao.YoubanPublishCollectEvent.Table()+" e", "e.id=d.event_id").
		Fields("d.event_id").
		Where("d.status", sysin.CollectDispatchStatusSent).
		Where("e.status", sysin.CollectEventStatusDispatched).
		WhereGT("d.event_id", 0).
		Group("d.event_id").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取已发送采集事件失败")
	}
	now := gtime.Now()
	for _, row := range rows {
		eventId := row["event_id"].Int64()
		if eventId <= 0 {
			continue
		}
		if _, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Where("id", eventId).
			Where("status", sysin.CollectEventStatusDispatched).
			Data(g.Map{
				"status":        sysin.CollectEventStatusProcessed,
				"error_message": "",
				"processed_at":  now,
				"updated_at":    now,
			}).
			Update(); err != nil {
			return gerror.Wrap(err, "恢复采集事件完成状态失败")
		}
	}
	return nil
}
