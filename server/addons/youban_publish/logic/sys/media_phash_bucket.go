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
	mediaPHashBucketCacheVersionKey = "youban_publish:media_phash_bucket:version:v2"
	mediaPHashBucketResultTTL       = 10 * time.Minute
	mediaPHashBucketMaxCandidates   = 8000
	mediaPHashBucketMaxScopedIds    = 32
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
	owners, err := mediaPHashBucketOwners(ctx, "media_id", mediaId)
	if err != nil {
		return err
	}
	_, err = g.DB().Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).Where("media_id", mediaId).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除媒体哈希索引失败")
	}
	return bumpMediaPHashBucketVersions(ctx, owners)
}

func (s *sSysPublish) replaceMediaPHashBucketByMediaRow(ctx context.Context, media gdb.Record) error {
	mediaId := media["id"].Int64()
	hash := strings.TrimSpace(media["perceptual_hash"].String())
	buckets := mediaPHashBucketValues(hash)
	if mediaId <= 0 || len(buckets) == 0 {
		return s.deleteMediaPHashBucketByMediaId(ctx, mediaId)
	}
	now := gtime.Now()
	if err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
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
	}); err != nil {
		return err
	}
	return bumpMediaPHashBucketVersion(ctx, media["tenant_id"].Int64(), media["account_id"].Int64())
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

type mediaPHashBucketScopePart struct {
	TenantId   int64
	AccountIds []int64
}

type mediaPHashBucketSource struct {
	Id        int64
	MediaType string
	HashValue string
}

func mediaPHashBucketVersion(ctx context.Context, tenantId int64, accountIds []int64) string {
	ids := uniqueIds(accountIds)
	if tenantId <= 0 || len(ids) == 0 || len(ids) > mediaPHashBucketMaxScopedIds {
		return mediaPHashBucketVersionValue(ctx, mediaPHashBucketTenantVersionKey(tenantId))
	}
	parts := make([]string, 0, len(ids))
	for _, accountId := range ids {
		parts = append(parts, fmt.Sprintf("%d=%s", accountId, mediaPHashBucketVersionValue(ctx, mediaPHashBucketAccountVersionKey(tenantId, accountId))))
	}
	return mediaPHashHashKey(strings.Join(parts, ","))
}

func mediaPHashBucketVersionValue(ctx context.Context, key string) string {
	value, err := cache.Instance().Get(ctx, key)
	if err != nil || value.IsNil() {
		return "0"
	}
	return value.String()
}

func mediaPHashBucketTenantVersionKey(tenantId int64) string {
	return fmt.Sprintf("%s:tenant:%d", mediaPHashBucketCacheVersionKey, tenantId)
}

func mediaPHashBucketAccountVersionKey(tenantId int64, accountId int64) string {
	return fmt.Sprintf("%s:account:%d:%d", mediaPHashBucketCacheVersionKey, tenantId, accountId)
}

func bumpMediaPHashBucketVersion(ctx context.Context, tenantId int64, accountId int64) error {
	version := gtime.Now().UnixNano()
	if tenantId <= 0 {
		return nil
	}
	if err := cache.Instance().Set(ctx, mediaPHashBucketTenantVersionKey(tenantId), version, 24*time.Hour); err != nil {
		return err
	}
	if accountId > 0 {
		return cache.Instance().Set(ctx, mediaPHashBucketAccountVersionKey(tenantId, accountId), version, 24*time.Hour)
	}
	return nil
}

func (s *sSysPublish) deleteMediaPHashBucketByProfileId(ctx context.Context, profileId int64) error {
	if profileId <= 0 {
		return nil
	}
	owners, err := mediaPHashBucketOwners(ctx, "profile_id", profileId)
	if err != nil {
		return err
	}
	_, err = g.DB().Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).Where("profile_id", profileId).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除资料哈希索引失败")
	}
	return bumpMediaPHashBucketVersions(ctx, owners)
}

func (s *sSysPublish) deleteMediaPHashBucketByTaskId(ctx context.Context, taskId int64) error {
	if taskId <= 0 {
		return nil
	}
	owners, err := mediaPHashBucketOwners(ctx, "task_id", taskId)
	if err != nil {
		return err
	}
	_, err = g.DB().Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).Where("task_id", taskId).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除任务哈希索引失败")
	}
	return bumpMediaPHashBucketVersions(ctx, owners)
}

type mediaPHashBucketOwner struct {
	TenantId  int64 `orm:"tenant_id"`
	AccountId int64 `orm:"account_id"`
}

