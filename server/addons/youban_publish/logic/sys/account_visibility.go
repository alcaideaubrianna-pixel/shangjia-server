package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

type adminProfileVisibleScope struct {
	AccountIds []int64
	Strict     bool
	TenantId   int64
	TenantIds  []int64
}

const accountVisibilityCacheTTL = 10 * time.Minute
const accountVisibilityVersionTTL = 24 * time.Hour

const accountVisibilityReadModelTable = "hg_youban_publish_account_visibility"

// refreshAccountVisibilityAccountEdge keeps self/managed edges current after
// account lifecycle changes. Follow edges are deliberately preserved because
// they are owned by the follow relation lifecycle.
func refreshAccountVisibilityAccountEdge(ctx context.Context, tenantID, accountID, parentID int64, active bool) error {
	if tenantID <= 0 || accountID <= 0 {
		return nil
	}
	if active {
		now := gtime.Now()
		// An account transfer can change parent_id. Remove the old managed edge
		// before writing the new one so the previous administrator does not keep
		// seeing the account through the read model.
		if _, err := g.DB().Model(accountVisibilityReadModelTable).Ctx(ctx).
			Where("tenant_id", tenantID).Where("visible_account_id", accountID).
			Where("relation_type", "managed").Delete(); err != nil {
			return err
		}
		_, err := g.DB().Model(accountVisibilityReadModelTable).Ctx(ctx).Data(g.Map{
			"tenant_id": tenantID, "viewer_account_id": accountID, "visible_account_id": accountID,
			"relation_type": "self", "updated_at": now,
		}).OnConflict("(tenant_id,viewer_account_id,visible_account_id)").OnDuplicateEx("relation_type", "updated_at").Save()
		if err != nil {
			return err
		}
		if parentID > 0 && parentID != accountID {
			_, err = g.DB().Model(accountVisibilityReadModelTable).Ctx(ctx).Data(g.Map{
				"tenant_id": tenantID, "viewer_account_id": parentID, "visible_account_id": accountID,
				"relation_type": "managed", "updated_at": now,
			}).OnConflict("(tenant_id,viewer_account_id,visible_account_id)").OnDuplicateEx("relation_type", "updated_at").Save()
		}
		return err
	}
	_, err := g.DB().Model(accountVisibilityReadModelTable).Ctx(ctx).
		Where("tenant_id", tenantID).Where("visible_account_id", accountID).
		WhereIn("relation_type", []string{"self", "managed", "tenant_scope"}).Delete()
	return err
}

// refreshAccountVisibilityFollowEdge keeps the additive visibility read model
// in sync after a follow mutation. The legacy resolver remains authoritative;
// failures are returned to the caller so a deployment cannot silently claim
// the projection was refreshed when the relation table is unavailable.
func refreshAccountVisibilityFollowEdge(ctx context.Context, tenantID, firstID, secondID int64) error {
	if tenantID <= 0 || firstID <= 0 || secondID <= 0 || firstID == secondID {
		return nil
	}
	var accounts []struct {
		Id       int64 `orm:"id"`
		TenantId int64 `orm:"tenant_id"`
	}
	if err := pdao.YoubanPublishAccount.Ctx(ctx).Fields("id,tenant_id").WhereIn("id", []int64{firstID, secondID}).Scan(&accounts); err != nil {
		return err
	}
	accountTenants := make(map[int64]int64, len(accounts))
	for _, account := range accounts {
		accountTenants[account.Id] = account.TenantId
	}
	if len(accountTenants) != 2 || accountTenants[firstID] != tenantID || accountTenants[secondID] != tenantID {
		// Follow rows can historically cross tenant boundaries. Never project
		// such a relation into the tenant-scoped visibility model.
		_, err := g.DB().Model(accountVisibilityReadModelTable).Ctx(ctx).
			Where("tenant_id", tenantID).
			WhereIn("viewer_account_id", []int64{firstID, secondID}).
			WhereIn("visible_account_id", []int64{firstID, secondID}).Delete()
		return err
	}
	type relation struct{ Status string }
	read := func(follower, following int64) (string, error) {
		var row relation
		err := pdao.YoubanPublishAccountFollow.Ctx(ctx).
			Fields("status").Where("tenant_id", tenantID).
			Where("follower_account_id", follower).Where("following_account_id", following).
			WhereNull("deleted_at").Scan(&row)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(row.Status), nil
	}
	forward, err := read(firstID, secondID)
	if err != nil {
		return err
	}
	backward, err := read(secondID, firstID)
	if err != nil {
		return err
	}
	upsert := func(viewer, visible int64, relationType string) error {
		_, err := g.DB().Model(accountVisibilityReadModelTable).Ctx(ctx).Data(g.Map{
			"tenant_id": tenantID, "viewer_account_id": viewer, "visible_account_id": visible,
			"relation_type": relationType, "updated_at": gtime.Now(),
		}).OnConflict("(tenant_id,viewer_account_id,visible_account_id)").OnDuplicateEx("relation_type", "updated_at").Save()
		return err
	}
	remove := func(viewer, visible int64) error {
		_, err := g.DB().Model(accountVisibilityReadModelTable).Ctx(ctx).
			Where("tenant_id", tenantID).Where("viewer_account_id", viewer).Where("visible_account_id", visible).Delete()
		return err
	}
	syncTenantScope := func(viewer, adminID int64, enabled bool) error {
		if !enabled {
			_, err := g.DB().Model(accountVisibilityReadModelTable).Ctx(ctx).
				Where("tenant_id", tenantID).Where("viewer_account_id", viewer).
				Where("relation_type", "tenant_scope").Delete()
			return err
		}
		var accountType string
		if err := pdao.YoubanPublishAccount.Ctx(ctx).Fields("account_type").Where("tenant_id", tenantID).Where("id", adminID).WhereNull("deleted_at").Scan(&accountType); err != nil {
			return err
		}
		if accountType != sysin.PublishAccountTypeAdmin {
			return nil
		}
		var visibleIDs []int64
		if err := pdao.YoubanPublishAccount.Ctx(ctx).Fields("id").Where("tenant_id", tenantID).Where("status", 1).WhereNull("deleted_at").Scan(&visibleIDs); err != nil {
			return err
		}
		for _, visible := range visibleIDs {
			if visible <= 0 || visible == viewer {
				continue
			}
			if err := upsert(viewer, visible, "tenant_scope"); err != nil {
				return err
			}
		}
		return nil
	}
	if forward == sysin.AccountFollowStatusApproved {
		typ := "following"
		if backward == sysin.AccountFollowStatusApproved {
			typ = "mutual"
		}
		if err = upsert(firstID, secondID, typ); err != nil {
			return err
		}
		if err = syncTenantScope(firstID, secondID, true); err != nil {
			return err
		}
	} else if err = remove(firstID, secondID); err != nil {
		return err
	} else if err = syncTenantScope(firstID, secondID, false); err != nil {
		return err
	}
	if backward == sysin.AccountFollowStatusApproved {
		typ := "following"
		if forward == sysin.AccountFollowStatusApproved {
			typ = "mutual"
		}
		if err = upsert(secondID, firstID, typ); err != nil {
			return err
		}
		if err = syncTenantScope(secondID, firstID, true); err != nil {
			return err
		}
	} else if err = remove(secondID, firstID); err != nil {
		return err
	} else if err = syncTenantScope(secondID, firstID, false); err != nil {
		return err
	}
	return nil
}

