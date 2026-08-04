package sys

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/token"
	"hotgo/internal/model"
)

type accountRegisterTxResult struct {
	Invite    *registerInviteCodeRow
	Tenant    *sysin.TenantSaveModel
	AccountId int64
	Binding   *botService.AccountBoundEvent
}

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
	registration, err := s.registerAccountWithInvite(ctx, in)
	if err != nil {
		return nil, err
	}
	account, err := s.verifyPublishAccountPassword(ctx, in.Username, in.Password)
	if err != nil {
		return nil, err
	}
	if notifyErr := botService.SysBot().NotifySuperAdmins(ctx, 0, "register", buildRegisterInviteNotifyText(in.Name, account.Username, account.Nickname, registration.Invite.Code)); notifyErr != nil {
		g.Log().Warningf(ctx, "推送邀请码注册通知失败 tenantId:%d accountId:%d err:%+v", registration.Tenant.Id, account.Id, notifyErr)
	}
	if registration.Binding != nil {
		for _, hookErr := range botService.TriggerAccountBoundHooks(ctx, registration.Binding) {
			g.Log().Warningf(ctx, "立即注册Telegram绑定后置处理失败 accountId:%d err:%+v", registration.AccountId, hookErr)
		}
	}
	loginRes, err := s.loginPublishAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	loginRes.TenantId = registration.Tenant.Id
	return &sysin.AccountRegisterModel{AccountLoginModel: loginRes}, nil
}

func (s *sSysPublish) registerAccountWithInvite(ctx context.Context, in *sysin.AccountRegisterInp) (*accountRegisterTxResult, error) {
	tenantIn := &sysin.TenantSaveInp{
		Name:     in.Name,
		Username: in.Username,
		Password: in.Password,
		Remark:   "",
		Status:   consts.StatusEnabled,
	}
	if err := tenantIn.Filter(ctx); err != nil {
		return nil, err
	}
	result := &accountRegisterTxResult{}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		invite, err := s.validateRegisterInviteCodeTx(ctx, tx, in.InviteCode, true)
		if err != nil {
			return err
		}
		tenant, accountId, err := s.adminTenantSaveTx(ctx, tx, tenantIn)
		if err != nil {
			return err
		}
		if err = s.markRegisterInviteUsedTx(ctx, tx, invite.Id, tenant.Id, accountId, in.Username); err != nil {
			return err
		}
		binding, err := s.bindRegisterTelegramTx(ctx, tx, invite, accountId)
		if err != nil {
			return err
		}
		result.Invite = invite
		result.Tenant = tenant
		result.AccountId = accountId
		result.Binding = binding
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
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
