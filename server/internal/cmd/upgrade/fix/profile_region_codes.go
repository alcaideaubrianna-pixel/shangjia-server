package fix

import (
	"context"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/dao"
	"hotgo/internal/library/location"
	"hotgo/internal/library/profileextractor"
)

const profileRegionBackfillBatchSize = 1000

var canonicalProfileRegionCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

// BackfillContentProfileRegionsFromText repairs missing or malformed region
// fields only when explicit labels in the original text resolve uniquely.
func BackfillContentProfileRegionsFromText(ctx context.Context) error {
	columns := dao.ContentProfile.Columns()
	var lastID int64
	processed, updated := 0, 0
	for {
		var rows []struct {
			ID        int64  `orm:"id"`
			Province  string `orm:"province"`
			City      string `orm:"city"`
			PlainText string `orm:"plain_text"`
		}
		if err := dao.ContentProfile.Ctx(ctx).
			Fields(columns.Id, columns.Province, columns.City, columns.PlainText).
			WhereGT(columns.Id, lastID).
			WhereNull(columns.DeletedAt).
			OrderAsc(columns.Id).
			Limit(profileRegionBackfillBatchSize).
			Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取原文地区待回填资料失败")
		}
		if len(rows) == 0 {
			break
		}
		updates := make([]profileRegionUpdate, 0, len(rows))
		for _, row := range rows {
			lastID, processed = row.ID, processed+1
			province, city, ok, normalizeErr := profileRegionCodesFromText(ctx, row.PlainText, row.Province, row.City)
			if normalizeErr != nil {
				return normalizeErr
			}
			if !ok {
				continue
			}
			updates = append(updates, profileRegionUpdate{ID: row.ID, Province: province, City: city})
			updated++
		}
		if err := applyProfileRegionUpdates(ctx, updates); err != nil {
			return err
		}
		g.Log().Infof(ctx, "资料原文地区回填进度：lastProfileId=%d processed=%d updated=%d", lastID, processed, updated)
	}
	g.Log().Infof(ctx, "资料原文地区回填完成：processed=%d updated=%d", processed, updated)
	return nil
}

func profileRegionCodesFromText(ctx context.Context, text, currentProvince, currentCity string) (province, city string, ok bool, err error) {
	if canonicalProfileRegionCodePattern.MatchString(strings.TrimSpace(currentProvince)) {
		return "", "", false, nil
	}
	parsedProvince, parsedCity := profileextractor.RegionLabels(text)
	if parsedProvince == "" && parsedCity == "" {
		return "", "", false, nil
	}
	province, city, _, err = location.NormalizeRegionCodes(ctx, parsedProvince, parsedCity)
	if err != nil || !canonicalProfileRegionCodePattern.MatchString(province) {
		return "", "", false, err
	}
	if city == "" && canonicalProfileRegionCodePattern.MatchString(strings.TrimSpace(currentCity)) {
		city = strings.TrimSpace(currentCity)
	}
	return province, city, province != strings.TrimSpace(currentProvince) || city != strings.TrimSpace(currentCity), nil
}

// BackfillContentProfileRegionCodes canonicalizes recognized domestic region
// names. It is idempotent and leaves ambiguous or overseas values untouched.
func BackfillContentProfileRegionCodes(ctx context.Context) error {
	columns := dao.ContentProfile.Columns()
	var (
		lastID int64
		err    error
	)
	processed, updated, skipped := 0, 0, 0
	for {
		var rows []struct {
			ID       int64  `orm:"id"`
			Province string `orm:"province"`
			City     string `orm:"city"`
		}
		if err = dao.ContentProfile.Ctx(ctx).Fields(columns.Id, columns.Province, columns.City).
			WhereGT(columns.Id, lastID).OrderAsc(columns.Id).Limit(profileRegionBackfillBatchSize).Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取地区待回填资料失败")
		}
		if len(rows) == 0 {
			break
		}
		updates := make([]profileRegionUpdate, 0, len(rows))
		for _, row := range rows {
			lastID, processed = row.ID, processed+1
			province, city, changed, normalizeErr := location.NormalizeRegionCodes(ctx, row.Province, row.City)
			if normalizeErr != nil {
				return normalizeErr
			}
			if !changed {
				skipped++
				continue
			}
			updates = append(updates, profileRegionUpdate{ID: row.ID, Province: province, City: city})
			updated++
		}
		if err = applyProfileRegionUpdates(ctx, updates); err != nil {
			return err
		}
		g.Log().Infof(ctx, "资料地区编码回填进度：lastProfileId=%d processed=%d updated=%d skipped=%d", lastID, processed, updated, skipped)
	}
	g.Log().Infof(ctx, "资料地区编码回填完成：processed=%d updated=%d skipped=%d", processed, updated, skipped)
	return nil
}

type profileRegionUpdate struct {
	ID       int64
	Province string
	City     string
}

func applyProfileRegionUpdates(ctx context.Context, updates []profileRegionUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	values := make([]string, 0, len(updates))
	args := make([]interface{}, 0, len(updates)*3)
	for _, update := range updates {
		values = append(values, "(CAST(? AS BIGINT),CAST(? AS VARCHAR),CAST(? AS VARCHAR))")
		args = append(args, update.ID, update.Province, update.City)
	}
	source := strings.Join(values, ",")
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("UPDATE hg_content_profile p SET province=v.province,city=v.city FROM (VALUES "+source+") AS v(id,province,city) WHERE p.id=v.id", args...); err != nil {
			return gerror.Wrap(err, "批量更新资料地区编码失败")
		}
		if _, err := tx.Exec("UPDATE hg_youban_publish_note_index i SET province=v.province,city=v.city FROM (VALUES "+source+") AS v(id,province,city) WHERE i.profile_id=v.id", args...); err != nil {
			return gerror.Wrap(err, "批量同步资料索引地区编码失败")
		}
		return nil
	})
}
