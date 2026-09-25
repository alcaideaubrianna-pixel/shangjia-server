package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) CurrentAccount(ctx context.Context) (*sysin.CurrentAccountModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	vip, err := s.tenantVipStatusForAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	capability, err := s.AccountCapability(ctx, "api", account.Id)
	if err != nil {
		return nil, err
	}
	if account.AccountType == sysin.PublishAccountTypeAdmin {
		capability.GroupPushEnabled = 1
	}
	return &sysin.CurrentAccountModel{
		Id:                    account.Id,
		TenantId:              account.TenantId,
		ParentId:              account.ParentId,
		AccountType:           account.AccountType,
		Nickname:              account.Nickname,
		Username:              account.Username,
		Remark:                account.Remark,
		Status:                account.Status,
		CreatedAt:             account.CreatedAt,
		UpdatedAt:             account.UpdatedAt,
		Vip:                   vip,
		GroupPushEnabled:      capability.GroupPushEnabled,
		CycleFreeIntervalDays: maxConfigInt(ctx, "youbanPublish.cycle.freeIntervalDays", 15),
	}, nil
}

func maxConfigInt(ctx context.Context, key string, fallback int) int {
	value := g.Cfg().MustGet(ctx, key, fallback).Int()
	if value <= 0 {
		return fallback
	}
	return value
}
