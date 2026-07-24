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

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const mediaSimilarResultCacheTTL = 10 * time.Minute

type mediaSimilarCandidate struct {
	ProfileId int64
	Distance  int
}

type mediaSimilarSource struct {
	AccountId      int64  `json:"accountId"`
	Id             int64  `json:"id"`
	MediaType      string `json:"mediaType"`
	PerceptualHash string `json:"perceptualHash"`
	ProfileId      int64  `json:"profileId"`
	TenantId       int64  `json:"tenantId"`
	UpdatedAt      string `json:"updatedAt"`
}

type mediaSimilarScope struct {
	AccountIds []int64
	CacheKey   string
	Partitions []mediaPHashBucketScopePart
}

func (s *sSysPublish) MediaSimilarCount(ctx context.Context, in *sysin.MediaSimilarCountInp) (*sysin.MediaSimilarCountModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.ensureTenantVipFeature(ctx, account.TenantId, sysin.TenantVipFeatureSimilarMedia); err != nil {
		return nil, err
	}
	if in == nil || in.MediaId <= 0 {
		return nil, gerror.New("媒体ID不能为空")
	}
	scope, err := s.mediaSimilarVisibleScope(ctx, account)
	if err != nil {
		return nil, err
	}
	sources, err := s.visibleSourceMediaMap(ctx, scope, []int64{in.MediaId})
	if err != nil {
		return nil, err
	}
	source := sources[in.MediaId]
	if source == nil {
		return &sysin.MediaSimilarCountModel{MediaId: in.MediaId, Count: 0}, nil
	}
	cacheKey := mediaSimilarCountCacheKey(scope, source, 12)
	if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
		return &sysin.MediaSimilarCountModel{MediaId: in.MediaId, Count: value.Int()}, nil
	}
	items, err := s.cachedMediaSimilarCandidates(ctx, scope, source, 12)
	if err != nil {
		return nil, err
	}
	count := len(items)
	_ = cache.Instance().Set(ctx, cacheKey, count, mediaSimilarResultCacheTTL)
	return &sysin.MediaSimilarCountModel{MediaId: in.MediaId, Count: count}, nil
}

func (s *sSysPublish) MediaSimilarList(ctx context.Context, in *sysin.MediaSimilarListInp) (*sysin.MediaSimilarListModel, int, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if err = s.ensureTenantVipFeature(ctx, account.TenantId, sysin.TenantVipFeatureSimilarMedia); err != nil {
		return nil, 0, err
	}
	normalizeMediaSimilarListInput(in)
	scope, err := s.mediaSimilarVisibleScope(ctx, account)
	if err != nil {
		return nil, 0, err
	}
	source, err := s.visibleSourceMedia(ctx, scope, in.MediaId)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.cachedMediaSimilarCandidates(ctx, scope, source, in.Threshold)
	if err != nil {
		return nil, 0, err
	}
	total := len(items)
	start := (in.Page - 1) * in.PerPage
	if start < 0 {
		start = 0
	}
	if start >= total {
		return &sysin.MediaSimilarListModel{MediaId: in.MediaId, List: []*sysin.NoteModel{}}, total, nil
	}
	end := int(math.Min(float64(start+in.PerPage), float64(total)))
	list, err := s.mediaSimilarNotes(ctx, account, items[start:end])
	if err != nil {
		return nil, 0, err
	}
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
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,profile_id,media_type,perceptual_hash,updated_at").
		WhereIn("id", mediaIds).
		WhereNot("perceptual_hash", "").
		WhereNull("deleted_at")
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
	partitions, err := s.mediaSimilarScopePartitions(ctx, ids)
	if err != nil {
		return nil, err
	}
	scope := &mediaSimilarScope{
		AccountIds: ids,
		CacheKey:   mediaSimilarScopeCacheKey(ctx, account.TenantId, account.Id),
		Partitions: partitions,
	}
	_ = cache.Instance().Set(ctx, cacheKey, scope, accountVisibilityCacheTTL)
	return scope, nil
}

func (s *sSysPublish) mediaSimilarScopePartitions(ctx context.Context, accountIds []int64) ([]mediaPHashBucketScopePart, error) {
	accountIds = uniqueIds(accountIds)
	if len(accountIds) == 0 {
		return []mediaPHashBucketScopePart{}, nil
	}
	columns := pdao.YoubanPublishAccount.Columns()
	var rows []struct {
		Id       int64 `json:"id"`
		TenantId int64 `json:"tenantId"`
	}
	if err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(columns.Id, columns.TenantId).
		WhereIn(columns.Id, accountIds).
		Where(columns.Status, 1).
		WhereNull(columns.DeletedAt).
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取相似资料权限范围失败")
	}
	grouped := make(map[int64][]int64)
	for _, row := range rows {
		if row.Id > 0 && row.TenantId > 0 {
			grouped[row.TenantId] = append(grouped[row.TenantId], row.Id)
		}
	}
	partitions := make([]mediaPHashBucketScopePart, 0, len(grouped))
	for tenantId, ids := range grouped {
		partitions = append(partitions, mediaPHashBucketScopePart{TenantId: tenantId, AccountIds: uniqueIds(ids)})
	}
	return partitions, nil
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
		items = append(items, mediaSimilarCandidate{ProfileId: row.ProfileId, Distance: distance})
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
	distanceByProfile := map[int64]int{}
	for _, item := range items {
		if item.ProfileId <= 0 {
			continue
		}
		current, exists := distanceByProfile[item.ProfileId]
		if !exists || item.Distance < current {
			distanceByProfile[item.ProfileId] = item.Distance
		}
	}
	res := make([]mediaSimilarCandidate, 0, len(distanceByProfile))
	for profileId, distance := range distanceByProfile {
		res = append(res, mediaSimilarCandidate{ProfileId: profileId, Distance: distance})
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

func (s *sSysPublish) mediaSimilarNotes(ctx context.Context, viewer *sysin.AccountModel, items []mediaSimilarCandidate) ([]*sysin.NoteModel, error) {
	profileIds := make([]int64, 0, len(items))
	for _, item := range items {
		if item.ProfileId > 0 {
			profileIds = append(profileIds, item.ProfileId)
		}
	}
	return s.profileImageSearchNotesByProfileIds(ctx, profileIds, 0, nil, viewer, "")
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
		in.Threshold = 12
	}
	if in.Threshold > 32 {
		in.Threshold = 32
	}
}

func mediaSimilarResultCacheKey(scope *mediaSimilarScope, source *mediaSimilarSource, threshold int) string {
	updatedAt := strings.TrimSpace(source.UpdatedAt)
	return fmt.Sprintf(
		"youban_publish:media_similar:v2:%s:%d:%d:%s",
		scope.CacheKey,
		source.Id,
		threshold,
		mediaSimilarHashKey(firstNonEmpty(source.PerceptualHash, "-")+":"+updatedAt),
	)
}

func mediaSimilarCountCacheKey(scope *mediaSimilarScope, source *mediaSimilarSource, threshold int) string {
	return "youban_publish:media_similar:count:" + mediaSimilarResultCacheKey(scope, source, threshold)
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