func mediaPHashBucketOwners(ctx context.Context, field string, value int64) ([]mediaPHashBucketOwner, error) {
	if field != "media_id" && field != "profile_id" && field != "task_id" {
		return nil, gerror.New("媒体哈希索引字段不合法")
	}
	owners := make([]mediaPHashBucketOwner, 0)
	if err := g.DB().Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).
		Fields("tenant_id,account_id").
		Where(field, value).
		Group("tenant_id,account_id").
		Scan(&owners); err != nil {
		return nil, gerror.Wrap(err, "读取媒体哈希索引归属失败")
	}
	return owners, nil
}

func bumpMediaPHashBucketVersions(ctx context.Context, owners []mediaPHashBucketOwner) error {
	for _, owner := range owners {
		if err := bumpMediaPHashBucketVersion(ctx, owner.TenantId, owner.AccountId); err != nil {
			return err
		}
	}
	return nil
}

func mediaPHashBucketCandidateRows(ctx context.Context, normalizedHash string, threshold int, tenantId int64, accountIds []int64, profileIds []int64, mediaType string, excludeProfileId int64) ([]mediaPHashBucketCandidateRow, error) {
	return mediaPHashBucketCandidateRowsWithScopes(ctx, normalizedHash, threshold, []mediaPHashBucketScopePart{{TenantId: tenantId, AccountIds: accountIds}}, profileIds, mediaType, excludeProfileId)
}

func mediaPHashBucketCandidateRowsWithScopes(ctx context.Context, normalizedHash string, threshold int, scopes []mediaPHashBucketScopePart, profileIds []int64, mediaType string, excludeProfileId int64) ([]mediaPHashBucketCandidateRow, error) {
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
		branch, branchArgs := mediaPHashBucketBranchSQL(i+1, string(item), scopes, profileIds, mediaType, excludeProfileId)
		branches = append(branches, branch)
		args = append(args, branchArgs...)
	}
	rows := make([]mediaPHashBucketCandidateRow, 0)
	sql := fmt.Sprintf(`
WITH bucket_match AS (
%s
), candidate AS (
SELECT media_id, profile_id, account_id, tenant_id, media_type, hash_value, COUNT(*) AS bucket_hits
FROM bucket_match
GROUP BY media_id, profile_id, account_id, tenant_id, media_type, hash_value
HAVING COUNT(*) >= ?
)
SELECT candidate.media_id, candidate.profile_id, candidate.account_id,
       candidate.tenant_id, candidate.media_type, candidate.hash_value,
       candidate.bucket_hits
FROM candidate
WHERE EXISTS (
    SELECT 1
    FROM hg_content_profile p
    WHERE p.id = candidate.profile_id AND p.deleted_at IS NULL
)
AND EXISTS (
    SELECT 1
    FROM hg_youban_publish_task t
    WHERE t.profile_id = candidate.profile_id
      AND t.tenant_id = candidate.tenant_id
      AND t.account_id = candidate.account_id
      AND t.deleted_at IS NULL
)
ORDER BY candidate.bucket_hits DESC
LIMIT %d
`, strings.Join(branches, " UNION ALL "), mediaPHashBucketMaxCandidates)
	args = append(args, minEqualNibbles)
	if err := g.DB().Raw(sql, args...).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "查询相似媒体分桶失败")
	}
	return rows, nil
}

