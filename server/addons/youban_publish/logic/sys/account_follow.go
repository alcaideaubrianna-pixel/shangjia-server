package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) AccountProfileView(ctx context.Context, in *sysin.AccountProfileViewInp) (*sysin.AccountProfileModel, error) {
	current, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		in = &sysin.AccountProfileViewInp{}
	}
	accountId := in.AccountId
	if accountId <= 0 && strings.TrimSpace(in.Username) == "" {
		accountId = current.Id
	}
	return s.accountProfile(ctx, current, accountId, strings.TrimSpace(in.Username))
}

func (s *sSysPublish) AccountProfileSave(ctx context.Context, in *sysin.AccountProfileSaveInp) (*sysin.AccountProfileModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("账号资料不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	_, err = pdao.YoubanPublishAccount.Ctx(ctx).Where("id", account.Id).Where("tenant_id", account.TenantId).Data(g.Map{
		"nickname":                 in.Nickname,
		"avatar_url":               in.AvatarUrl,
		"contact_telegram":         in.ContactTelegram,
		"contact_wechat":           in.ContactWechat,
		"contact_phone":            in.ContactPhone,
		"contact_other":            in.ContactOther,
		"remark":                   in.Remark,
		"follow_approval_required": in.FollowApprovalRequired,
		"public_follow_enabled":    in.PublicFollowEnabled,
		"updated_at":               gtime.Now(),
	}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "保存账号资料失败")
	}
	return s.accountProfile(ctx, account, account.Id, "")
}

func (s *sSysPublish) AccountFollowList(ctx context.Context, in *sysin.AccountFollowListInp) (list []*sysin.AccountFollowModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.AccountFollowListInp{}
	}
	listType := strings.TrimSpace(in.ListType)
	if listType == "" {
		listType = "following"
	}
	if listType == "public" {
		return s.publicFollowAccounts(ctx, account, in)
	}
	mod := pdao.YoubanPublishAccountFollow.DB().Model(pdao.YoubanPublishAccountFollow.Table() + " f").Safe().Ctx(ctx)
	switch listType {
	case "follower", "request":
		mod = mod.LeftJoin(pdao.YoubanPublishAccount.Table()+" a", "a.id=f.follower_account_id").
			Where("f.following_account_id", account.Id)
		if listType == "request" {
			mod = mod.Where("f.status", sysin.AccountFollowStatusPending)
		} else {
			mod = mod.Where("f.status", sysin.AccountFollowStatusApproved)
		}
	case "blocked":
		mod = mod.LeftJoin(pdao.YoubanPublishAccount.Table()+" a", "a.id=f.follower_account_id").
			Where("f.blocked_by", account.Id).
			Where("f.status", sysin.AccountFollowStatusBlocked)
	default:
		mod = mod.LeftJoin(pdao.YoubanPublishAccount.Table()+" a", "a.id=f.following_account_id").
			Where("f.follower_account_id", account.Id)
	}
	if listType == "following" {
		mod = mod.Where("f.tenant_id", account.TenantId).
			Where("f.status", sysin.AccountFollowStatusApproved)
	}
	mod = mod.WhereNull("f.deleted_at")
	if in.Status != "" {
		mod = mod.Where("f.status", strings.TrimSpace(in.Status))
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(a.nickname LIKE ? OR a.username LIKE ?)", like, like)
	}
	if totalCount, err = mod.Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "统计关注列表失败")
	}
	fields := "f.id,a.id AS account_id,a.nickname,a.username,a.avatar_url,a.remark,f.status,f.approval_required_snapshot,f.created_at,f.approved_at"
	if err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("f.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取关注列表失败")
	}
	if err = s.enrichAccountFollowModels(ctx, list); err != nil {
		return nil, 0, err
	}
	return
}

