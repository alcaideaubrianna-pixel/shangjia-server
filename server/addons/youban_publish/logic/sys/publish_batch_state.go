package sys

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type publishBatchJobCounts struct {
	Total   int `orm:"total"`
	Pending int `orm:"pending"`
	Sent    int `orm:"sent"`
	Failed  int `orm:"failed"`
}

func publishBatchTerminalState(ctx context.Context, operationPrefix string) (bool, string, string, error) {
	var counts publishBatchJobCounts
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields(`COUNT(*) AS total,
			SUM(CASE WHEN status IN ('pending','sending','failed_retry','unknown') THEN 1 ELSE 0 END) AS pending,
			SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) AS sent,
			SUM(CASE WHEN status IN ('failed','superseded') THEN 1 ELSE 0 END) AS failed`).
		WhereLike("operation_no", operationPrefix+"%").
		Scan(&counts)
	if err != nil {
		return false, "", "", gerror.Wrap(err, "汇总批次TG任务状态失败")
	}
	return resolvePublishBatchState(counts)
}

func resolvePublishBatchState(counts publishBatchJobCounts) (bool, string, string, error) {
	if counts.Pending > 0 {
		return false, "", "", nil
	}
	if counts.Failed == 0 {
		return true, "completed", "", nil
	}
	message := fmt.Sprintf("TG任务完成但存在失败：成功%d，失败%d", counts.Sent, counts.Failed)
	if counts.Sent == 0 && counts.Total > 0 {
		return true, "failed", message, nil
	}
	return true, "partial_failed", message, nil
}
