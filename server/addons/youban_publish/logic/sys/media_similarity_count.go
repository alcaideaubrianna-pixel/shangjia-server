package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/sync/singleflight"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

const (
	mediaSimilarCountCacheTTL    = time.Hour
	mediaSimilarDefaultThreshold = 8
)

var mediaSimilarCountGroup singleflight.Group

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
	count, err := s.cachedMediaSimilarCount(ctx, scope, source, mediaSimilarDefaultThreshold)
	if err != nil {
		return nil, err
	}
	return &sysin.MediaSimilarCountModel{MediaId: in.MediaId, Count: count}, nil
}

func (s *sSysPublish) cachedMediaSimilarCount(ctx context.Context, scope *mediaSimilarScope, source *mediaSimilarSource, threshold int) (int, error) {
	if source == nil {
		return 0, nil
	}
	cacheKey := mediaSimilarCountCacheKey(ctx, scope, source, threshold)
	if value, err := cache.Instance().Get(ctx, cacheKey); err == nil && !value.IsNil() {
		return value.Int(), nil
	}
	result, err, _ := mediaSimilarCountGroup.Do(cacheKey, func() (any, error) {
		if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
			return value.Int(), nil
		}
		count, countErr := s.computeMediaSimilarCount(ctx, scope, source, threshold)
		if countErr != nil {
			return 0, countErr
		}
		_ = cache.Instance().Set(ctx, cacheKey, count, mediaSimilarCountCacheTTL)
		return count, nil
	})
	if err != nil {
		return 0, err
	}
	count, ok := result.(int)
	if !ok {
		return 0, gerror.New("解析相似媒体数量失败")
	}
	return count, nil
}

func (s *sSysPublish) computeMediaSimilarCount(ctx context.Context, scope *mediaSimilarScope, source *mediaSimilarSource, threshold int) (int, error) {
	if source == nil || scope == nil || len(scope.Partitions) == 0 {
		return 0, nil
	}
	if strings.EqualFold(g.DB().GetConfig().Type, "pgsql") && mediaPHashLshReady(ctx) && threshold <= 12 {
		count, err := mediaPHashLshProfileCountWithScopes(ctx, source.PerceptualHash, threshold, scope.Partitions, source.MediaType, source.ProfileId)
		if err == nil {
			return count, nil
		}
		g.Log().Warningf(ctx, "pHash LSH相似数量统计失败，回退候选计数 mediaId:%d err:%v", source.Id, err)
	}
	items, err := s.cachedMediaSimilarCandidates(ctx, scope, source, threshold)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

func mediaPHashLshProfileCountWithScopes(ctx context.Context, normalizedHash string, threshold int, scopes []mediaPHashBucketScopePart, mediaType string, excludeProfileId int64) (int, error) {
	query, args, err := mediaPHashLshProfileCountSQL(normalizedHash, threshold, scopes, mediaType, excludeProfileId)
	if err != nil {
		return 0, err
	}
	var row struct {
		Count int `json:"count" orm:"count"`
	}
	if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, execErr := tx.Exec("SET LOCAL work_mem = '" + mediaPHashCandidateWorkMem + "'"); execErr != nil {
			return gerror.Wrap(execErr, "设置相似媒体计数内存失败")
		}
		if _, execErr := tx.Exec("SET LOCAL jit = off"); execErr != nil {
			return gerror.Wrap(execErr, "关闭相似媒体计数JIT失败")
		}
		if scanErr := tx.Raw(query, args...).Scan(&row); scanErr != nil {
			return gerror.Wrap(scanErr, "统计相似媒体失败")
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return row.Count, nil
}

func mediaPHashLshProfileCountSQL(normalizedHash string, threshold int, scopes []mediaPHashBucketScopePart, mediaType string, excludeProfileId int64) (string, []any, error) {
	cells := mediaPHashLshCells(normalizedHash, threshold)
	if len(cells) == 0 {
		return "", nil, gerror.New("pHash LSH 计数参数无效")
	}
	branches := make([]string, 0, mediaPHashLshBlockCount*len(scopes))
	args := make([]any, 0, len(cells)+mediaPHashLshBlockCount*8+2)
	for pos := 1; pos <= mediaPHashLshBlockCount; pos++ {
		values := make([]int, 0)
		for _, cell := range cells {
			if cell.Pos == pos {
				values = append(values, cell.Value)
			}
		}
		for _, scope := range scopes {
			if len(scope.AccountIds) == 0 {
				continue
			}
			branch, branchArgs := mediaPHashLshBranchSQL(pos, values, []mediaPHashBucketScopePart{scope}, nil, mediaType, excludeProfileId)
			branches = append(branches, branch)
			args = append(args, branchArgs...)
		}
	}
	if len(branches) == 0 {
		return "", nil, gerror.New("pHash LSH 计数权限范围为空")
	}
	args = append(args, strings.ToLower(strings.TrimSpace(normalizedHash)), threshold)
	query := fmt.Sprintf(`
WITH bucket_match AS (
%s
), candidate AS (
    SELECT media_id, profile_id, account_id, tenant_id, MAX(hash_value) AS hash_value
    FROM bucket_match
    GROUP BY media_id, profile_id, account_id, tenant_id
)
SELECT COUNT(DISTINCT candidate.profile_id) AS count
FROM candidate
WHERE bit_count(
    (('x' || candidate.hash_value)::bit(64)) # (('x' || ?)::bit(64))
) <= ?
AND %s
AND EXISTS (
    SELECT 1 FROM hg_youban_publish_profile_state ps
    WHERE ps.profile_id = candidate.profile_id
      AND ps.tenant_id = candidate.tenant_id
      AND ps.account_id = candidate.account_id
      AND ps.deleted_at IS NULL
)
AND EXISTS (
    SELECT 1 FROM hg_youban_publish_media m
    WHERE m.id = candidate.media_id
      AND m.profile_id = candidate.profile_id
      AND m.tenant_id = candidate.tenant_id
      AND m.account_id = candidate.account_id
      AND m.deleted_at IS NULL
      AND m.perceptual_hash IS NOT NULL
      AND m.perceptual_hash <> ''
)
`, strings.Join(branches, " UNION ALL "), mediaSimilarLiveProfileIndexExistsSQL("candidate.profile_id", "candidate.tenant_id", "candidate.account_id"))
	return query, args, nil
}
