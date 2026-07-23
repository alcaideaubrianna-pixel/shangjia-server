package sys

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/corona10/goimagehash"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

type mediaPHashBucketCandidateRow struct {
	AccountId  int64  `json:"accountId" orm:"account_id"`
	BucketHits int    `json:"bucketHits" orm:"bucket_hits"`
	HashValue  string `json:"hashValue" orm:"hash_value"`
	MediaId    int64  `json:"mediaId" orm:"media_id"`
	MediaType  string `json:"mediaType" orm:"media_type"`
	ProfileId  int64  `json:"profileId" orm:"profile_id"`
	TenantId   int64  `json:"tenantId" orm:"tenant_id"`
}

func (s *sSysPublish) findSimilarProfileIdsByPHashBucket(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, accountIds []int64) ([]int64, int, error) {
	candidateProfileIds, err := s.profileImageSearchCandidateProfileIds(ctx, &in.ProfileListInp, accountIds)
	if err != nil {
		return nil, 0, err
	}
	if candidateProfileIds != nil && len(candidateProfileIds) == 0 {
		return []int64{}, 0, nil
	}
	items, err := s.cachedProfilePHashSearchCandidates(ctx, queryHash, in, accountIds, candidateProfileIds)
	if err != nil {
		return nil, 0, err
	}
	items, err = s.filterVisibleProfilePHashItems(ctx, items, &in.ProfileListInp, accountIds)
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
		return []int64{}, 0, nil
	}
	start := (in.Page - 1) * in.PerPage
	if start < 0 {
		start = 0
	}
	if start >= totalCount {
		return []int64{}, totalCount, nil
	}
	end := int(math.Min(float64(start+in.PerPage), float64(totalCount)))
	profileIds := make([]int64, 0, end-start)
	for _, item := range items[start:end] {
		profileIds = append(profileIds, item.ProfileId)
	}
	return profileIds, totalCount, nil
}

func (s *sSysPublish) cachedProfilePHashSearchCandidates(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	if queryHash == nil {
		return []publishProfilePHashDistance{}, nil
	}
	cacheKey := mediaPHashSearchCacheKey(ctx, queryHash.GetHash(), in, accountIds, candidateProfileIds)
	if value, err := cache.Instance().Get(ctx, cacheKey); err == nil && !value.IsNil() {
		var cached []publishProfilePHashDistance
		if scanErr := value.Scan(&cached); scanErr == nil {
			return cached, nil
		}
	}
	items, err := s.profilePHashSearchCandidates(ctx, queryHash, in, accountIds, candidateProfileIds)
	if err != nil {
		return nil, err
	}
	_ = cache.Instance().Set(ctx, cacheKey, items, mediaPHashBucketResultTTL)
	return items, nil
}

func (s *sSysPublish) profilePHashSearchCandidates(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	rows, err := mediaPHashBucketCandidateRows(ctx, mediaPHashNormalizedValue(queryHash), in.Threshold, in.TenantId, accountIds, candidateProfileIds, "image", 0)
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
		items = append(items, publishProfilePHashDistance{ProfileId: row.ProfileId, Distance: distance})
	}
	return mediaPHashDeduplicateProfiles(items), nil
}

func (s *sSysPublish) mediaPHashBucketCandidateRows(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) ([]mediaPHashBucketCandidateRow, error) {
	return mediaPHashBucketCandidateRows(ctx, mediaPHashNormalizedValue(queryHash), in.Threshold, in.TenantId, accountIds, candidateProfileIds, "image", 0)
}

func mediaPHashDeduplicateProfiles(items []publishProfilePHashDistance) []publishProfilePHashDistance {
	distanceByProfile := map[int64]int{}
	for _, item := range items {
		current, exists := distanceByProfile[item.ProfileId]
		if !exists || item.Distance < current {
			distanceByProfile[item.ProfileId] = item.Distance
		}
	}
	res := make([]publishProfilePHashDistance, 0, len(distanceByProfile))
	for profileId, distance := range distanceByProfile {
		res = append(res, publishProfilePHashDistance{ProfileId: profileId, Distance: distance})
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

func mediaPHashSearchCacheKey(ctx context.Context, queryHash uint64, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) string {
	parts := []string{
		fmt.Sprintf("youban_publish:profile_image_search:v3:%d", queryHash),
		fmt.Sprintf("tenant=%d", in.TenantId),
		fmt.Sprintf("account=%d", in.AccountId),
		fmt.Sprintf("threshold=%d", in.Threshold),
		fmt.Sprintf("version=%s", mediaPHashBucketVersion(ctx)),
	}
	if len(accountIds) > 0 {
		parts = append(parts, fmt.Sprintf("accounts=%v", uniqueIds(accountIds)))
	}
	if len(candidateProfileIds) > 0 {
		parts = append(parts, fmt.Sprintf("profiles=%v", uniqueIds(candidateProfileIds)))
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
