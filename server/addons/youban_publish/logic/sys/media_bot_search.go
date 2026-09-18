package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"

	publishmodel "hotgo/addons/youban_publish/model"
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
	searchScope, err := s.mediaSearchScopeByAccountIds(ctx, accountIds)
	if err != nil {
		return nil, 0, err
	}
	searchIn := &sysin.ProfileImageSearchInp{
		Threshold: in.Threshold,
	}
	normalizeProfileImageSearchInput(searchIn)
	searchItems := make([]*sysin.BotMediaSearchItem, 0, len(in.Items))
	for _, item := range in.Items {
		if item == nil {
			continue
		}
		url := normalizeBotMediaCacheURL(item.FileUrl)
		if url == "" {
			continue
		}
		cloned := *item
		cloned.FileUrl = url
		searchItems = append(searchItems, &cloned)
	}
	seenHashes := make(map[string]struct{}, len(searchItems))
	coldItems := make([]*sysin.BotMediaSearchItem, 0, len(searchItems))
	// First search every fingerprint already available in Redis. This phase does
	// not download Telegram media and usually resolves forwarded/published media
	// immediately.
	for _, item := range searchItems {
		fingerprint, ok := telegramImageFingerprintFromCache(ctx, item.FileUniqueId)
		if !ok {
			coldItems = append(coldItems, item)
			continue
		}
		list, total, found, searchErr := s.botMediaSearchFingerprint(ctx, fingerprint, searchIn, searchScope, seenHashes)
		if searchErr != nil {
			return nil, 0, searchErr
		}
		if found {
			return list, total, nil
		}
	}
	// Cold fingerprints are intentionally processed one by one. As soon as one
	// image finds a profile, later images are never downloaded or decoded.
	var lastHashErr error
	coldProcessed := false
	for _, item := range coldItems {
		fingerprint, _, hashErr := cachedTelegramImageFingerprint(ctx, item.FileUniqueId, item.FileUrl)
		if hashErr != nil {
			lastHashErr = hashErr
			continue
		}
		coldProcessed = true
		list, total, found, searchErr := s.botMediaSearchFingerprint(ctx, fingerprint, searchIn, searchScope, seenHashes)
		if searchErr != nil {
			return nil, 0, searchErr
		}
		if found {
			return list, total, nil
		}
	}
	if lastHashErr != nil && !coldProcessed {
		return nil, 0, lastHashErr
	}
	return []*sysin.NoteModel{}, 0, nil
}

func (s *sSysPublish) botMediaSearchFingerprint(ctx context.Context, fingerprint *mediaFingerprint, searchIn *sysin.ProfileImageSearchInp, searchScope *publishmodel.MediaSearchScope, seenHashes map[string]struct{}) ([]*sysin.NoteModel, int, bool, error) {
	if fingerprint == nil || fingerprint.PHash == nil {
		return nil, 0, false, nil
	}
	hashKey := fmt.Sprintf("%s:%016x", fingerprint.MD5, fingerprint.PHash.GetHash())
	if _, exists := seenHashes[hashKey]; exists {
		return nil, 0, false, nil
	}
	seenHashes[hashKey] = struct{}{}
	matches, err := s.cachedProfileFingerprintSearchCandidates(ctx, fingerprint, searchIn, searchScope, nil)
	if err != nil {
		return nil, 0, false, err
	}
	if len(matches) == 0 {
		return nil, 0, false, nil
	}
	candidates := make(map[int64]*botMediaSearchCandidate)
	for _, item := range matches {
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
		return []*sysin.NoteModel{}, total, false, nil
	}
	list, err := s.profileImageSearchNotesByScope(ctx, profileIds, searchScope, nil, "")
	if err != nil {
		return nil, 0, false, err
	}
	orderedNotes := orderBotMediaSearchNotes(list, profileIds)
	return orderedNotes, total, len(orderedNotes) > 0, nil
}

func (s *sSysPublish) botMediaSearchAccountIds(ctx context.Context, in *sysin.BotMediaSearchInp) ([]int64, error) {
	viewer := &sysin.AccountModel{Id: in.AccountId, TenantId: in.TenantId, AccountType: strings.TrimSpace(in.AccountType)}
	return s.botProfileViewAccountIds(ctx, &sysin.BotProfileViewInp{
		TenantId: viewer.TenantId, AccountId: viewer.Id, AccountType: viewer.AccountType,
	})
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
		WhereIn("ps.account_id", uniqueIds(accountIds)).
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
	if err = s.applyProfileCollectionMetadata(ctx, []*sysin.ProfileModel{profile}); err != nil {
		return nil, err
	}
	return profile, nil
}
