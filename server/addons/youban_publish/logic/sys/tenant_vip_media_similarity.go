package sys

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/sync/singleflight"
)

const (
	mediaSimilarResultCacheTTL = 3 * time.Minute
	mediaSimilarSlowThreshold  = 500 * time.Millisecond
)

var mediaSimilarCandidateGroup singleflight.Group

type mediaSimilarCandidate struct {
	MediaId   int64
	ProfileId int64
	Distance  int
}

type mediaSimilarSource struct {
	AccountId      int64  `orm:"account_id"`
	Id             int64  `orm:"id"`
	MediaType      string `orm:"media_type"`
	PerceptualHash string `orm:"perceptual_hash"`
	ProfileId      int64  `orm:"profile_id"`
	TenantId       int64  `orm:"tenant_id"`
	UpdatedAt      string `orm:"updated_at"`
}

type mediaSimilarScope struct {
	AccountIds []int64
	CacheKey   string
	Partitions []mediaPHashBucketScopePart
}

func (s *sSysPublish) MediaSimilarList(ctx context.Context, in *sysin.MediaSimilarListInp) (*sysin.MediaSimilarListModel, int, error) {
	startedAt := time.Now()
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if err = s.ensureTenantVipFeature(ctx, account.TenantId, sysin.TenantVipFeatureSimilarMedia); err != nil {
		return nil, 0, err
	}
	normalizeMediaSimilarListInput(in)
	scopeStartedAt := time.Now()
	scope, err := s.mediaSimilarVisibleScope(ctx, account)
	if err != nil {
		return nil, 0, err
	}
	scopeDuration := time.Since(scopeStartedAt)
	sourceStartedAt := time.Now()
	source, err := s.visibleSourceMedia(ctx, scope, in.MediaId)
	if err != nil {
		return nil, 0, err
	}
	sourceDuration := time.Since(sourceStartedAt)
	candidateStartedAt := time.Now()
	items, err := s.cachedMediaSimilarCandidates(ctx, scope, source, in.Threshold)
	if err != nil {
		return nil, 0, err
	}
	candidateDuration := time.Since(candidateStartedAt)
	total := len(items)
	start := (in.Page - 1) * in.PerPage
	if start < 0 {
		start = 0
	}
	if start >= total {
		s.logSlowMediaSimilarList(ctx, in, total, scopeDuration, sourceDuration, candidateDuration, 0, time.Since(startedAt))
		return &sysin.MediaSimilarListModel{MediaId: in.MediaId, List: []*sysin.NoteModel{}}, total, nil
	}
	end := int(math.Min(float64(start+in.PerPage), float64(total)))
	noteStartedAt := time.Now()
	list, err := s.mediaSimilarNotes(ctx, account, scope, items[start:end])
	if err != nil {
		return nil, 0, err
	}
	noteDuration := time.Since(noteStartedAt)
	s.logSlowMediaSimilarList(ctx, in, total, scopeDuration, sourceDuration, candidateDuration, noteDuration, time.Since(startedAt))
	return &sysin.MediaSimilarListModel{MediaId: in.MediaId, List: list}, total, nil
}

func (s *sSysPublish) cachedMediaSimilarCandidates(ctx context.Context, scope *mediaSimilarScope, source *mediaSimilarSource, threshold int) ([]mediaSimilarCandidate, error) {
	if source == nil {
		return []mediaSimilarCandidate{}, nil
	}
	cacheKey := mediaSimilarResultCacheKey(scope, source, threshold)
	if value, err := cache.Instance().Get(ctx, cacheKey); err == nil && !value.IsNil() {
		var cached []mediaSimilarCandidate
		if scanErr := value.Scan(&cached); scanErr == nil {
			return filterMediaSimilarCandidates(source, cached), nil
		}
	}
	result, err, _ := mediaSimilarCandidateGroup.Do(cacheKey, func() (any, error) {
		return s.computeMediaSimilarCandidates(ctx, scope, source, threshold, cacheKey)
	})
	if err != nil {
		return nil, err
	}
	items, ok := result.([]mediaSimilarCandidate)
	if !ok {
		return nil, gerror.New("解析相似媒体查询结果失败")
	}
	return filterMediaSimilarCandidates(source, items), nil
}

