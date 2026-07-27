package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	hglock "hotgo/internal/library/hgrds/lock"
)

const (
	publishFullPushBatchTable = "hg_youban_publish_full_push_batch"
	fullPushBatchPending      = "pending"
	fullPushBatchRunning      = "running"
	fullPushBatchCompleted    = "completed"
	fullPushBatchFailed       = "failed"
)

type fullPushBatchRecord struct {
	Id                int64       `json:"id"`
	BatchNo           string      `json:"batchNo"`
	TenantId          int64       `json:"tenantId"`
	ChannelId         int64       `json:"channelId"`
	RequestedBy       int64       `json:"requestedBy"`
	SnapshotMaxTaskId int64       `json:"snapshotMaxTaskId"`
	CursorTaskId      int64       `json:"cursorTaskId"`
	TotalCount        int         `json:"totalCount"`
	QueuedCount       int         `json:"queuedCount"`
	RetryCount        int         `json:"retryCount"`
	Status            string      `json:"status"`
	ActiveKey         string      `json:"activeKey"`
	ErrorMessage      string      `json:"errorMessage"`
	FinishedAt        *gtime.Time `json:"finishedAt"`
}

type fullPushSnapshot struct {
	SnapshotMaxTaskId int64 `json:"snapshotMaxTaskId"`
	TotalCount        int   `json:"totalCount"`
}

func ensureFullPushBatchSchema(ctx context.Context) error {
	isPgsql := strings.EqualFold(g.DB().GetConfig().Type, "pgsql")
	statement := ""
	if isPgsql {
		statement = `CREATE TABLE IF NOT EXISTS "hg_youban_publish_full_push_batch" (
			"id" bigserial PRIMARY KEY,
			"batch_no" varchar(128) NOT NULL,
			"tenant_id" bigint NOT NULL DEFAULT 0,
			"channel_id" bigint NOT NULL DEFAULT 0,
			"requested_by" bigint NOT NULL DEFAULT 0,
			"snapshot_max_task_id" bigint NOT NULL DEFAULT 0,
			"cursor_task_id" bigint NOT NULL DEFAULT 0,
			"total_count" integer NOT NULL DEFAULT 0,
			"queued_count" integer NOT NULL DEFAULT 0,
			"retry_count" integer NOT NULL DEFAULT 0,
			"status" varchar(16) NOT NULL DEFAULT 'pending',
			"active_key" varchar(64) DEFAULT NULL,
			"error_message" text,
			"created_at" timestamp DEFAULT NULL,
			"updated_at" timestamp DEFAULT NULL,
			"finished_at" timestamp DEFAULT NULL
		)`
	} else {
		statement = "CREATE TABLE IF NOT EXISTS `hg_youban_publish_full_push_batch` (`id` bigint(20) NOT NULL AUTO_INCREMENT,`batch_no` varchar(128) NOT NULL,`tenant_id` bigint(20) NOT NULL DEFAULT '0',`channel_id` bigint(20) NOT NULL DEFAULT '0',`requested_by` bigint(20) NOT NULL DEFAULT '0',`snapshot_max_task_id` bigint(20) NOT NULL DEFAULT '0',`cursor_task_id` bigint(20) NOT NULL DEFAULT '0',`total_count` int(11) NOT NULL DEFAULT '0',`queued_count` int(11) NOT NULL DEFAULT '0',`retry_count` int(11) NOT NULL DEFAULT '0',`status` varchar(16) NOT NULL DEFAULT 'pending',`active_key` varchar(64) DEFAULT NULL,`error_message` text,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,`finished_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_full_push_batch_no` (`batch_no`),UNIQUE KEY `uk_ybp_full_push_active` (`active_key`),KEY `idx_ybp_full_push_schedule` (`status`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='频道全量推送批次'"
	}
	if _, err := g.DB().Exec(ctx, statement); err != nil {
		return gerror.Wrap(err, "初始化全量推送批次表失败")
	}
	if !isPgsql {
		return nil
	}
	for _, indexSQL := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_full_push_batch_no" ON "hg_youban_publish_full_push_batch" ("batch_no")`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_full_push_active" ON "hg_youban_publish_full_push_batch" ("active_key")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_full_push_schedule" ON "hg_youban_publish_full_push_batch" ("status", "id")`,
	} {
		if _, err := g.DB().Exec(ctx, indexSQL); err != nil {
			return gerror.Wrap(err, "初始化全量推送批次索引失败")
		}
	}
	return nil
}

