package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) BotProfileSearch(ctx context.Context, in *sysin.BotProfileSearchInp) (list []*sysin.NoteModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.BotProfileSearchInp{}
	}
	if in.TenantId <= 0 {
		return nil, 0, gerror.New("上架账号信息不完整")
	}
	profileIn := &sysin.NoteListInp{ProfileListInp: sysin.ProfileListInp{TenantId: in.TenantId, AccountId: in.AccountId, Keyword: strings.TrimSpace(in.Keyword), Status: in.Status}}
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
	return s.noteList(ctx, profileIn)
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
	if len(in.AccountIds) == 0 && strings.EqualFold(strings.TrimSpace(in.AccountType), sysin.PublishAccountTypeAdmin) {
		in.AccountIds, err = s.botMediaSearchAccountIds(ctx, &sysin.BotMediaSearchInp{TenantId: in.TenantId, AccountId: in.AccountId, AccountType: in.AccountType})
	}
	if len(in.AccountIds) > 0 && profileId > 0 {
		profile, err = s.botProfileViewByAccountIds(ctx, profileId, in.TenantId, in.AccountIds)
	} else if profileId <= 0 {
		profileId, err = s.botResolveProfileId(ctx, in.TenantId, in.AccountId, in.ProfileNo, in.PublicOnly)
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
