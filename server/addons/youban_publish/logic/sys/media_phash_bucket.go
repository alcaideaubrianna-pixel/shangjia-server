package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/library/cache"
)

const (
	mediaPHashBucketCacheVersionKey = "youban_publish:media_phash_bucket:version"
	mediaPHashBucketResultTTL       = 10 * time.Minute
	mediaPHashBucketMaxCandidates   = 8000
)

func (s *sSysPublish) syncMediaPHashBucketByMediaId(ctx context.Context, mediaId int64) error {
	if mediaId <= 0 {
		return nil
	}
	row, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,profile_id,task_id,media_type,perceptual_hash,deleted_at").
		Where("id", mediaId).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取媒体哈希失败")
	}
	if row.IsEmpty() || !row["deleted_at"].IsNil() || strings.TrimSpace(row["perceptual_hash"].String()) == "" {
		return s.deleteMediaPHashBucketByMediaId(ctx, mediaId)
	}
	return s.replaceMediaPHashBucketByMediaRow(ctx, row)
}

func (s *sSysPublish) SyncMediaPHashBucketByMediaId(ctx context.Context, mediaId int64) error {
	return s.syncMediaPHashBucketByMediaId(ctx, mediaId)
}

func (s *sSysPublish) deleteMediaPHashBucketByMediaId(ctx context.Context, mediaId int64) error {
	if mediaId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).Where("media_id", mediaId).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除媒体哈希索引失败")
	}
	_ = bumpMediaPHashBucketVersion(ctx)
	return nil
}

func (s *sSysPublish) replaceMediaPHashBucketByMediaRow(ctx context.Context, media gdb.Record) error {
	mediaId := media["id"].Int64()
	hash := strings.TrimSpace(media["perceptual_hash"].String())
	buckets := mediaPHashBucketValues(hash)
	if mediaId <= 0 || len(buckets) == 0 {
		return s.deleteMediaPHashBucketByMediaId(ctx, mediaId)
	}
	now := gtime.Now()
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).Where("media_id", mediaId).Delete(); err != nil {
			return gerror.Wrap(err, "清理媒体哈希索引失败")
		}
		rows := make([]g.Map, 0, len(buckets))
		for _, bucket := range buckets {
			rows = append(rows, g.Map{
				"tenant_id":    media["tenant_id"].Int64(),
				"account_id":   media["account_id"].Int64(),
				"profile_id":   media["profile_id"].Int64(),
				"media_id":     mediaId,
				"task_id":      media["task_id"].Int64(),
				"media_type":   strings.TrimSpace(media["media_type"].String()),
				"hash_value":   hash,
				"bucket_pos":   bucket.Pos,
				"bucket_value": bucket.Value,
				"created_at":   now,
				"updated_at":   now,
			})
		}
		for _, row := range rows {
			if _, err := tx.Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).Data(row).Insert(); err != nil {
				return gerror.Wrap(err, "写入媒体哈希索引失败")
			}
		}
		return nil
	})
}

func mediaPHashBucketValues(hash string) []mediaPHashBucketCell {
	value, ok := parseUploadPHash(hash)
	if !ok {
		return nil
	}
	normalized := fmt.Sprintf("%016x", value.GetHash())
	items := make([]mediaPHashBucketCell, 0, len(normalized))
	for i := 0; i < len(normalized); i++ {
		items = append(items, mediaPHashBucketCell{Pos: i + 1, Value: string(normalized[i])})
	}
	return items
}

type mediaPHashBucketCell struct {
	Pos   int
	Value string
}

func mediaPHashBucketVersion(ctx context.Context) string {
	value, err := cache.Instance().Get(ctx, mediaPHashBucketCacheVersionKey)
	if err != nil || value.IsNil() {
		return "0"
	}
	return value.String()
}

func bumpMediaPHashBucketVersion(ctx context.Context) error {
	return cache.Instance().Set(ctx, mediaPHashBucketCacheVersionKey, gtime.Now().UnixNano(), 24*time.Hour)
}

