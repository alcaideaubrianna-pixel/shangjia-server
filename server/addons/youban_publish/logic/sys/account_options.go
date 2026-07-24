package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

type accountOptionRow struct {
	Id       int64
	Nickname string
}

func (s *sSysPublish) AdminAccountOptions(ctx context.Context, in *sysin.AccountOptionsInp) (list []*sysin.AccountOptionModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		in = &sysin.AccountOptionsInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}

	cacheKey := publishAccountOptionsCacheKey(ctx, account.TenantId, account.Id, in.Scope)
	if cacheVar, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !cacheVar.IsNil() {
		if scanErr := cacheVar.Scan(&list); scanErr == nil && list != nil {
			return list, nil
		}
	}
	defer func() {
		if err == nil && len(list) > 0 {
			_ = cache.Instance().Set(ctx, cacheKey, list, 5*time.Minute)
		}
	}()

	if in.Scope == sysin.AccountOptionsScopeFollowing {
		return s.followAccountOptions(ctx, account)
	}

	myOptions, err := s.allAccountOptions(ctx, account)
	if err != nil {
		return nil, err
	}
	followOptions, err := s.followAccountOptions(ctx, account)
	if err != nil {
		return nil, err
	}

	seen := make(map[int64]struct{}, len(myOptions)+len(followOptions))
	list = make([]*sysin.AccountOptionModel, 0, len(myOptions)+len(followOptions))
	for _, item := range append(myOptions, followOptions...) {
		if item == nil || item.Value <= 0 {
			continue
		}
		if _, ok := seen[item.Value]; ok {
			continue
		}
		seen[item.Value] = struct{}{}
		list = append(list, item)
	}
	return list, nil
}

func (s *sSysPublish) allAccountOptions(ctx context.Context, account *sysin.AccountModel) ([]*sysin.AccountOptionModel, error) {
	accountColumns := pdao.YoubanPublishAccount.Columns()
	rows := make([]accountOptionRow, 0, 32)
	if err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.TenantId, account.TenantId).
		WhereNull(accountColumns.DeletedAt).
		WhereIn(accountColumns.AccountType, []string{
			sysin.PublishAccountTypeAdmin,
			sysin.PublishAccountTypeUploader,
		}).
		Fields(accountColumns.Id, accountColumns.Nickname).
		OrderAsc(accountColumns.AccountType).
		OrderAsc(accountColumns.Id).
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "获取账号筛选选项失败")
	}
	return buildAccountOptions(rows, ""), nil
}

func (s *sSysPublish) followAccountOptions(ctx context.Context, account *sysin.AccountModel) ([]*sysin.AccountOptionModel, error) {
	rows := make([]accountOptionRow, 0, 32)
	ids, err := s.followNoteDirectAccountIds(ctx, account, nil)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*sysin.AccountOptionModel{}, nil
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	if err = pdao.YoubanPublishAccount.Ctx(ctx).
		WhereIn(accountColumns.Id, ids).
		Where(accountColumns.Status, 1).
		WhereNull(accountColumns.DeletedAt).
		Fields(accountColumns.Id, accountColumns.Nickname).
		OrderAsc(accountColumns.Id).
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "获取关注账号筛选选项失败")
	}
	return buildAccountOptions(rows, "关注："), nil
}

func buildAccountOptions(rows []accountOptionRow, prefix string) []*sysin.AccountOptionModel {
	list := make([]*sysin.AccountOptionModel, 0, len(rows))
	for _, row := range rows {
		label := strings.TrimSpace(row.Nickname)
		if label == "" {
			label = fmt.Sprintf("账号 %d", row.Id)
		}
		list = append(list, &sysin.AccountOptionModel{
			Label: prefix + label,
			Value: row.Id,
		})
	}
	return list
}

func publishAccountOptionsCacheKey(ctx context.Context, tenantId int64, accountId int64, scope string) string {
	return fmt.Sprintf("youban_publish:account_options:%d:%d:%s:%s", tenantId, accountId, scope, accountVisibilityVersionValue(ctx, tenantId))
}
