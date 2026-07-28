package sys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/dao"
	hglock "hotgo/internal/library/hgrds/lock"
)

const (
	publishFullPushBatchTable = "hg_youban_publish_full_push_batch"
	fullPushBatchPending      = "pending"
	fullPushBatchRunning      = "running"
	fullPushBatchDispatching  = "dispatching"
	fullPushBatchCompleted    = "completed"
	fullPushBatchPartial      = "partial_failed"
	fullPushBatchFailed       = "failed"
)

type fullPushBatchRecord struct {
	Id                   int64       `json:"id"`
	BatchNo              string      `json:"batchNo"`
	TenantId             int64       `json:"tenantId"`
	ChannelId            int64       `json:"channelId"`
	RequestedBy          int64       `json:"requestedBy"`
	SnapshotMaxProfileId int64       `json:"snapshotMaxProfileId" orm:"snapshot_max_profile_id"`
	CursorProfileId      int64       `json:"cursorProfileId" orm:"cursor_profile_id"`
	TotalCount           int         `json:"totalCount"`
	QueuedCount          int         `json:"queuedCount"`
	RetryCount           int         `json:"retryCount"`
	Status               string      `json:"status"`
	ActiveKey            string      `json:"activeKey"`
	ErrorMessage         string      `json:"errorMessage"`
	FinishedAt           *gtime.Time `json:"finishedAt"`
}

type fullPushSnapshot struct {
	SnapshotMaxProfileId int64 `json:"snapshotMaxProfileId" orm:"snapshot_max_profile_id"`
	TotalCount           int   `json:"totalCount" orm:"total_count"`
}

type fullPushProfile struct {
	TenantId  int64 `orm:"tenant_id"`
	AccountId int64 `orm:"account_id"`
	ProfileId int64 `orm:"profile_id"`
}

func (s *sSysPublish) createFullPushBatch(ctx context.Context, tenantId, channelId, requestedBy int64) (*fullPushBatchRecord, error) {
	activeKey := fmt.Sprintf("%d:%d", tenantId, channelId)
	activeRow, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).
		Where("active_key", activeKey).
		WhereIn("status", []string{fullPushBatchPending, fullPushBatchRunning, fullPushBatchDispatching}).
		OrderDesc("id").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取进行中的全量推送批次失败")
	}
	if !activeRow.IsEmpty() {
		var active fullPushBatchRecord
		if err = activeRow.Struct(&active); err != nil {
			return nil, gerror.Wrap(err, "解析进行中的全量推送批次失败")
		}
		if active.Id > 0 {
			return &active, nil
		}
	}
	snapshot, err := s.fullPushSnapshot(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	now := gtime.Now()
	batchNo := newFullPushBatchNo(channelId, now.TimestampNano())
	id, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Data(g.Map{
		"batch_no":                batchNo,
		"tenant_id":               tenantId,
		"channel_id":              channelId,
		"requested_by":            requestedBy,
		"snapshot_max_profile_id": snapshot.SnapshotMaxProfileId,
		"cursor_profile_id":       0,
		"total_count":             snapshot.TotalCount,
		"queued_count":            0,
		"retry_count":             0,
		"status":                  fullPushBatchPending,
		"active_key":              activeKey,
		"error_message":           "",
		"created_at":              now,
		"updated_at":              now,
	}).InsertAndGetId()
	if err != nil {
		if isDuplicateKeyError(err) {
			existing, scanErr := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).
				Where("active_key", activeKey).
				One()
			if scanErr == nil && !existing.IsEmpty() {
				var active fullPushBatchRecord
				if structErr := existing.Struct(&active); structErr == nil && active.Id > 0 {
					return &active, nil
				}
			}
		}
		return nil, gerror.Wrap(err, "创建全量推送批次失败")
	}
	return &fullPushBatchRecord{
		Id:                   id,
		BatchNo:              batchNo,
		TenantId:             tenantId,
		ChannelId:            channelId,
		RequestedBy:          requestedBy,
		SnapshotMaxProfileId: snapshot.SnapshotMaxProfileId,
		TotalCount:           snapshot.TotalCount,
		Status:               fullPushBatchPending,
		ActiveKey:            activeKey,
	}, nil
}