func (s *sSysPublish) computeMediaSimilarCandidates(ctx context.Context, scope *mediaSimilarScope, source *mediaSimilarSource, threshold int, cacheKey string) ([]mediaSimilarCandidate, error) {
	queryHash, ok := parseUploadPHash(source.PerceptualHash)
	if !ok {
		return []mediaSimilarCandidate{}, nil
	}
	items, err := s.mediaSimilarBucketCandidates(ctx, scope, source, queryHash, threshold)
	if err != nil {
		return nil, err
	}
	items = filterMediaSimilarCandidates(source, items)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Distance == items[j].Distance {
			return items[i].ProfileId > items[j].ProfileId
		}
		return items[i].Distance < items[j].Distance
	})
	_ = cache.Instance().Set(ctx, cacheKey, items, mediaSimilarResultCacheTTL)
	return items, nil
}

func (s *sSysPublish) visibleSourceMedia(ctx context.Context, scope *mediaSimilarScope, mediaId int64) (*mediaSimilarSource, error) {
	rows, err := s.visibleSourceMediaMap(ctx, scope, []int64{mediaId})
	if err != nil {
		return nil, err
	}
	source, ok := rows[mediaId]
	if !ok {
		return nil, gerror.New("媒体不存在或无权操作")
	}
	return source, nil
}

func (s *sSysPublish) visibleSourceMediaMap(ctx context.Context, scope *mediaSimilarScope, mediaIds []int64) (map[int64]*mediaSimilarSource, error) {
	res := make(map[int64]*mediaSimilarSource)
	if scope == nil || len(mediaIds) == 0 {
		return res, nil
	}
	mod := g.DB().Model(publishMediaTable+" m").Safe().Ctx(ctx).
		Fields("m.id,m.tenant_id,m.account_id,m.profile_id,m.media_type,m.perceptual_hash,m.updated_at").
		WhereIn("m.id", mediaIds).
		WhereNot("m.perceptual_hash", "").
		WhereNull("m.deleted_at").
		Where(mediaSimilarLiveProfileIndexExistsSQL("m.profile_id", "m.tenant_id", "m.account_id"))
	mod = applyMediaSimilarScope(mod, scope)
	rows, err := mod.All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取媒体失败")
	}
	for _, row := range rows {
		id := row["id"].Int64()
		if id <= 0 {
			continue
		}
		res[id] = &mediaSimilarSource{
			AccountId:      row["account_id"].Int64(),
			Id:             id,
			MediaType:      row["media_type"].String(),
			PerceptualHash: row["perceptual_hash"].String(),
			ProfileId:      row["profile_id"].Int64(),
			TenantId:       row["tenant_id"].Int64(),
			UpdatedAt:      row["updated_at"].String(),
		}
	}
	return res, nil
}

func (s *sSysPublish) mediaSimilarVisibleScope(ctx context.Context, account *sysin.AccountModel) (*mediaSimilarScope, error) {
	if account == nil || account.Id <= 0 || account.TenantId <= 0 {
		return nil, gerror.New("当前账号无相似查询权限")
	}
	cacheKey := mediaSimilarScopeCacheKey(ctx, account.TenantId, account.Id)
	if value, err := cache.Instance().Get(ctx, cacheKey); err == nil && !value.IsNil() {
		var cached mediaSimilarScope
		if scanErr := value.Scan(&cached); scanErr == nil {
			return &cached, nil
		}
	}
	var ids []int64
	if account.AccountType != "admin" {
		ids = []int64{account.Id}
	} else {
		managedIds, err := s.adminManagedAccountIds(ctx, account)
		if err != nil {
			return nil, err
		}
		followIds, err := s.followNoteDirectAccountIds(ctx, account, nil)
		if err != nil {
			return nil, err
		}
		ids = append(ids, managedIds...)
		ids = append(ids, account.Id)
		ids = append(ids, followIds...)
	}
	ids = uniqueIds(ids)
	searchScope, err := s.mediaSearchScopeByAccountIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	scope := &mediaSimilarScope{
		AccountIds: searchScope.AccountIds,
		CacheKey:   mediaSimilarScopeCacheKey(ctx, account.TenantId, account.Id),
		Partitions: searchScope.Partitions,
	}
	_ = cache.Instance().Set(ctx, cacheKey, scope, accountVisibilityCacheTTL)
	return scope, nil
}

