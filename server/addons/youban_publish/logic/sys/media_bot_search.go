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

const botMediaSearchMaxDistance = 12

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
	if in.SearchProgress != nil {
		*in.SearchProgress = sysin.BotMediaSearchProgress{}
	}
	searchCtx, cancelSearch := context.WithTimeout(ctx, botMediaSearchTimeout)
	defer cancelSearch()
	ctx = searchCtx
	requestID := gctx.CtxId(ctx)
	ctx = scantrace.With(ctx, g.Map{"requestId": requestID, "traceId": in.TraceId, "queryId": in.TraceId, "tenantId": in.TenantId, "accountId": in.AccountId, "items": len(in.Items)})
	stats := g.Map{"inputItems": len(in.Items), "incomplete": false}
	finish := scantrace.Begin(ctx, "search.total")
	defer func() { finish(resultErr) }()
	startedAt := time.Now()
	defer func() {
		stats["durationMs"], stats["resultTotal"], stats["returnedNotes"] = time.Since(startedAt).Milliseconds(), count, len(notes)
		stats["outcome"] = "complete"
		if stats["incomplete"] == true {
			stats["outcome"] = "partial"
		}
		if resultErr != nil {
			stats["outcome"] = "failed"
		}
		scantrace.Record(ctx, "search.summary", stats, resultErr)
	}()
	step := scantrace.Begin(ctx, "access.check")
	accessErr := s.ensureTenantVipFeature(ctx, in.TenantId, sysin.TenantVipFeatureSimilarMedia)
	step(accessErr)
	if err := accessErr; err != nil {
		return nil, 0, err
	}
	step = scantrace.Begin(ctx, "scope.accounts")
	accountIds, err := s.botMediaSearchAccountIds(ctx, in)
	stats["scopeAccounts"] = len(accountIds)
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
	stats["scopePartitions"] = len(searchScope.Partitions)
	searchIn := &sysin.ProfileImageSearchInp{
		Threshold: botMediaSearchMaxDistance,
	}
	normalizeProfileImageSearchInput(searchIn)
	searchItems := make([]*sysin.BotMediaSearchItem, 0, len(in.Items))
	for _, item := range in.Items {
		if item == nil {
			continue
		}
		url := normalizeBotMediaCacheURL(item.FileUrl)
		if url == "" && len(botMediaItemUniqueIDs(item)) == 0 && item.SourceMessageId <= 0 && item.FileId == "" {
			continue
		}
		cloned := *item
		cloned.FileUrl = url
		searchItems = append(searchItems, &cloned)
	}
	// Reused Telegram files have a stable identity across bots. Consult the
	// durable send ledger before downloading bytes or using approximate hashes.
	direct, err := botMediaIdentityCandidates(ctx, searchItems, searchScope)
	if err != nil {
		return nil, 0, err
	}
	matches := make([][]*botMediaSearchCandidate, 0, len(searchItems))
	remaining := make([]*sysin.BotMediaSearchItem, 0, len(searchItems))
	for i, item := range searchItems {
		if candidates := direct[i]; len(candidates) > 0 {
			matches = append(matches, candidates)
		} else {
			remaining = append(remaining, item)
		}
	}
	searchItems = remaining
	stats["identityHits"], stats["fingerprintItems"] = len(matches), len(searchItems)
	fingerprints := make([]*mediaFingerprint, 0, len(searchItems))
	coldItems := make([]*sysin.BotMediaSearchItem, 0, len(searchItems))
	for _, item := range searchItems {
		step = scantrace.Begin(ctx, "fingerprint.cache")
		fingerprint, ok := telegramImageFingerprintFromCache(ctx, item.FileUniqueId)
		step(nil)
		if ok {
			fingerprints = append(fingerprints, fingerprint)
		} else {
			coldItems = append(coldItems, item)
		}
	}
	var lastHashErr error
	stats["fingerprintCacheHits"], stats["coldItems"] = len(fingerprints), len(coldItems)
	failedItems := 0
	for _, result := range loadBotMediaColdFingerprints(ctx, in.TraceId, coldItems, cachedTelegramImageFingerprint) {
		if result.err != nil {
			lastHashErr = result.err
			failedItems++
			continue
		}
		fingerprints = append(fingerprints, result.fingerprint)
	}
	stats["coldFailures"] = failedItems
	stats["incomplete"] = failedItems > 0
	if len(fingerprints) == 0 && len(matches) == 0 && lastHashErr != nil {
		return nil, 0, lastHashErr
	}
	queryCtx, cancelQueries := context.WithTimeout(ctx, botMediaCandidateQueryTimeout(ctx))
	fingerprintMatches, err := loadBotMediaSearchCandidatesExactFirst(queryCtx, fingerprints,
		func(exactCtx context.Context, items []*mediaFingerprint) ([][]*botMediaSearchCandidate, error) {
			if !isPostgresDatabase() {
				return make([][]*botMediaSearchCandidate, len(items)), nil
			}
			return botMediaExactCandidates(exactCtx, items, searchScope)
		}, func(queryCtx context.Context, fingerprint *mediaFingerprint) ([]*botMediaSearchCandidate, error) {
			return s.botMediaFingerprintCandidates(queryCtx, fingerprint, searchIn, searchScope, isPostgresDatabase())
		})
	cancelQueries()
	if err != nil && len(matches) == 0 && len(rankBotMediaSearchCandidates(fingerprintMatches)) == 0 {
		return nil, 0, err
	}
	if err != nil {
		failedItems++
		stats["incomplete"] = true
		g.Log().Warning(ctx, "部分扫图查询失败，保留已确认的媒体匹配", g.Map{"err": err, "matchedItems": len(matches)})
	}
	matches = append(matches, fingerprintMatches...)
	if in.SearchProgress != nil {
		in.SearchProgress.Incomplete = failedItems > 0
	}
	profileIds := rankBotMediaSearchCandidates(matches)
	total := len(profileIds)
	if len(profileIds) > botMediaSearchMaxResults {
		profileIds = profileIds[:botMediaSearchMaxResults]
	}
	if len(profileIds) == 0 {
		if lastHashErr != nil {
			return nil, 0, lastHashErr
		}
		return []*sysin.NoteModel{}, 0, nil
	}
	// Load permission-scoped profile details once after merging the album.
	step = scantrace.Begin(ctx, "result.hydrate")
	list, err := s.profileImageSearchNotesByScope(ctx, profileIds, searchScope, nil, "")
	step(err)
	if err != nil {
		return nil, 0, err
	}
	return orderBotMediaSearchNotes(list, profileIds), total, nil
}

