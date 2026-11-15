package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const profileCycleCancelMessage = "资料循环上架已取消"

// cancelProfileCycle cancels only the current profile-level run. Channel cycle
// configuration and runs belonging to other profiles are deliberately outside
// this operation.
func (s *sSysPublish) cancelProfileCycle(ctx context.Context, ids []int64, tenantId int64, accountId int64) (*sysin.ProfileStatusModel, error) {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return nil, gerror.New("资料不存在或无权操作")
	}
	var cleanupJobIDs []int64
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, profileID := range ids {
			if err := s.withProfileLifecycleLock(ctx, tenantId, profileID, func() error {
				var run struct {
					ID int64 `orm:"id"`
				}
				runQuery := tx.Model(publishProfileCycleRunTable).Safe().Ctx(ctx).
					Where("tenant_id", tenantId).Where("profile_id", profileID).
					WhereIn("status", []string{"pending", "running", "dispatching", "dispatched"}).
					OrderDesc("id").Limit(1)
				if accountId > 0 {
					runQuery = runQuery.Where("account_id", accountId)
				}
				if err := runQuery.LockUpdate().Scan(&run); err != nil {
					return gerror.Wrap(err, "读取资料循环任务失败")
				}
				if run.ID <= 0 {
					return gerror.New("该资料当前没有可取消的循环上架任务")
				}

				var jobs []struct {
					ID     int64  `orm:"id"`
					Status string `orm:"status"`
				}
				if err := tx.Model(publishTgJobTable+" j").Safe().Ctx(ctx).
					InnerJoin(publishProfileCycleRunChannelTable+" rc", "rc.job_id=j.id").
					Where("rc.run_id", run.ID).
					Fields("j.id,j.status").Scan(&jobs); err != nil {
					return gerror.Wrap(err, "读取资料循环TG任务失败")
				}
				for _, job := range jobs {
					if job.ID <= 0 {
						continue
					}
					switch job.Status {
					case telegramJobStatusPending, telegramJobStatusRetryWait, telegramJobStatusConfirming, telegramJobStatusSending, telegramJobStatusSent:
						cleanupJobIDs = append(cleanupJobIDs, job.ID)
					}
				}
				now := gtime.Now()
				if _, err := tx.Model(publishProfileCycleRunChannelTable).Safe().Ctx(ctx).
					Where("run_id", run.ID).
					Data(g.Map{"status": "superseded", "error_message": profileCycleCancelMessage, "updated_at": now}).Update(); err != nil {
					return gerror.Wrap(err, "取消资料循环频道任务失败")
				}
				if _, err := tx.Model(publishProfileCycleRunTable).Safe().Ctx(ctx).
					Where("id", run.ID).
					Data(g.Map{"status": "canceled", "error_message": profileCycleCancelMessage, "completed_at": now, "updated_at": now}).Update(); err != nil {
					return gerror.Wrap(err, "取消资料循环任务失败")
				}
				if _, err := tx.Model(publishTgJobTable+" j").Safe().Ctx(ctx).
					Where("j.profile_id", profileID).
					WhereIn("status", []string{telegramJobStatusPending, telegramJobStatusRetryWait, telegramJobStatusConfirming, telegramJobStatusSending}).
					Where("EXISTS (SELECT 1 FROM "+publishProfileCycleRunChannelTable+" rc WHERE rc.run_id=? AND rc.job_id=j.id)", run.ID).
					Data(g.Map{"status": telegramJobStatusSuperseded, "dispatch_status": telegramDispatchDone, "next_retry_at": nil, "last_dispatch_error": profileCycleCancelMessage, "updated_at": now}).Update(); err != nil {
					return gerror.Wrap(err, "废弃资料循环TG任务失败")
				}
				stateQuery := tx.Model(publishProfileStateTable).Safe().Ctx(ctx).
					Where("tenant_id", tenantId).Where("profile_id", profileID).
					Where("(publish_operation_no LIKE ? OR publish_operation_no LIKE ?)", "profile_cycle:%", "cycle_batch:%").
					WhereIn("publish_task_status", []string{sysin.PublishTaskStatusPending.String(), sysin.PublishTaskStatusPublishing.String(), sysin.PublishTaskStatusCycling.String()}).
					Data(g.Map{"publish_operation_no": "", "publish_task_status": "", "publish_task_updated_at": now, "updated_at": now})
				if accountId > 0 {
					stateQuery = stateQuery.Where("account_id", accountId)
				}
				if _, err := stateQuery.Update(); err != nil {
					return gerror.Wrap(err, "恢复资料循环发布状态失败")
				}
				indexQuery := tx.Model(publishNoteIndexTable).Safe().Ctx(ctx).
					Where("tenant_id", tenantId).Where("profile_id", profileID).
					WhereIn("task_status", []string{sysin.PublishTaskStatusPending.String(), sysin.PublishTaskStatusPublishing.String(), sysin.PublishTaskStatusCycling.String()}).
					Data(g.Map{"task_status": ""})
				if accountId > 0 {
					indexQuery = indexQuery.Where("account_id", accountId)
				}
				if _, err := indexQuery.Update(); err != nil {
					return gerror.Wrap(err, "恢复资料列表循环状态失败")
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, jobID := range uniqueIds(cleanupJobIDs) {
		if err := s.enqueueTelegramCleanupJob(ctx, jobID, 0); err != nil {
			g.Log().Warningf(ctx, "资料循环取消后加入TG清理队列失败 jobId:%d err:%+v", jobID, err)
		}
	}
	return &sysin.ProfileStatusModel{Message: "资料循环上架已取消，资料状态已恢复"}, nil
}

func (s *sSysPublish) markProfilesUnpublishing(ctx context.Context, ids []int64, tenantId int64, operationNo string) error {
	now := gtime.Now()
	for _, profileID := range uniqueIds(ids) {
		state, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
			Fields("account_id").Where("tenant_id", tenantId).Where("profile_id", profileID).WhereNull("deleted_at").One()
		if err != nil {
			return gerror.Wrap(err, "读取下架资料归属失败")
		}
		if state.IsEmpty() {
			continue
		}
		if _, err = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).Where("profile_id", profileID).WhereNull("deleted_at").
			Data(g.Map{"publish_operation_no": strings.TrimSpace(operationNo), "publish_task_status": sysin.PublishTaskStatusUnpublishing.String(), "publish_task_updated_at": now, "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "记录资料下架进行中状态失败")
		}
		if _, err = g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).Where("profile_id", profileID).WhereNull("deleted_at").
			Data(g.Map{"task_status": sysin.PublishTaskStatusUnpublishing.String(), "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "同步资料列表下架进行中状态失败")
		}
	}
	return nil
}

func (s *sSysPublish) clearProfilesUnpublishing(ctx context.Context, ids []int64, tenantId int64, operationNo string) error {
	now := gtime.Now()
	_, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).WhereIn("profile_id", uniqueIds(ids)).
		Where("publish_operation_no", strings.TrimSpace(operationNo)).
		Data(g.Map{"publish_operation_no": "", "publish_task_status": "", "publish_task_updated_at": now, "updated_at": now}).Update()
	if err != nil {
		return gerror.Wrap(err, "清理资料下架进行中状态失败")
	}
	_, err = g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).WhereIn("profile_id", uniqueIds(ids)).
		Where("task_status", sysin.PublishTaskStatusUnpublishing.String()).
		Data(g.Map{"task_status": ""}).Update()
	return err
}
