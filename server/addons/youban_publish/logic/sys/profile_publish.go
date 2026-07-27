package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/contexts"
)

func (s *sSysPublish) MyProfilePublish(ctx context.Context, in *sysin.ProfileViewInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return gerror.New("资料UUID不能为空")
	}
	profileId, err := s.resolveProfileId(ctx, in.Id, in.Uuid, account.TenantId, account.Id)
	if err != nil {
		return err
	}
	taskId, err := s.prepareProfilePublishTask(ctx, profileId, account.TenantId, account.Id)
	if err != nil {
		return err
	}
	return s.submitTask(ctx, taskId, account.Id)
}

func (s *sSysPublish) AdminProfilePublish(ctx context.Context, in *sysin.ProfileViewInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return gerror.New("资料UUID不能为空")
	}
	view, err := s.AdminProfileView(ctx, in)
	if err != nil {
		return err
	}
	if view == nil || view.Profile == nil {
		return gerror.New("资料不存在或无权操作")
	}
	taskId, err := s.prepareProfilePublishTask(ctx, view.Profile.Id, view.Profile.TenantId, view.Profile.AccountId)
	if err != nil {
		return err
	}
	return s.submitTaskByTenant(ctx, taskId, account.TenantId, account.Id)
}

func (s *sSysPublish) prepareProfilePublishTask(ctx context.Context, profileId int64, tenantId int64, accountId int64) (taskId int64, err error) {
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		task, taskErr := s.profileTask(ctx, tx, profileId, tenantId, accountId)
		if taskErr != nil {
			return taskErr
		}
		if task["status"].String() != sysin.PublishTaskStatusDraft {
			task, taskErr = s.cloneEditableProfileTask(ctx, tx, task, contexts.GetUserId(ctx))
			if taskErr != nil {
				return taskErr
			}
		}
		taskId = task["id"].Int64()
		if taskId <= 0 {
			return gerror.New("资料发布事件创建失败")
		}
		return nil
	})
	return taskId, err
}
