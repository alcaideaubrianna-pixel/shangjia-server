package location

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/internal/dao"
	"hotgo/internal/model/entity"
)

var regionCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

var regionDirectoryCache struct {
	sync.RWMutex
	value     *regionDirectory
	expiresAt time.Time
}

type regionDirectory struct {
	byCode          map[string]*entity.SysProvinces
	provincesByName map[string][]*entity.SysProvinces
	citiesByName    map[string][]*entity.SysProvinces
}

// NormalizeRegionCodes converts recognized domestic province/city names to
// the canonical codes maintained by hg_sys_provinces. Unknown overseas values
// are preserved, and ambiguous names are never guessed.
func NormalizeRegionCodes(ctx context.Context, province, city string) (string, string, bool, error) {
	directory, err := loadRegionDirectory(ctx)
	if err != nil {
		return province, city, false, err
	}
	province, city, changed := directory.normalize(province, city)
	return province, city, changed, nil
}

func (d *regionDirectory) normalize(province, city string) (string, string, bool) {
	province, city = strings.TrimSpace(province), strings.TrimSpace(city)
	provinceCode, provinceChanged := d.normalizeProvince(province)
	cityCode, cityChanged := d.normalizeCity(city, provinceCode)
	if !regionCodePattern.MatchString(provinceCode) && cityCode == "" && city == "" {
		if inferredCity, changed := d.normalizeCity(province, ""); changed {
			cityCode, cityChanged = inferredCity, true
			provinceCode = ""
		}
	}
	if provinceCode == "" && cityCode != "" {
		if row := d.byCode[cityCode]; row != nil && row.Pid > 0 {
			provinceCode = strconv.FormatInt(row.Pid, 10)
			provinceChanged = provinceCode != province
		}
	}
	return provinceCode, cityCode, provinceChanged || cityChanged
}

func loadRegionDirectory(ctx context.Context) (*regionDirectory, error) {
	regionDirectoryCache.RLock()
	cached, valid := regionDirectoryCache.value, time.Now().Before(regionDirectoryCache.expiresAt)
	regionDirectoryCache.RUnlock()
	if cached != nil && valid {
		return cached, nil
	}
	regionDirectoryCache.Lock()
	defer regionDirectoryCache.Unlock()
	if regionDirectoryCache.value != nil && time.Now().Before(regionDirectoryCache.expiresAt) {
		return regionDirectoryCache.value, nil
	}
	rows := make([]*entity.SysProvinces, 0)
	if err := dao.SysProvinces.Ctx(ctx).Where(dao.SysProvinces.Columns().Status, 1).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取地区编码字典失败")
	}
	directory := &regionDirectory{
		byCode:          make(map[string]*entity.SysProvinces, len(rows)),
		provincesByName: make(map[string][]*entity.SysProvinces),
		citiesByName:    make(map[string][]*entity.SysProvinces),
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		directory.byCode[strconv.FormatInt(row.Id, 10)] = row
		name := normalizeRegionLookupName(row.Title)
		if row.Level == 1 || row.Pid == 0 {
			directory.provincesByName[name] = append(directory.provincesByName[name], row)
		} else if row.Level == 2 {
			directory.citiesByName[name] = append(directory.citiesByName[name], row)
		}
	}
	regionDirectoryCache.value = directory
	regionDirectoryCache.expiresAt = time.Now().Add(time.Hour)
	return directory, nil
}

func (d *regionDirectory) normalizeProvince(value string) (string, bool) {
	if value == "" || regionCodePattern.MatchString(value) {
		return value, false
	}
	matches := d.provincesByName[normalizeRegionLookupName(value)]
	if len(matches) != 1 {
		return value, false
	}
	code := strconv.FormatInt(matches[0].Id, 10)
	return code, code != value
}

func (d *regionDirectory) normalizeCity(value, provinceCode string) (string, bool) {
	if value == "" || regionCodePattern.MatchString(value) {
		return value, false
	}
	matches := d.citiesByName[normalizeRegionLookupName(value)]
	if regionCodePattern.MatchString(provinceCode) {
		provinceID, _ := strconv.ParseInt(provinceCode, 10, 64)
		filtered := make([]*entity.SysProvinces, 0, len(matches))
		for _, row := range matches {
			if row.Pid == provinceID {
				filtered = append(filtered, row)
			}
		}
		matches = filtered
	}
	if len(matches) != 1 {
		return value, false
	}
	code := strconv.FormatInt(matches[0].Id, 10)
	return code, code != value
}

func normalizeRegionLookupName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "\u3000", "", "特别行政区", "", "壮族自治区", "", "回族自治区", "", "维吾尔自治区", "", "自治区", "", "自治州", "", "地区", "", "省", "", "市", "")
	return replacer.Replace(value)
}