func mediaPHashBucketBatchCandidateRows(ctx context.Context, sources []mediaPHashBucketSource, threshold int, scopes []mediaPHashBucketScopePart) (map[int64][]mediaPHashBucketCandidateRow, error) {
	result := make(map[int64][]mediaPHashBucketCandidateRow)
	if len(sources) == 0 || len(scopes) == 0 {
		return result, nil
	}
	values := make([]string, 0, len(sources)*16)
	args := make([]any, 0, len(sources)*64)
	for _, source := range sources {
		for _, bucket := range mediaPHashBucketValues(source.HashValue) {
			values = append(values, "(CAST(? AS bigint), CAST(? AS text), CAST(? AS integer), CAST(? AS text))")
			args = append(args, source.Id, source.MediaType, bucket.Pos, bucket.Value)
		}
	}
	if len(values) == 0 {
		return result, nil
	}
	scopeSQL, scopeArgs := mediaPHashBucketScopeSQL("b", scopes)
	args = append(args, scopeArgs...)
	minEqualNibbles := mediaPHashMinEqualNibbles(threshold)
	if minEqualNibbles <= 0 {
		minEqualNibbles = 1
	}
	args = append(args, minEqualNibbles)
	sql := fmt.Sprintf(`
WITH source_bucket(source_media_id, media_type, bucket_pos, bucket_value) AS (
    VALUES %s
), bucket_match AS (
    SELECT sb.source_media_id, b.media_id, b.profile_id, b.account_id,
           b.tenant_id, b.media_type, b.hash_value, COUNT(*) AS bucket_hits
    FROM source_bucket sb
    JOIN %s b ON b.media_type = sb.media_type
        AND b.bucket_pos = sb.bucket_pos
        AND b.bucket_value = sb.bucket_value
    WHERE %s
    GROUP BY sb.source_media_id, b.media_id, b.profile_id, b.account_id,
             b.tenant_id, b.media_type, b.hash_value
    HAVING COUNT(*) >= ?
), ranked AS (
    SELECT bucket_match.*,
           ROW_NUMBER() OVER (PARTITION BY source_media_id ORDER BY bucket_hits DESC) AS row_number
    FROM bucket_match
)
SELECT source_media_id, media_id, profile_id, account_id, tenant_id,
       media_type, hash_value, bucket_hits
FROM ranked
WHERE row_number <= %d
AND EXISTS (
    SELECT 1 FROM hg_content_profile p
    WHERE p.id = ranked.profile_id AND p.deleted_at IS NULL
)
AND EXISTS (
    SELECT 1 FROM hg_youban_publish_task t
    WHERE t.profile_id = ranked.profile_id
      AND t.tenant_id = ranked.tenant_id
      AND t.account_id = ranked.account_id
      AND t.deleted_at IS NULL
)
ORDER BY source_media_id, bucket_hits DESC
`, strings.Join(values, ","), publishMediaPHashBucketTable, scopeSQL, mediaPHashBucketMaxCandidates)
	var rows []struct {
		SourceMediaId int64  `json:"sourceMediaId"`
		MediaId       int64  `json:"mediaId" orm:"media_id"`
		ProfileId     int64  `json:"profileId" orm:"profile_id"`
		AccountId     int64  `json:"accountId" orm:"account_id"`
		TenantId      int64  `json:"tenantId" orm:"tenant_id"`
		MediaType     string `json:"mediaType" orm:"media_type"`
		HashValue     string `json:"hashValue" orm:"hash_value"`
		BucketHits    int    `json:"bucketHits" orm:"bucket_hits"`
	}
	if err := g.DB().Raw(sql, args...).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "批量查询相似媒体分桶失败")
	}
	for _, row := range rows {
		if row.SourceMediaId > 0 {
			result[row.SourceMediaId] = append(result[row.SourceMediaId], mediaPHashBucketCandidateRow{
				MediaId: row.MediaId, ProfileId: row.ProfileId, AccountId: row.AccountId,
				TenantId: row.TenantId, MediaType: row.MediaType, HashValue: row.HashValue,
				BucketHits: row.BucketHits,
			})
		}
	}
	return result, nil
}

func mediaPHashBucketBranchSQL(bucketPos int, bucketValue string, scopes []mediaPHashBucketScopePart, profileIds []int64, mediaType string, excludeProfileId int64) (string, []any) {
	conds := []string{
		"b.bucket_pos = ?",
		"b.bucket_value = ?",
	}
	args := []any{bucketPos, bucketValue}
	if mediaType != "" {
		conds = append(conds, "b.media_type = ?")
		args = append(args, mediaType)
	}
	if scopeSQL, scopeArgs := mediaPHashBucketScopeSQL("b", scopes); scopeSQL != "" {
		conds = append(conds, "("+scopeSQL+")")
		args = append(args, scopeArgs...)
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

func mediaPHashBucketScopeSQL(alias string, scopes []mediaPHashBucketScopePart) (string, []any) {
	conditions := make([]string, 0, len(scopes))
	args := make([]any, 0)
	for _, scope := range scopes {
		if len(scope.AccountIds) == 0 {
			continue
		}
		ids := uniqueIds(scope.AccountIds)
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		fieldPrefix := alias
		if fieldPrefix != "" {
			fieldPrefix += "."
		}
		condition := fieldPrefix + "account_id IN (" + placeholders + ")"
		if scope.TenantId > 0 {
			condition = fieldPrefix + "tenant_id = ? AND " + condition
			args = append(args, scope.TenantId)
		}
		conditions = append(conditions, condition)
		for _, id := range ids {
			args = append(args, id)
		}
	}
	return strings.Join(conditions, " OR "), args
}
