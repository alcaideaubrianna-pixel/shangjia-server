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

type adminProfileVisibleScope struct {
	AccountIds []int64
	Strict     bool
	TenantId   int64
}

const accountVisibilityCacheTTL = 10 * time.Minute
const accountVisibilityVersionTTL = 24 * time.Hour

func (s *sSysPublish) adminProfileVisibleScope(ctx context.Context, account *sysin.AccountModel, in *sysin.ProfileListInp) (*adminProfileVisibleScope, error) {
	if account == nil || account.Id <= 0 || account.TenantId <= 0 {
		return nil, gerror.New("当前账号无管理权限")
	}
	scope := "mine"
	if in != nil && strings.TrimSpace(in.AccountScope) != "" {
		scope = strings.TrimSpace(in.AccountScope)
	}
	cacheKey := accountVisibilityScopeCacheKey(ctx, account.TenantId, account.Id, scope, accountScopeSelectedID(in))
	if cached, err := cache.Instance().Get(ctx, cacheKey); err == nil && !cached.IsNil() {
		var res *adminProfileVisibleScope
		if scanErr := cached.Scan(&res); scanErr == nil && res != nil {
			if res.TenantId <= 0 {
				res.TenantId = account.TenantId
			}
			return res, nil
		}
	}
	switch scope {
	case "", "mine":
		if in != nil && in.AccountId > 0 {
			if err := s.ensureAdminManageableAccount(ctx, account, in.AccountId); err != nil {
				return nil, err
			}
			ids, err := s.expandFollowNoteAccountIds(ctx, []int64{in.AccountId})
			res := &adminProfileVisibleScope{AccountIds: ids, TenantId: account.TenantId}
			_ = cache.Instance().Set(ctx, cacheKey, res, accountVisibilityCacheTTL)
			return res, err
		}
		ids, err := s.expandFollowNoteAccountIds(ctx, []int64{account.Id})
		res := &adminProfileVisibleScope{AccountIds: ids, TenantId: account.TenantId}
		_ = cache.Instance().Set(ctx, cacheKey, res, accountVisibilityCacheTTL)
		return res, err
	case "following":
		if in != nil && in.AccountId > 0 {
			ids, err := s.adminFollowNoteSelectedAccountIds(ctx, account, in.AccountId)
			res := &adminProfileVisibleScope{AccountIds: ids, Strict: true, TenantId: account.TenantId}
			_ = cache.Instance().Set(ctx, cacheKey, res, accountVisibilityCacheTTL)
			return res, err
		}
		ids, err := s.followNoteDirectAccountIds(ctx, account, nil)
		res := &adminProfileVisibleScope{AccountIds: ids, Strict: true, TenantId: account.TenantId}
		_ = cache.Instance().Set(ctx, cacheKey, res, accountVisibilityCacheTTL)
		return res, err
	case "all":
		if in != nil && in.AccountId > 0 {
			ids, err := s.adminFollowNoteSelectedAccountIds(ctx, account, in.AccountId)
			res := &adminProfileVisibleScope{AccountIds: ids, TenantId: account.TenantId}
			_ = cache.Instance().Set(ctx, cacheKey, res, accountVisibilityCacheTTL)
			return res, err
		}
		mineIds, err := s.expandFollowNoteAccountIds(ctx, []int64{account.Id})
		if err != nil {
			return nil, err
		}
		followIds, err := s.followNoteDirectAccountIds(ctx, account, nil)
		if err != nil {
			return nil, err
		}
		res := &adminProfileVisibleScope{
			AccountIds: uniqueIds(append(mineIds, followIds...)),
			TenantId:   account.TenantId,
		}
		_ = cache.Instance().Set(ctx, cacheKey, res, accountVisibilityCacheTTL)
		return res, nil
	default:
		return nil, gerror.New("账号筛选范围不合法")
	}
}

func (s *sSysPublish) adminManagedAccountIds(ctx context.Context, account *sysin.AccountModel) ([]int64, error) {
	cacheKey := adminManagedAccountIdsCacheKey(ctx, account.TenantId)
	if cached, err := cache.Instance().Get(ctx, cacheKey); err == nil && !cached.IsNil() {
		var ids []int64
		if scanErr := cached.Scan(&ids); scanErr == nil {
			return ids, nil
		}
	}
	columns := pdao.YoubanPublishAccount.Columns()
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(columns.Id).
		Where(columns.TenantId, account.TenantId).
		Where(columns.Status, 1).
		WhereNull(columns.DeletedAt).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架账号失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			ids = append(ids, row.Id)
		}
	}
	ids = uniqueIds(ids)
	_ = cache.Instance().Set(ctx, cacheKey, ids, accountVisibilityCacheTTL)
	return ids, nil
}