func refreshAccountVisibilityFollowEdgeBestEffort(ctx context.Context, tenantID, firstID, secondID int64) {
	if err := refreshAccountVisibilityFollowEdge(ctx, tenantID, firstID, secondID); err != nil {
		g.Log().Warning(ctx, "账户可见性关注边增量刷新失败", g.Map{
			"tenantId": tenantID, "firstAccountId": firstID, "secondAccountId": secondID,
			"error": err.Error(),
		})
	}
}

func (s *sSysPublish) adminProfileVisibleScope(ctx context.Context, account *sysin.AccountModel, in *sysin.ProfileListInp) (result *adminProfileVisibleScope, err error) {
	defer func() {
		if result != nil && err == nil {
			compareAccountVisibilityReadModel(ctx, account, result)
		}
	}()
	if account == nil || account.Id <= 0 || account.TenantId <= 0 {
		return nil, gerror.New("当前账号无管理权限")
	}
	scope := "mine"
	if in != nil && strings.TrimSpace(in.AccountScope) != "" {
		scope = strings.TrimSpace(in.AccountScope)
	}
	if in == nil || in.AccountId <= 0 {
		if readModelScope, enabled, readErr := accountVisibilityReadModelScope(ctx, account.TenantId, account.Id, scope); readErr != nil {
			g.Log().Warning(ctx, "读取账户可见范围读模型失败，回退旧权限解析", g.Map{"tenantId": account.TenantId, "viewerAccountId": account.Id, "error": readErr.Error()})
		} else if enabled {
			return readModelScope, nil
		}
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
			res := &adminProfileVisibleScope{AccountIds: []int64{in.AccountId}, TenantId: account.TenantId}
			_ = cache.Instance().Set(ctx, cacheKey, res, accountVisibilityCacheTTL)
			return res, nil
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

// accountVisibilityReadModelScope is opt-in. The legacy resolver remains the
// default and the fallback whenever the projection is disabled or unavailable.
func accountVisibilityReadModelScope(ctx context.Context, tenantID, viewerID int64, scope string) (*adminProfileVisibleScope, bool, error) {
	if !g.Cfg().MustGet(ctx, "youbanPublish.accountVisibilityReadModelEnabled", false).Bool() || tenantID <= 0 || viewerID <= 0 {
		return nil, false, nil
	}
	relationTypes := accountVisibilityRelationTypes(scope)
	var rows []struct {
		VisibleAccountId int64 `orm:"visible_account_id"`
	}
	if err := g.DB().Model(accountVisibilityReadModelTable).Ctx(ctx).
		Fields("visible_account_id").Where("tenant_id", tenantID).
		Where("viewer_account_id", viewerID).WhereIn("relation_type", relationTypes).
		OrderAsc("visible_account_id").Scan(&rows); err != nil {
		return nil, false, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.VisibleAccountId > 0 {
			ids = append(ids, row.VisibleAccountId)
		}
	}
	return &adminProfileVisibleScope{AccountIds: uniqueIds(ids), Strict: scope == "following", TenantId: tenantID}, true, nil
}

func accountVisibilityRelationTypes(scope string) []string {
	if strings.TrimSpace(scope) == "following" {
		// Following a tenant administrator expands to that tenant's active
		// accounts, represented by tenant_scope edges.
		return []string{"following", "mutual", "tenant_scope"}
	}
	return []string{"self", "managed", "tenant_scope", "following", "mutual"}
}

// compareAccountVisibilityReadModel is deliberately observational. The legacy
// resolver remains authoritative until all relation mutation paths have been
// deployed and production parity is proven.
func compareAccountVisibilityReadModel(ctx context.Context, account *sysin.AccountModel, scope *adminProfileVisibleScope) {
	// Keep the parity query fully opt-in. The read model is disabled by default;
	// running a second unfiltered visibility query on every list request defeats
	// the fast path and can dominate latency for large tenants.
	if !g.Cfg().MustGet(ctx, "youbanPublish.accountVisibilityReadModelEnabled", false).Bool() {
		return
	}
	if account == nil || scope == nil || account.Id <= 0 || scope.TenantId <= 0 || len(scope.TenantIds) > 0 {
		return
	}
	var rows []struct {
		VisibleAccountId int64 `json:"visible_account_id"`
	}
	if err := g.DB().Model(accountVisibilityReadModelTable).Ctx(ctx).
		Fields("visible_account_id").Where("tenant_id", scope.TenantId).
		Where("viewer_account_id", account.Id).Scan(&rows); err != nil {
		g.Log().Warning(ctx, "账户可见性读模型双读失败", g.Map{"tenantId": scope.TenantId, "viewerAccountId": account.Id, "error": err.Error()})
		return
	}
	legacy := make(map[int64]struct{}, len(scope.AccountIds))
	for _, id := range scope.AccountIds {
		if id > 0 {
			legacy[id] = struct{}{}
		}
	}
	model := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		if row.VisibleAccountId > 0 {
			model[row.VisibleAccountId] = struct{}{}
		}
	}
	missing, extra := 0, 0
	for id := range legacy {
		if _, ok := model[id]; !ok {
			missing++
		}
	}
	for id := range model {
		if _, ok := legacy[id]; !ok {
			extra++
		}
	}
	if missing > 0 || extra > 0 {
		g.Log().Warning(ctx, "账户可见性读模型与旧权限结果不一致", g.Map{"tenantId": scope.TenantId, "viewerAccountId": account.Id, "legacyCount": len(legacy), "modelCount": len(model), "missing": missing, "extra": extra})
	}
}

func (s *sSysPublish) ensureAdminProfileScopeTenants(ctx context.Context, scope *adminProfileVisibleScope) error {
	if scope == nil || len(scope.AccountIds) == 0 {
		return nil
	}
	if len(scope.TenantIds) > 0 {
		return nil
	}
	columns := pdao.YoubanPublishAccount.Columns()
	var rows []struct {
		TenantId int64 `json:"tenantId"`
	}
	if err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(columns.TenantId).
		WhereIn(columns.Id, scope.AccountIds).
		Where(columns.Status, 1).
		WhereNull(columns.DeletedAt).
		Group(columns.TenantId).
		Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取资料租户范围失败")
	}
	for _, row := range rows {
		if row.TenantId > 0 {
			scope.TenantIds = append(scope.TenantIds, row.TenantId)
		}
	}
	scope.TenantIds = uniqueIds(scope.TenantIds)
	if len(scope.TenantIds) == 0 && scope.TenantId > 0 {
		scope.TenantIds = []int64{scope.TenantId}
	}
	return nil
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
	expandedIds, err := s.expandFollowNoteAccountIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	if in == nil || in.AccountId <= 0 {
		return expandedIds, nil
	}
	selectedIds, err := s.directFollowNoteAccountIds(ctx, []int64{in.AccountId})
	if err != nil {
		return nil, err
	}
	return intersectInt64(expandedIds, selectedIds), nil
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
	return fmt.Sprintf("youban_publish:account_visibility:scope:v2:%d:%d:%s:%d:%s", tenantId, accountId, scope, selectedAccountId, accountVisibilityVersionValue(ctx, tenantId))
}

func accountVisibilityVersionValue(ctx context.Context, tenantId int64) string {
	value, err := cache.Instance().Get(ctx, accountVisibilityVersionKey(tenantId))
	if err != nil || value.IsNil() {
		return "0"
	}
	return value.String()
}

func bumpAccountVisibilityVersion(ctx context.Context, tenantId int64) error {
	if tenantId <= 0 || !cache.Initialized() {
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
