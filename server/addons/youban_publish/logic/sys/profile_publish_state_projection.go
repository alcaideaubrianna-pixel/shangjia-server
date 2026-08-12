package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/service"
)

type profilePublishOperationState struct {
	TenantId           int64  `orm:"tenant_id"`
	AccountId          int64  `orm:"account_id"`
	ProfileId          int64  `orm:"profile_id"`
	PublishOperationNo string `orm:"publish_operation_no"`
	PublishTaskStatus  string `orm:"publish_task_status"`
}

func (s *sSysPublish) beginProfilePublishOperation(ctx context.Context, tenantId, accountId, profileId int64, operationNo string) error {
	operationNo = strings.TrimSpace(operationNo)
	if tenantId <= 0 || accountId <= 0 || profileId <= 0 || operationNo == "" {
		return gerror.New("上架操作状态参数不完整")
	}
	return s.writeProfilePublishOperationState(ctx, tenantId, accountId, profileId, operationNo, sysin.PublishTaskStatusPending, false)
}

func (s *sSysPublish) updateProfilePublishOperationState(ctx context.Context, job telegramJobRecord, status string) error {
	status = strings.TrimSpace(status)
	if job.TenantId <= 0 || job.AccountId <= 0 || job.ProfileId <= 0 || strings.TrimSpace(job.OperationNo) == "" || status == "" {
		return nil
	}
	return s.writeProfilePublishOperationState(ctx, job.TenantId, job.AccountId, job.ProfileId, job.OperationNo, status, true)
}

func (s *sSysPublish) clearProfilePublishOperationState(ctx context.Context, job telegramJobRecord) error {
	if job.TenantId <= 0 || job.AccountId <= 0 || job.ProfileId <= 0 || strings.TrimSpace(job.OperationNo) == "" {
		return nil
	}
	return s.writeProfilePublishOperationState(ctx, job.TenantId, job.AccountId, job.ProfileId, job.OperationNo, "", true)
}

func (s *sSysPublish) cancelProfilePublishOperation(ctx context.Context, profileId int64) error {
	if profileId <= 0 {
		return nil
	}
	now := gtime.Now()
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model(publishProfileStateTable).Safe().Ctx(ctx).
			Where("profile_id", profileId).
			WhereNull("deleted_at").
			Where("publish_task_status <> ''").
			Data(g.Map{"publish_task_status": "", "publish_task_updated_at": now, "updated_at": now}).
			Update(); err != nil {
			return gerror.Wrap(err, "取消资料上架操作状态失败")
		}
		if _, err := tx.Model(publishNoteIndexTable).Unscoped().Safe().Ctx(ctx).
			Where("profile_id", profileId).
			WhereNull("deleted_at").
			Where("task_status <> ''").
			Data(g.Map{"task_status": ""}).
			Update(); err != nil {
			return gerror.Wrap(err, "清理资料列表上架状态失败")
		}
		return nil
	})
}

func (s *sSysPublish) profilePublishOperationIsCurrent(ctx context.Context, job telegramJobRecord) (bool, error) {
	if job.TenantId <= 0 || job.AccountId <= 0 || job.ProfileId <= 0 || strings.TrimSpace(job.OperationNo) == "" {
		return false, nil
	}
	count, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).
		Where("publish_operation_no", strings.TrimSpace(job.OperationNo)).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查当前资料上架操作失败")
	}
	return count > 0, nil
}

func (s *sSysPublish) writeProfilePublishOperationState(ctx context.Context, tenantId, accountId, profileId int64, operationNo, status string, requireCurrentOperation bool) error {
	now := gtime.Now()
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		stateMod := tx.Model(publishProfileStateTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("account_id", accountId).
			Where("profile_id", profileId).
			WhereNull("deleted_at")
		if requireCurrentOperation {
			stateMod = stateMod.Where("publish_operation_no", strings.TrimSpace(operationNo))
		}
		if requireCurrentOperation && (status == sysin.PublishTaskStatusPending || status == sysin.PublishTaskStatusPublishing) {
			stateMod = stateMod.Where("publish_task_status <> ?", sysin.PublishTaskStatusFailed)
		}
		data := g.Map{
			"publish_task_status":     status,
			"publish_task_updated_at": now,
			"updated_at":              now,
		}
		if !requireCurrentOperation {
			data["publish_operation_no"] = strings.TrimSpace(operationNo)
		}
		result, err := stateMod.Where("publish_task_status <> ? OR publish_operation_no <> ?", status, strings.TrimSpace(operationNo)).Data(data).Update()
		if err != nil {
			return gerror.Wrap(err, "更新资料上架操作状态失败")
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return nil
		}
		indexMod := tx.Model(publishNoteIndexTable).Unscoped().Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("account_id", accountId).
			Where("profile_id", profileId).
			WhereNull("deleted_at").
			Where("task_status <> ?", status)
		if _, err = indexMod.Data(g.Map{"task_status": status}).Update(); err != nil {
			return gerror.Wrap(err, "更新资料列表上架状态失败")
		}
		return nil
	})
}

