package sys

import (
	"context"

	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/token"
	"hotgo/internal/model"
)

func (s *sSysPublish) AccountLogin(ctx context.Context, in *sysin.AccountLoginInp) (res *sysin.AccountLoginModel, err error) {
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	account, err := s.verifyPublishAccountPassword(ctx, in.Username, in.Password)
	if err != nil {
		return nil, err
	}
	return s.loginPublishAccount(ctx, account)
}

func (s *sSysPublish) AccountRegister(ctx context.Context, in *sysin.AccountRegisterInp) (res *sysin.AccountRegisterModel, err error) {
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	tenant, err := s.AdminTenantSave(ctx, &sysin.TenantSaveInp{
		Name:     in.Name,
		Username: in.Username,
		Password: in.Password,
		Remark:   "邀请码：" + in.InviteCode,
		Status:   consts.StatusEnabled,
	})
	if err != nil {
		return nil, err
	}
	account, err := s.verifyPublishAccountPassword(ctx, in.Username, in.Password)
	if err != nil {
		return nil, err
	}
	loginRes, err := s.loginPublishAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	loginRes.TenantId = tenant.Id
	return &sysin.AccountRegisterModel{AccountLoginModel: loginRes}, nil
}

func (s *sSysPublish) loginPublishAccount(ctx context.Context, account *sysin.AccountModel) (res *sysin.AccountLoginModel, err error) {
	user := &model.Identity{
		Id:       account.Id,
		Pid:      account.ParentId,
		DeptType: account.AccountType,
		Username: account.Username,
		RealName: account.Nickname,
		App:      consts.AppApi,
		LoginAt:  gtime.Now(),
	}
	loginToken, expires, err := token.Login(ctx, user)
	if err != nil {
		return nil, err
	}
	return &sysin.AccountLoginModel{
		Id:          account.Id,
		TenantId:    account.TenantId,
		AccountType: account.AccountType,
		Username:    account.Username,
		Nickname:    account.Nickname,
		Token:       loginToken,
		Expires:     expires,
	}, nil
}