func (s *sSysPublish) mediaSimilarBucketCandidates(ctx context.Context, scope *mediaSimilarScope, source *mediaSimilarSource, queryHash *goimagehash.ImageHash, threshold int) ([]mediaSimilarCandidate, error) {
	if queryHash == nil || source == nil {
		return []mediaSimilarCandidate{}, nil
	}
	rows, err := mediaPHashBucketCandidateRowsWithScopes(ctx, fmt.Sprintf("%016x", queryHash.GetHash()), threshold, scope.Partitions, nil, source.MediaType, source.ProfileId)
	if err != nil {
		return nil, err
	}
	return mediaSimilarCandidatesFromRows(source, queryHash, rows, threshold), nil
}

func mediaSimilarCandidatesFromRows(source *mediaSimilarSource, queryHash *goimagehash.ImageHash, rows []mediaPHashBucketCandidateRow, threshold int) []mediaSimilarCandidate {
	items := make([]mediaSimilarCandidate, 0, len(rows))
	for _, row := range rows {
		if row.ProfileId <= 0 || row.ProfileId == source.ProfileId {
			continue
		}
		hash, ok := parseUploadPHash(row.HashValue)
		if !ok {
			continue
		}
		distance, distanceErr := queryHash.Distance(hash)
		if distanceErr != nil || distance > threshold {
			continue
		}
		items = append(items, mediaSimilarCandidate{MediaId: row.MediaId, ProfileId: row.ProfileId, Distance: distance})
	}
	items = mediaSimilarDeduplicate(items)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Distance == items[j].Distance {
			return items[i].ProfileId > items[j].ProfileId
		}
		return items[i].Distance < items[j].Distance
	})
	return items
}

func filterMediaSimilarCandidates(source *mediaSimilarSource, items []mediaSimilarCandidate) []mediaSimilarCandidate {
	if len(items) == 0 {
		return []mediaSimilarCandidate{}
	}
	excludeProfileId := int64(0)
	if source != nil {
		excludeProfileId = source.ProfileId
	}
	res := make([]mediaSimilarCandidate, 0, len(items))
	for _, item := range items {
		if item.ProfileId <= 0 || item.ProfileId == excludeProfileId {
			continue
		}
		res = append(res, item)
	}
	return res
}

func mediaSimilarDeduplicate(items []mediaSimilarCandidate) []mediaSimilarCandidate {
	bestByProfile := make(map[int64]mediaSimilarCandidate, len(items))
	for _, item := range items {
		if item.ProfileId <= 0 {
			continue
		}
		current, exists := bestByProfile[item.ProfileId]
		if !exists || item.Distance < current.Distance ||
			(item.Distance == current.Distance && item.MediaId < current.MediaId) {
			bestByProfile[item.ProfileId] = item
		}
	}
	res := make([]mediaSimilarCandidate, 0, len(bestByProfile))
	for _, item := range bestByProfile {
		res = append(res, item)
	}
	return res
}

func applyMediaSimilarScope(mod *gdb.Model, scope *mediaSimilarScope) *gdb.Model {
	conditions, args := mediaPHashBucketScopeSQL("", scope.Partitions)
	if conditions != "" {
		conditions = strings.ReplaceAll(conditions, ".tenant_id", "tenant_id")
		conditions = strings.ReplaceAll(conditions, ".account_id", "account_id")
		mod = mod.Where("("+conditions+")", args...)
	}
	return mod
}

func (s *sSysPublish) mediaByIds(ctx context.Context, mediaIds []int64) ([]*sysin.MediaModel, error) {
	ids := uniqueIds(mediaIds)
	if len(ids) == 0 {
		return []*sysin.MediaModel{}, nil
	}
	var media []*sysin.MediaModel
	err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		WhereNull("deleted_at").
		OrderAsc("sort_index").
		OrderAsc("id").
		Scan(&media)
	if err != nil {
		return nil, gerror.Wrap(err, "读取相似媒体失败")
	}
	normalizeMediaListFileURL(media)
	return media, nil
}

