package fix

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/library/profileextractor"
)

const profileAgeBackfillBatchSize = 200

// BackfillContentProfileAge fills only currently-zero ages when the source text
// contains an explicit, valid age. Birth-year text is rejected by the parser.
func BackfillContentProfileAge(ctx context.Context) error {
	lastID := int64(0)
	processed, updated := 0, 0
	for {
		var rows []struct {
			ID        int64  `orm:"id"`
			Age       int    `orm:"age"`
			PlainText string `orm:"plain_text"`
		}
		if err := g.DB().Model("hg_content_profile").Safe().Ctx(ctx).
			Fields("id,age,plain_text").WhereGT("id", lastID).Where("age", 0).
			WhereNull("deleted_at").OrderAsc("id").Limit(profileAgeBackfillBatchSize).Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取年龄回填资料失败")
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			lastID = row.ID
			processed++
			age := profileextractor.Parse(row.PlainText).Age
			if age == 0 {
				continue
			}
			if _, err := g.DB().Model("hg_content_profile").Ctx(ctx).
				Where("id", row.ID).Where("age", 0).
				Data(g.Map{"age": age}).Update(); err != nil {
				return gerror.Wrapf(err, "更新资料年龄失败 profileId:%d", row.ID)
			}
			updated++
		}
		g.Log().Infof(ctx, "资料年龄回填进度：lastProfileId=%d processed=%d updated=%d", lastID, processed, updated)
	}
	g.Log().Infof(ctx, "资料年龄回填完成：processed=%d updated=%d", processed, updated)
	return nil
}

// BackfillContentProfileVirgin fills unknown values from explicit source text.
func BackfillContentProfileVirgin(ctx context.Context) error {
	lastID := int64(0)
	processed, yes, no := 0, 0, 0
	for {
		var rows []struct {
			ID        int64  `orm:"id"`
			PlainText string `orm:"plain_text"`
		}
		if err := g.DB().Model("hg_content_profile").Safe().Ctx(ctx).
			Fields("id,plain_text").WhereGT("id", lastID).Where("is_virgin", 0).
			WhereNull("deleted_at").OrderAsc("id").Limit(profileAgeBackfillBatchSize).Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取是否处回填资料失败")
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			lastID = row.ID
			processed++
			value := profileextractor.Parse(row.PlainText).Virgin
			if value == 0 {
				continue
			}
			if _, err := g.DB().Model("hg_content_profile").Ctx(ctx).
				Where("id", row.ID).Where("is_virgin", 0).
				Data(g.Map{"is_virgin": value}).Update(); err != nil {
				return gerror.Wrapf(err, "更新资料是否处失败 profileId:%d", row.ID)
			}
			if value == 1 {
				yes++
			} else {
				no++
			}
		}
		g.Log().Infof(ctx, "资料是否处回填进度：lastProfileId=%d processed=%d yes=%d no=%d", lastID, processed, yes, no)
	}
	g.Log().Infof(ctx, "资料是否处回填完成：processed=%d yes=%d no=%d", processed, yes, no)
	return nil
}
