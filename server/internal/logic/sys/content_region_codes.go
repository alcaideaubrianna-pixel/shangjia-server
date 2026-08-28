package sys

import (
	"context"
	"strconv"
	"strings"

	"hotgo/internal/dao"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/sysin"
)

type contentRegionDirectory struct {
	byCode map[string]*entity.SysProvinces
}

func loadContentRegionDirectory(ctx context.Context) *contentRegionDirectory {
	rows := make([]*entity.SysProvinces, 0)
	_ = dao.SysProvinces.Ctx(ctx).Where(dao.SysProvinces.Columns().Status, 1).Scan(&rows)
	directory := &contentRegionDirectory{
		byCode: make(map[string]*entity.SysProvinces),
	}
	for _, row := range rows {
		directory.byCode[strconv.FormatInt(row.Id, 10)] = row
	}
	return directory
}

func decorateProfileRegionWithDirectory(directory *contentRegionDirectory, profile *sysin.ContentProfileListModel) {
	if profile == nil {
		return
	}
	provinceRaw, cityRaw := strings.TrimSpace(profile.Province), strings.TrimSpace(profile.City)
	province, provinceKnown := resolveRegionCodes(directory, provinceRaw)
	city, cityKnown := resolveRegionCodes(directory, cityRaw)
	if provinceKnown {
		profile.ProvinceCode = provinceRaw
	}
	if cityKnown {
		profile.CityCode = cityRaw
	}
	province, city = normalizeProfileRegionForOption(province, city)
	profile.Province, profile.City = province, city
	profile.LocationLabel = strings.TrimSpace(strings.Join([]string{province, city}, " "))
	if city == "" || city == province {
		profile.LocationLabel = province
	}
}

// resolveRegionCodes handles both a single code and legacy comma-separated codes.
func resolveRegionCodes(directory *contentRegionDirectory, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' || r == '、' })
	if len(parts) == 0 {
		return cleanRegionToken(raw), false
	}
	labels := make([]string, 0, len(parts))
	known := false
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		label := cleanRegionToken(part)
		if row := directory.byCode[part]; row != nil {
			label = cleanRegionToken(row.Title)
			known = true
		}
		if label != "" {
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, ","), known
}
