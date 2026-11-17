package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// finishCollectEventDispatchHandoff seals fanout admission and folds in delivery
// callbacks that already finished every branch. Serialize on the parent row:
// an earlier failure prevents admission; a later callback records parent failure.
func (s *sSysPublish) finishCollectEventDispatchHandoff(ctx context.Context, eventID int64) error {
	if eventID <= 0 {
		return gerror.New("采集事件交接参数不完整")
	}
	completed := false
	var failureErr error
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		eventCols := pdao.YoubanPublishCollectEvent.Columns()
		eventTable := pdao.YoubanPublishCollectEvent.Table()
		dispatchTable := pdao.YoubanPublishCollectDispatch.Table()
		dispatchCols := pdao.YoubanPublishCollectDispatch.Columns()
		event, err := tx.Model(eventTable).Ctx(ctx).
			Fields(eventCols.Id, eventCols.Status, eventCols.ErrorMessage).Where(eventCols.Id, eventID).LockUpdate().One()
		if err != nil {
			return gerror.Wrap(err, "锁定采集事件交接状态失败")
		}
		if event.IsEmpty() {
			return gerror.New("采集事件不存在")
		}
		if event[eventCols.Status].String() == sysin.CollectEventStatusIgnored {
			return gerror.New("采集事件已忽略，不能完成分发交接")
		}
		finishedCondition := "(" + eventCols.Status + "='processed' OR (" + collectEventAllDispatchesFinishedCondition(eventTable) + "))"
		result, err := tx.Model(eventTable).Ctx(ctx).Where(eventCols.Id, eventID).
			Where("EXISTS (SELECT 1 FROM "+dispatchTable+" branch WHERE branch."+dispatchCols.EventId+"="+eventTable+"."+eventCols.Id+")").
			Where("NOT EXISTS (SELECT 1 FROM "+dispatchTable+" branch WHERE branch."+dispatchCols.EventId+"="+eventTable+"."+eventCols.Id+" AND branch."+dispatchCols.Status+"=?)", sysin.CollectDispatchStatusFailed).
			Data(g.Map{
				// Fast delivery may finish while the parent is still media_ready.
				// Settle here: pending-only recovery will not revisit sent branches.
				eventCols.Status:       gdb.Raw("CASE WHEN " + finishedCondition + " THEN 'processed' ELSE 'dispatched' END"),
				eventCols.ProcessedAt:  gdb.Raw("CASE WHEN " + finishedCondition + " THEN COALESCE(" + eventCols.ProcessedAt + ",CURRENT_TIMESTAMP) ELSE NULL END"),
				eventCols.ErrorMessage: "",
			}).Update()
		if err != nil {
			return gerror.Wrap(err, "保存采集事件交接状态失败")
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return gerror.Wrap(err, "确认采集事件交接结果失败")
		}
		if affected == 1 {
			completed = true
			return nil
		}
		failed, err := tx.Model(dispatchTable).Ctx(ctx).
			Fields(dispatchCols.Id, dispatchCols.ErrorMessage).
			Where(dispatchCols.EventId, eventID).Where(dispatchCols.Status, sysin.CollectDispatchStatusFailed).
			OrderAsc(dispatchCols.Id).Limit(1).One()
		if err != nil {
			return gerror.Wrap(err, "读取采集分支失败原因失败")
		}
		if !failed.IsEmpty() {
			message := failed[dispatchCols.ErrorMessage].String()
			if event[eventCols.Status].String() == sysin.CollectEventStatusFailed && event[eventCols.ErrorMessage].String() != "" {
				message = event[eventCols.ErrorMessage].String()
			}
			if message == "" {
				message = "采集分支发送失败"
			}
			result, err = tx.Model(eventTable).Ctx(ctx).Where(eventCols.Id, eventID).
				Data(g.Map{
					eventCols.Status: sysin.CollectEventStatusFailed, eventCols.ErrorMessage: message,
					eventCols.ProcessedAt: gdb.Raw("COALESCE(" + eventCols.ProcessedAt + ",CURRENT_TIMESTAMP)"),
				}).Update()
			if err != nil {
				return gerror.Wrap(err, "保留采集事件失败状态失败")
			}
			affected, err = result.RowsAffected()
			if err != nil {
				return gerror.Wrap(err, "确认采集事件失败状态失败")
			}
			if affected != 1 {
				return gerror.New("采集事件失败状态未保存")
			}
			// Commit failure before returning a normal error. The original
			// shouldRecoverCollectEvent policy decides whether it can retry.
			failureErr = gerror.New(message)
			return nil
		}
		branch, err := tx.Model(dispatchTable).Ctx(ctx).Fields(dispatchCols.Id).
			Where(dispatchCols.EventId, eventID).Limit(1).One()
		if err != nil {
			return gerror.Wrap(err, "复查采集分支交接状态失败")
		}
		if !branch.IsEmpty() {
			return gerror.New("采集分支交接状态已变化，请重新核对")
		}
		// Only missing dispatches use an explicit retry. Returning the retry
		// from inside this transaction would roll back the repair.
		result, err = tx.Model(eventTable).Ctx(ctx).Where(eventCols.Id, eventID).
			Data(g.Map{eventCols.Status: sysin.CollectEventStatusMediaReady, eventCols.ProcessedAt: nil}).Update()
		if err != nil {
			return gerror.Wrap(err, "保留采集事件待重试状态失败")
		}
		affected, err = result.RowsAffected()
		if err != nil {
			return gerror.Wrap(err, "确认采集事件待重试状态失败")
		}
		if affected != 1 {
			return gerror.New("采集事件待重试状态未保存")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if failureErr != nil {
		return failureErr
	}
	if !completed {
		return newCollectProcessRetryError(2*time.Second, "采集分支交接未完成，等待重试")
	}
	s.syncCollectSourceStatsForEvent(ctx, eventID)
	return nil
}

// The UPDATE takes the same parent row lock as the finalizer. Keep asynchronous
// failures terminal unless the existing shouldRecoverCollectEvent policy says
// otherwise; do not turn a hard TG failure into an unconditional process retry.
func (s *sSysPublish) reopenCollectEventAfterDispatchFailure(ctx context.Context, eventID int64, message string) error {
	cols := pdao.YoubanPublishCollectEvent.Columns()
	_, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where(cols.Id, eventID).
		WhereIn(cols.Status, []string{
			sysin.CollectEventStatusMediaReady, sysin.CollectEventStatusDispatched,
			sysin.CollectEventStatusProcessed, sysin.CollectEventStatusFailed,
		}).
		Data(g.Map{
			cols.Status: sysin.CollectEventStatusFailed, cols.ErrorMessage: message,
			cols.ProcessedAt: gdb.Raw("COALESCE(" + cols.ProcessedAt + ",CURRENT_TIMESTAMP)"),
		}).Update()
	return gerror.Wrap(err, "保存采集分支失败事件状态失败")
}
