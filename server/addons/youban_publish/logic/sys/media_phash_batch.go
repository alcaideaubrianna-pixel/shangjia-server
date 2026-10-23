package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/utility/scantrace"
)

// Bound SQL inputs and per-image output. Large albums use sequential chunks
// under the caller's single deadline, not more DB workers. Intermediate work
// still depends on the permission-scoped corpus and is measured separately.
const mediaPHashBatchSize = 8

// The union of bucket probes is scanned once. Each retrieved row is then
// matched back to its own input's bucket and exact distance. Unioning probes
// alone would inflate bucket_hits and change per-image candidate limits.
func mediaPHashBatchSQL(hashes []string, threshold int, scopes []mediaPHashBucketScopePart, scanScope bool) (string, []any, error) {
	if len(hashes) == 0 || len(mediaPHashBucketValidScopes(scopes)) == 0 {
		return "", nil, nil
	}
	if len(hashes) > mediaPHashBatchSize || threshold < 0 || threshold >= 16 {
		return "", nil, gerror.New("批量指纹查询参数不合法")
	}
	scopes = mediaPHashBucketValidScopes(scopes)
	inputs := make([]string, 0, len(hashes))
	distanceFilters := make([]string, 0, len(hashes))
	distanceArgs := make([]any, 0, 2*len(hashes))
	args := make([]any, 0)
	values := make([]map[int]bool, mediaPHashLshBlockCount)
	for i, hash := range hashes {
		if _, ok := parseUploadPHash(hash); !ok {
			return "", nil, gerror.New("批量指纹格式不合法")
		}
		inputs = append(inputs, "(?::integer,('x'||?)::bit(64))")
		args = append(args, i, hash)
		distanceFilters = append(distanceFilters, "bit_count((('x'||source.hash_value)::bit(64)) # (('x'||?)::bit(64))) <= ?")
		distanceArgs = append(distanceArgs, hash, threshold)
		if !scanScope {
			for _, cell := range mediaPHashLshCells(hash, threshold) {
				if values[cell.Pos-1] == nil {
					values[cell.Pos-1] = make(map[int]bool)
				}
				values[cell.Pos-1][cell.Value] = true
			}
		}
	}
	branches := make([]string, 0, 4*len(scopes))
	for pos := 1; pos <= mediaPHashLshBlockCount; pos++ {
		if scanScope && pos > 1 {
			break
		}
		probes := make([]int, 0, len(values[pos-1]))
		for value := range values[pos-1] {
			probes = append(probes, value)
		}
		sort.Ints(probes)
		for _, scope := range scopes {
			branch, branchArgs := mediaPHashLshBranchWithProbesSQL(pos, probes, []mediaPHashBucketScopePart{scope}, nil, "image", 0, true)
			// Reject distant hashes while reading each index branch, before the
			// materialized union multiplies them by every album input. Keep all
			// buckets of a matching hash so bucket_hits and recall stay unchanged.
			// Bind constant query hashes directly. A correlated scan of the
			// input CTE here repeats tuple iteration for every index row.
			branch = "SELECT source.* FROM (" + branch + ") source WHERE (" + strings.Join(distanceFilters, " OR ") + ")"
			branches = append(branches, fmt.Sprintf("SELECT source.*,%d AS bucket_pos FROM (%s) source", pos, branch))
			args = append(args, branchArgs...)
			args = append(args, distanceArgs...)
		}
	}
	bucketCondition := "TRUE"
	if !scanScope {
		bucketCondition = fmt.Sprintf("bit_count(substring(b.hash_bits from (b.bucket_pos-1)*16+1 for 16) # substring(q.hash_bits from (b.bucket_pos-1)*16+1 for 16)) <= %d", threshold/4)
	}
	query := fmt.Sprintf(`WITH input_hash(input_index,hash_bits) AS (VALUES %s),
bucket_match AS MATERIALIZED (
 SELECT source.*,('x'||hash_value)::bit(64) AS hash_bits FROM (%s) source
), candidate AS MATERIALIZED (
 SELECT q.input_index,b.media_id,b.profile_id,b.account_id,b.tenant_id,
        b.media_type,b.hash_value,COUNT(*) AS bucket_hits
 FROM bucket_match b JOIN input_hash q ON %s
 WHERE bit_count(b.hash_bits # q.hash_bits) <= ?
 GROUP BY q.input_index,b.media_id,b.profile_id,b.account_id,b.tenant_id,b.media_type,b.hash_value
), live_media AS MATERIALIZED (
 SELECT DISTINCT media_id,profile_id,account_id,tenant_id FROM candidate
), valid_media AS MATERIALIZED (
 SELECT v.* FROM live_media v
 WHERE EXISTS (SELECT 1 FROM hg_youban_publish_profile_state ps
   WHERE ps.profile_id=v.profile_id AND ps.tenant_id=v.tenant_id
     AND ps.account_id=v.account_id AND ps.deleted_at IS NULL)
 AND EXISTS (SELECT 1 FROM hg_youban_publish_media m
   WHERE m.id=v.media_id AND m.profile_id=v.profile_id AND m.tenant_id=v.tenant_id
     AND m.account_id=v.account_id AND m.deleted_at IS NULL
     AND m.perceptual_hash IS NOT NULL AND m.perceptual_hash<>'')
 AND %s
), ranked AS (
 SELECT c.*,row_number() OVER (PARTITION BY c.input_index ORDER BY c.bucket_hits DESC,c.media_id,c.hash_value) AS ordinal
 FROM candidate c JOIN valid_media v USING(media_id,profile_id,account_id,tenant_id)
)
SELECT input_index,media_id,profile_id,account_id,tenant_id,media_type,hash_value,bucket_hits
FROM ranked WHERE ordinal<=%d ORDER BY input_index,ordinal`,
		strings.Join(inputs, ","), strings.Join(branches, " UNION ALL "), bucketCondition,
		mediaSimilarLiveProfileIndexExistsSQL("v.profile_id", "v.tenant_id", "v.account_id"), mediaPHashBucketMaxCandidates)
	return query, append(args, threshold), nil
}

func mediaPHashBatchRows(ctx context.Context, hashes []string, threshold int, scopes []mediaPHashBucketScopePart) (rows []mediaPHashBucketCandidateRow, err error) {
	scanScope, err := mediaPHashLshScanSmallScope(ctx, scopes, nil, "image", 0)
	if err != nil {
		return nil, err
	}
	query, args, err := mediaPHashBatchSQL(hashes, threshold, scopes, scanScope)
	if err != nil || query == "" {
		return nil, err
	}
	finish := scantrace.Begin(scantrace.With(ctx, g.Map{"inputItems": len(hashes), "scopeScan": scanScope}), "query.phash_batch")
	defer func() { finish(err) }()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("SET LOCAL work_mem = '64MB'"); err != nil {
			return err
		}
		if _, err := tx.Exec("SET LOCAL jit = off"); err != nil {
			return err
		}
		return tx.Raw(query, args...).Scan(&rows)
	})
	if err != nil {
		return nil, gerror.Wrap(err, "批量查询pHash候选失败")
	}
	return rows, nil
}
