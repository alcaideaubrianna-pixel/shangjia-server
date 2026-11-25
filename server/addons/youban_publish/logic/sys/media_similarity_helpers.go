package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type profilePHashSetRow struct {
	ProfileId      int64  `orm:"profile_id"`
	PerceptualHash string `orm:"perceptual_hash"`
}

// Shared helpers for the unified media similarity service.
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
	return profilePHashMatchCount(left, right, threshold) == len(left)
}

func profilePHashMatchCount(left, right []string, threshold int) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	edges := make([][]int, len(left))
	for i, leftValue := range left {
		leftHash, ok := parseUploadPHash(leftValue)
		if !ok {
			return 0
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
			continue
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
	matchCount := 0
	for i := range left {
		if assign(i, make([]bool, len(right))) {
			matchCount++
		}
	}
	return matchCount
}