// Exact lookups run for the whole album before approximate queries consume the
// shared budget. Preserve input slots so multi-image ranking remains unchanged.
func loadBotMediaSearchCandidatesExactFirst(ctx context.Context, fingerprints []*mediaFingerprint, exact func(context.Context, []*mediaFingerprint) ([][]*botMediaSearchCandidate, error), approximate func(context.Context, *mediaFingerprint) ([]*botMediaSearchCandidate, error)) ([][]*botMediaSearchCandidate, error) {
	unique := uniqueBotMediaFingerprints(fingerprints)
	if len(unique) == 0 {
		return nil, nil
	}
	results, err := exact(ctx, unique)
	exactHits := 0
	for _, group := range results {
		if len(group) > 0 {
			exactHits++
		}
	}
	scantrace.Record(ctx, "query.exact_batch.summary", g.Map{"uniqueItems": len(unique), "matchedItems": exactHits, "remainingItems": len(unique) - exactHits}, err)
	if err != nil {
		return results, err
	}
	if len(results) != len(unique) {
		return nil, gerror.New("扫图精确匹配结果数量不一致")
	}
	remaining := make([]*mediaFingerprint, 0, len(unique))
	indices := make([]int, 0, len(unique))
	for i, f := range unique {
		if len(results[i]) == 0 {
			remaining = append(remaining, f)
			indices = append(indices, i)
		}
	}
	if len(remaining) == 0 {
		return results, nil
	}
	more, err := loadBotMediaSearchCandidates(ctx, remaining, approximate)
	scantrace.Record(ctx, "query.approx_batch.summary", g.Map{"inputItems": len(remaining), "matchedProfiles": len(rankBotMediaSearchCandidates(more)), "incomplete": err != nil}, err)
	for i, group := range more {
		results[indices[i]] = group
	}
	return results, err
}

