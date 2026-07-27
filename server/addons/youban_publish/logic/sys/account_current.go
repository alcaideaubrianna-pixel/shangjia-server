package sys

import (
	"context"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) CurrentAccount(ctx context.Context) (*sysin.CurrentAccountModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return &sysin.CurrentAccountModel{
		Id:          account.Id,
		TenantId:    account.TenantId,
		ParentId:    account.ParentId,
		AccountType: account.AccountType,
		Nickname:    account.Nickname,
		Username:    account.Username,
		Remark:      account.Remark,
		Status:      account.Status,
		CreatedAt:   account.CreatedAt,
		UpdatedAt:   account.UpdatedAt,
	}, nil
}
