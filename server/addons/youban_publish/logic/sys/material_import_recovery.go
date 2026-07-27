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

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	materialImportRecoverAfter      = 3 * time.Minute
	materialImportRecoveryInterval  = time.Minute
	materialImportHeartbeatInterval = 30 * time.Second
)

func (s *sSysPublish) runMaterialImportRecovery(ctx context.Context) {
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	s.recoverMaterialImportOnce(ctx)
	ticker := time.NewTicker(materialImportRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverMaterialImportOnce(ctx)
		}
	}
}

func (s *sSysPublish) recoverMaterialImportOnce(ctx context.Context) {
	if err := s.recoverMaterialImportTasks(ctx, 20); err != nil {
		g.Log().Warningf(ctx, "恢复滞留资料导入任务失败：%+v", err)
	}
}

func (s *sSysPublish) recoverMaterialImportTasks(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	cols := pdao.YoubanPublishMaterialImportTask.Columns()
	now := gtime.Now()
	staleBefore := now.Add(-materialImportRecoverAfter)
	condition := fmt.Sprintf(
		"(%s=? AND %s IN(?,?) AND %s<=?) OR (%s=? AND %s IS NOT NULL AND %s<=?)",
		cols.Status,
		cols.Stage,
		cols.UpdatedAt,
		cols.Status,
		cols.NextRunAt,
		cols.NextRunAt,
	)
	rows, err := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).
		Where(
			condition,
			sysin.MaterialImportStatusRunning,
			sysin.MaterialImportStagePulling,
			sysin.MaterialImportStageMedia,
			staleBefore,
			sysin.MaterialImportStatusWaiting,
			now,
		).
		OrderAsc(cols.UpdatedAt).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取滞留资料导入任务失败")
	}
	for _, row := range rows {
		if row.IsEmpty() || row[cols.Id].Int64() <= 0 {
			continue
		}
		if err = s.recoverMaterialImportTask(ctx, row, staleBefore, now); err != nil {
			g.Log().Warningf(ctx, "恢复资料导入任务失败 task:%d err:%+v", row[cols.Id].Int64(), err)
		}
	}
	return nil
}

func (s *sSysPublish) recoverMaterialImportTask(ctx context.Context, row gdb.Record, staleBefore *gtime.Time, now *gtime.Time) error {
	cols := pdao.YoubanPublishMaterialImportTask.Columns()
	taskId := row[cols.Id].Int64()
	status := strings.TrimSpace(row[cols.Status].String())
	stage := strings.TrimSpace(row[cols.Stage].String())
	if taskId <= 0 || !materialImportRecoveryCandidate(status, stage) {
		return nil
	}
	mod := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).
		Where(cols.Id, taskId).
		Where(cols.Status, status)
	if status == sysin.MaterialImportStatusRunning {
		mod = mod.WhereLTE(cols.UpdatedAt, staleBefore)
	} else {
		mod = mod.WhereNotNull(cols.NextRunAt).WhereLTE(cols.NextRunAt, now)
	}
	result, err := mod.Data(g.Map{
		cols.Status:       sysin.MaterialImportStatusRunning,
		cols.ErrorMessage: "",
		cols.NextRunAt:    nil,
		cols.UpdatedAt:    now,
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "认领滞留资料导入任务失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认滞留资料导入任务失败")
	}
	if affected == 0 {
		return nil
	}
	if err = s.enqueueMaterialImportTask(ctx, taskId, 0); err != nil {
		return gerror.Wrap(err, "重新投递资料导入任务失败")
	}
	g.Log().Infof(ctx, "已恢复滞留资料导入任务 task:%d stage:%s", taskId, row[cols.Stage].String())
	return nil
}

func materialImportRecoveryCandidate(status string, stage string) bool {
	if stage != sysin.MaterialImportStagePulling && stage != sysin.MaterialImportStageMedia {
		return false
	}
	return status == sysin.MaterialImportStatusRunning || status == sysin.MaterialImportStatusWaiting
}

func (s *sSysPublish) runMaterialImportTaskHeartbeat(ctx context.Context, taskId int64) {
	if taskId <= 0 {
		return
	}
	ticker := time.NewTicker(materialImportHeartbeatInterval)
	defer ticker.Stop()
	cols := pdao.YoubanPublishMaterialImportTask.Columns()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).
				Where(cols.Id, taskId).
				Where(cols.Status, sysin.MaterialImportStatusRunning).
				Data(g.Map{cols.UpdatedAt: gtime.Now()}).
				Update(); err != nil && ctx.Err() == nil {
				g.Log().Warningf(ctx, "更新资料导入任务心跳失败 task:%d err:%+v", taskId, err)
			}
		}
	}
}
