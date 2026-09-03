package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) markCollectDispatchFailed(ctx context.Context, dispatchId int64, message string) error {
	if dispatchId <= 0 {
		return nil
	}
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("event_id").
		Where("id", dispatchId).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取采集失败分发事件失败")
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("id", dispatchId).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusFailed,
			"error_message": message,
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集分发失败状态失败")
	}
	if err = s.markCollectEventsFailedByDispatchRows(ctx, rows, message); err != nil {
		return err
	}
	return releaseCollectDedupeLedgerByDispatches(ctx, []int64{dispatchId})
}

func (s *sSysPublish) markCollectEventsFailedByDispatchRows(ctx context.Context, rows gdb.Result, message string) error {
	now := gtime.Now()
	for _, row := range rows {
		eventId := row["event_id"].Int64()
		if eventId <= 0 {
			continue
		}
		if _, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Where("id", eventId).
			Where("status", sysin.CollectEventStatusDispatched).
			Data(g.Map{
				"status":        sysin.CollectEventStatusFailed,
				"error_message": message,
				"processed_at":  now,
				"updated_at":    now,
			}).
			Update(); err != nil {
			return gerror.Wrap(err, "更新采集事件失败状态失败")
		}
	}
	return nil
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