func (s *sSysPublish) createFullPushBatch(ctx context.Context, tenantId, channelId, requestedBy int64) (*fullPushBatchRecord, error) {
	if err := ensureFullPushBatchSchema(ctx); err != nil {
		return nil, err
	}
	activeKey := fmt.Sprintf("%d:%d", tenantId, channelId)
	var active fullPushBatchRecord
	if err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).
		Where("active_key", activeKey).
		WhereIn("status", []string{fullPushBatchPending, fullPushBatchRunning}).
		OrderDesc("id").
		Scan(&active); err != nil {
		return nil, gerror.Wrap(err, "读取进行中的全量推送批次失败")
	}
	if active.Id > 0 {
		return &active, nil
	}
	snapshot, err := s.fullPushSnapshot(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	now := gtime.Now()
	batchNo := newFullPushBatchNo(channelId, now.TimestampNano())
	id, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Data(g.Map{
		"batch_no":             batchNo,
		"tenant_id":            tenantId,
		"channel_id":           channelId,
		"requested_by":         requestedBy,
		"snapshot_max_task_id": snapshot.SnapshotMaxTaskId,
		"cursor_task_id":       0,
		"total_count":          snapshot.TotalCount,
		"queued_count":         0,
		"retry_count":          0,
		"status":               fullPushBatchPending,
		"active_key":           activeKey,
		"error_message":        "",
		"created_at":           now,
		"updated_at":           now,
	}).InsertAndGetId()
	if err != nil {
		if isDuplicateKeyError(err) {
			if scanErr := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).
				Where("active_key", activeKey).
				Scan(&active); scanErr == nil && active.Id > 0 {
				return &active, nil
			}
		}
		return nil, gerror.Wrap(err, "创建全量推送批次失败")
	}
	return &fullPushBatchRecord{
		Id:                id,
		BatchNo:           batchNo,
		TenantId:          tenantId,
		ChannelId:         channelId,
		RequestedBy:       requestedBy,
		SnapshotMaxTaskId: snapshot.SnapshotMaxTaskId,
		TotalCount:        snapshot.TotalCount,
		Status:            fullPushBatchPending,
		ActiveKey:         activeKey,
	}, nil
}

func (s *sSysPublish) fullPushSnapshot(ctx context.Context, tenantId int64) (*fullPushSnapshot, error) {
	var snapshot fullPushSnapshot
	err := fullPushPublishedTaskBaseModel(ctx, tenantId).
		Fields("COALESCE(MAX(t.id), 0) AS snapshot_max_task_id, COUNT(*) AS total_count").
		Scan(&snapshot)
	if err != nil {
		return nil, gerror.Wrap(err, "读取全量推送资料快照失败")
	}
	return &snapshot, nil
}

func (s *sSysPublish) runFullPushBatchScheduler(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	time.Sleep(time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.dispatchFullPushBatches(ctx, 5); err != nil && ctx.Err() == nil {
				g.Log().Warningf(ctx, "调度全量推送批次失败：%+v", err)
			}
		}
	}
}

func (s *sSysPublish) dispatchFullPushBatches(ctx context.Context, limit int) error {
	if err := ensureFullPushBatchSchema(ctx); err != nil {
		return err
	}
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	lock := hglock.NewConfig(15*time.Second, 100*time.Millisecond).Mutex("youban_publish:full_push_batch")
	if err := lock.TryLock(lockCtx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return nil
		}
		return gerror.Wrap(err, "获取全量推送调度锁失败")
	}
	defer s.releaseTelegramChannelLease(context.Background(), lock)
	var batches []fullPushBatchRecord
	if err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).
		WhereIn("status", []string{fullPushBatchPending, fullPushBatchRunning}).
		OrderAsc("id").
		Limit(limit).
		Scan(&batches); err != nil {
		return gerror.Wrap(err, "读取待处理全量推送批次失败")
	}
	for _, batch := range batches {
		if err := s.advanceFullPushBatch(ctx, batch, 200); err != nil {
			g.Log().Warningf(ctx, "推进全量推送批次失败 batch:%s err:%+v", batch.BatchNo, err)
		}
	}
	return nil
}

