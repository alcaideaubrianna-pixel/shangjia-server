package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	taskBatchTable     = "hg_youban_publish_task_batch"
	taskItemTable      = "hg_youban_publish_task_item"
	taskExecutionTable = "hg_youban_publish_task_execution"
)

type taskBatchInput struct {
	TenantID  int64
	AccountID int64
	Type      string
	Deadline  time.Time
}

type taskItemInput struct {
	ProfileID int64
	ChannelID int64
	Deadline  time.Time
}

// createTaskBatchTx creates only the durable business batch. Queue delivery is
// intentionally separate so callers can add all items in the same transaction.
func createTaskBatchTx(ctx context.Context, tx gdb.TX, in taskBatchInput, items []taskItemInput) (int64, error) {
	if in.TenantID <= 0 || in.AccountID <= 0 || in.Type == "" {
		return 0, gerror.New("统一任务批次参数不完整")
	}
	now := gtime.Now()
	deadline := gtime.New(in.Deadline)
	result, err := tx.Model(taskBatchTable).Ctx(ctx).Data(g.Map{
		"tenant_id": in.TenantID, "account_id": in.AccountID, "type": in.Type,
		"status": taskStatusPending, "total_count": len(items), "pending_count": len(items),
		"deadline_at": deadline, "created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		return 0, gerror.Wrap(err, "创建统一任务批次失败")
	}
	batchID, err := result.LastInsertId()
	if err != nil || batchID <= 0 {
		return 0, gerror.Wrap(err, "读取统一任务批次编号失败")
	}
	if len(items) == 0 {
		return batchID, nil
	}
	rows := make([]g.Map, 0, len(items))
	for _, item := range items {
		rows = append(rows, g.Map{
			"batch_id": batchID, "tenant_id": in.TenantID, "account_id": in.AccountID,
			"profile_id": item.ProfileID, "channel_id": item.ChannelID,
			"status": taskStatusPending, "deadline_at": gtime.New(item.Deadline),
			"created_at": now, "updated_at": now,
		})
	}
	if _, err = tx.Model(taskItemTable).Ctx(ctx).Data(rows).Insert(); err != nil {
		return 0, gerror.Wrap(err, "批量创建统一任务项失败")
	}
	return batchID, nil
}

func createTaskBatch(ctx context.Context, in taskBatchInput, items []taskItemInput) (int64, error) {
	var id int64
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var err error
		id, err = createTaskBatchTx(ctx, tx, in, items)
		return err
	})
	return id, err
}

// claimTaskItem atomically creates the next execution attempt and marks the
// business item running. It is safe for concurrent workers.
func claimTaskItem(ctx context.Context, itemID int64, owner string, lease time.Duration) (int64, bool, error) {
	if itemID <= 0 || owner == "" || lease <= 0 {
		return 0, false, gerror.New("领取统一任务项参数不合法")
	}
	var executionID int64
	claimed := false
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		result, err := tx.Model(taskItemTable).Ctx(ctx).
			Where("id", itemID).
			Where("status", taskStatusPending).
			Data(g.Map{"status": taskStatusRunning, "started_at": now, "updated_at": now}).Update()
		if err != nil {
			return gerror.Wrap(err, "领取统一任务项失败")
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return nil
		}
		claimed = true
		var last struct {
			AttemptNo int `orm:"attempt_no"`
		}
		if err = tx.Model(taskExecutionTable).Ctx(ctx).Fields("attempt_no").Where("item_id", itemID).OrderDesc("attempt_no").Limit(1).Scan(&last); err != nil {
			return gerror.Wrap(err, "读取统一任务重试次数失败")
		}
		result, err = tx.Model(taskExecutionTable).Ctx(ctx).Data(g.Map{
			"item_id": itemID, "attempt_no": last.AttemptNo + 1,
			"status": taskExecutionRunning, "lease_owner": owner, "lease_until": now.Add(lease),
			"started_at": now, "created_at": now, "updated_at": now,
		}).Insert()
		if err != nil {
			return gerror.Wrap(err, "创建统一任务执行记录失败")
		}
		executionID, err = result.LastInsertId()
		return err
	})
	return executionID, claimed, err
}

