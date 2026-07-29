package fix

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const collectEventRecoveryStatuses = "'pending','group_collecting','waiting_order','prechecked','media_pending','media_ready'"

// RequeueYoubanPublishCollectEvents marks recoverable collection events stale.
// The runtime recovery loop will enqueue them without changing their business data.
func RequeueYoubanPublishCollectEvents(ctx context.Context) error {
	if err := clearLegacyYoubanPublishCollectQueues(ctx); err != nil {
		return err
	}
	if err := RebuildYoubanPublishCollectMaterialGroups(ctx); err != nil {
		return err
	}
	mediaResult, err := g.DB().Model("hg_youban_publish_collect_event_media").Safe().Ctx(ctx).
		Where("cache_status", "downloading").
		Data(g.Map{
			"cache_status":  "pending",
			"error_message": "旧版采集队列已迁移，媒体任务重新排队",
			"updated_at":    gtime.Now().Add(-time.Minute),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "重置旧版采集媒体失败")
	}
	result, err := g.DB().Model("hg_youban_publish_collect_event").Safe().Ctx(ctx).
		Where("status IN (" + collectEventRecoveryStatuses + ")").
		WhereNull("processed_at").
		Data(g.Map{
			"material_role":            "pending",
			"material_parent_event_id": 0,
			"material_group_status":    "pending",
			"status":                   "pending",
			"error_message":            "",
			"processed_at":             nil,
			"updated_at":               gtime.Now().Add(-time.Minute),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "标记采集事件待恢复失败")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "读取采集事件恢复数量失败")
	}
	g.Log().Infof(ctx, "采集事件待恢复标记完成 count=%d", rows)
	mediaRows, _ := mediaResult.RowsAffected()
	g.Log().Infof(ctx, "采集媒体队列迁移完成 reset=%d", mediaRows)
	return nil
}

// RebuildYoubanPublishCollectMaterialGroups retires reviews created by the old
// realtime verification flow and lets the new delayed grouping flow rebuild them.
func RebuildYoubanPublishCollectMaterialGroups(ctx context.Context) error {
	rows, err := g.DB().Model("hg_youban_publish_collect_review").Safe().Ctx(ctx).
		Fields("id,dispatch_id").
		Where("status", "pending").
		All()
	if err != nil {
		return gerror.Wrap(err, "读取旧采集审核失败")
	}
	now := gtime.Now()
	for _, row := range rows {
		if _, err = g.DB().Model("hg_youban_publish_collect_review").Safe().Ctx(ctx).
			Where("id", row["id"].Int64()).
			Data(g.Map{
				"status":        "rejected",
				"review_reason": "采集分组链路升级，等待重新处理",
				"reviewed_at":   now,
				"updated_at":    now,
			}).Update(); err != nil {
			return gerror.Wrap(err, "关闭旧采集审核失败")
		}
		if dispatchID := row["dispatch_id"].Int64(); dispatchID > 0 {
			if _, err = g.DB().Model("hg_youban_publish_collect_dispatch").Safe().Ctx(ctx).
				Where("id", dispatchID).
				Where("status", "reviewing").
				Data(g.Map{
					"status":        "skipped",
					"error_message": "旧采集分组审核已关闭，等待重新分组",
					"updated_at":    now,
				}).Update(); err != nil {
				return gerror.Wrap(err, "关闭旧采集审核分发失败")
			}
		}
	}
	result, err := g.DB().Model("hg_youban_publish_collect_event").Safe().Ctx(ctx).
		Where("status IN (" + collectEventRecoveryStatuses + ")").
		WhereNull("processed_at").
		Data(g.Map{
			"material_role":            "pending",
			"material_parent_event_id": 0,
			"material_group_status":    "pending",
			"status":                   "pending",
			"error_message":            "",
			"processed_at":             nil,
			"updated_at":               now.Add(-time.Minute),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "重置采集资料组失败")
	}
	count, _ := result.RowsAffected()
	g.Log().Infof(ctx, "采集资料组重建准备完成 reviews=%d events=%d", len(rows), count)
	return nil
}

func clearLegacyYoubanPublishCollectQueues(ctx context.Context) error {
	patterns := make([]string, 0, 32)
	for index := 0; index < 16; index++ {
		patterns = append(patterns,
			fmt.Sprintf("asynq:{youban_publish_collect_%02d}:*", index),
			fmt.Sprintf("asynq:{youban_publish_collect_media_%02d}:*", index),
		)
	}
	deleted := int64(0)
	for _, pattern := range patterns {
		keys, err := g.Redis().Keys(ctx, pattern)
		if err != nil {
			return gerror.Wrapf(err, "读取旧版采集队列失败 pattern=%s", pattern)
		}
		if len(keys) == 0 {
			continue
		}
		count, err := g.Redis().Del(ctx, keys...)
		if err != nil {
			return gerror.Wrapf(err, "清理旧版采集队列失败 pattern=%s", pattern)
		}
		deleted += count
	}
	g.Log().Infof(ctx, "旧版采集队列清理完成 keys=%d", deleted)
	return nil
}
