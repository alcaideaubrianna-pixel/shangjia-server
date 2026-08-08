package sys

import (
	"context"
	"errors"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/contexts"
)

var errPublishProfileUnavailable = errors.New("资料已不满足循环上架条件")

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
	return s.submitProfilePublish(ctx, profileId, account.TenantId, account.Id, contexts.GetUserId(ctx), "", nil, false)
}

func (s *sSysPublish) AdminProfilePublish(ctx context.Context, in *sysin.AdminProfilePublishInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return gerror.New("资料UUID不能为空")
	}
	view, err := s.AdminProfileView(ctx, &in.ProfileViewInp)
	if err != nil {
		return err
	}
	if view == nil || view.Profile == nil {
		return gerror.New("资料不存在或无权操作")
	}
	operationNo, err := adminBatchTextOperationNo(account.Id, in.BatchId, view.Profile.Id)
	if err != nil {
		return err
	}
	return s.submitProfilePublish(ctx, view.Profile.Id, view.Profile.TenantId, view.Profile.AccountId, account.Id, operationNo, nil, false)
}