func (s *sSysPublish) deleteMediaPHashBucketByProfileId(ctx context.Context, profileId int64) error {
	if profileId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).Where("profile_id", profileId).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除资料哈希索引失败")
	}
	_ = bumpMediaPHashBucketVersion(ctx)
	return nil
}

func (s *sSysPublish) deleteMediaPHashBucketByTaskId(ctx context.Context, taskId int64) error {
	if taskId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).Where("task_id", taskId).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除任务哈希索引失败")
	}
	_ = bumpMediaPHashBucketVersion(ctx)
	return nil
}

func mediaPHashBucketCandidateRows(ctx context.Context, normalizedHash string, threshold int, tenantId int64, accountIds []int64, profileIds []int64, mediaType string, excludeProfileId int64) ([]mediaPHashBucketCandidateRow, error) {
	normalizedHash = strings.TrimSpace(strings.ToLower(normalizedHash))
	if normalizedHash == "" {
		return []mediaPHashBucketCandidateRow{}, nil
	}
	minEqualNibbles := mediaPHashMinEqualNibbles(threshold)
	if minEqualNibbles <= 0 {
		minEqualNibbles = 1
	}
	branches := make([]string, 0, len(normalizedHash))
	args := make([]any, 0, len(normalizedHash)*8)
	for i, item := range normalizedHash {
		branch, branchArgs := mediaPHashBucketBranchSQL(i+1, string(item), tenantId, accountIds, profileIds, mediaType, excludeProfileId)
		branches = append(branches, branch)
		args = append(args, branchArgs...)
	}
	rows := make([]mediaPHashBucketCandidateRow, 0)
	sql := fmt.Sprintf(`
SELECT media_id, profile_id, account_id, tenant_id, media_type, hash_value, COUNT(*) AS bucket_hits
FROM (
%s
) AS bucket_match
GROUP BY media_id, profile_id, account_id, tenant_id, media_type, hash_value
HAVING COUNT(*) >= ?
ORDER BY bucket_hits DESC
LIMIT %d
`, strings.Join(branches, " UNION ALL "), mediaPHashBucketMaxCandidates)
	args = append(args, minEqualNibbles)
	if err := g.DB().Raw(sql, args...).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "查询相似媒体分桶失败")
	}
	return rows, nil
}

func mediaPHashBucketBranchSQL(bucketPos int, bucketValue string, tenantId int64, accountIds []int64, profileIds []int64, mediaType string, excludeProfileId int64) (string, []any) {
	conds := []string{
		"b.bucket_pos = ?",
		"b.bucket_value = ?",
		"EXISTS (SELECT 1 FROM hg_content_profile p WHERE p.id = b.profile_id AND p.deleted_at IS NULL)",
		"EXISTS (SELECT 1 FROM hg_youban_publish_task t WHERE t.profile_id = b.profile_id AND t.tenant_id = b.tenant_id AND t.account_id = b.account_id AND t.deleted_at IS NULL)",
	}
	args := []any{bucketPos, bucketValue}
	if tenantId > 0 {
		conds = append(conds, "b.tenant_id = ?")
		args = append(args, tenantId)
	}
	if mediaType != "" {
		conds = append(conds, "b.media_type = ?")
		args = append(args, mediaType)
	}
	if len(accountIds) > 0 {
		ids := uniqueIds(accountIds)
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		conds = append(conds, "b.account_id IN ("+placeholders+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if len(profileIds) > 0 {
		ids := uniqueIds(profileIds)
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		conds = append(conds, "b.profile_id IN ("+placeholders+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if excludeProfileId > 0 {
		conds = append(conds, "b.profile_id <> ?")
		args = append(args, excludeProfileId)
	}
	sql := fmt.Sprintf(
		`SELECT b.media_id, b.profile_id, b.account_id, b.tenant_id, b.media_type, b.hash_value FROM %s AS b WHERE %s`,
		publishMediaPHashBucketTable,
		strings.Join(conds, " AND "),
	)
	return sql, args
}
