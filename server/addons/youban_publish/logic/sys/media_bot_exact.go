package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/utility/scantrace"
)

// Resolve all exact matches before starting any approximate work. A slow first
// image must not prevent later album members from reaching their indexed lookup.
func botMediaExactCandidates(ctx context.Context, fingerprints []*mediaFingerprint, scope *publishmodel.MediaSearchScope) ([][]*botMediaSearchCandidate, error) {
	result := make([][]*botMediaSearchCandidate, len(fingerprints))
	if len(fingerprints) == 0 || scope == nil || len(scope.Partitions) == 0 {
		return result, nil
	}
	md5s, hashes := map[string][]int{}, map[string][]int{}
	for i, f := range fingerprints {
		if f == nil {
			continue
		}
		if key := strings.ToLower(strings.TrimSpace(f.MD5)); key != "" {
			md5s[key] = append(md5s[key], i)
		}
		if f.PHash != nil {
			key := fmt.Sprintf("%016x", f.PHash.GetHash())
			hashes[key] = append(hashes[key], i)
		}
	}
	query := func(table, alias, keyColumn, keyExpression string, keys map[string][]int, extra string) error {
		if len(keys) == 0 {
			return nil
		}
		values := make([]string, 0, len(keys))
		for key := range keys {
			values = append(values, key)
		}
		scopeSQL, args := mediaPHashBucketScopeSQL(alias, scope.Partitions)
		if scopeSQL == "" {
			return nil
		}
		mod := g.DB().Model(table+" "+alias).Safe().Ctx(ctx).
			Fields(keyExpression+" AS fingerprint_key,"+alias+".profile_id").
			WhereIn(keyColumn, values).Where("("+scopeSQL+")", args...).Where(extra).
			Where("EXISTS (SELECT 1 FROM hg_content_profile p WHERE p.id=" + alias + ".profile_id AND p.deleted_at IS NULL)").
			Where("EXISTS (SELECT 1 FROM hg_youban_publish_profile_state ps WHERE ps.profile_id=" + alias + ".profile_id AND ps.tenant_id=" + alias + ".tenant_id AND ps.account_id=" + alias + ".account_id AND ps.deleted_at IS NULL)")
		finish := scantrace.Begin(ctx, "query.exact_batch."+alias)
		rows, err := mod.Group(keyExpression + "," + alias + ".profile_id").All()
		finish(err)
		if err != nil {
			return gerror.Wrap(err, "批量查询媒体精确匹配失败")
		}
		for _, row := range rows {
			for _, index := range keys[row["fingerprint_key"].String()] {
				result[index] = append(result[index], &botMediaSearchCandidate{ProfileId: row["profile_id"].Int64(), MatchCount: 1})
			}
		}
		return nil
	}
	if err := query(publishMediaTable, "m", "m.md5", "m.md5", md5s, "m.deleted_at IS NULL AND m.media_type IN ('image','video')"); err != nil {
		return result, err
	}
	// Preserve MD5 precedence: a byte-exact match must not be widened by pHash.
	for key, indices := range hashes {
		remaining := make([]int, 0, len(indices))
		for _, i := range indices {
			if len(result[i]) == 0 {
				remaining = append(remaining, i)
			}
		}
		if len(remaining) == 0 {
			delete(hashes, key)
		} else {
			hashes[key] = remaining
		}
	}
	if len(hashes) == 0 {
		return result, nil
	}
	// Use bit values for the indexed column; formatting only belongs in SELECT.
	bitKeys := make(map[string][]int, len(hashes))
	for key, indices := range hashes {
		var value uint64
		_, _ = fmt.Sscanf(key, "%x", &value)
		bitKeys[fmt.Sprintf("%064b", value)] = indices
	}
	if err := query("hg_youban_publish_media_fingerprint", "f", "f.phash_bits", "f.phash_bits::text", bitKeys, "f.deleted_at IS NULL AND f.media_type='image'"); err != nil {
		return result, err
	}
	// The alias table/index are deployment migrations. Never perform DDL on
	// the latency-sensitive search path.
	if err := query(publishMediaPHashAliasBucketTable, "a", "a.hash_value", "a.hash_value", hashes, "a.media_type='image'"); err != nil {
		return result, err
	}
	return result, nil
}
