package sys

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	collectProfilePHashCandidateThreshold = 12
	collectProfilePHashDuplicateThreshold = 20
	mediaSimilarityLSHCandidateThreshold  = 12
	mediaSimilarityLSHMatchThreshold      = 20
)

// mediaSimilarityMatch is the single result shape shared by collection
// dedupe, Bot image search, and administrative similarity queries.
type mediaSimilarityMatch struct {
	ProfileID       int64
	MatchedCount    int
	BestDistance    int
	MatchedMediaIDs []int64
}

// findMediaSimilarityMatches performs LSH recall, scope filtering, and
// one-to-one perceptual matching in one place. Callers decide what a match
// means for their business flow.
func findMediaSimilarityMatches(ctx context.Context, tenantID, accountID int64, channelIDs []int64, media []collectMediaItem) ([]mediaSimilarityMatch, error) {
	sourceHashes := collectDisplayPHashes(media)
	if len(sourceHashes) == 0 {
		return nil, nil
	}
	scopes := []mediaPHashBucketScopePart{{TenantId: tenantID, AccountIds: []int64{accountID}}}
	candidateIDs := make(map[int64]struct{})
	for _, hash := range sourceHashes {
		rows, err := mediaPHashBucketCandidateRowsWithScopes(ctx, hash, mediaSimilarityCandidateThreshold(ctx), scopes, nil, "image", 0)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			candidateIDs[row.ProfileId] = struct{}{}
		}
	}
	if len(candidateIDs) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(candidateIDs))
	for id := range candidateIDs {
		ids = append(ids, id)
	}
	if len(channelIDs) > 0 {
		var err error
		ids, err = duplicateProfileIdsInChannels(ctx, ids, channelIDs)
		if err != nil {
			return nil, err
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := duplicateProfilePHashRows(ctx, ids)
	if err != nil {
		return nil, err
	}
	byProfile := make(map[int64][]string, len(ids))
	for _, row := range rows {
		hash := strings.ToLower(strings.TrimSpace(row.PerceptualHash))
		if hash != "" {
			byProfile[row.ProfileId] = append(byProfile[row.ProfileId], hash)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	result := make([]mediaSimilarityMatch, 0, len(ids))
	for _, profileID := range ids {
		matched := profilePHashMatchCount(sourceHashes, byProfile[profileID], mediaSimilarityMatchThreshold(ctx))
		if matched == 0 {
			continue
		}
		result = append(result, mediaSimilarityMatch{ProfileID: profileID, MatchedCount: matched, BestDistance: mediaSimilarityLSHMatchThreshold})
	}
	return result, nil
}

func mediaSimilarityCandidateThreshold(ctx context.Context) int {
	value := mediaSimilarityEnvInt(ctx, "YOUBAN_MEDIA_LSH_CANDIDATE_THRESHOLD", mediaSimilarityLSHCandidateThreshold)
	if value < 0 {
		return 0
	}
	if value > 12 {
		return 12
	}
	return value
}

func mediaSimilarityMatchThreshold(ctx context.Context) int {
	value := mediaSimilarityEnvInt(ctx, "YOUBAN_MEDIA_LSH_MATCH_THRESHOLD", mediaSimilarityLSHMatchThreshold)
	if value < 0 {
		return 0
	}
	if value > 64 {
		return 64
	}
	return value
}

func mediaSimilarityEnvInt(ctx context.Context, name string, fallback int) int {
	_ = ctx
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func firstMediaSimilarityMatch(matches []mediaSimilarityMatch, minimumMatchedCount int) (mediaSimilarityMatch, bool) {
	for _, match := range matches {
		if match.ProfileID > 0 && match.MatchedCount >= minimumMatchedCount {
			return match, true
		}
	}
	return mediaSimilarityMatch{}, false
}
