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

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/frame/g"

	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/utility/scantrace"
)

type mediaPHashBucketCandidateRow struct {
	InputIndex int    `orm:"input_index"`
	AccountId  int64  `orm:"account_id"`
	BucketHits int    `orm:"bucket_hits"`
	HashValue  string `orm:"hash_value"`
	MediaId    int64  `orm:"media_id"`
	MediaType  string `orm:"media_type"`
	ProfileId  int64  `orm:"profile_id"`
	TenantId   int64  `orm:"tenant_id"`
}

func (s *sSysPublish) findSimilarProfileMatchesByPHashBucket(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, scope *publishmodel.MediaSearchScope) ([]publishProfilePHashDistance, int, error) {
	candidateProfileIds, err := s.profileImageSearchCandidateProfileIds(ctx, &in.ProfileListInp, scope)
	if err != nil {
		return nil, 0, err
	}
	if candidateProfileIds != nil && len(candidateProfileIds) == 0 {
		return []publishProfilePHashDistance{}, 0, nil
	}
	items, err := s.cachedProfilePHashSearchCandidates(ctx, queryHash, in, scope, candidateProfileIds)
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Distance == items[j].Distance {
			return items[i].ProfileId > items[j].ProfileId
		}
		return items[i].Distance < items[j].Distance
	})
	totalCount := len(items)
	if totalCount == 0 {
		return []publishProfilePHashDistance{}, 0, nil
	}
	start := (in.Page - 1) * in.PerPage
	if start < 0 {
		start = 0
	}
	if start >= totalCount {
		return []publishProfilePHashDistance{}, totalCount, nil
	}
	end := int(math.Min(float64(start+in.PerPage), float64(totalCount)))
	return items[start:end], totalCount, nil
}

func (s *sSysPublish) cachedProfilePHashSearchCandidates(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, scope *publishmodel.MediaSearchScope, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	return s.cachedProfileFingerprintSearchCandidates(ctx, &mediaFingerprint{PHash: queryHash}, in, scope, candidateProfileIds)
}

func (s *sSysPublish) cachedProfileFingerprintSearchCandidates(ctx context.Context, fingerprint *mediaFingerprint, in *sysin.ProfileImageSearchInp, scope *publishmodel.MediaSearchScope, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	return s.cachedProfileFingerprintCandidates(ctx, fingerprint, in, scope, candidateProfileIds, false)
}

func (s *sSysPublish) cachedProfileFingerprintCandidates(ctx context.Context, fingerprint *mediaFingerprint, in *sysin.ProfileImageSearchInp, scope *publishmodel.MediaSearchScope, candidateProfileIds []int64, exactAlreadyChecked bool) ([]publishProfilePHashDistance, error) {
	if fingerprint == nil || scope == nil || len(scope.Partitions) == 0 {
		return []publishProfilePHashDistance{}, nil
	}
	queryHash := fingerprint.PHash
	if queryHash == nil {
		return []publishProfilePHashDistance{}, nil
	}
	cacheKey := mediaPHashSearchCacheKey(ctx, queryHash.GetHash(), fingerprint.MD5, in, scope, candidateProfileIds)
	finish := scantrace.Begin(ctx, "query.cache_get")
	value, cacheErr := cache.Instance().Get(ctx, cacheKey)
	finish(cacheErr)
	if cacheErr == nil && !value.IsNil() {
		var cached []publishProfilePHashDistance
		if scanErr := value.Scan(&cached); scanErr == nil {
			return cached, nil
		}
	}
	var items []publishProfilePHashDistance
	var err error
	if exactAlreadyChecked {
		finish := scantrace.Begin(ctx, "query.phash_approx")
		items, err = s.profilePHashSearchCandidates(ctx, queryHash, in, scope, candidateProfileIds)
		finish(err)
	} else {
		items, err = s.profileFingerprintSearchCandidatesExactFirst(ctx, fingerprint, in, scope, candidateProfileIds)
	}
	if err != nil {
		return nil, err
	}
	finish = scantrace.Begin(ctx, "query.cache_set")
	cacheErr = cache.Instance().Set(ctx, cacheKey, items, mediaPHashBucketResultTTL)
	finish(cacheErr)
	return items, nil
}

func (s *sSysPublish) findSimilarProfileMatchesByFingerprint(ctx context.Context, fingerprint *mediaFingerprint, in *sysin.ProfileImageSearchInp, scope *publishmodel.MediaSearchScope) ([]publishProfilePHashDistance, int, error) {
	items, err := s.cachedProfileFingerprintSearchCandidates(ctx, fingerprint, in, scope, nil)
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Distance == items[j].Distance {
			return items[i].ProfileId > items[j].ProfileId
		}
		return items[i].Distance < items[j].Distance
	})
	totalCount := len(items)
	start := (in.Page - 1) * in.PerPage
	if start < 0 {
		start = 0
	}
	if start >= totalCount {
		return []publishProfilePHashDistance{}, totalCount, nil
	}
	end := int(math.Min(float64(start+in.PerPage), float64(totalCount)))
	return items[start:end], totalCount, nil
}

// profilePHashSearchCandidatesExactFirst follows the same precision-first
// strategy as the source search service: approximate matches are only useful
// when the permission-scoped exact pHash search has no result.
func (s *sSysPublish) profilePHashSearchCandidatesExactFirst(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, scope *publishmodel.MediaSearchScope, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	return s.profileFingerprintSearchCandidatesExactFirst(ctx, &mediaFingerprint{PHash: queryHash}, in, scope, candidateProfileIds)
}

