package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) accountProfile(ctx context.Context, current *sysin.AccountModel, accountId int64, username string) (*sysin.AccountProfileModel, error) {
	mod := pdao.YoubanPublishAccount.Ctx(ctx).
		Where("tenant_id", current.TenantId).
		Where("status", 1).
		WhereNull("deleted_at")
	if accountId > 0 {
		mod = mod.Where("id", accountId)
	} else {
		mod = mod.Where("username", username)
	}
	var profile *sysin.AccountProfileModel
	if err := mod.Scan(&profile); err != nil {
		return nil, gerror.Wrap(err, "读取账号主页失败")
	}
	if profile == nil || profile.Id <= 0 {
		return nil, gerror.New("账号不存在")
	}
	profile.NoteCount = s.accountNoteCount(ctx, profile.TenantId, profile.Id)
	profile.FollowingCount = s.accountFollowCount(ctx, profile.TenantId, profile.Id, "following")
	profile.FollowerCount = s.accountFollowCount(ctx, profile.TenantId, profile.Id, "follower")
	profile.FollowStatus = s.accountFollowStatus(ctx, current.Id, profile.Id)
	return profile, nil
}

func (s *sSysPublish) accountNoteCount(ctx context.Context, tenantId int64, accountId int64) int {
	count, _ := pdao.YoubanPublishTask.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		Count()
	return count
}

func (s *sSysPublish) accountFollowCount(ctx context.Context, tenantId int64, accountId int64, mode string) int {
	mod := pdao.YoubanPublishAccountFollow.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("status", sysin.AccountFollowStatusApproved).
		WhereNull("deleted_at")
	if mode == "follower" {
		mod = mod.Where("following_account_id", accountId)
	} else {
		mod = mod.Where("follower_account_id", accountId)
	}
	count, _ := mod.Count()
	return count
}

func (s *sSysPublish) accountFollowStatus(ctx context.Context, followerId int64, followingId int64) string {
	if followerId == followingId {
		return "self"
	}
	value, _ := pdao.YoubanPublishAccountFollow.Ctx(ctx).
		Fields("status").
		Where("follower_account_id", followerId).
		Where("following_account_id", followingId).
		WhereNull("deleted_at").
		Value()
	return value.String()
}

func (s *sSysPublish) publicFollowAccounts(ctx context.Context, account *sysin.AccountModel, in *sysin.AccountFollowListInp) ([]*sysin.AccountFollowModel, int, error) {
	mod := pdao.YoubanPublishAccount.Ctx(ctx).
		Where("tenant_id", account.TenantId).
		Where("account_type", sysin.PublishAccountTypeAdmin).
		Where("public_follow_enabled", 1).
		Where("status", 1).
		WhereNot("id", account.Id).
		WhereNull("deleted_at")
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(nickname LIKE ? OR username LIKE ?)", like, like)
	}
	totalCount, err := mod.Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计公开关注账号失败")
	}
	var rows []*sysin.AccountFollowModel
	fields := "id AS account_id,nickname,username,avatar_url,remark"
	if err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("id").Scan(&rows); err != nil {
		return nil, 0, gerror.Wrap(err, "获取公开关注账号失败")
	}
	for _, row := range rows {
		row.Status = s.accountFollowStatus(ctx, account.Id, row.AccountId)
	}
	return rows, totalCount, nil
}

func (s *sSysPublish) noteListByAccounts(ctx context.Context, in *sysin.ProfileListInp, tenantId int64, accountIds []int64) ([]*sysin.NoteModel, int, error) {
	if len(accountIds) == 0 {
		return []*sysin.NoteModel{}, 0, nil
	}
	mod, err := s.profileBaseModel(ctx, tenantId, 0)
	if err != nil {
		return nil, 0, err
	}
	mod = mod.WhereIn("t.account_id", accountIds)
	mod = s.applyProfileFilters(ctx, mod, in)
	totalCount, err := mod.Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计共享笔记失败")
	}
	var profiles []*sysin.ProfileModel
	if err = mod.Fields(profileListFields()).Page(in.Page, in.PerPage).OrderDesc("p.updated_at").OrderDesc("p.id").Scan(&profiles); err != nil {
		return nil, 0, gerror.Wrap(err, "获取共享笔记失败")
	}
	if err = s.applyProfileOwnerNames(ctx, profiles); err != nil {
		return nil, 0, err
	}
	list := make([]*sysin.NoteModel, 0, len(profiles))
	for _, item := range profiles {
		note := &sysin.NoteModel{ProfileModel: *item}
		note.Media, err = s.mediaListByProfile(ctx, item.Id, item.TenantId, item.AccountId)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, note)
	}
	return list, totalCount, nil
}
