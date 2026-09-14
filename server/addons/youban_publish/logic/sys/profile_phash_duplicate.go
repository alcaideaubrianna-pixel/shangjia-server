package sys

import (
	"context"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	// Candidate recall stays within the indexed LSH range. The wider threshold is
	// only applied to the small candidate set during the final one-to-one match.
	collectProfilePHashCandidateThreshold = 12
	collectProfilePHashDuplicateThreshold = 20
)

type profilePHashSetRow struct {
	ProfileId      int64  `orm:"profile_id"`
	PerceptualHash string `orm:"perceptual_hash"`
}

// findCollectProfilePHashDuplicate uses the existing pHash bucket/LSH index
// for recall and only accepts a candidate when its complete display image set
// can be matched one-to-one with the incoming set.
func (s *sSysPublish) findCollectProfilePHashDuplicate(ctx context.Context, tenantId, accountId int64, channelIds []int64, media []collectMediaItem) (int64, error) {
	sourceHashes := collectDisplayPHashes(media)
	if len(sourceHashes) == 0 {
		return 0, nil
	}
	scopes := []mediaPHashBucketScopePart{{TenantId: tenantId, AccountIds: []int64{accountId}}}
	candidateIds := make(map[int64]struct{})
	for _, value := range sourceHashes {
		rows, err := mediaPHashBucketCandidateRowsWithScopes(ctx, value, collectProfilePHashCandidateThreshold, scopes, nil, "image", 0)
		if err != nil {
			return 0, err
		}
		for _, row := range rows {
			candidateIds[row.ProfileId] = struct{}{}
		}
	}
	if len(candidateIds) == 0 {
		return 0, nil
	}
	ids := make([]int64, 0, len(candidateIds))
	for id := range candidateIds {
		ids = append(ids, id)
	}
	if len(channelIds) > 0 {
		scopedIds, err := duplicateProfileIdsInChannels(ctx, ids, channelIds)
		if err != nil {
			return 0, err
		}
		ids = uniqueIds(scopedIds)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	rows, err := duplicateProfilePHashRows(ctx, ids)
	if err != nil {
		return 0, err
	}
	byProfile := make(map[int64][]string, len(ids))
	for _, row := range rows {
		byProfile[row.ProfileId] = append(byProfile[row.ProfileId], strings.ToLower(strings.TrimSpace(row.PerceptualHash)))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	for _, id := range ids {
		if profilePHashSetsMatch(sourceHashes, byProfile[id], collectProfilePHashDuplicateThreshold) {
			return id, nil
		}
	}
	return 0, nil
}

func duplicateProfileIdsInChannels(ctx context.Context, profileIds, channelIds []int64) ([]int64, error) {
	profileIds, channelIds = uniqueIds(profileIds), uniqueIds(channelIds)
	result := make([]int64, 0)
	for start := 0; start < len(profileIds); start += duplicateScanChunkSize {
		end := start + duplicateScanChunkSize
		if end > len(profileIds) {
			end = len(profileIds)
		}
		var rows []int64
		if err := g.DB().Model(publishProfileFingerprintTable).Safe().Ctx(ctx).
			Fields("profile_id").WhereIn("profile_id", profileIds[start:end]).WhereIn("channel_id", channelIds).
			Where("owner_marker", "owner").Group("profile_id").Scan(&rows); err != nil {
			return nil, gerror.Wrap(err, "校验相似资料频道范围失败")
		}
		result = append(result, rows...)
	}
	return uniqueIds(result), nil
}

func duplicateProfilePHashRows(ctx context.Context, profileIds []int64) ([]profilePHashSetRow, error) {
	profileIds = uniqueIds(profileIds)
	result := make([]profilePHashSetRow, 0)
	for start := 0; start < len(profileIds); start += duplicateScanChunkSize {
		end := start + duplicateScanChunkSize
		if end > len(profileIds) {
			end = len(profileIds)
		}
		var rows []profilePHashSetRow
		if err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Fields("profile_id,perceptual_hash").WhereIn("profile_id", profileIds[start:end]).
			WhereNull("deleted_at").WhereIn("media_type", []string{"image", "photo"}).
			Where("purpose IS NULL OR purpose='' OR purpose='display'").
			WhereNot("perceptual_hash", "").OrderAsc("profile_id").OrderAsc("sort_index").OrderAsc("id").Scan(&rows); err != nil {
			return nil, gerror.Wrap(err, "读取相似资料图片指纹失败")
		}
		result = append(result, rows...)
	}
	return result, nil
}

func collectDisplayPHashes(media []collectMediaItem) []string {
	result := make([]string, 0, len(media))
	for _, item := range media {
		if item.Type != "image" && item.Type != "photo" {
			continue
		}
		if item.Purpose != "" && item.Purpose != "display" {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(item.FilePhash))
		if _, ok := parseUploadPHash(value); ok {
			result = append(result, value)
		}
	}
	return result
}

func profilePHashSetsMatch(left, right []string, threshold int) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	edges := make([][]int, len(left))
	for i, leftValue := range left {
		leftHash, ok := parseUploadPHash(leftValue)
		if !ok {
			return false
		}
		for j, rightValue := range right {
			rightHash, valid := parseUploadPHash(rightValue)
			if !valid {
				continue
			}
			distance, err := leftHash.Distance(rightHash)
			if err == nil && distance <= threshold {
				edges[i] = append(edges[i], j)
			}
		}
		if len(edges[i]) == 0 {
			return false
		}
	}
	matched := make([]int, len(right))
	for i := range matched {
		matched[i] = -1
	}
	var assign func(int, []bool) bool
	assign = func(leftIndex int, seen []bool) bool {
		for _, rightIndex := range edges[leftIndex] {
			if seen[rightIndex] {
				continue
			}
			seen[rightIndex] = true
			if matched[rightIndex] == -1 || assign(matched[rightIndex], seen) {
				matched[rightIndex] = leftIndex
				return true
			}
		}
		return false
	}
	for i := range left {
		if !assign(i, make([]bool, len(right))) {
			return false
		}
	}
	return true
}