func (s *sSysPublish) profileFingerprintSearchCandidatesExactFirst(ctx context.Context, fingerprint *mediaFingerprint, in *sysin.ProfileImageSearchInp, scope *publishmodel.MediaSearchScope, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	if fingerprint == nil || fingerprint.PHash == nil {
		return []publishProfilePHashDistance{}, nil
	}
	startedAt := time.Now()
	finish := scantrace.Begin(ctx, "query.phash_lsh")
	items, err := s.profilePHashSearchCandidates(ctx, fingerprint.PHash, in, scope, candidateProfileIds)
	finish(err)
	observeBotMediaSearchStage(ctx, "phash_lsh", startedAt, err)
	return items, err
}

// profilePHashSearchCandidates is the single media similarity query path.
// LSH performs the recall and the Go-side distance check keeps the result
// threshold exact without maintaining a second exact-match index.
func (s *sSysPublish) profilePHashSearchCandidates(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, scope *publishmodel.MediaSearchScope, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	if queryHash == nil || in == nil || scope == nil || len(scope.Partitions) == 0 {
		return []publishProfilePHashDistance{}, nil
	}
	rows, err := mediaPHashBucketCandidateRowsWithScopes(ctx, mediaPHashNormalizedValue(queryHash), in.Threshold, scope.Partitions, candidateProfileIds, "image", 0)
	if err != nil {
		return nil, err
	}
	items := make([]publishProfilePHashDistance, 0, len(rows))
	for _, row := range rows {
		hash, ok := parseUploadPHash(row.HashValue)
		if !ok {
			continue
		}
		distance, distanceErr := queryHash.Distance(hash)
		if distanceErr != nil || distance > in.Threshold {
			continue
		}
		items = append(items, publishProfilePHashDistance{ProfileId: row.ProfileId, Distance: distance, MediaId: row.MediaId, MediaType: row.MediaType})
	}
	return mediaPHashDeduplicateProfiles(items), nil
}

func isPostgresDatabase() bool {
	dbType := strings.ToLower(g.DB().GetConfig().Type)
	return dbType == "pgsql" || dbType == "postgres"
}

func isVectorPHashEnabled(ctx context.Context) bool {
	return false
}

func mediaPHashDeduplicateProfiles(items []publishProfilePHashDistance) []publishProfilePHashDistance {
	matchByProfile := map[int64]publishProfilePHashDistance{}
	for _, item := range items {
		current, exists := matchByProfile[item.ProfileId]
		if !exists || item.Distance < current.Distance ||
			(item.Distance == current.Distance && item.MediaId < current.MediaId) {
			matchByProfile[item.ProfileId] = item
		}
	}
	res := make([]publishProfilePHashDistance, 0, len(matchByProfile))
	for _, item := range matchByProfile {
		res = append(res, item)
	}
	return res
}

func mediaPHashNormalizedValue(queryHash *goimagehash.ImageHash) string {
	if queryHash == nil {
		return ""
	}
	return fmt.Sprintf("%016x", queryHash.GetHash())
}

func mediaPHashMinEqualNibbles(threshold int) int {
	if threshold <= 0 {
		threshold = 12
	}
	if threshold >= 16 {
		return 0
	}
	return 16 - threshold
}

func mediaPHashSearchCacheKey(ctx context.Context, queryHash uint64, md5Value string, in *sysin.ProfileImageSearchInp, scope *publishmodel.MediaSearchScope, candidateProfileIds []int64) string {
	return mediaPHashSearchCacheKeyWithScopeVersion(queryHash, md5Value, in, candidateProfileIds, mediaSearchScopeCacheKey(scope), mediaSearchScopeVersion(ctx, scope))
}

func mediaPHashSearchCacheKeyWithScopeVersion(queryHash uint64, md5Value string, in *sysin.ProfileImageSearchInp, candidateProfileIds []int64, scopeKey, version string) string {
	parts := []string{
		fmt.Sprintf("youban_publish:profile_image_search:v9:%d", queryHash),
		"md5=" + strings.TrimSpace(strings.ToLower(md5Value)),
		fmt.Sprintf("threshold=%d", in.Threshold),
		fmt.Sprintf("scope=%s", scopeKey),
		fmt.Sprintf("version=%s", version),
	}
	if len(candidateProfileIds) > 0 {
		ids := uniqueIds(candidateProfileIds)
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		parts = append(parts, fmt.Sprintf("profiles=%v", ids))
	}
	if strings.TrimSpace(in.Keyword) != "" {
		parts = append(parts, "keyword="+strings.TrimSpace(in.Keyword))
	}
	if strings.TrimSpace(in.Province) != "" {
		parts = append(parts, "province="+strings.TrimSpace(in.Province))
	}
	if strings.TrimSpace(in.City) != "" {
		parts = append(parts, "city="+strings.TrimSpace(in.City))
	}
	if strings.TrimSpace(in.Tag) != "" {
		parts = append(parts, "tag="+strings.TrimSpace(in.Tag))
	}
	if strings.TrimSpace(in.ReviewStatus) != "" {
		parts = append(parts, "reviewStatus="+strings.TrimSpace(in.ReviewStatus))
	}
	if strings.TrimSpace(in.Visibility) != "" {
		parts = append(parts, "visibility="+strings.TrimSpace(in.Visibility))
	}
	if in.Status > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", in.Status))
	}
	return mediaPHashHashKey(strings.Join(parts, "|"))
}

func mediaPHashHashKey(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
