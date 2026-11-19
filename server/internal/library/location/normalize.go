package location

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/internal/dao"
	"hotgo/internal/model/entity"
)

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

// RegionRecord is a read-only snapshot of a dictionary row, without entity pointers.
type RegionRecord struct {
	Id    int64
	Pid   int64
	Level int
	Title string
}

// RegionRecords shares the normalization directory's single database load path.
// Each call returns a fresh value slice sorted by ID, including district records.
func RegionRecords(ctx context.Context) ([]RegionRecord, error) {
	directory, err := loadRegionDirectory(ctx)
	if err != nil {
		return nil, err
	}
	return directory.records(), nil
}

func (d *regionDirectory) records() []RegionRecord {
	records := make([]RegionRecord, 0, len(d.byCode))
	for _, row := range d.byCode {
		if row != nil {
			records = append(records, RegionRecord{Id: row.Id, Pid: row.Pid, Level: row.Level, Title: row.Title})
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Id < records[j].Id })
	return records
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

// ResolveCityRegion resolves a city-field value independently of an old province.
// Every token must be a known dictionary region or an exact overseas seed.
// Unmatched input is returned verbatim as city, so callers can preserve raw data.
func ResolveCityRegion(ctx context.Context, value string) (province, city string, matched bool, err error) {
	directory, err := loadRegionDirectory(ctx)
	if err != nil {
		return "", value, false, err
	}
	province, city, matched = directory.resolveCityRegion(value)
	return province, city, matched, nil
}

func (d *regionDirectory) resolveCityRegion(value string) (province, city string, matched bool) {
	terms := ExpandRegionTerms(value)
	// Preserve exact dictionary names with internal whitespace before splitting.
	key := normalizeRegionLookupName(value)
	if len(d.provincesByName[key]) > 0 || len(d.citiesByName[key]) > 0 {
		terms = []string{strings.TrimSpace(value)}
	}
	if len(terms) == 0 {
		return "", value, false
	}
	var provinces, cities []string
	for _, term := range terms {
		if row := d.byCode[term]; row != nil && (row.Level == 1 || row.Pid == 0) {
			provinces = append(provinces, strconv.FormatInt(row.Id, 10))
			continue
		}
		matches := d.provincesByName[normalizeRegionLookupName(term)]
		if len(matches) > 1 {
			return "", value, false
		}
		if len(matches) == 1 {
			provinces = append(provinces, strconv.FormatInt(matches[0].Id, 10))
			continue
		}
		if _, country := overseasRegionCities[term]; country {
			// Missing dictionary IDs remain names; never synthesize a country ID.
			provinces = append(provinces, term)
			continue
		}
		resolvedCity, parent := d.normalizeCityToken(term, nil)
		if parent == "" {
			return "", value, false
		}
		provinces = append(provinces, parent)
		cities = append(cities, resolvedCity)
	}
	return joinRegionTerms(provinces), joinRegionTerms(cities), true
}

func (d *regionDirectory) normalize(province, city string) (string, string, bool) {
	province, city = strings.TrimSpace(province), strings.TrimSpace(city)
	provinceCode, _ := d.normalizeProvince(province)
	// Historical city fields may contain only countries or a country group.
	// Recognize complete overseas tokens before city normalization drops them.
	if province == "" && city != "" {
		countries, _ := d.normalizeProvince(city)
		terms := ExpandRegionTerms(countries)
		allCountries := len(terms) > 0
		for _, term := range terms {
			name := term
			if row := d.byCode[term]; row != nil && (row.Level == 1 || row.Pid == 0) {
				name = row.Title
			}
			if _, known := overseasRegionCities[name]; !known {
				allCountries = false
				break
			}
		}
		if allCountries {
			return countries, "", true
		}
	}
	cityCode, _ := d.normalizeCity(city, provinceCode)
	if city == "" && len(ExpandRegionTerms(province)) == 1 && !d.isProvinceValue(provinceCode) {
		if inferredCity, parent := d.normalizeCityToken(province, nil); parent != "" {
			provinceCode, cityCode = parent, inferredCity
		}
	}
	if provinceCode == "" && cityCode != "" {
		parents := make([]string, 0)
		for _, token := range ExpandRegionTerms(cityCode) {
			_, parent := d.normalizeCityToken(token, nil)
			if parent != "" {
				parents = append(parents, parent)
			}
		}
		provinceCode = joinRegionTerms(parents)
	}
	return provinceCode, cityCode, provinceCode != province || cityCode != city
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
	cols := dao.SysProvinces.Columns()
	if err := dao.SysProvinces.Ctx(ctx).Fields(cols.Id, cols.Pid, cols.Level, cols.Title).
		Where(cols.Status, 1).Scan(&rows); err != nil {
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
	// Preserve existing domestic name normalization before tokenizing lists.
	if matches := d.provincesByName[normalizeRegionLookupName(value)]; len(matches) == 1 {
		code := strconv.FormatInt(matches[0].Id, 10)
		return code, code != value
	}
	terms := ExpandRegionTerms(value)
	for i, term := range terms {
		if row := d.byCode[term]; row != nil && (row.Level == 1 || row.Pid == 0) {
			terms[i] = strconv.FormatInt(row.Id, 10)
			continue
		}
		if matches := d.provincesByName[normalizeRegionLookupName(term)]; len(matches) == 1 {
			terms[i] = strconv.FormatInt(matches[0].Id, 10)
		}
	}
	normalized := joinRegionTerms(terms)
	return normalized, normalized != value
}

func (d *regionDirectory) isProvinceValue(value string) bool {
	if row := d.byCode[value]; row != nil && (row.Level == 1 || row.Pid == 0) {
		return true
	}
	_, overseas := overseasRegionCities[value]
	return overseas
}

func (d *regionDirectory) normalizeCity(value, provinceCode string) (string, bool) {
	allowed := make(map[string]bool)
	for _, token := range ExpandRegionTerms(provinceCode) {
		if d.isProvinceValue(token) {
			allowed[token] = true
			if row := d.byCode[token]; row != nil {
				allowed[row.Title] = true
			}
		}
	}
	if len(d.citiesByName[normalizeRegionLookupName(value)]) > 0 {
		city, _ := d.normalizeCityToken(value, allowed)
		return city, city != value
	}
	terms := ExpandRegionTerms(value)
	normalized := make([]string, 0, len(terms))
	for _, term := range terms {
		// Legacy city options may carry "country/city" text values.
		if len(terms) > 1 && d.isProvinceValue(term) {
			continue
		}
		city, _ := d.normalizeCityToken(term, allowed)
		normalized = append(normalized, city)
	}
	result := joinRegionTerms(normalized)
	return result, result != value
}

func (d *regionDirectory) normalizeCityToken(value string, allowed map[string]bool) (string, string) {
	parentAllowed := func(parent string) bool { return len(allowed) == 0 || allowed[parent] }
	if row := d.byCode[value]; row != nil && row.Level == 2 {
		parent := strconv.FormatInt(row.Pid, 10)
		if d.byCode[parent] != nil && parentAllowed(parent) {
			return strconv.FormatInt(row.Id, 10), parent
		}
		return value, ""
	}
	matches := d.citiesByName[normalizeRegionLookupName(value)]
	var match *entity.SysProvinces
	for _, row := range matches {
		parent := strconv.FormatInt(row.Pid, 10)
		if d.byCode[parent] == nil || !parentAllowed(parent) {
			continue
		}
		if match != nil {
			return value, ""
		}
		match = row
	}
	if match != nil {
		return strconv.FormatInt(match.Id, 10), strconv.FormatInt(match.Pid, 10)
	}
	// A unique exact city name remains identifiable when a client echoes an
	// old province. Return its existing ID, but leave province correction to
	// the caller's explicit-edit intent policy. Never guess ambiguous names.
	if len(allowed) > 0 && len(matches) == 1 {
		row := matches[0]
		parent := strconv.FormatInt(row.Pid, 10)
		if d.byCode[parent] != nil {
			return strconv.FormatInt(row.Id, 10), parent
		}
	}
	if country := overseasCityCountries[value]; country != "" {
		parent, _ := d.normalizeProvince(country)
		if parentAllowed(parent) || parentAllowed(country) {
			return value, parent
		}
	}
	return value, ""
}

func joinRegionTerms(terms []string) string {
	seen := make(map[string]bool, len(terms))
	unique := make([]string, 0, len(terms))
	for _, term := range terms {
		if term != "" && !seen[term] {
			seen[term] = true
			unique = append(unique, term)
		}
	}
	return strings.Join(unique, ",")
}

func normalizeRegionLookupName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "\u3000", "", "特别行政区", "", "壮族自治区", "", "回族自治区", "", "维吾尔自治区", "", "自治区", "", "自治州", "", "地区", "", "省", "", "市", "")
	return replacer.Replace(value)
}
