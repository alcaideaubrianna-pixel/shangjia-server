package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"

	pdao "hotgo/addons/youban_publish/internal/dao"
	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/internal/consts"

	"github.com/gogf/gf/v2/errors/gerror"
)

func mediaSearchScopeForTenant(tenantId int64, accountIds []int64) *publishmodel.MediaSearchScope {
	return mediaSearchScopeFromPartitions([]publishmodel.MediaSearchScopePartition{{TenantId: tenantId, AccountIds: accountIds}})
}

func mediaSearchScopeFromPartitions(partitions []publishmodel.MediaSearchScopePartition) *publishmodel.MediaSearchScope {
	partitionAccounts := make(map[int64][]int64)
	for _, partition := range partitions {
		if partition.TenantId <= 0 {
			continue
		}
		partitionAccounts[partition.TenantId] = append(partitionAccounts[partition.TenantId], partition.AccountIds...)
	}
	tenantIds := make([]int64, 0, len(partitionAccounts))
	for tenantId := range partitionAccounts {
		tenantIds = append(tenantIds, tenantId)
	}
	sort.Slice(tenantIds, func(i, j int) bool { return tenantIds[i] < tenantIds[j] })
	scope := &publishmodel.MediaSearchScope{Partitions: make([]publishmodel.MediaSearchScopePartition, 0, len(tenantIds))}
	for _, tenantId := range tenantIds {
		accountIds := uniqueIds(partitionAccounts[tenantId])
		if len(accountIds) == 0 {
			continue
		}
		sort.Slice(accountIds, func(i, j int) bool { return accountIds[i] < accountIds[j] })
		scope.AccountIds = append(scope.AccountIds, accountIds...)
		scope.Partitions = append(scope.Partitions, publishmodel.MediaSearchScopePartition{TenantId: tenantId, AccountIds: accountIds})
	}
	scope.AccountIds = uniqueIds(scope.AccountIds)
	sort.Slice(scope.AccountIds, func(i, j int) bool { return scope.AccountIds[i] < scope.AccountIds[j] })
	return scope
}

func (s *sSysPublish) mediaSearchScopeByAccountIds(ctx context.Context, accountIds []int64) (*publishmodel.MediaSearchScope, error) {
	accountIds = uniqueIds(accountIds)
	if len(accountIds) == 0 {
		return &publishmodel.MediaSearchScope{}, nil
	}
	columns := pdao.YoubanPublishAccount.Columns()
	var rows []struct {
		Id       int64 `orm:"id"`
		TenantId int64 `orm:"tenant_id"`
	}
	if err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(columns.Id, columns.TenantId).
		WhereIn(columns.Id, accountIds).
		Where(columns.Status, consts.StatusEnabled).
		WhereNull(columns.DeletedAt).
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取图片搜索账号范围失败")
	}
	partitions := make([]publishmodel.MediaSearchScopePartition, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 && row.TenantId > 0 {
			partitions = append(partitions, publishmodel.MediaSearchScopePartition{TenantId: row.TenantId, AccountIds: []int64{row.Id}})
		}
	}
	return mediaSearchScopeFromPartitions(partitions), nil
}

func mediaSearchScopeTenantId(scope *publishmodel.MediaSearchScope) int64 {
	if scope == nil || len(scope.Partitions) != 1 {
		return 0
	}
	return scope.Partitions[0].TenantId
}

func mediaSearchScopeVersion(ctx context.Context, scope *publishmodel.MediaSearchScope) string {
	if scope == nil || len(scope.Partitions) == 0 {
		return "0"
	}
	normalized := mediaSearchScopeFromPartitions(scope.Partitions)
	parts := make([]string, 0, len(normalized.Partitions))
	for _, partition := range normalized.Partitions {
		parts = append(parts, fmt.Sprintf("%d=%s", partition.TenantId, mediaPHashBucketVersion(ctx, partition.TenantId, partition.AccountIds)))
	}
	sort.Strings(parts)
	return mediaPHashHashKey(strings.Join(parts, ","))
}

func mediaSearchScopeCacheKey(scope *publishmodel.MediaSearchScope) string {
	if scope == nil || len(scope.Partitions) == 0 {
		return "empty"
	}
	normalized := mediaSearchScopeFromPartitions(scope.Partitions)
	parts := make([]string, 0, len(normalized.Partitions))
	for _, partition := range normalized.Partitions {
		parts = append(parts, fmt.Sprintf("%d:%v", partition.TenantId, partition.AccountIds))
	}
	sort.Strings(parts)
	return mediaPHashHashKey(strings.Join(parts, "|"))
}