func botMediaSentFileCandidates(ctx context.Context, fileIDs []string, scope *publishmodel.MediaSearchScope) (map[string][]*botMediaSearchCandidate, error) {
	result := make(map[string][]*botMediaSearchCandidate)
	if len(fileIDs) == 0 || scope == nil || len(scope.Partitions) == 0 {
		return result, nil
	}
	finish := scantrace.Begin(ctx, "query.telegram_file")
	mod := g.DB().Model(publishTgMessageTable+" m").Safe().Ctx(ctx).
		Fields("m.tg_file_unique_id,m.profile_id").WhereIn("m.tg_file_unique_id", fileIDs).
		Where("m.tg_file_unique_id <> ''").Where("m.status", "sent").WhereNull("m.deleted_at")
	scopeSQL, scopeArgs := mediaPHashBucketScopeSQL("m", scope.Partitions)
	if scopeSQL == "" {
		finish(nil)
		return result, nil
	}
	mod = mod.Where("("+scopeSQL+")", scopeArgs...).Where("EXISTS (SELECT 1 FROM hg_content_profile p WHERE p.id=m.profile_id AND p.deleted_at IS NULL)").Where("EXISTS (SELECT 1 FROM hg_youban_publish_profile_state ps WHERE ps.profile_id=m.profile_id AND ps.tenant_id=m.tenant_id AND ps.account_id=m.account_id AND ps.deleted_at IS NULL)")
	rows, err := mod.Group("m.tg_file_unique_id,m.profile_id").Limit(botMediaSearchMaxResults * len(fileIDs)).All()
	finish(err)
	if err != nil {
		return nil, gerror.Wrap(err, "按TG媒体标识查询已推送资料失败")
	}
	for _, row := range rows {
		id := row["tg_file_unique_id"].String()
		result[id] = append(result[id], &botMediaSearchCandidate{ProfileId: row["profile_id"].Int64(), Distance: 0, MatchCount: 1})
	}
	return result, nil
}

// Each slot belongs to one distinct input fingerprint, so matching several media
// rows in a profile cannot inflate the number of matching album members.
func rankBotMediaSearchCandidates(groups [][]*botMediaSearchCandidate) []int64 {
	merged := make(map[int64]*botMediaSearchCandidate)
	ids := make([]int64, 0)
	for _, group := range groups {
		seen := make(map[int64]bool)
		for _, item := range group {
			if item == nil || item.ProfileId <= 0 || item.Distance > botMediaSearchMaxDistance || seen[item.ProfileId] {
				continue
			}
			seen[item.ProfileId] = true
			candidate := merged[item.ProfileId]
			if candidate == nil {
				candidate = &botMediaSearchCandidate{ProfileId: item.ProfileId, Distance: item.Distance}
				merged[item.ProfileId] = candidate
				ids = append(ids, item.ProfileId)
			}
			candidate.MatchCount++
			if item.Distance < candidate.Distance {
				candidate.Distance = item.Distance
			}
		}
	}
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := merged[ids[i]], merged[ids[j]]
		if a.MatchCount != b.MatchCount {
			return a.MatchCount > b.MatchCount
		}
		return a.Distance < b.Distance
	})
	return ids
}

// botMediaCandidateQueryTimeout bounds all album candidate queries together.
// Invalid values retain the default; parent request deadlines still take priority.
func botMediaCandidateQueryTimeout(ctx context.Context) time.Duration {
	seconds := g.Cfg().MustGet(ctx, "youbanPublish.mediaSearch.queryTimeoutSeconds", 5).Int()
	if seconds <= 0 || int64(seconds) > int64((1<<63-1)/time.Second) {
		seconds = 5
	}
	return time.Duration(seconds) * time.Second
}

