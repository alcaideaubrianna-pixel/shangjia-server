package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"golang.org/x/sync/singleflight"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
	"hotgo/utility/scantrace"
)

const (
	mediaPHashLshReadyKey       = "youban_publish:media_phash_lsh:ready"
	mediaPHashLshReadyTTL       = 365 * 24 * time.Hour
	mediaPHashLshBlockCount     = 4
	mediaPHashLshBlockBits      = 16
	mediaPHashLshMaxQueryRadius = 3
	mediaPHashLshScopeScanLimit = 100000
)

type mediaPHashLshCell struct {
	Pos   int
	Value int
}

var mediaPHashLshScopeSizeGroup singleflight.Group

func mediaPHashLshReady(ctx context.Context) bool {
	value, err := cache.Instance().Get(ctx, mediaPHashLshReadyKey)
	return err == nil && !value.IsNil() && value.Int() == 1
}

func markMediaPHashLshReady(ctx context.Context) error {
	return cache.Instance().Set(ctx, mediaPHashLshReadyKey, 1, mediaPHashLshReadyTTL)
}

func mediaPHashLshCells(hash string, threshold int) []mediaPHashLshCell {
	value, ok := parseUploadPHash(hash)
	if !ok || threshold < 0 || threshold >= mediaPHashLshBlockCount*(mediaPHashLshMaxQueryRadius+1) {
		return nil
	}
	radius := threshold / mediaPHashLshBlockCount
	if radius > mediaPHashLshMaxQueryRadius {
		radius = mediaPHashLshMaxQueryRadius
	}
	cells := make([]mediaPHashLshCell, 0, mediaPHashLshBlockCount*700)
	for pos := 0; pos < mediaPHashLshBlockCount; pos++ {
		block := uint16(value.GetHash() >> uint((mediaPHashLshBlockCount-pos-1)*mediaPHashLshBlockBits))
		for _, item := range mediaPHashLshNeighborhood(block, radius) {
			cells = append(cells, mediaPHashLshCell{Pos: pos + 1, Value: int(item)})
		}
	}
	return cells
}

func mediaPHashLshNeighborhood(value uint16, radius int) []uint16 {
	if radius <= 0 {
		return []uint16{value}
	}
	result := []uint16{value}
	for bit := 0; bit < 16; bit++ {
		result = append(result, value^(1<<bit))
	}
	if radius < 2 {
		return result
	}
	for first := 0; first < 16; first++ {
		for second := first + 1; second < 16; second++ {
			result = append(result, value^(1<<first)^(1<<second))
		}
	}
	if radius < 3 {
		return result
	}
	for first := 0; first < 16; first++ {
		for second := first + 1; second < 16; second++ {
			for third := second + 1; third < 16; third++ {
				result = append(result, value^(1<<first)^(1<<second)^(1<<third))
			}
		}
	}
	return result
}

func mediaPHashLshBucketValues(hash string) []mediaPHashLshCell {
	value, ok := parseUploadPHash(hash)
	if !ok {
		return nil
	}
	cells := make([]mediaPHashLshCell, 0, mediaPHashLshBlockCount)
	for pos := 0; pos < mediaPHashLshBlockCount; pos++ {
		block := uint16(value.GetHash() >> uint((mediaPHashLshBlockCount-pos-1)*mediaPHashLshBlockBits))
		cells = append(cells, mediaPHashLshCell{Pos: pos + 1, Value: int(block)})
	}
	return cells
}