func (s *sSysPublish) AccountFollowApply(ctx context.Context, in *sysin.AccountFollowApplyInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("关注参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	target, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where("account_type", sysin.PublishAccountTypeAdmin).
		Where("username", in.Username).
		Where("status", 1).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return gerror.Wrap(err, "读取关注账号失败")
	}
	if target.IsEmpty() {
		return gerror.New("账号不存在")
	}
	targetId := target["id"].Int64()
	if targetId == account.Id {
		return gerror.New("不能关注自己")
	}
	blocked, err := pdao.YoubanPublishAccountFollow.Ctx(ctx).
		Where("((follower_account_id=? AND following_account_id=?) OR (follower_account_id=? AND following_account_id=?))", account.Id, targetId, targetId, account.Id).
		Where("status", sysin.AccountFollowStatusBlocked).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查关注黑名单失败")
	}
	if blocked > 0 {
		return gerror.New("该账号暂不可关注")
	}
	approval := target["follow_approval_required"].Int()
	status := sysin.AccountFollowStatusApproved
	var approvedAt *gtime.Time
	if approval == 1 {
		status = sysin.AccountFollowStatusPending
	} else {
		approvedAt = gtime.Now()
	}
	now := gtime.Now()
	followDao := pdao.YoubanPublishAccountFollow.Ctx(ctx)
	existing, err := followDao.
		Unscoped().
		Where("follower_account_id", account.Id).
		Where("following_account_id", targetId).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取关注关系失败")
	}
	data := g.Map{
		"tenant_id":                  account.TenantId,
		"follower_account_id":        account.Id,
		"following_account_id":       targetId,
		"status":                     status,
		"approval_required_snapshot": approval,
		"remark":                     in.Remark,
		"approved_at":                approvedAt,
		"updated_at":                 now,
		"deleted_at":                 nil,
	}
	if !existing.IsEmpty() {
		_, err = pdao.YoubanPublishAccountFollow.Ctx(ctx).
			Unscoped().
			Where("id", existing["id"].Int64()).
			Data(data).
			Update()
		return gerror.Wrap(err, "提交关注失败")
	}
	data["created_at"] = now
	_, err = pdao.YoubanPublishAccountFollow.Ctx(ctx).Data(g.Map{
		"tenant_id":                  account.TenantId,
		"follower_account_id":        account.Id,
		"following_account_id":       targetId,
		"status":                     status,
		"approval_required_snapshot": approval,
		"remark":                     in.Remark,
		"approved_at":                approvedAt,
		"created_at":                 now,
		"updated_at":                 now,
	}).Insert()
	return gerror.Wrap(err, "提交关注失败")
}

func (s *sSysPublish) AccountFollowAction(ctx context.Context, in *sysin.AccountFollowActionInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("关注操作不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	if in.Action == "block" {
		targetAccountId := in.AccountId
		if in.Id > 0 {
			row, err := pdao.YoubanPublishAccountFollow.Ctx(ctx).
				Where("id", in.Id).
				Where("(follower_account_id=? OR following_account_id=? OR blocked_by=?)", account.Id, account.Id, account.Id).
				One()
			if err != nil {
				return gerror.Wrap(err, "读取关注记录失败")
			}
			if row.IsEmpty() {
				return gerror.New("关注记录不存在")
			}
			targetAccountId = row["follower_account_id"].Int64()
			if targetAccountId == account.Id {
				targetAccountId = row["following_account_id"].Int64()
			}
		}
		return s.blockAccountFollow(ctx, account, targetAccountId, in.Remark)
	}
	mod := pdao.YoubanPublishAccountFollow.Ctx(ctx)
	if in.Id > 0 {
		mod = mod.Where("id", in.Id)
	} else {
		switch in.Action {
		case "approve", "reject":
			mod = mod.Where("following_account_id", account.Id).Where("follower_account_id", in.AccountId)
		case "unblock":
			mod = mod.Where("blocked_by", account.Id).Where("(follower_account_id=? OR following_account_id=?)", in.AccountId, in.AccountId)
		default:
			mod = mod.Where("following_account_id", in.AccountId).Where("follower_account_id", account.Id)
		}
	}
	data := g.Map{"updated_at": gtime.Now(), "remark": in.Remark}
	switch in.Action {
	case "approve":
		data["status"] = sysin.AccountFollowStatusApproved
		data["approved_at"] = gtime.Now()
		mod = mod.Where("following_account_id", account.Id)
	case "reject":
		data["status"] = sysin.AccountFollowStatusRejected
		mod = mod.Where("following_account_id", account.Id)
	case "block":
		data["status"] = sysin.AccountFollowStatusBlocked
		data["blocked_by"] = account.Id
	case "unblock", "remove":
		data["deleted_at"] = gtime.Now()
	}
	mod = mod.WhereNull("deleted_at")
	if in.Id > 0 {
		switch in.Action {
		case "approve", "reject":
			mod = mod.Where("following_account_id", account.Id)
		case "unblock":
			mod = mod.Where("blocked_by", account.Id)
		case "remove":
			mod = mod.Where("(follower_account_id=? OR following_account_id=? OR blocked_by=?)", account.Id, account.Id, account.Id)
		}
	}
	_, err = mod.Data(data).Update()
	return gerror.Wrap(err, "处理关注失败")
}

