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
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

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

func (s *sSysPublish) findSimilarProfileMatchesByPHashBucket(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, accountIds []int64) ([]publishProfilePHashDistance, int, error) {
	accountIds = uniqueIds(accountIds)
	if len(accountIds) == 0 && in.AccountId > 0 {
		accountIds = []int64{in.AccountId}
	}
	candidateProfileIds, err := s.profileImageSearchCandidateProfileIds(ctx, &in.ProfileListInp, accountIds)
	if err != nil {
		return nil, 0, err
	}
	if candidateProfileIds != nil && len(candidateProfileIds) == 0 {
		return []publishProfilePHashDistance{}, 0, nil
	}
	items, err := s.cachedProfilePHashSearchCandidates(ctx, queryHash, in, accountIds, candidateProfileIds)
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

func (s *sSysPublish) cachedProfilePHashSearchCandidates(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	return s.cachedProfileFingerprintSearchCandidates(ctx, &mediaFingerprint{PHash: queryHash}, in, accountIds, candidateProfileIds)
}

func (s *sSysPublish) cachedProfileFingerprintSearchCandidates(ctx context.Context, fingerprint *mediaFingerprint, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	if fingerprint == nil {
		return []publishProfilePHashDistance{}, nil
	}
	queryHash := fingerprint.PHash
	if queryHash == nil {
		return []publishProfilePHashDistance{}, nil
	}
	cacheKey := mediaPHashSearchCacheKey(ctx, queryHash.GetHash(), fingerprint.MD5, in, accountIds, candidateProfileIds)
	if value, err := cache.Instance().Get(ctx, cacheKey); err == nil && !value.IsNil() {
		var cached []publishProfilePHashDistance
		if scanErr := value.Scan(&cached); scanErr == nil {
			return cached, nil
		}
	}
	items, err := s.profileFingerprintSearchCandidatesExactFirst(ctx, fingerprint, in, accountIds, candidateProfileIds)
	if err != nil {
		return nil, err
	}
	_ = cache.Instance().Set(ctx, cacheKey, items, mediaPHashBucketResultTTL)
	return items, nil
}

func (s *sSysPublish) findSimilarProfileMatchesByFingerprint(ctx context.Context, fingerprint *mediaFingerprint, in *sysin.ProfileImageSearchInp, accountIds []int64) ([]publishProfilePHashDistance, int, error) {
	items, err := s.cachedProfileFingerprintSearchCandidates(ctx, fingerprint, in, accountIds, nil)
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
func (s *sSysPublish) profilePHashSearchCandidatesExactFirst(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	return s.profileFingerprintSearchCandidatesExactFirst(ctx, &mediaFingerprint{PHash: queryHash}, in, accountIds, candidateProfileIds)
}

func (s *sSysPublish) profileFingerprintSearchCandidatesExactFirst(ctx context.Context, fingerprint *mediaFingerprint, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	if fingerprint == nil || fingerprint.PHash == nil {
		return []publishProfilePHashDistance{}, nil
	}
	if fingerprint.MD5 != "" {
		exact, err := mediaMD5CandidateMatches(ctx, fingerprint.MD5, in, accountIds, candidateProfileIds)
		if err != nil {
			return nil, err
		}
		if len(exact) > 0 {
			return exact, nil
		}
	}
	queryHash := fingerprint.PHash
	exactInput := *in
	exactInput.Threshold = 0
	exact, err := s.profilePHashSearchCandidates(ctx, queryHash, &exactInput, accountIds, candidateProfileIds)
	if err != nil {
		return nil, err
	}
	if len(exact) > 0 {
		return exact, nil
	}
	return s.profilePHashSearchCandidates(ctx, queryHash, in, accountIds, candidateProfileIds)
}

func mediaMD5CandidateMatches(ctx context.Context, md5Value string, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) ([]publishProfilePHashDistance, error) {
	md5Value = strings.TrimSpace(strings.ToLower(md5Value))
	if md5Value == "" {
		return []publishProfilePHashDistance{}, nil
	}
	mod := g.DB().Model(publishMediaTable+" m").Safe().Ctx(ctx).
		Fields("m.id AS media_id,m.profile_id,m.media_type").
		Where("m.md5", md5Value).
		Where("m.media_type IN ('image','video')").
		WhereNull("m.deleted_at")
	if scopeSQL, scopeArgs := mediaPHashBucketScopeSQL("m", []mediaPHashBucketScopePart{{TenantId: in.TenantId, AccountIds: accountIds}}); scopeSQL != "" {
		mod = mod.Where("("+scopeSQL+")", scopeArgs...)
	}
	if len(candidateProfileIds) > 0 {
		mod = mod.WhereIn("m.profile_id", uniqueIds(candidateProfileIds))
	}
	mod = mod.Where("EXISTS (SELECT 1 FROM hg_content_profile p WHERE p.id=m.profile_id AND p.deleted_at IS NULL)")
	mod = mod.Where("EXISTS (SELECT 1 FROM hg_youban_publish_profile_state ps WHERE ps.profile_id=m.profile_id AND ps.account_id=m.account_id AND ps.tenant_id=m.tenant_id AND ps.deleted_at IS NULL)")
	rows := make([]mediaPHashBucketCandidateRow, 0)
	if err := mod.Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "查询媒体MD5精确匹配失败")
	}
	items := make([]publishProfilePHashDistance, 0, len(rows))
	for _, row := range rows {
		items = append(items, publishProfilePHashDistance{ProfileId: row.ProfileId, Distance: 0, MediaId: row.MediaId, MediaType: row.MediaType})
	}
	return mediaPHashDeduplicateProfiles(items), nil
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
		items = append(items, publishProfilePHashDistance{
			ProfileId: row.ProfileId,
			Distance:  distance,
			MediaId:   row.MediaId,
			MediaType: row.MediaType,
		})
	}
	return mediaPHashDeduplicateProfiles(items), nil
}

func (s *sSysPublish) mediaPHashBucketCandidateRows(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) ([]mediaPHashBucketCandidateRow, error) {
	return mediaPHashBucketCandidateRows(ctx, mediaPHashNormalizedValue(queryHash), in.Threshold, in.TenantId, accountIds, candidateProfileIds, "image", 0)
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

func mediaPHashSearchCacheKey(ctx context.Context, queryHash uint64, md5Value string, in *sysin.ProfileImageSearchInp, accountIds []int64, candidateProfileIds []int64) string {
	parts := []string{
		fmt.Sprintf("youban_publish:profile_image_search:v6:%d", queryHash),
		"md5=" + strings.TrimSpace(strings.ToLower(md5Value)),
		fmt.Sprintf("tenant=%d", in.TenantId),
		fmt.Sprintf("account=%d", in.AccountId),
		fmt.Sprintf("threshold=%d", in.Threshold),
		fmt.Sprintf("version=%s", mediaPHashBucketVersion(ctx, in.TenantId, accountIds)),
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
