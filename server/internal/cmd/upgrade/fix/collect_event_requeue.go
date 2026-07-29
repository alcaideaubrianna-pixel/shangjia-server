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
		Data(g.Map{"updated_at": gtime.Now().Add(-time.Minute)}).
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