func (s *sSysPublish) mediaSimilarNotes(ctx context.Context, viewer *sysin.AccountModel, scope *mediaSimilarScope, items []mediaSimilarCandidate) ([]*sysin.NoteModel, error) {
	profileIds := make([]int64, 0, len(items))
	mediaIds := make([]int64, 0, len(items))
	for _, item := range items {
		if item.ProfileId > 0 {
			profileIds = append(profileIds, item.ProfileId)
		}
		if item.MediaId > 0 {
			mediaIds = append(mediaIds, item.MediaId)
		}
	}
	notes, err := s.profileImageSearchNotesByScope(ctx, profileIds, mediaSearchScopeFromPartitions(scope.Partitions), viewer, "")
	if err != nil || len(notes) == 0 || len(mediaIds) == 0 {
		return notes, err
	}
	matchedMedia, err := s.mediaByIds(ctx, mediaIds)
	if err != nil {
		return nil, err
	}
	mediaByProfile := make(map[int64]*sysin.MediaModel, len(matchedMedia))
	for _, media := range matchedMedia {
		if media == nil || media.ProfileId <= 0 {
			continue
		}
		mediaByProfile[media.ProfileId] = media
	}
	for _, note := range notes {
		if note == nil {
			continue
		}
		if media := mediaByProfile[note.Id]; media != nil {
			note.Media = []*sysin.MediaModel{media}
		}
	}
	return notes, nil
}

func normalizeMediaSimilarListInput(in *sysin.MediaSimilarListInp) {
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 {
		in.PerPage = 20
	}
	if in.PerPage > 50 {
		in.PerPage = 50
	}
	if in.Threshold <= 0 {
		in.Threshold = mediaSimilarDefaultThreshold
	}
	if in.Threshold > 32 {
		in.Threshold = 32
	}
}

func mediaSimilarResultCacheKey(scope *mediaSimilarScope, source *mediaSimilarSource, threshold int) string {
	updatedAt := strings.TrimSpace(source.UpdatedAt)
	return fmt.Sprintf(
		"youban_publish:media_similar:v8:%s:%d:%d:%s",
		scope.CacheKey,
		source.Id,
		threshold,
		mediaSimilarHashKey(firstNonEmpty(source.PerceptualHash, "-")+":"+updatedAt),
	)
}

func mediaSimilarCountCacheKey(scope *mediaSimilarScope, source *mediaSimilarSource, threshold int) string {
	return "youban_publish:media_similar:count:v5:" + mediaSimilarResultCacheKey(scope, source, threshold)
}

func (s *sSysPublish) logSlowMediaSimilarList(ctx context.Context, in *sysin.MediaSimilarListInp, total int, scopeDuration time.Duration, sourceDuration time.Duration, candidateDuration time.Duration, noteDuration time.Duration, totalDuration time.Duration) {
	if totalDuration < mediaSimilarSlowThreshold {
		return
	}
	g.Log().Warning(ctx, "相似媒体列表查询耗时过长", g.Map{
		"mediaId":     in.MediaId,
		"page":        in.Page,
		"perPage":     in.PerPage,
		"threshold":   in.Threshold,
		"total":       total,
		"scopeMs":     scopeDuration.Milliseconds(),
		"sourceMs":    sourceDuration.Milliseconds(),
		"candidateMs": candidateDuration.Milliseconds(),
		"noteMs":      noteDuration.Milliseconds(),
		"totalMs":     totalDuration.Milliseconds(),
	})
}

func mediaSimilarLiveProfileIndexExistsSQL(profileIdExpr string, tenantIdExpr string, accountIdExpr string) string {
	return fmt.Sprintf(`EXISTS (
    SELECT 1
    FROM hg_content_profile p
    INNER JOIN hg_youban_publish_note_index i
      ON i.profile_id = p.id
     AND i.tenant_id = %s
     AND i.account_id = %s
     AND i.deleted_at IS NULL
    WHERE p.id = %s
		AND p.deleted_at IS NULL
		AND p.status IN (1, 2)
		AND i.status = p.status
      AND i.visibility = p.visibility
)`, tenantIdExpr, accountIdExpr, profileIdExpr)
}

func mediaSimilarScopeCacheKey(ctx context.Context, tenantId int64, accountId int64) string {
	return fmt.Sprintf(
		"youban_publish:media_similar:scope:%d:%d:%s",
		tenantId,
		accountId,
		accountVisibilityVersionValue(ctx, tenantId),
	)
}

func mediaSimilarHashKey(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