func (s *sSysPublish) recoverProfilePublishOperationStates(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var states []profilePublishOperationState
	if err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("tenant_id,account_id,profile_id,publish_operation_no,publish_task_status").
		WhereNull("deleted_at").
		Where("publish_task_status <> ''").
		OrderAsc("publish_task_updated_at").
		OrderAsc("profile_id").
		Limit(limit).
		Scan(&states); err != nil {
		return gerror.Wrap(err, "读取待恢复资料上架状态失败")
	}
	for _, state := range states {
		if err := s.recoverProfilePublishOperationState(ctx, state); err != nil {
			g.Log().Warningf(ctx, "恢复资料上架状态失败 profileId:%d operationNo:%s err:%+v", state.ProfileId, state.PublishOperationNo, err)
		}
	}
	return nil
}

func (s *sSysPublish) recoverMissingProfilePublishOperationStates(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var jobs []telegramJobRecord
	if err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=j.profile_id AND ps.tenant_id=j.tenant_id AND ps.account_id=j.account_id AND ps.deleted_at IS NULL").
		Fields("j.tenant_id,j.account_id,j.profile_id,j.operation_no,j.status").
		WhereNull("j.task_id").
		WhereIn("j.status", []string{"pending", "sending", "failed_retry", "unknown"}).
		Where("j.operation_no <> ''").
		Where("ps.publish_task_status = ''").
		OrderDesc("j.id").
		Limit(limit * 3).
		Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取缺失投影的资料上架任务失败")
	}
	seen := make(map[int64]struct{}, limit)
	recovered := 0
	for _, job := range jobs {
		if job.ProfileId <= 0 || recovered >= limit {
			continue
		}
		if _, ok := seen[job.ProfileId]; ok {
			continue
		}
		seen[job.ProfileId] = struct{}{}
		if err := s.beginProfilePublishOperation(ctx, job.TenantId, job.AccountId, job.ProfileId, job.OperationNo); err != nil {
			g.Log().Warningf(ctx, "补建资料上架状态失败 profileId:%d operationNo:%s err:%+v", job.ProfileId, job.OperationNo, err)
			continue
		}
		if job.Status == "sending" {
			if err := s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPublishing); err != nil {
				g.Log().Warningf(ctx, "恢复资料发送中状态失败 profileId:%d operationNo:%s err:%+v", job.ProfileId, job.OperationNo, err)
				continue
			}
		}
		recovered++
	}
	if recovered > 0 {
		g.Log().Infof(ctx, "已补建资料上架状态：%d条", recovered)
	}
	return nil
}

func (s *sSysPublish) recoverProfilePublishOperationState(ctx context.Context, state profilePublishOperationState) error {
	job := telegramJobRecord{
		TenantId: state.TenantId, AccountId: state.AccountId, ProfileId: state.ProfileId,
		OperationNo: strings.TrimSpace(state.PublishOperationNo),
	}
	if job.OperationNo == "" {
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	}
	row, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("profile_id", job.ProfileId).
		Where("operation_no", job.OperationNo).
		Fields("COUNT(1) AS total," +
			"SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END) AS failed," +
			"SUM(CASE WHEN status='sending' THEN 1 ELSE 0 END) AS sending," +
			"SUM(CASE WHEN status IN ('pending','failed_retry','unknown') THEN 1 ELSE 0 END) AS pending," +
			"SUM(CASE WHEN status IN ('sent','superseded') THEN 1 ELSE 0 END) AS completed").
		One()
	if err != nil {
		return gerror.Wrap(err, "统计资料上架任务状态失败")
	}
	total := row["total"].Int()
	switch {
	case total == 0 || row["failed"].Int() > 0:
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	case row["sending"].Int() > 0:
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPublishing)
	case row["pending"].Int() > 0:
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPending)
	case row["completed"].Int() == total:
		if !isCycleBatchOperation(job.OperationNo) {
			if _, err = s.syncProfilePublishState(ctx, job.ProfileId, 1, consts.ContentVisibilityPublic, gtime.Now()); err != nil {
				return err
			}
		}
		if err = s.clearProfilePublishOperationState(ctx, job); err != nil {
			return err
		}
		if err = s.syncProfileNoteIndex(ctx, job.ProfileId); err != nil {
			return err
		}
		service.SysContent().ClearHomeProfileCardsCache(ctx)
	}
	return nil
}
