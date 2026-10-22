package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/sync/singleflight"
	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/utility/scantrace"
)

var botMediaBatchCandidateGroup singleflight.Group

// Identical cold batches share DB work. A waiter can leave immediately when
// its own deadline expires; it must not wait unboundedly on another request.
func botMediaSharedBatchRows(ctx context.Context, key string, load func() ([]mediaPHashBucketCandidateRow, error)) ([]mediaPHashBucketCandidateRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	call := botMediaBatchCandidateGroup.DoChan(key, func() (any, error) { return load() })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-call:
		if result.Err != nil {
			return nil, result.Err
		}
		rows, ok := result.Val.([]mediaPHashBucketCandidateRow)
		if !ok {
			return nil, gerror.New("批量指纹查询结果不合法")
		}
		return rows, nil
	}
}

// Called only after the whole album's exact lookup. Cached and completed chunks
// survive a later error; the caller marks the response incomplete, never empty
// success. No failed or partial query is cached.
func (s *sSysPublish) botMediaApproximateBatch(ctx context.Context, fingerprints []*mediaFingerprint, in *sysin.ProfileImageSearchInp, scope *publishmodel.MediaSearchScope) ([][]*botMediaSearchCandidate, error) {
	if len(fingerprints) < 2 || !isPostgresDatabase() || isVectorPHashEnabled(ctx) ||
		!mediaPHashLshReady(ctx) || in.Threshold < 0 || in.Threshold >= 16 ||
		!g.Cfg().MustGet(ctx, "youbanPublish.mediaSearch.batchApproximateEnabled", true).Bool() {
		return loadBotMediaSearchCandidates(ctx, fingerprints, func(ctx context.Context, f *mediaFingerprint) ([]*botMediaSearchCandidate, error) {
			return s.botMediaFingerprintCandidates(ctx, f, in, scope, isPostgresDatabase())
		})
	}
	results := make([][]*botMediaSearchCandidate, len(fingerprints))
	if scope == nil || len(mediaPHashBucketValidScopes(scope.Partitions)) == 0 {
		return results, nil
	}
	indices := make([]int, 0, len(fingerprints))
	keys := make([]string, len(fingerprints))
	// Resolve the same permission/index version once for the album. Repeating
	// it per image also repeats the account-version cache reads.
	scopeKey, version := mediaSearchScopeCacheKey(scope), mediaSearchScopeVersion(ctx, scope)
	for i, f := range fingerprints {
		if f == nil || f.PHash == nil {
			continue
		}
		keys[i] = mediaPHashSearchCacheKeyWithScopeVersion(f.PHash.GetHash(), f.MD5, in, nil, scopeKey, version)
		if value, err := cache.Instance().Get(ctx, keys[i]); err == nil && !value.IsNil() {
			var matches []publishProfilePHashDistance
			if err := value.Scan(&matches); err == nil {
				results[i] = botMediaCandidatesFromMatches(matches)
				continue
			}
		}
		indices = append(indices, i)
	}
	scantrace.Record(ctx, "query.phash_batch.plan", g.Map{"inputItems": len(fingerprints), "queryItems": len(indices), "batchSize": mediaPHashBatchSize}, nil)
	for start := 0; start < len(indices); start += mediaPHashBatchSize {
		end := start + mediaPHashBatchSize
		if end > len(indices) {
			end = len(indices)
		}
		chunk := indices[start:end]
		hashes := make([]string, len(chunk))
		chunkKeys := make([]string, len(chunk))
		for i, index := range chunk {
			hashes[i] = mediaPHashNormalizedValue(fingerprints[index].PHash)
			chunkKeys[i] = keys[index]
		}
		rows, err := botMediaSharedBatchRows(ctx, mediaPHashHashKey(strings.Join(chunkKeys, "|")), func() ([]mediaPHashBucketCandidateRow, error) {
			return mediaPHashBatchRows(ctx, hashes, in.Threshold, scope.Partitions)
		})
		if err != nil {
			return results, err
		}
		matches := make([][]publishProfilePHashDistance, len(chunk))
		for _, row := range rows {
			if row.InputIndex < 0 || row.InputIndex >= len(chunk) {
				continue
			}
			hash, ok := parseUploadPHash(row.HashValue)
			if !ok {
				continue
			}
			distance, err := fingerprints[chunk[row.InputIndex]].PHash.Distance(hash)
			if err != nil || distance > in.Threshold {
				continue
			}
			matches[row.InputIndex] = append(matches[row.InputIndex], publishProfilePHashDistance{
				ProfileId: row.ProfileId, MediaId: row.MediaId, MediaType: row.MediaType, Distance: distance,
			})
		}
		for i, index := range chunk {
			items := mediaPHashDeduplicateProfiles(matches[i])
			results[index] = botMediaCandidatesFromMatches(items)
			_ = cache.Instance().Set(ctx, keys[index], items, mediaPHashBucketResultTTL)
		}
	}
	return results, nil
}