func (s *sSysPublish) followNoteAccountIds(ctx context.Context, account *sysin.AccountModel, in *sysin.FollowNoteListInp) ([]int64, error) {
	scope := "all"
	if in != nil && strings.TrimSpace(in.Scope) != "" {
		scope = strings.TrimSpace(in.Scope)
	}
	cacheKey := followNoteAccountIdsCacheKey(ctx, account.TenantId, account.Id, scope, accountScopeSelectedIDFromFollow(in))
	if cached, err := cache.Instance().Get(ctx, cacheKey); err == nil && !cached.IsNil() {
		var ids []int64
		if scanErr := cached.Scan(&ids); scanErr == nil {
			return ids, nil
		}
	}
	if scope == "mine" {
		ids := []int64{account.Id}
		_ = cache.Instance().Set(ctx, cacheKey, ids, accountVisibilityCacheTTL)
		return ids, nil
	}
	if scope == "following" {
		ids, err := s.followNoteDirectAccountIds(ctx, account, in)
		if err == nil {
			_ = cache.Instance().Set(ctx, cacheKey, ids, accountVisibilityCacheTTL)
		}
		return ids, err
	}
	if account.AccountType == sysin.PublishAccountTypeAdmin && scope == "all" {
		if in != nil && in.AccountId > 0 {
			ids, err := s.adminFollowNoteSelectedAccountIds(ctx, account, in.AccountId)
			if err == nil {
				_ = cache.Instance().Set(ctx, cacheKey, ids, accountVisibilityCacheTTL)
			}
			return ids, err
		}
		ids, err := s.adminManagedAccountIds(ctx, account)
		if err == nil {
			_ = cache.Instance().Set(ctx, cacheKey, ids, accountVisibilityCacheTTL)
		}
		return ids, err
	}
	var rows []struct {
		FollowingAccountId int64 `json:"followingAccountId"`
	}
	if err := pdao.YoubanPublishAccountFollow.Ctx(ctx).
		Fields("following_account_id").
		Where("tenant_id", account.TenantId).
		Where("follower_account_id", account.Id).
		Where("status", sysin.AccountFollowStatusApproved).
		WhereNull("deleted_at").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取关注账号失败")
	}
	ids := make([]int64, 0, len(rows)+1)
	if scope != "following" {
		ids = append(ids, account.Id)
	}
	for _, row := range rows {
		ids = append(ids, row.FollowingAccountId)
	}
	accountIds, err := s.expandFollowNoteAccountIds(ctx, uniqueIds(ids))
	if err != nil {
		return nil, err
	}
	if in == nil || in.AccountId <= 0 {
		_ = cache.Instance().Set(ctx, cacheKey, accountIds, accountVisibilityCacheTTL)
		return accountIds, nil
	}
	selectedIds, err := s.expandFollowNoteAccountIds(ctx, []int64{in.AccountId})
	if err != nil {
		return nil, err
	}
	result := intersectInt64(accountIds, selectedIds)
	_ = cache.Instance().Set(ctx, cacheKey, result, accountVisibilityCacheTTL)
	return result, nil
}

func (s *sSysPublish) followNoteDirectAccountIds(ctx context.Context, account *sysin.AccountModel, in *sysin.FollowNoteListInp) ([]int64, error) {
	var rows []struct {
		FollowingAccountId int64 `json:"followingAccountId"`
	}
	if err := pdao.YoubanPublishAccountFollow.Ctx(ctx).
		Fields("following_account_id").
		Where("tenant_id", account.TenantId).
		Where("follower_account_id", account.Id).
		Where("status", sysin.AccountFollowStatusApproved).
		WhereNull("deleted_at").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取关注账号失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.FollowingAccountId)
	}
	ids = uniqueIds(ids)
	if in == nil || in.AccountId <= 0 {
		return ids, nil
	}
	selectedIds, err := s.directFollowNoteAccountIds(ctx, []int64{in.AccountId})
	if err != nil {
		return nil, err
	}
	return intersectInt64(ids, selectedIds), nil
}

func (s *sSysPublish) directFollowNoteAccountIds(ctx context.Context, accountIds []int64) ([]int64, error) {
	accountIds = uniqueIds(accountIds)
	if len(accountIds) == 0 {
		return []int64{}, nil
	}
	columns := pdao.YoubanPublishAccount.Columns()
	var accounts []struct {
		Id int64 `json:"id"`
	}
	if err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(columns.Id).
		WhereIn(columns.Id, accountIds).
		WhereNull(columns.DeletedAt).
		Scan(&accounts); err != nil {
		return nil, gerror.Wrap(err, "读取关注账号失败")
	}
	result := make([]int64, 0, len(accounts))
	for _, item := range accounts {
		if item.Id > 0 {
			result = append(result, item.Id)
		}
	}
	return uniqueIds(result), nil
}

func adminManagedAccountIdsCacheKey(ctx context.Context, tenantId int64) string {
	_ = ctx
	return fmt.Sprintf("youban_publish:account_visibility:managed:%d:%s", tenantId, accountVisibilityVersionValue(ctx, tenantId))
}