func (s *sSysPublish) advanceFullPushBatch(ctx context.Context, batch fullPushBatchRecord, limit int) error {
	if _, err := s.fullPushChannel(ctx, batch.TenantId, batch.ChannelId); err != nil {
		return s.failFullPushBatch(ctx, batch, err)
	}
	_, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).
		Where("id", batch.Id).
		WhereIn("status", []string{fullPushBatchPending, fullPushBatchRunning}).
		Data(g.Map{"status": fullPushBatchRunning, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新全量推送批次状态失败")
	}
	ids, err := s.fullPushPublishedTaskIds(ctx, batch.TenantId, batch.CursorTaskId, batch.SnapshotMaxTaskId, limit)
	if err != nil {
		return s.retryFullPushBatch(ctx, batch, err)
	}
	if len(ids) == 0 {
		return s.completeFullPushBatch(ctx, batch.Id)
	}
	lastTaskId := batch.CursorTaskId
	queued := 0
	for _, taskId := range ids {
		operationNo := fullPushTaskOperationNo(batch.BatchNo, taskId)
		if err = s.enqueueFullPushTelegramJob(ctx, taskId, batch.ChannelId, operationNo); err != nil {
			if updateErr := s.checkpointFullPushBatch(ctx, batch.Id, lastTaskId, queued); updateErr != nil {
				return updateErr
			}
			if queued > 0 {
				batch.RetryCount = 0
			}
			return s.retryFullPushBatch(ctx, batch, err)
		}
		lastTaskId = taskId
		queued++
	}
	if err = s.checkpointFullPushBatch(ctx, batch.Id, lastTaskId, queued); err != nil {
		return err
	}
	if len(ids) < limit || lastTaskId >= batch.SnapshotMaxTaskId {
		return s.completeFullPushBatch(ctx, batch.Id)
	}
	return nil
}

func newFullPushBatchNo(channelId, timestampNano int64) string {
	return fmt.Sprintf("full_push:%d:%d", channelId, timestampNano)
}

func fullPushTaskOperationNo(batchNo string, taskId int64) string {
	return fmt.Sprintf("%s:%d", batchNo, taskId)
}

func (s *sSysPublish) checkpointFullPushBatch(ctx context.Context, batchId, cursorTaskId int64, queued int) error {
	data := g.Map{"cursor_task_id": cursorTaskId, "retry_count": 0, "error_message": "", "updated_at": gtime.Now()}
	if queued > 0 {
		data["queued_count"] = gdb.Raw(fmt.Sprintf("queued_count + %d", queued))
	}
	_, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Where("id", batchId).Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "保存全量推送批次游标失败")
	}
	return nil
}

func (s *sSysPublish) retryFullPushBatch(ctx context.Context, batch fullPushBatchRecord, cause error) error {
	retryCount := batch.RetryCount + 1
	data := g.Map{
		"status":        fullPushBatchPending,
		"retry_count":   retryCount,
		"error_message": cause.Error(),
		"updated_at":    gtime.Now(),
	}
	if retryCount >= 10 {
		data["status"] = fullPushBatchFailed
		data["active_key"] = nil
		data["finished_at"] = gtime.Now()
	}
	_, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Where("id", batch.Id).Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "保存全量推送批次重试状态失败")
	}
	return cause
}

func (s *sSysPublish) failFullPushBatch(ctx context.Context, batch fullPushBatchRecord, cause error) error {
	_, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Where("id", batch.Id).Data(g.Map{
		"status":        fullPushBatchFailed,
		"active_key":    nil,
		"error_message": cause.Error(),
		"finished_at":   gtime.Now(),
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "标记全量推送批次失败")
	}
	return cause
}

func (s *sSysPublish) completeFullPushBatch(ctx context.Context, batchId int64) error {
	_, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Where("id", batchId).Data(g.Map{
		"status":        fullPushBatchCompleted,
		"active_key":    nil,
		"error_message": "",
		"finished_at":   gtime.Now(),
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "完成全量推送批次失败")
	}
	return nil
}
