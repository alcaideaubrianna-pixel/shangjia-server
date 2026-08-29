package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

func (s *sSysPublish) BotProfileSearch(ctx context.Context, in *sysin.BotProfileSearchInp) (list []*sysin.NoteModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.BotProfileSearchInp{}
	}
	if in.TenantId <= 0 {
		return nil, 0, gerror.New("上架账号信息不完整")
	}
	profileIn := &sysin.NoteListInp{ProfileListInp: sysin.ProfileListInp{Keyword: strings.TrimSpace(in.Keyword), Status: in.Status}}
	profileIn.Page = in.Page
	profileIn.PerPage = in.PerPage
	if profileIn.Page <= 0 {
		profileIn.Page = 1
	}
	if profileIn.PerPage <= 0 || profileIn.PerPage > 10 {
		profileIn.PerPage = 10
	}
	if no := normalizeBotProfileNo(in.ProfileNo); no != "" {
		profileIn.Keyword = no
	}
	accountIds, err := s.botProfileSearchAccountIds(ctx, in)
	if err != nil {
		return nil, 0, err
	}
	if len(accountIds) == 0 {
		return []*sysin.NoteModel{}, 0, nil
	}
	return s.noteListByAccountIds(ctx, profileIn, accountIds)
}

func (s *sSysPublish) botResolveProfileIdByAccountIds(ctx context.Context, tenantId int64, accountIds []int64, profileNo string, publicOnly bool) (int64, error) {
	no := normalizeBotProfileNo(profileNo)
	if no == "" {
		return 0, gerror.New("资料编号不能为空")
	}
	if len(accountIds) == 0 {
		return 0, gerror.New("资料不存在或无权操作")
	}
	base, err := s.profileBaseModel(ctx, tenantId, 0)
	if err != nil {
		return 0, err
	}
	base = base.WhereIn("ps.account_id", uniqueIds(accountIds))
	if publicOnly {
		base = base.Where("p."+dao.ContentProfile.Columns().Status, 1)
	}
	row, err := base.Fields("p."+dao.ContentProfile.Columns().Id).
		Where("p."+dao.ContentProfile.Columns().ProfileNo, no).One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取资料失败")
	}
	if row.IsEmpty() {
		return 0, gerror.New("资料不存在或无权操作")
	}
	id := row[dao.ContentProfile.Columns().Id].Int64()
	if id <= 0 {
		return 0, gerror.New("资料不存在或无权操作")
	}
	return id, nil
}

func (s *sSysPublish) botProfileSearchAccountIds(ctx context.Context, in *sysin.BotProfileSearchInp) ([]int64, error) {
	// Bot 调用前已由绑定层实时校验账号；管理员的搜索范围保持为当前账号。
	if strings.TrimSpace(in.AccountType) == sysin.PublishAccountTypeAdmin && in.AccountId > 0 {
		return []int64{in.AccountId}, nil
	}
	capability, err := s.activeAccountCapability(ctx, in.TenantId, in.AccountId)
	if err != nil {
		return nil, err
	}
	return s.sharedProfileAccountIds(ctx, capability)
}

func (s *sSysPublish) botProfileViewAccountIds(ctx context.Context, in *sysin.BotProfileViewInp) ([]int64, error) {
	capability, err := s.activeAccountCapability(ctx, in.TenantId, in.AccountId)
	if err != nil {
		return nil, err
	}
	account := &sysin.AccountModel{Id: capability.AccountId, TenantId: capability.TenantId, AccountType: capability.AccountType}
	if account.AccountType == sysin.PublishAccountTypeAdmin {
		scope, err := s.adminProfileVisibleScope(ctx, account, &sysin.ProfileListInp{AccountScope: "all"})
		if err != nil {
			return nil, err
		}
		return uniqueIds(scope.AccountIds), nil
	}
	followIds, err := s.followNoteAccountIds(ctx, account, &sysin.FollowNoteListInp{Scope: "all"})
	if err != nil {
		return nil, err
	}
	sharedIds, err := s.sharedProfileAccountIds(ctx, capability)
	if err != nil {
		return nil, err
	}
	return uniqueIds(append(sharedIds, followIds...)), nil
}

func (s *sSysPublish) BotProfileImageSearch(ctx context.Context, in *sysin.BotProfileImageSearchInp) (list []*sysin.NoteModel, totalCount int, err error) {
	if in == nil {
		return nil, 0, gerror.New("图片搜索信息不能为空")
	}
	imageUrl := normalizeBotMediaCacheURL(in.ImageUrl)
	if strings.TrimSpace(imageUrl) == "" {
		return nil, 0, gerror.New("请发送要搜索的图片")
	}
	return s.BotProfileMediaSearch(ctx, &sysin.BotMediaSearchInp{
		TenantId: in.TenantId, AccountId: in.AccountId, AccountType: in.AccountType,
		Threshold: in.Threshold, Items: []*sysin.BotMediaSearchItem{{FileUrl: imageUrl, MediaType: "image"}},
	})
}

func (s *sSysPublish) BotProfileView(ctx context.Context, in *sysin.BotProfileViewInp) (res *sysin.NoteModel, err error) {
	if in == nil {
		return nil, gerror.New("资料信息不能为空")
	}
	profileId := in.ProfileId
	var profile *sysin.ProfileModel
	if in.AccountId > 0 {
		visibleIds, scopeErr := s.botProfileViewAccountIds(ctx, in)
		if scopeErr != nil {
			return nil, scopeErr
		}
		if len(in.AccountIds) > 0 {
			in.AccountIds = intersectInt64(uniqueIds(in.AccountIds), visibleIds)
		} else {
			in.AccountIds = visibleIds
		}
	}
	if len(in.AccountIds) > 0 && profileId > 0 {
		// AccountIds is the permission boundary and may contain approved
		// followed accounts from another tenant.
		profile, err = s.botProfileViewByAccountIds(ctx, profileId, 0, in.AccountIds)
	} else if profileId <= 0 {
		// AccountIds has already been derived from the bound account's own/follow
		// scope. Do not re-apply the viewer tenant here because followed profiles
		// may belong to another tenant.
		profileId, err = s.botResolveProfileIdByAccountIds(ctx, 0, in.AccountIds, in.ProfileNo, in.PublicOnly)
		if err == nil && len(in.AccountIds) > 0 {
			profile, err = s.botProfileViewByAccountIds(ctx, profileId, 0, in.AccountIds)
		}
	}
	if err != nil {
		return nil, err
	}
	if profile == nil {
		profile, err = s.profileView(ctx, profileId, in.TenantId, in.AccountId)
	}
	if err != nil {
		return nil, err
	}
	if in.PublicOnly && profile.Status != 1 {
		return nil, gerror.New("资料未上架，不能分享")
	}
	media, err := s.mediaListByProfile(ctx, profile.Id, profile.TenantId, profile.AccountId)
	if err != nil {
		return nil, err
	}
	return &sysin.NoteModel{ProfileModel: *profile, Media: media}, nil
}