func loadBotMediaSearchCandidates(ctx context.Context, fingerprints []*mediaFingerprint, loader func(context.Context, *mediaFingerprint) ([]*botMediaSearchCandidate, error)) ([][]*botMediaSearchCandidate, error) {
	unique := uniqueBotMediaFingerprints(fingerprints)
	return loadUniqueBotMediaSearchCandidates(ctx, unique, loader)
}

func uniqueBotMediaFingerprints(fingerprints []*mediaFingerprint) []*mediaFingerprint {
	unique := make([]*mediaFingerprint, 0, len(fingerprints))
	seen := make(map[string]bool)
	for _, fingerprint := range fingerprints {
		if fingerprint == nil || (fingerprint.MD5 == "" && fingerprint.PHash == nil) {
			continue
		}
		key := fingerprint.MD5
		if fingerprint.PHash != nil {
			key = fmt.Sprintf("%s:%016x", key, fingerprint.PHash.GetHash())
		}
		if !seen[key] {
			seen[key] = true
			unique = append(unique, fingerprint)
		}
	}
	return unique
}

func loadUniqueBotMediaSearchCandidates(ctx context.Context, unique []*mediaFingerprint, loader func(context.Context, *mediaFingerprint) ([]*botMediaSearchCandidate, error)) ([][]*botMediaSearchCandidate, error) {
	results := make([][]*botMediaSearchCandidate, len(unique))
	errors := make([]error, len(unique))
	queryCtx, cancelQueries := context.WithTimeout(ctx, botMediaCandidateQueryTimeout(ctx))
	defer cancelQueries()
	group := new(errgroup.Group)
	// Keep database fan-out small even for a ten-image Telegram album.
	group.SetLimit(2)
	for index, fingerprint := range unique {
		index, fingerprint := index, fingerprint
		group.Go(func() error {
			itemCtx := scantrace.With(queryCtx, g.Map{"itemIndex": index})
			items, err := loader(itemCtx, fingerprint)
			if err == nil {
				results[index] = items
			}
			errors[index] = err
			// One failed album member must not cancel other valid matches.
			return nil
		})
	}
	_ = group.Wait()
	var firstErr error
	matched := false
	for index, items := range results {
		matched = matched || len(items) > 0
		if errors[index] != nil {
			if firstErr == nil {
				firstErr = errors[index]
			}
			g.Log().Warning(ctx, "扫图单张候选查询失败", g.Map{"itemIndex": index, "err": errors[index]})
		}
	}
	if firstErr != nil && !matched {
		return nil, firstErr
	}
	return results, firstErr
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

func (s *sSysPublish) botMediaSearchFingerprintCandidates(ctx context.Context, fingerprint *mediaFingerprint, searchIn *sysin.ProfileImageSearchInp, searchScope *publishmodel.MediaSearchScope) (candidatesResult []*botMediaSearchCandidate, resultErr error) {
	return s.botMediaFingerprintCandidates(ctx, fingerprint, searchIn, searchScope, false)
}

func (s *sSysPublish) botMediaFingerprintCandidates(ctx context.Context, fingerprint *mediaFingerprint, searchIn *sysin.ProfileImageSearchInp, searchScope *publishmodel.MediaSearchScope, exactAlreadyChecked bool) (candidatesResult []*botMediaSearchCandidate, resultErr error) {
	finish := scantrace.Begin(ctx, "fingerprint.search")
	defer func() { finish(resultErr) }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fallbackCtx, cancelFallback := context.WithTimeout(ctx, botMediaCandidateQueryTimeout(ctx))
	defer cancelFallback()
	var matches []publishProfilePHashDistance
	var err error
	if fingerprint.PHash == nil {
		if exactAlreadyChecked {
			return nil, nil
		}
		matches, err = mediaMD5CandidateMatches(fallbackCtx, fingerprint.MD5, searchScope, nil)
	} else {
		matches, err = s.cachedProfileFingerprintCandidates(fallbackCtx, fingerprint, searchIn, searchScope, nil, exactAlreadyChecked)
	}
	if err != nil {
		if fallbackCtx.Err() != nil {
			return nil, gerror.Wrap(fallbackCtx.Err(), "扫图相似查询超时，请稍后重试")
		}
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
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
	return ordered, nil
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
