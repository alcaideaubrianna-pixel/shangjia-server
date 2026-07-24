package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const botMediaSearchMaxResults = 20

type botMediaSearchCandidate struct {
	ProfileId  int64
	Distance   int
	MatchCount int
}

func (s *sSysPublish) EnsureBotMediaSearchAccess(ctx context.Context, tenantId int64) error {
	return s.ensureTenantVipFeature(ctx, tenantId, sysin.TenantVipFeatureSimilarMedia)
}

func (s *sSysPublish) BotProfileMediaSearch(ctx context.Context, in *sysin.BotMediaSearchInp) ([]*sysin.NoteModel, int, error) {
	if in == nil || in.TenantId <= 0 || in.AccountId <= 0 {
		return nil, 0, gerror.New("上架账号信息不完整")
	}
	if err := s.ensureTenantVipFeature(ctx, in.TenantId, sysin.TenantVipFeatureSimilarMedia); err != nil {
		return nil, 0, err
	}
	accountIds, err := s.botMediaSearchAccountIds(ctx, in)
	if err != nil {
		return nil, 0, err
	}
	if len(accountIds) == 0 {
		return []*sysin.NoteModel{}, 0, nil
	}
	searchIn := &sysin.ProfileImageSearchInp{
		ProfileListInp: sysin.ProfileListInp{TenantId: in.TenantId, AccountId: in.AccountId},
		Threshold:      in.Threshold,
	}
	normalizeProfileImageSearchInput(searchIn)
	if strings.EqualFold(strings.TrimSpace(in.AccountType), sysin.PublishAccountTypeAdmin) {
		searchIn.AccountId = 0
	}
	candidates := make(map[int64]*botMediaSearchCandidate)
	seenHashes := make(map[string]struct{})
	for _, item := range in.Items {
		if item == nil {
			continue
		}
		url := normalizeBotMediaCacheURL(item.FileUrl)
		if url == "" {
			continue
		}
		hash, hashErr := cachedRemoteImagePHashValue(ctx, url)
		if hashErr != nil {
			return nil, 0, hashErr
		}
		hashKey := fmt.Sprintf("%016x", hash.GetHash())
		if _, exists := seenHashes[hashKey]; exists {
			continue
		}
		seenHashes[hashKey] = struct{}{}
		items, searchErr := s.cachedProfilePHashSearchCandidates(ctx, hash, searchIn, accountIds, nil)
		if searchErr != nil {
			return nil, 0, searchErr
		}
		for _, item := range items {
			candidate, exists := candidates[item.ProfileId]
			if !exists {
				candidates[item.ProfileId] = &botMediaSearchCandidate{ProfileId: item.ProfileId, Distance: item.Distance, MatchCount: 1}
				continue
			}
			candidate.MatchCount++
			if item.Distance < candidate.Distance {
				candidate.Distance = item.Distance
			}
		}
	}
	ordered := make([]*botMediaSearchCandidate, 0, len(candidates))
	for _, item := range candidates {
		ordered = append(ordered, item)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Distance == ordered[j].Distance {
			if ordered[i].MatchCount == ordered[j].MatchCount {
				return ordered[i].ProfileId > ordered[j].ProfileId
			}
			return ordered[i].MatchCount > ordered[j].MatchCount
		}
		return ordered[i].Distance < ordered[j].Distance
	})
	total := len(ordered)
	if len(ordered) > botMediaSearchMaxResults {
		ordered = ordered[:botMediaSearchMaxResults]
	}
	profileIds := make([]int64, 0, len(ordered))
	for _, item := range ordered {
		profileIds = append(profileIds, item.ProfileId)
	}
	if len(profileIds) == 0 {
		return []*sysin.NoteModel{}, total, nil
	}
	list, err := s.profileImageSearchNotesByProfileIds(ctx, profileIds, in.TenantId, accountIds, nil, "")
	if err != nil {
		return nil, 0, err
	}
	return orderBotMediaSearchNotes(list, profileIds), total, nil
}

func (s *sSysPublish) botMediaSearchAccountIds(ctx context.Context, in *sysin.BotMediaSearchInp) ([]int64, error) {
	if !strings.EqualFold(strings.TrimSpace(in.AccountType), sysin.PublishAccountTypeAdmin) {
		return []int64{in.AccountId}, nil
	}
	viewer := &sysin.AccountModel{Id: in.AccountId, TenantId: in.TenantId, AccountType: sysin.PublishAccountTypeAdmin}
	scope, err := s.adminProfileVisibleScope(ctx, viewer, &sysin.ProfileListInp{AccountScope: "all"})
	if err != nil {
		return nil, err
	}
	return uniqueIds(scope.AccountIds), nil
}

func orderBotMediaSearchNotes(list []*sysin.NoteModel, profileIds []int64) []*sysin.NoteModel {
	byId := make(map[int64]*sysin.NoteModel, len(list))
	for _, item := range list {
		if item != nil {
			byId[item.Id] = item
		}
	}
	ordered := make([]*sysin.NoteModel, 0, len(profileIds))
	for _, id := range profileIds {
		if item := byId[id]; item != nil {
			ordered = append(ordered, item)
		}
	}
	return ordered
}

func (s *sSysPublish) botProfileViewByAccountIds(ctx context.Context, profileId int64, tenantId int64, accountIds []int64) (*sysin.ProfileModel, error) {
	base, err := s.profileBaseModel(ctx, tenantId, 0)
	if err != nil {
		return nil, err
	}
	var profile *sysin.ProfileModel
	err = base.Where("p.id", profileId).
		WhereIn("t.account_id", uniqueIds(accountIds)).
		Fields(profileListFields()).
		Scan(&profile)
	if err != nil {
		return nil, gerror.Wrap(err, "获取资料详情失败")
	}
	if profile == nil || profile.Id <= 0 {
		return nil, gerror.New("资料不存在或无权操作")
	}
	if err = s.ensureProfileModelUUID(ctx, profile); err != nil {
		return nil, err
	}
	if err = s.applyProfileTagNames(ctx, []*sysin.ProfileModel{profile}); err != nil {
		return nil, err
	}
	return profile, nil
}
