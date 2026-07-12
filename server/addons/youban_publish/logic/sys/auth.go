package sys

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	botService "hotgo/addons/youban_bot/service"
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
	invite, err := s.validateRegisterInviteCode(ctx, in.InviteCode)
	if err != nil {
		return nil, err
	}
	tenant, err := s.AdminTenantSave(ctx, &sysin.TenantSaveInp{
		Name:     in.Name,
		Username: in.Username,
		Password: in.Password,
		Remark:   "",
		Status:   consts.StatusEnabled,
	})
	if err != nil {
		return nil, err
	}
	account, err := s.verifyPublishAccountPassword(ctx, in.Username, in.Password)
	if err != nil {
		return nil, err
	}
	if markErr := s.markRegisterInviteUsed(ctx, invite.Code, tenant.Id, account.Id, account.Username); markErr != nil {
		g.Log().Warningf(ctx, "标记邀请码已使用失败 code:%s tenantId:%d accountId:%d err:%+v", invite.Code, tenant.Id, account.Id, markErr)
	}
	if notifyErr := botService.SysBot().NotifySuperAdmins(ctx, 0, "register", buildRegisterInviteNotifyText(in.Name, account.Username, account.Nickname, invite.Code)); notifyErr != nil {
		g.Log().Warningf(ctx, "推送邀请码注册通知失败 tenantId:%d accountId:%d err:%+v", tenant.Id, account.Id, notifyErr)
	}
	loginRes, err := s.loginPublishAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	loginRes.TenantId = tenant.Id
	return &sysin.AccountRegisterModel{AccountLoginModel: loginRes}, nil
}

func buildRegisterInviteNotifyText(tenantName, username, nickname, inviteCode string) string {
	return fmt.Sprintf(
		"用户注册通知\n"+
			"租户：%s\n"+
			"账号：%s\n"+
			"昵称：%s\n"+
			"邀请码：%s",
		firstNonEmpty(tenantName, "-"),
		firstNonEmpty(username, "-"),
		firstNonEmpty(nickname, "-"),
		firstNonEmpty(inviteCode, "-"),
	)
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