func finishTaskItem(ctx context.Context, itemID, executionID int64, status, errorCode, message string) error {
	if itemID <= 0 || executionID <= 0 {
		return gerror.New("完成统一任务参数不合法")
	}
	if status != taskStatusSuccess && status != taskStatusFailed && status != taskStatusUnknown && status != taskStatusCancelled {
		return gerror.Newf("统一任务终态不合法：%s", status)
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		if _, err := tx.Model(taskExecutionTable).Ctx(ctx).Where("id", executionID).Data(g.Map{
			"status": status, "finished_at": now, "lease_owner": "", "lease_until": nil,
			"error_code": errorCode, "error_message": message, "updated_at": now,
		}).Update(); err != nil {
			return gerror.Wrap(err, "更新统一任务执行状态失败")
		}
		if _, err := tx.Model(taskItemTable).Ctx(ctx).Where("id", itemID).
			WhereIn("status", []string{taskStatusRunning, taskStatusPending}).Data(g.Map{
			"status": status, "completed_at": now, "error_code": errorCode,
			"error_message": message, "updated_at": now,
		}).Update(); err != nil {
			return gerror.Wrap(err, "更新统一任务项状态失败")
		}
		return refreshTaskBatchSummaryTx(ctx, tx, itemID)
	})
}

// expireTaskExecutions is deliberately bounded. A cron/runtime caller can
// invoke it repeatedly without scanning the full history table.
func expireTaskExecutions(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var rows []struct {
		ID     int64 `orm:"id"`
		ItemID int64 `orm:"item_id"`
	}
	if err := g.DB().Model(taskExecutionTable).Ctx(ctx).Fields("id,item_id").
		Where("status", taskExecutionRunning).WhereLT("lease_until", gtime.Now()).
		OrderAsc("lease_until").Limit(limit).Scan(&rows); err != nil {
		return 0, gerror.Wrap(err, "读取过期统一任务执行记录失败")
	}
	count := 0
	for _, row := range rows {
		if err := finishTaskItem(ctx, row.ItemID, row.ID, taskStatusUnknown, "execution_lease_expired", "统一任务执行租约已过期"); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func refreshTaskBatchSummaryTx(ctx context.Context, tx gdb.TX, itemID int64) error {
	var row struct {
		BatchID int64 `orm:"batch_id"`
	}
	if err := tx.Model(taskItemTable).Ctx(ctx).Fields("batch_id").Where("id", itemID).Scan(&row); err != nil {
		return err
	}
	if row.BatchID <= 0 {
		return fmt.Errorf("统一任务项不存在：%d", itemID)
	}
	var s taskBatchSummary
	if err := tx.Model(taskItemTable).Ctx(ctx).Fields("COUNT(*) AS total, SUM(CASE WHEN status='pending' THEN 1 ELSE 0 END) AS pending, SUM(CASE WHEN status='running' THEN 1 ELSE 0 END) AS running, SUM(CASE WHEN status='success' THEN 1 ELSE 0 END) AS success, SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END) AS failed, SUM(CASE WHEN status='unknown' THEN 1 ELSE 0 END) AS unknown, SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END) AS cancelled").Where("batch_id", row.BatchID).Scan(&s); err != nil {
		return err
	}
	data := g.Map{
		"status": s.status(), "total_count": s.Total, "pending_count": s.Pending, "running_count": s.Running,
		"success_count": s.Success, "failed_count": s.Failed, "unknown_count": s.Unknown, "cancelled_count": s.Cancelled,
		"updated_at": gtime.Now(),
	}
	if s.Pending == 0 && s.Running == 0 {
		data["completed_at"] = gtime.Now()
	}
	_, err := tx.Model(taskBatchTable).Ctx(ctx).Where("id", row.BatchID).Data(data).Update()
	return err
}
