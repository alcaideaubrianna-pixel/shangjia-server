package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gctx"
	"golang.org/x/sync/errgroup"

	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/utility/scantrace"
)

const botMediaSearchMaxResults = 20

const (
	botMediaSearchTimeout   = 45 * time.Second
	botMediaColdItemTimeout = 12 * time.Second
	botMediaColdConcurrency = 4
)

type botMediaSearchCandidate struct {
	ProfileId  int64
	Distance   int
	MatchCount int
}

type botMediaColdFingerprintResult struct {
	fingerprint *mediaFingerprint
	err         error
}

type botMediaColdFingerprintLoader func(context.Context, string, string) (*mediaFingerprint, bool, error)

func (s *sSysPublish) EnsureBotMediaSearchAccess(ctx context.Context, tenantId int64) error {
	return s.ensureTenantVipFeature(ctx, tenantId, sysin.TenantVipFeatureSimilarMedia)
}

func (s *sSysPublish) BotProfileMediaSearch(ctx context.Context, in *sysin.BotMediaSearchInp) (notes []*sysin.NoteModel, count int, resultErr error) {
	if in == nil || in.TenantId <= 0 || in.AccountId <= 0 {
		return nil, 0, gerror.New("上架账号信息不完整")
	}
	searchCtx, cancelSearch := context.WithTimeout(ctx, botMediaSearchTimeout)
	defer cancelSearch()
	ctx = searchCtx
	requestID := gctx.CtxId(ctx)
	ctx = scantrace.With(ctx, g.Map{"requestId": requestID, "queryId": in.TraceId, "tenantId": in.TenantId, "accountId": in.AccountId, "items": len(in.Items)})
	finish := scantrace.Begin(ctx, "search.total")
	defer func() { finish(resultErr) }()
	startedAt := time.Now()
	defer func() {
		g.Log().Info(ctx, "Bot扫图服务完成", g.Map{"requestId": requestID, "queryId": in.TraceId, "tenantId": in.TenantId, "accountId": in.AccountId, "items": len(in.Items), "durationMs": time.Since(startedAt).Milliseconds()})
	}()
	step := scantrace.Begin(ctx, "access.check")
	accessErr := s.ensureTenantVipFeature(ctx, in.TenantId, sysin.TenantVipFeatureSimilarMedia)
	step(accessErr)
	if err := accessErr; err != nil {
		return nil, 0, err
	}
	step = scantrace.Begin(ctx, "scope.accounts")
	accountIds, err := s.botMediaSearchAccountIds(ctx, in)
	step(err)
	if err != nil {
		return nil, 0, err
	}
	if len(accountIds) == 0 {
		return []*sysin.NoteModel{}, 0, nil
	}
	step = scantrace.Begin(ctx, "scope.partitions")
	searchScope, err := s.mediaSearchScopeByAccountIds(ctx, accountIds)
	step(err)
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
	notesById := make(map[int64]*sysin.NoteModel)
	orderedProfileIds := make([]int64, 0, botMediaSearchMaxResults)
	coldItems := make([]*sysin.BotMediaSearchItem, 0, len(searchItems))
	// First search every fingerprint already available in Redis. This phase does
	// not download Telegram media and usually resolves forwarded/published media
	// immediately.
	for _, item := range searchItems {
		step = scantrace.Begin(ctx, "fingerprint.cache")
		fingerprint, ok := telegramImageFingerprintFromCache(ctx, item.FileUniqueId)
		step(nil)
		if !ok {
			coldItems = append(coldItems, item)
			continue
		}
		list, _, _, searchErr := s.botMediaSearchFingerprint(ctx, fingerprint, searchIn, searchScope, seenHashes)
		if searchErr != nil {
			return nil, 0, searchErr
		}
		orderedProfileIds = mergeBotMediaSearchNotes(notesById, orderedProfileIds, list)
	}
	// Download and decode cold fingerprints concurrently, then search them in
	// the original message order so concurrency cannot change result priority.
	coldResults := loadBotMediaColdFingerprints(ctx, in.TraceId, coldItems, cachedTelegramImageFingerprint)
	var lastHashErr error
	coldProcessed := false
	for _, result := range coldResults {
		if result.err != nil {
			lastHashErr = result.err
			if ctx.Err() != nil {
				break
			}
			continue
		}
		coldProcessed = true
		list, _, _, searchErr := s.botMediaSearchFingerprint(ctx, result.fingerprint, searchIn, searchScope, seenHashes)
		if searchErr != nil {
			return nil, 0, searchErr
		}
		orderedProfileIds = mergeBotMediaSearchNotes(notesById, orderedProfileIds, list)
	}
	if lastHashErr != nil && !coldProcessed && len(orderedProfileIds) == 0 {
		return nil, 0, lastHashErr
	}
	total := len(orderedProfileIds)
	if len(orderedProfileIds) > botMediaSearchMaxResults {
		orderedProfileIds = orderedProfileIds[:botMediaSearchMaxResults]
	}
	list := make([]*sysin.NoteModel, 0, len(orderedProfileIds))
	for _, profileId := range orderedProfileIds {
		list = append(list, notesById[profileId])
	}
	return list, total, nil
}

