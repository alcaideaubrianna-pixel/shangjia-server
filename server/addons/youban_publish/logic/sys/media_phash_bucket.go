package sys

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

var mediaPHashBucketSchemaOnce sync.Once

func (s *sSysPublish) syncMediaPHashBucketByMediaId(ctx context.Context, mediaId int64) error {
	if mediaId <= 0 {
		return nil
	}
	if err := ensureMediaPHashBucketSchema(ctx); err != nil {
		return err
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
	if err := ensureMediaPHashBucketSchema(ctx); err != nil {
		return err
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
	if err := ensureMediaPHashBucketSchema(ctx); err != nil {
		return err
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

func ensureMediaPHashBucketSchema(ctx context.Context) error {
	var ensureErr error
	mediaPHashBucketSchemaOnce.Do(func() {
		switch strings.ToLower(g.DB().GetConfig().Type) {
		case "pgsql", "postgres", "postgresql":
			_, ensureErr = g.DB().Exec(ctx, `
CREATE TABLE IF NOT EXISTS "hg_youban_publish_media_phash_bucket" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "media_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT '',
  "hash_value" varchar(64) NOT NULL DEFAULT '',
  "bucket_pos" smallint NOT NULL DEFAULT 0,
  "bucket_value" varchar(1) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_media_phash_bucket_media_pos" ON "hg_youban_publish_media_phash_bucket" ("media_id", "bucket_pos");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_bucket_lookup" ON "hg_youban_publish_media_phash_bucket" ("tenant_id", "media_type", "bucket_pos", "bucket_value", "account_id", "profile_id", "task_id", "media_id");
`)
			if ensureErr == nil {
				_, ensureErr = g.DB().Exec(ctx, `ALTER TABLE "hg_youban_publish_media_phash_bucket" ADD COLUMN IF NOT EXISTS "task_id" bigint NOT NULL DEFAULT 0`)
			}
		default:
			_, ensureErr = g.DB().Exec(ctx, `
CREATE TABLE IF NOT EXISTS `+"`hg_youban_publish_media_phash_bucket`"+` (
  `+"`id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',"+`
  `+"`tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',"+`
  `+"`account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',"+`
  `+"`profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',"+`
  `+"`media_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '媒体ID',"+`
  `+"`task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID',"+`
  `+"`media_type` varchar(16) NOT NULL DEFAULT '' COMMENT '媒体类型',"+`
  `+"`hash_value` varchar(64) NOT NULL DEFAULT '' COMMENT '感知哈希',"+`
  `+"`bucket_pos` smallint(6) NOT NULL DEFAULT '0' COMMENT '分桶位置',"+`
  `+"`bucket_value` varchar(1) NOT NULL DEFAULT '' COMMENT '分桶值',"+`
  `+"`created_at` datetime DEFAULT NULL COMMENT '创建时间',"+`
  `+"`updated_at` datetime DEFAULT NULL COMMENT '更新时间',"+`
  PRIMARY KEY (`+"`id`"+`), UNIQUE KEY `+"`uk_ybp_media_phash_bucket_media_pos`"+` (`+"`media_id`,`bucket_pos`"+`), KEY `+"`idx_ybp_media_phash_bucket_lookup`"+` (`+"`tenant_id`,`media_type`,`bucket_pos`,`bucket_value`,`account_id`,`profile_id`,`task_id`,`media_id`"+`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='媒体感知哈希分桶';
`)
		}
	})
	return ensureErr
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
	if err := ensureMediaPHashBucketSchema(ctx); err != nil {
		return err
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
	if err := ensureMediaPHashBucketSchema(ctx); err != nil {
		return err
	}
	_, err := g.DB().Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).Where("task_id", taskId).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除任务哈希索引失败")
	}
	_ = bumpMediaPHashBucketVersion(ctx)
	return nil
}

func mediaPHashBucketCandidateRows(ctx context.Context, normalizedHash string, threshold int, tenantId int64, accountIds []int64, profileIds []int64, mediaType string, excludeProfileId int64) ([]mediaPHashBucketCandidateRow, error) {
	if err := ensureMediaPHashBucketSchema(ctx); err != nil {
		return nil, err
	}
	normalizedHash = strings.TrimSpace(strings.ToLower(normalizedHash))
	if normalizedHash == "" {
		return []mediaPHashBucketCandidateRow{}, nil
	}
	minEqualNibbles := mediaPHashMinEqualNibbles(threshold)
	if minEqualNibbles <= 0 {
		minEqualNibbles = 1
	}
	conds := make([]string, 0, len(normalizedHash))
	args := make([]any, 0, len(normalizedHash)*2)
	for i, item := range normalizedHash {
		conds = append(conds, "(bucket_pos=? AND bucket_value=?)")
		args = append(args, i+1, string(item))
	}
	mod := g.DB().Model(publishMediaPHashBucketTable).Safe().Ctx(ctx).
		Fields("media_id,profile_id,account_id,tenant_id,media_type,hash_value,COUNT(*) AS bucket_hits").
		Where(strings.Join(conds, " OR "), args...)
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if len(accountIds) > 0 {
		mod = mod.WhereIn("account_id", uniqueIds(accountIds))
	}
	if len(profileIds) > 0 {
		mod = mod.WhereIn("profile_id", uniqueIds(profileIds))
	}
	if excludeProfileId > 0 {
		mod = mod.WhereNot("profile_id", excludeProfileId)
	}
	if mediaType != "" {
		mod = mod.Where("media_type", mediaType)
	}
	rows := make([]mediaPHashBucketCandidateRow, 0)
	if err := mod.Group("media_id,profile_id,account_id,tenant_id,media_type,hash_value").
		Having("COUNT(*) >= ?", minEqualNibbles).
		OrderDesc("bucket_hits").
		Limit(mediaPHashBucketMaxCandidates).
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "查询相似媒体分桶失败")
	}
	return rows, nil
}