func mediaPHashLshCandidateRowsWithScopes(ctx context.Context, normalizedHash string, threshold int, scopes []mediaPHashBucketScopePart, profileIds []int64, mediaType string, excludeProfileId int64) ([]mediaPHashBucketCandidateRow, error) {
	scanScope, err := mediaPHashLshScanSmallScope(ctx, scopes, profileIds, mediaType, excludeProfileId)
	if err != nil {
		return nil, err
	}
	query, args, err := mediaPHashLshCandidateSQLWithStrategy(normalizedHash, threshold, scopes, profileIds, mediaType, excludeProfileId, scanScope)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return []mediaPHashBucketCandidateRow{}, nil
	}
	rows := make([]mediaPHashBucketCandidateRow, 0)
	finish := scantrace.Begin(scantrace.With(ctx, g.Map{"scopeScan": scanScope}), "query.phash_candidates")
	defer func() { finish(err) }()
	if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
			if _, err = tx.Exec("SET LOCAL work_mem = '64MB'"); err != nil {
				return gerror.Wrap(err, "设置pHash LSH查询内存失败")
			}
			if _, err = tx.Exec("SET LOCAL jit = off"); err != nil {
				return gerror.Wrap(err, "关闭pHash LSH查询JIT失败")
			}
		}
		if err = tx.Raw(query, args...).Scan(&rows); err != nil {
			return gerror.Wrap(err, "查询pHash LSH候选失败")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

// Only the query strategy is cached: estimates cannot add or remove matches.
// EXPLAIN (without ANALYZE) consults planner statistics without scanning media.
// Counting even a bounded scope first can consume the entire cold-query budget.
func mediaPHashLshScanSmallScope(ctx context.Context, scopes []mediaPHashBucketScopePart, profileIds []int64, mediaType string, excludeProfileId int64) (bool, error) {
	if len(mediaPHashBucketValidScopes(scopes)) == 0 {
		return false, nil
	}
	branch, args := mediaPHashLshBranchSQL(1, nil, scopes, profileIds, mediaType, excludeProfileId)
	key := publishconsts.MediaPHashScopeSizeKeyPrefix + mediaPHashHashKey(fmt.Sprint(scopes, profileIds, mediaType, excludeProfileId))
	result, err, _ := mediaPHashLshScopeSizeGroup.Do(key, func() (interface{}, error) {
		value, err := cache.Instance().GetOrSetFunc(ctx, key, func(ctx context.Context) (interface{}, error) {
			finish := scantrace.Begin(ctx, "query.phash_scope_estimate")
			value, err := g.DB().GetValue(ctx, "EXPLAIN (FORMAT JSON) "+branch, args...)
			finish(err)
			if err != nil {
				return nil, gerror.Wrap(err, "估算扫图指纹查询范围失败")
			}
			var plans []struct {
				Plan struct {
					Rows int64 `json:"Plan Rows"`
				} `json:"Plan"`
			}
			if err := json.Unmarshal(value.Bytes(), &plans); err != nil || len(plans) != 1 {
				return nil, gerror.New("解析扫图指纹查询范围失败")
			}
			return plans[0].Plan.Rows <= mediaPHashLshScopeScanLimit, nil
		}, 30*time.Second)
		if err != nil {
			return nil, err
		}
		return value.Bool(), nil
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func mediaPHashLshCandidateSQL(normalizedHash string, threshold int, scopes []mediaPHashBucketScopePart, profileIds []int64, mediaType string, excludeProfileId int64) (string, []any, error) {
	return mediaPHashLshCandidateSQLWithStrategy(normalizedHash, threshold, scopes, profileIds, mediaType, excludeProfileId, false)
}

func mediaPHashLshCandidateSQLWithStrategy(normalizedHash string, threshold int, scopes []mediaPHashBucketScopePart, profileIds []int64, mediaType string, excludeProfileId int64, scanScope bool) (string, []any, error) {
	cells := mediaPHashLshCells(normalizedHash, threshold)
	if len(cells) == 0 {
		return "", nil, gerror.New("pHash LSH 查询参数无效")
	}
	scopes = mediaPHashBucketValidScopes(scopes)
	profileIds = uniqueIds(profileIds)
	if len(scopes) == 0 && len(profileIds) == 0 {
		return "", nil, nil
	}
	branchCapacity := mediaPHashLshBlockCount
	if len(scopes) > 0 {
		branchCapacity *= len(scopes)
	}
	branches := make([]string, 0, branchCapacity)
	args := make([]any, 0, len(cells)+mediaPHashLshBlockCount*8)
	for pos := 1; pos <= mediaPHashLshBlockCount; pos++ {
		if scanScope && pos > 1 {
			break
		}
		values := make([]int, 0)
		for _, cell := range cells {
			if !scanScope && cell.Pos == pos {
				values = append(values, cell.Value)
			}
		}
		if len(scopes) == 0 {
			branch, branchArgs := mediaPHashLshBranchSQL(pos, values, nil, profileIds, mediaType, excludeProfileId)
			branches = append(branches, branch)
			args = append(args, branchArgs...)
			continue
		}
		for _, scope := range scopes {
			branch, branchArgs := mediaPHashLshBranchSQL(pos, values, []mediaPHashBucketScopePart{scope}, profileIds, mediaType, excludeProfileId)
			branches = append(branches, branch)
			args = append(args, branchArgs...)
		}
	}
	args = append(args, normalizedHash, threshold)
	query := fmt.Sprintf(`
WITH bucket_match AS (
%s
), candidate AS (
SELECT media_id, profile_id, account_id, tenant_id, media_type,
       hash_value, COUNT(*) AS bucket_hits
FROM bucket_match
-- Apply the exact distance before expensive live-media and permission joins.
-- The same threshold is checked again by Go after candidate retrieval.
WHERE bit_count((('x' || hash_value)::bit(64)) # (('x' || ?)::bit(64))) <= ?
GROUP BY media_id, profile_id, account_id, tenant_id, media_type, hash_value
)
SELECT candidate.media_id, candidate.profile_id, candidate.account_id,
       candidate.tenant_id, candidate.media_type, candidate.hash_value,
       candidate.bucket_hits
FROM candidate
WHERE EXISTS (
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
AND %s
ORDER BY candidate.bucket_hits DESC
LIMIT %d`, strings.Join(branches, " UNION ALL "), mediaSimilarLiveProfileIndexExistsSQL("candidate.profile_id", "candidate.tenant_id", "candidate.account_id"), mediaPHashBucketMaxCandidates)
	return query, args, nil
}

func mediaPHashLshBranchSQL(pos int, values []int, scopes []mediaPHashBucketScopePart, profileIds []int64, mediaType string, excludeProfileId int64) (string, []any) {
	conds := []string{"b.bucket_pos = ?"}
	args := []any{pos}
	if len(values) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
		conds = append(conds, "b.bucket_value IN ("+placeholders+")")
		for _, value := range values {
			args = append(args, value)
		}
	}
	if mediaType != "" {
		condition, conditionArgs := mediaPHashBucketMediaTypeCondition("b.media_type", mediaType)
		conds = append(conds, condition)
		args = append(args, conditionArgs...)
	}
	if scopeSQL, scopeArgs := mediaPHashBucketScopeSQL("b", scopes); scopeSQL != "" {
		conds = append(conds, "("+scopeSQL+")")
		args = append(args, scopeArgs...)
	}
	if len(profileIds) > 0 {
		ids := uniqueIds(profileIds)
		idsSQL := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		conds = append(conds, "b.profile_id IN ("+idsSQL+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if excludeProfileId > 0 {
		conds = append(conds, "b.profile_id <> ?")
		args = append(args, excludeProfileId)
	}
	condition := strings.Join(conds, " AND ")
	selectSQL := "SELECT b.media_id,b.profile_id,b.account_id,b.tenant_id,b.media_type,b.hash_value FROM %s b WHERE %s"
	query := fmt.Sprintf(selectSQL, publishMediaPHashLshTable, condition) + " UNION ALL " +
		fmt.Sprintf(selectSQL, publishMediaPHashAliasBucketTable, condition)
	return query, append(args, args...)
}

func mediaPHashLshRowsForMedia(media gdb.Record, hash string, now *gtime.Time) []g.Map {
	rows := make([]g.Map, 0, mediaPHashLshBlockCount)
	for _, cell := range mediaPHashLshBucketValues(hash) {
		rows = append(rows, g.Map{
			"tenant_id": media["tenant_id"].Int64(), "account_id": media["account_id"].Int64(),
			"profile_id": media["profile_id"].Int64(), "media_id": media["id"].Int64(),
			"media_type": strings.TrimSpace(media["media_type"].String()),
			"hash_value": hash, "bucket_pos": cell.Pos, "bucket_value": cell.Value,
			"created_at": now, "updated_at": now,
		})
	}
	return rows
}