func mergeBotMediaSearchNotes(notesById map[int64]*sysin.NoteModel, orderedProfileIds []int64, list []*sysin.NoteModel) []int64 {
	for _, note := range list {
		if note == nil || note.Id <= 0 {
			continue
		}
		if _, exists := notesById[note.Id]; exists {
			continue
		}
		notesById[note.Id] = note
		orderedProfileIds = append(orderedProfileIds, note.Id)
	}
	return orderedProfileIds
}

func loadBotMediaColdFingerprints(ctx context.Context, traceId string, items []*sysin.BotMediaSearchItem, loader botMediaColdFingerprintLoader) []botMediaColdFingerprintResult {
	results := make([]botMediaColdFingerprintResult, len(items))
	if len(items) == 0 || loader == nil {
		return results
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(botMediaColdConcurrency)
	for index, item := range items {
		index, item := index, item
		group.Go(func() error {
			if item == nil {
				return nil
			}
			startedAt := time.Now()
			itemCtx, cancelItem := context.WithTimeout(groupCtx, botMediaColdItemTimeout)
			itemCtx = scantrace.With(itemCtx, g.Map{"requestId": gctx.CtxId(ctx), "queryId": traceId, "itemIndex": index})
			finish := scantrace.Begin(itemCtx, "fingerprint.cold")
			fingerprint, cacheHit, err := loader(itemCtx, item.FileUniqueId, item.FileUrl)
			finish(err)
			cancelItem()
			results[index] = botMediaColdFingerprintResult{fingerprint: fingerprint, err: err}
			g.Log().Info(ctx, "Bot扫图冷媒体处理完成", g.Map{
				"requestId": gctx.CtxId(ctx), "queryId": traceId, "stage": "fingerprint_cold",
				"fileUniqueId": item.FileUniqueId, "cacheHit": cacheHit,
				"durationMs": time.Since(startedAt).Milliseconds(), "err": err,
			})
			return nil
		})
	}
	_ = group.Wait()
	return results
}

func (s *sSysPublish) botMediaSearchFingerprint(ctx context.Context, fingerprint *mediaFingerprint, searchIn *sysin.ProfileImageSearchInp, searchScope *publishmodel.MediaSearchScope, seenHashes map[string]struct{}) (notes []*sysin.NoteModel, count int, matched bool, resultErr error) {
	finish := scantrace.Begin(ctx, "fingerprint.search")
	defer func() { finish(resultErr) }()
	if err := ctx.Err(); err != nil {
		return nil, 0, false, err
	}
	if fingerprint == nil || fingerprint.PHash == nil {
		return nil, 0, false, nil
	}
	hashKey := fmt.Sprintf("%s:%016x", fingerprint.MD5, fingerprint.PHash.GetHash())
	if _, exists := seenHashes[hashKey]; exists {
		return nil, 0, false, nil
	}
	seenHashes[hashKey] = struct{}{}
	fallbackCtx, cancelFallback := context.WithTimeout(ctx, 2*time.Second)
	defer cancelFallback()
	matches, err := s.cachedProfileFingerprintSearchCandidates(fallbackCtx, fingerprint, searchIn, searchScope, nil)
	if err != nil {
		if fallbackCtx.Err() != nil {
			return []*sysin.NoteModel{}, 0, false, nil
		}
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
	startedAt := time.Now()
	list, err := s.profileImageSearchNotesByScope(ctx, profileIds, searchScope, nil, "")
	observeBotMediaSearchStage(ctx, "profile_load", startedAt, err)
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
