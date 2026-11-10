package sys

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

func deliverMediaProcessIntent(ctx context.Context, row mediaProcessDelivery, client *asynq.Client, inspector *asynq.Inspector) (string, string, error) {
	if row.TaskID == "" {
		id, ended, err := legacyMediaProcessOwner(ctx, inspector, row.MediaID)
		if err != nil {
			return "", "", err
		}
		if (row.Bootstrap && ended) || id == "" {
			g.Log().Warningf(ctx, "媒体历史任务已结束，补录不重置重试次数 mediaId:%d", row.MediaID)
			return "blocked", "旧任务已归档或完成，需显式确认后重新处理", nil
		}
		// Freeze the compatible task ID before the Redis write. Retrying this
		// intent after a crash must not select another archived-repair generation.
		result, err := g.DB().Exec(ctx, `UPDATE hg_youban_publish_media_process_outbox
 SET task_id=?,updated_at=clock_timestamp()
 WHERE media_id=? AND generation=? AND status='leased' AND lease_token=? AND task_id=''`, id, row.MediaID, row.Generation, row.LeaseToken)
		if err != nil {
			return "", "", err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return "", "", err
		}
		if n != 1 {
			return "", "", fmt.Errorf("媒体 %d 的投递租约已变更", row.MediaID)
		}
		row.TaskID = id
	}
	return "sent", "", enqueueMediaProcessTask(ctx, client, row.MediaID, row.TaskID, 0)
}