func (s *sSysPublish) fullPushSnapshot(ctx context.Context, tenantId int64) (*fullPushSnapshot, error) {
	var snapshot fullPushSnapshot
	err := fullPushOnlineProfileBaseModel(ctx, tenantId).
		Fields("COALESCE(MAX(p.id), 0) AS snapshot_max_profile_id, COUNT(*) AS total_count").
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
		WhereIn("status", []string{fullPushBatchPending, fullPushBatchRunning, fullPushBatchDispatching}).
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
	if batch.Status == fullPushBatchDispatching {
		return s.finalizeFullPushBatch(ctx, batch)
	}
	_, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).
		Where("id", batch.Id).
		WhereIn("status", []string{fullPushBatchPending, fullPushBatchRunning}).
		Data(g.Map{"status": fullPushBatchRunning, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新全量推送批次状态失败")
	}
	profiles, err := s.fullPushProfiles(ctx, batch.TenantId, batch.CursorProfileId, batch.SnapshotMaxProfileId, limit)
	if err != nil {
		return s.retryFullPushBatch(ctx, batch, err)
	}
	if len(profiles) == 0 {
		return s.beginFullPushDispatch(ctx, batch)
	}
	lastProfileId := batch.CursorProfileId
	queued := 0
	for _, profile := range profiles {
		if err = s.enqueueFullPushProfile(ctx, batch, profile); err != nil {
			if updateErr := s.checkpointFullPushBatch(ctx, batch.Id, lastProfileId, queued); updateErr != nil {
				return updateErr
			}
			if queued > 0 {
				batch.RetryCount = 0
			}
			return s.retryFullPushBatch(ctx, batch, err)
		}
		lastProfileId = profile.ProfileId
		queued++
	}
	if err = s.checkpointFullPushBatch(ctx, batch.Id, lastProfileId, queued); err != nil {
		return err
	}
	batch.CursorProfileId = lastProfileId
	batch.QueuedCount += queued
	if len(profiles) < limit || lastProfileId >= batch.SnapshotMaxProfileId {
		return s.beginFullPushDispatch(ctx, batch)
	}
	return nil
}

func newFullPushBatchNo(channelId, timestampNano int64) string {
	return fmt.Sprintf("full_push:%d:%d", channelId, timestampNano)
}

func fullPushProfileOperationNo(batchNo string, profileId int64) string {
	return fmt.Sprintf("%s:%d", batchNo, profileId)
}

func (s *sSysPublish) checkpointFullPushBatch(ctx context.Context, batchId, cursorTaskId int64, queued int) error {
	data := g.Map{"cursor_profile_id": cursorTaskId, "retry_count": 0, "error_message": "", "updated_at": gtime.Now()}
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

func (s *sSysPublish) beginFullPushDispatch(ctx context.Context, batch fullPushBatchRecord) error {
	_, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Where("id", batch.Id).Data(g.Map{
		"status": fullPushBatchDispatching, "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新全量推送等待发送状态失败")
	}
	batch.Status = fullPushBatchDispatching
	return s.finalizeFullPushBatch(ctx, batch)
}

func (s *sSysPublish) finalizeFullPushBatch(ctx context.Context, batch fullPushBatchRecord) error {
	done, status, message, err := publishBatchTerminalState(ctx, batch.BatchNo+":")
	if err != nil || !done {
		return err
	}
	_, err = g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Where("id", batch.Id).Data(g.Map{
		"status": status, "active_key": nil, "error_message": message,
		"finished_at": gtime.Now(), "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "完成全量推送批次失败")
	}
	return nil
}

func (s *sSysPublish) fullPushProfiles(ctx context.Context, tenantId, lastId, maxProfileId int64, limit int) ([]fullPushProfile, error) {
	if limit <= 0 {
		limit = 500
	}
	var profiles []fullPushProfile
	err := fullPushOnlineProfileBaseModel(ctx, tenantId).
		Fields("ps.tenant_id,ps.account_id,p.id AS profile_id").
		WhereGT("p.id", lastId).
		WhereLTE("p.id", maxProfileId).
		OrderAsc("p.id").
		Limit(limit).
		Scan(&profiles)
	if err != nil {
		return nil, gerror.Wrap(err, "读取全量推送当前资料失败")
	}
	return profiles, nil
}

func fullPushOnlineProfileBaseModel(ctx context.Context, tenantId int64) *gdb.Model {
	return g.DB().Model(publishProfileStateTable+" ps").Safe().Ctx(ctx).
		InnerJoin(dao.ContentProfile.Table()+" p", "p.id=ps.profile_id AND p.deleted_at IS NULL").
		InnerJoin(publishAccountTable+" a", "a.id=ps.account_id AND a.deleted_at IS NULL").
		Where("ps.tenant_id", tenantId).
		WhereNull("ps.deleted_at").
		Where("a.tenant_id", tenantId).
		Where("a.status", 1).
		Where("p.status", 1)
}

func (s *sSysPublish) enqueueFullPushProfile(ctx context.Context, batch fullPushBatchRecord, profile fullPushProfile) error {
	err := s.submitProfilePublish(ctx, profile.ProfileId, profile.TenantId, profile.AccountId, batch.RequestedBy,
		fullPushProfileOperationNo(batch.BatchNo, profile.ProfileId), []int64{batch.ChannelId}, true)
	if errors.Is(err, errPublishProfileUnavailable) {
		return nil
	}
	return err
}
