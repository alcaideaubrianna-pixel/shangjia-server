package fix

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	youbanPublishMediaTable            = "hg_youban_publish_media"
	youbanPublishMediaPHashBucketTable = "hg_youban_publish_media_phash_bucket"
	mediaPHashBackfillBatchSize        = 300
)

type mediaPHashBackfillRow struct {
	AccountId      int64  `orm:"account_id"`
	Id             int64  `orm:"id"`
	MediaType      string `orm:"media_type"`
	PerceptualHash string `orm:"perceptual_hash"`
	ProfileId      int64  `orm:"profile_id"`
	TaskId         int64  `orm:"task_id"`
	TenantId       int64  `orm:"tenant_id"`
}

// BackfillYoubanPublishMediaPHashBucket backfills media pHash bucket index in bounded batches.
func BackfillYoubanPublishMediaPHashBucket(ctx context.Context) error {
	if err := ensureYoubanPublishMediaPHashBucket(ctx); err != nil {
		return err
	}
	lastId := int64(0)
	totalMedia := 0
	totalBucket := 0
	for {
		rows, err := mediaPHashBackfillRows(ctx, lastId, mediaPHashBackfillBatchSize)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		lastId = rows[len(rows)-1].Id
		inserted, err := insertMediaPHashBackfillBuckets(ctx, rows)
		if err != nil {
			return err
		}
		totalMedia += len(rows)
		totalBucket += inserted
		g.Log().Infof(ctx, "上架媒体感知哈希分桶回填进度：lastId=%d media=%d buckets=%d", lastId, totalMedia, totalBucket)
	}
	g.Log().Infof(ctx, "上架媒体感知哈希分桶回填完成：media=%d buckets=%d", totalMedia, totalBucket)
	return nil
}

func mediaPHashBackfillRows(ctx context.Context, lastId int64, limit int) ([]mediaPHashBackfillRow, error) {
	rows := make([]mediaPHashBackfillRow, 0)
	err := g.DB().Model(youbanPublishMediaTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,profile_id,task_id,media_type,perceptual_hash").
		WhereGT("id", lastId).
		WhereNot("perceptual_hash", "").
		WhereNull("deleted_at").
		OrderAsc("id").
		Limit(limit).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取媒体感知哈希失败")
	}
	return rows, nil
}

func insertMediaPHashBackfillBuckets(ctx context.Context, rows []mediaPHashBackfillRow) (int, error) {
	data := make([]g.Map, 0, len(rows)*16)
	mediaIds := make([]int64, 0, len(rows))
	now := gtime.Now()
	for _, row := range rows {
		hash := normalizeMediaPHash(row.PerceptualHash)
		if len(hash) != 16 {
			continue
		}
		mediaIds = append(mediaIds, row.Id)
		for pos := 1; pos <= 16; pos++ {
			data = append(data, g.Map{
				"tenant_id":    row.TenantId,
				"account_id":   row.AccountId,
				"profile_id":   row.ProfileId,
				"media_id":     row.Id,
				"task_id":      row.TaskId,
				"media_type":   strings.TrimSpace(row.MediaType),
				"hash_value":   hash,
				"bucket_pos":   pos,
				"bucket_value": hash[pos-1 : pos],
				"created_at":   now,
				"updated_at":   now,
			})
		}
	}
	if len(data) == 0 {
		return 0, nil
	}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model(youbanPublishMediaPHashBucketTable).Safe().Ctx(ctx).WhereIn("media_id", mediaIds).Delete(); err != nil {
			return gerror.Wrap(err, "清理媒体感知哈希分桶失败")
		}
		if _, err := tx.Model(youbanPublishMediaPHashBucketTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
			return gerror.Wrap(err, "写入媒体感知哈希分桶失败")
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func ensureYoubanPublishMediaPHashBucket(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == "pgsql" {
		_, err := g.DB().Exec(ctx, `
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
		return gerror.Wrap(err, "初始化媒体感知哈希分桶表失败")
	}
	_, err := g.DB().Exec(ctx, `
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
	return gerror.Wrap(err, "初始化媒体感知哈希分桶表失败")
}

func normalizeMediaPHash(hash string) string {
	hash = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(hash)), "0x")
	if len(hash) != 16 {
		return ""
	}
	return hash
}