func followNoteAccountIdsCacheKey(ctx context.Context, tenantId int64, accountId int64, scope string, selectedAccountId int64) string {
	return fmt.Sprintf("youban_publish:account_visibility:follow:%d:%d:%s:%d:%s", tenantId, accountId, scope, selectedAccountId, accountVisibilityVersionValue(ctx, tenantId))
}

func accountVisibilityScopeCacheKey(ctx context.Context, tenantId int64, accountId int64, scope string, selectedAccountId int64) string {
	return fmt.Sprintf("youban_publish:account_visibility:scope:%d:%d:%s:%d:%s", tenantId, accountId, scope, selectedAccountId, accountVisibilityVersionValue(ctx, tenantId))
}

func accountVisibilityVersionValue(ctx context.Context, tenantId int64) string {
	value, err := cache.Instance().Get(ctx, accountVisibilityVersionKey(tenantId))
	if err != nil || value.IsNil() {
		return "0"
	}
	return value.String()
}

func bumpAccountVisibilityVersion(ctx context.Context, tenantId int64) error {
	if tenantId <= 0 {
		return nil
	}
	return cache.Instance().Set(ctx, accountVisibilityVersionKey(tenantId), time.Now().UnixNano(), accountVisibilityVersionTTL)
}

func accountVisibilityVersionKey(tenantId int64) string {
	return fmt.Sprintf("youban_publish:account_visibility:version:%d", tenantId)
}

func accountScopeSelectedID(in *sysin.ProfileListInp) int64 {
	if in == nil {
		return 0
	}
	return in.AccountId
}

func accountScopeSelectedIDFromFollow(in *sysin.FollowNoteListInp) int64 {
	if in == nil {
		return 0
	}
	return in.AccountId
}

func (s *sSysPublish) adminFollowNoteSelectedAccountIds(ctx context.Context, account *sysin.AccountModel, accountId int64) ([]int64, error) {
	columns := pdao.YoubanPublishAccount.Columns()
	var selected *sysin.AccountModel
	if err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(columns.Id, columns.TenantId, columns.AccountType, columns.Status).
		Where(columns.Id, accountId).
		WhereNull(columns.DeletedAt).
		Scan(&selected); err != nil {
		return nil, gerror.Wrap(err, "读取筛选账号失败")
	}
	if selected == nil || selected.Id <= 0 || selected.Status != 1 {
		return []int64{}, nil
	}
	if selected.TenantId == account.TenantId {
		return []int64{selected.Id}, nil
	}
	var count int
	var err error
	if count, err = pdao.YoubanPublishAccountFollow.Ctx(ctx).
		Where("tenant_id", account.TenantId).
		Where("follower_account_id", account.Id).
		Where("following_account_id", selected.Id).
		Where("status", sysin.AccountFollowStatusApproved).
		WhereNull("deleted_at").
		Count(); err != nil {
		return nil, gerror.Wrap(err, "检查关注账号权限失败")
	}
	if count == 0 {
		return []int64{}, nil
	}
	return s.expandFollowNoteAccountIds(ctx, []int64{selected.Id})
}

func (s *sSysPublish) expandFollowNoteAccountIds(ctx context.Context, accountIds []int64) ([]int64, error) {
	accountIds = uniqueIds(accountIds)
	if len(accountIds) == 0 {
		return []int64{}, nil
	}
	columns := pdao.YoubanPublishAccount.Columns()
	var accounts []struct {
		Id          int64  `json:"id"`
		TenantId    int64  `json:"tenantId"`
		AccountType string `json:"accountType"`
	}
	if err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(columns.Id, columns.TenantId, columns.AccountType).
		WhereIn(columns.Id, accountIds).
		WhereNull(columns.DeletedAt).
		Scan(&accounts); err != nil {
		return nil, gerror.Wrap(err, "读取关注账号失败")
	}
	tenantIds := make([]int64, 0, len(accounts))
	result := make([]int64, 0, len(accounts))
	for _, item := range accounts {
		if item.Id > 0 {
			result = append(result, item.Id)
		}
		if item.AccountType == sysin.PublishAccountTypeAdmin && item.TenantId > 0 {
			tenantIds = append(tenantIds, item.TenantId)
		}
	}
	tenantIds = uniqueIds(tenantIds)
	if len(tenantIds) == 0 {
		return uniqueIds(result), nil
	}
	var tenantAccounts []struct {
		Id int64 `json:"id"`
	}
	if err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(columns.Id).
		WhereIn(columns.TenantId, tenantIds).
		Where(columns.Status, 1).
		WhereNull(columns.DeletedAt).
		Scan(&tenantAccounts); err != nil {
		return nil, gerror.Wrap(err, "读取关注租户账号失败")
	}
	for _, item := range tenantAccounts {
		if item.Id > 0 {
			result = append(result, item.Id)
		}
	}
	return uniqueIds(result), nil
}
