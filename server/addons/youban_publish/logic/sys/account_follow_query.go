package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) accountProfile(ctx context.Context, current *sysin.AccountModel, accountId int64, username string) (*sysin.AccountProfileModel, error) {
	mod := pdao.YoubanPublishAccount.Ctx(ctx).
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
	profile.FollowingCount = s.accountFollowCount(ctx, profile.Id, "following")
	profile.FollowerCount = s.accountFollowCount(ctx, profile.Id, "follower")
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

func (s *sSysPublish) accountFollowCount(ctx context.Context, accountId int64, mode string) int {
	mod := pdao.YoubanPublishAccountFollow.Ctx(ctx).
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
	accountTable := pdao.YoubanPublishAccount.Table()
	followTable := pdao.YoubanPublishAccountFollow.Table()
	mod := pdao.YoubanPublishAccount.DB().Model(accountTable+" a").Safe().Ctx(ctx).
		Where("a.account_type", sysin.PublishAccountTypeAdmin).
		Where("a.public_follow_enabled", 1).
		Where("a.status", 1).
		WhereNot("a.id", account.Id).
		Where("NOT EXISTS (SELECT 1 FROM "+followTable+" f WHERE f.status=? AND f.deleted_at IS NULL AND ((f.follower_account_id=? AND f.following_account_id=a.id) OR (f.follower_account_id=a.id AND f.following_account_id=?)))", sysin.AccountFollowStatusBlocked, account.Id, account.Id).
		WhereNull("a.deleted_at")
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(a.nickname LIKE ? OR a.username LIKE ?)", like, like)
	}
	totalCount, err := mod.Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计公开关注账号失败")
	}
	var rows []*sysin.AccountFollowModel
	fields := "a.id AS account_id,a.nickname,a.username,a.avatar_url,a.remark"
	if err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("a.id").Scan(&rows); err != nil {
		return nil, 0, gerror.Wrap(err, "获取公开关注账号失败")
	}
	for _, row := range rows {
		row.Status = s.accountFollowStatus(ctx, account.Id, row.AccountId)
	}
	if err = s.enrichAccountFollowModels(ctx, rows); err != nil {
		return nil, 0, err
	}
	return rows, totalCount, nil
}

func (s *sSysPublish) enrichAccountFollowModels(ctx context.Context, rows []*sysin.AccountFollowModel) error {
	accountIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row != nil && row.AccountId > 0 {
			accountIds = append(accountIds, row.AccountId)
		}
	}
	accountIds = uniqueIds(accountIds)
	if len(accountIds) == 0 {
		return nil
	}
	stats, err := s.accountNoteStats(ctx, accountIds)
	if err != nil {
		return err
	}
	followingCounts, followerCounts, err := s.accountFollowCounts(ctx, accountIds)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if stat, ok := stats[row.AccountId]; ok {
			row.NoteCount = stat.Count
			row.LastNoteAt = stat.LastNoteAt
		}
		row.FollowingCount = followingCounts[row.AccountId]
		row.FollowerCount = followerCounts[row.AccountId]
	}
	return nil
}

type accountNoteStat struct {
	Count      int
	LastNoteAt *gtime.Time
}

func (s *sSysPublish) accountNoteStats(ctx context.Context, accountIds []int64) (map[int64]accountNoteStat, error) {
	var rows []struct {
		AccountId  int64       `json:"accountId"`
		NoteCount  int         `json:"noteCount"`
		LastNoteAt *gtime.Time `json:"lastNoteAt"`
	}
	err := pdao.YoubanPublishTask.Ctx(ctx).
		Fields("account_id,COUNT(*) AS note_count,MAX(published_at) AS last_note_at").
		WhereIn("account_id", accountIds).
		WhereNull("deleted_at").
		Group("account_id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计账号笔记失败")
	}
	stats := make(map[int64]accountNoteStat, len(rows))
	for _, row := range rows {
		stats[row.AccountId] = accountNoteStat{
			Count:      row.NoteCount,
			LastNoteAt: row.LastNoteAt,
		}
	}
	return stats, nil
}

func (s *sSysPublish) accountFollowCounts(ctx context.Context, accountIds []int64) (map[int64]int, map[int64]int, error) {
	following, err := s.accountFollowCountMap(ctx, accountIds, "follower_account_id")
	if err != nil {
		return nil, nil, err
	}
	follower, err := s.accountFollowCountMap(ctx, accountIds, "following_account_id")
	if err != nil {
		return nil, nil, err
	}
	return following, follower, nil
}

func (s *sSysPublish) accountFollowCountMap(ctx context.Context, accountIds []int64, field string) (map[int64]int, error) {
	var rows []struct {
		AccountId int64 `json:"accountId"`
		Total     int   `json:"total"`
	}
	err := pdao.YoubanPublishAccountFollow.Ctx(ctx).
		Fields(field+" AS account_id,COUNT(*) AS total").
		WhereIn(field, accountIds).
		Where("status", sysin.AccountFollowStatusApproved).
		WhereNull("deleted_at").
		Group(field).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计账号关注关系失败")
	}
	result := make(map[int64]int, len(rows))
	for _, row := range rows {
		result[row.AccountId] = row.Total
	}
	return result, nil
}

func (s *sSysPublish) noteListByAccounts(ctx context.Context, in *sysin.ProfileListInp, tenantId int64, accountIds []int64, viewer *sysin.AccountModel) ([]*sysin.NoteModel, int, error) {
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
		markProfilePermission(item, profilePermissionForViewer(viewer, item))
		note := &sysin.NoteModel{ProfileModel: *item}
		note.Media, err = s.mediaListByProfile(ctx, item.Id, item.TenantId, item.AccountId)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, note)
	}
	return list, totalCount, nil
}

func profilePermissionForViewer(viewer *sysin.AccountModel, profile *sysin.ProfileModel) string {
	if viewer == nil || profile == nil {
		return sysin.ProfilePermissionVisitor
	}
	if profile.AccountId == viewer.Id {
		return sysin.ProfilePermissionCreator
	}
	if viewer.AccountType == sysin.PublishAccountTypeAdmin && profile.TenantId == viewer.TenantId {
		return sysin.ProfilePermissionAdmin
	}
	return sysin.ProfilePermissionVisitor
}