func (s *sSysPublish) blockAccountFollow(ctx context.Context, account *sysin.AccountModel, targetAccountId int64, remark string) error {
	if targetAccountId <= 0 {
		return gerror.New("拉黑账号不能为空")
	}
	if targetAccountId == account.Id {
		return gerror.New("不能拉黑自己")
	}
	exists, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where("id", targetAccountId).
		Where("status", 1).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "读取拉黑账号失败")
	}
	if exists == 0 {
		return gerror.New("拉黑账号不存在")
	}
	now := gtime.Now()
	if _, err = pdao.YoubanPublishAccountFollow.Ctx(ctx).
		Where("((follower_account_id=? AND following_account_id=?) OR (follower_account_id=? AND following_account_id=?))", account.Id, targetAccountId, targetAccountId, account.Id).
		WhereNull("deleted_at").
		Data(g.Map{
			"deleted_at": now,
			"updated_at": now,
		}).
		Update(); err != nil {
		return gerror.Wrap(err, "清理原关注关系失败")
	}
	_, err = pdao.YoubanPublishAccountFollow.Ctx(ctx).Data(g.Map{
		"tenant_id":                  account.TenantId,
		"follower_account_id":        targetAccountId,
		"following_account_id":       account.Id,
		"status":                     sysin.AccountFollowStatusBlocked,
		"approval_required_snapshot": 0,
		"remark":                     remark,
		"blocked_by":                 account.Id,
		"created_at":                 now,
		"updated_at":                 now,
	}).OnDuplicate(g.Map{
		"status":     sysin.AccountFollowStatusBlocked,
		"remark":     remark,
		"blocked_by": account.Id,
		"updated_at": now,
		"deleted_at": nil,
	}).Insert()
	return gerror.Wrap(err, "拉黑账号失败")
}

func (s *sSysPublish) FollowNoteList(ctx context.Context, in *sysin.FollowNoteListInp) (list []*sysin.NoteModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	accountIds, err := s.followNoteAccountIds(ctx, account, in)
	if err != nil {
		return nil, 0, err
	}
	return s.noteListByAccounts(ctx, &in.ProfileListInp, 0, accountIds, account)
}

func (s *sSysPublish) FollowNoteView(ctx context.Context, in *sysin.ProfileViewInp) (res *sysin.ProfileViewModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return nil, gerror.New("资料UUID不能为空")
	}
	profileId, err := s.resolveProfileId(ctx, in.Id, in.Uuid, 0, 0)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileView(ctx, profileId, 0, 0)
	if err != nil {
		return nil, err
	}
	accountIds, err := s.followNoteAccountIds(ctx, account, &sysin.FollowNoteListInp{Scope: "all"})
	if err != nil {
		return nil, err
	}
	if !containsInt64(accountIds, profile.AccountId) {
		return nil, gerror.New("资料不存在或无权查看")
	}
	markProfilePermission(profile, profilePermissionForViewer(account, profile))
	media, err := s.mediaListByProfile(ctx, profile.Id, profile.TenantId, profile.AccountId)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileViewModel{Profile: profile, Media: media}, nil
}

func (s *sSysPublish) FollowNoteImageSearch(ctx context.Context, in *sysin.FollowNoteListInp, file *ghttp.UploadFile) ([]*sysin.NoteModel, int, error) {
	return s.FollowNoteList(ctx, in)
}

func (s *sSysPublish) followNoteAccountIds(ctx context.Context, account *sysin.AccountModel, in *sysin.FollowNoteListInp) ([]int64, error) {
	scope := "all"
	if in != nil && strings.TrimSpace(in.Scope) != "" {
		scope = strings.TrimSpace(in.Scope)
	}
	if scope == "mine" {
		return []int64{account.Id}, nil
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
	return s.expandFollowNoteAccountIds(ctx, uniqueIds(ids))
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
