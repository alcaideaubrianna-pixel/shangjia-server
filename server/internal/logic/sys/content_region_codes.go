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
	province, city := provinceRaw, cityRaw
	if row := directory.byCode[provinceRaw]; row != nil {
		profile.ProvinceCode = provinceRaw
		province = cleanRegionToken(row.Title)
	}
	if row := directory.byCode[cityRaw]; row != nil {
		profile.CityCode = cityRaw
		city = cleanRegionToken(row.Title)
	}
	province, city = normalizeProfileRegionForOption(province, city)
	profile.Province, profile.City = province, city
	profile.LocationLabel = strings.TrimSpace(strings.Join([]string{province, city}, " "))
	if city == "" || city == province {
		profile.LocationLabel = province
	}
}
