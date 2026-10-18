package fix

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/service"
)

// RepairYoubanPublishMediaSearchIndexes reuses the runtime index writer. It never
// downloads files or changes source pHashes, and processes only one bounded page.
// Run again with the logged afterId; omit apply=1 for a read-only audit.
func RepairYoubanPublishMediaSearchIndexes(ctx context.Context, args map[string]string) error {
	if !strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
		return gerror.New("媒体搜索索引审计目前只支持PostgreSQL")
	}
	options, err := parseMediaSearchRepairOptions(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	mod := g.DB().Model(youbanPublishMediaTable).Safe().Ctx(ctx).
		Fields("id,perceptual_hash").WhereGT("id", options.afterID).
		WhereNull("deleted_at").WhereIn("media_type", []string{"image", "photo", "video"}).OrderAsc("id").Limit(options.limit)
	if options.tenantID > 0 {
		mod = mod.Where("tenant_id", options.tenantID)
	}
	if options.profileID > 0 {
		mod = mod.Where("profile_id", options.profileID)
	}
	rows, err := mod.All()
	if err != nil {
		return gerror.Wrap(err, "读取媒体搜索索引审计批次失败")
	}
	missing, repaired, invalid := 0, 0, 0
	lastID := options.afterID
	for _, row := range rows {
		id := row["id"].Int64()
		if !validMediaSearchRepairHash(row["perceptual_hash"].String()) {
			invalid++
			g.Log().Info(ctx, "媒体指纹待补算", g.Map{"mediaId": id, "apply": options.apply})
			lastID = id
			continue
		}
		// Compare every bucket value and owner, not merely the row count. Corrupt
		// indexes containing all 16/4 rows are otherwise incorrectly called healthy.
		result, queryErr := g.DB().GetValue(ctx, mediaSearchIndexCompleteSQL, id)
		if queryErr != nil {
			return gerror.Wrapf(queryErr, "审计媒体搜索索引失败 mediaId:%d afterId:%d", id, lastID)
		}
		if !result.Bool() {
			missing++
			if options.apply {
				if err := service.SysPublish().SyncMediaPHashBucketByMediaId(ctx, id); err != nil {
					return gerror.Wrapf(err, "修复媒体搜索索引失败 mediaId:%d afterId:%d", id, lastID)
				}
				checked, err := g.DB().GetValue(ctx, mediaSearchIndexCompleteSQL, id)
				if err != nil {
					return gerror.Wrapf(err, "复核媒体搜索索引失败 mediaId:%d", id)
				}
				if !checked.Bool() {
					return gerror.Newf("媒体搜索索引复核未通过 mediaId:%d afterId:%d", id, lastID)
				}
				repaired++
			}
			g.Log().Info(ctx, "媒体搜索索引审计记录", g.Map{"mediaId": id, "apply": options.apply})
		}
		lastID = id
	}
	g.Log().Info(ctx, "媒体搜索索引批次完成", g.Map{"dryRun": !options.apply, "scanned": len(rows), "incomplete": missing, "repaired": repaired, "needsFingerprint": invalid, "afterId": lastID, "tenantId": options.tenantID, "profileId": options.profileID})
	return nil
}

type mediaSearchRepairOptions struct {
	afterID, tenantID, profileID int64
	limit                        int
	apply                        bool
}

func parseMediaSearchRepairOptions(args map[string]string) (mediaSearchRepairOptions, error) {
	options := mediaSearchRepairOptions{limit: 100}
	for name, target := range map[string]*int64{"afterId": &options.afterID, "tenantId": &options.tenantID, "profileId": &options.profileID} {
		if raw := strings.TrimSpace(args[name]); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 0 {
				return options, gerror.Newf("%s 必须为非负整数", name)
			}
			*target = value
		}
	}
	if raw := strings.TrimSpace(args["limit"]); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 1000 {
			return options, gerror.New("limit 必须为1至1000")
		}
		options.limit = limit
	}
	switch strings.TrimSpace(args["apply"]) {
	case "", "0":
	case "1":
		options.apply = true
	default:
		return options, gerror.New("apply 只允许0或1")
	}
	return options, nil
}

func validMediaSearchRepairHash(hash string) bool {
	hash = strings.TrimSpace(strings.ToLower(hash))
	if len(hash) != 16 {
		return false
	}
	_, err := strconv.ParseUint(hash, 16, 64)
	return err == nil
}

const mediaSearchIndexCompleteSQL = `
SELECT (
 (SELECT COUNT(*) FROM hg_youban_publish_media_phash_bucket b
  WHERE b.media_id=m.id AND b.tenant_id=m.tenant_id AND b.account_id=m.account_id
  AND b.profile_id=m.profile_id AND b.media_type=m.media_type
  AND b.hash_value=lower(trim(m.perceptual_hash)) AND b.bucket_pos BETWEEN 1 AND 16
  AND b.bucket_value=substr(lower(trim(m.perceptual_hash)),b.bucket_pos,1))=16
 AND
 (SELECT COUNT(*) FROM hg_youban_publish_media_phash_lsh b
  WHERE b.media_id=m.id AND b.tenant_id=m.tenant_id AND b.account_id=m.account_id
  AND b.profile_id=m.profile_id AND b.media_type=m.media_type
  AND b.hash_value=lower(trim(m.perceptual_hash)) AND b.bucket_pos BETWEEN 1 AND 4
  AND b.bucket_value=(('x'||substr(lower(trim(m.perceptual_hash)),(b.bucket_pos-1)*4+1,4))::bit(16)::int))=4
 AND EXISTS (SELECT 1 FROM hg_youban_publish_media_fingerprint f
  WHERE f.media_id=m.id AND f.tenant_id=m.tenant_id AND f.account_id=m.account_id
  AND f.profile_id=m.profile_id AND f.media_type=m.media_type AND f.deleted_at IS NULL
  AND f.phash_bits=('x'||lower(trim(m.perceptual_hash)))::bit(64)
  AND COALESCE(f.md5,'')=COALESCE(m.md5,''))
) FROM hg_youban_publish_media m WHERE m.id=? AND m.deleted_at IS NULL`
