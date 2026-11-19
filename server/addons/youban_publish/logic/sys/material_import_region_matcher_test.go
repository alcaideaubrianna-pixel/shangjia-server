package sys

import (
	"context"
	"sync"
	"testing"

	"hotgo/internal/library/location"
)

func materialImportMatcherFixture() *legacyCMSRegionIndex {
	index := &legacyCMSRegionIndex{
		provincesByName: make(map[string]*legacyCMSRegionOption),
		citiesByName:    make(map[string][]*legacyCMSRegionOption),
		districtsByName: make(map[string][]*legacyCMSRegionOption),
		childrenByPid:   make(map[int64][]*legacyCMSRegionOption),
		optionsById:     make(map[int64]*legacyCMSRegionOption),
	}
	rows := []*legacyCMSRegionOption{
		{Id: 330000, Level: 1, Title: "浙江省"},
		{Id: 330100, Pid: 330000, Level: 2, Title: "杭州市"},
		{Id: 330200, Pid: 330000, Level: 2, Title: "宁波市"},
		{Id: 520000, Level: 1, Title: "贵州省"},
		{Id: 522700, Pid: 520000, Level: 2, Title: "黔南布依族苗族自治州"},
		{Id: 330101, Pid: 330100, Level: 3, Title: "同名区"},
		{Id: 330201, Pid: 330200, Level: 3, Title: "同名区"},
		{Id: 900000001, Level: 1, Title: "新加坡"},
		{Id: 900000002, Level: 1, Title: "马来西亚"},
		{Id: 900000003, Level: 1, Title: "泰国"},
		{Id: 900392000, Level: 1, Title: "日本"},
		{Id: 900392001, Pid: 900392000, Level: 2, Title: "东京"},
		{Id: 50, Level: 1, Title: "英国"},
	}
	for _, row := range rows {
		index.optionsById[row.Id] = row
		index.childrenByPid[row.Pid] = append(index.childrenByPid[row.Pid], row)
		name := normalizeLegacyRegionName(row.Title)
		switch row.Level {
		case 1:
			index.provincesByName[name] = row
		case 2:
			index.citiesByName[name] = append(index.citiesByName[name], row)
		case 3:
			index.districtsByName[name] = append(index.districtsByName[name], row)
		}
	}
	return index
}

func TestMaterialImportRegionCodesCompiled(t *testing.T) {
	index := materialImportMatcherFixture()
	tests := []struct{ name, text, province, city string }{
		{"exact", "城市：杭州", "330000", "330100"},
		{"multiple cities", "城市：杭州/宁波", "330000", "330100,330200"},
		{"complex values", "城市：常驻杭州及宁波", "330000", "330100,330200"},
		{"autonomous alias", "城市：黔南", "520000", "522700"},
		{"ambiguous district", "城市：同名区", "", ""},
		{"single character", "城市：杭", "", ""},
		{"overseas city", "城市：曼谷", "900000003", "曼谷"},
		{"overseas city in province field", "省份：吉隆坡", "900000002", "吉隆坡"},
		{"country and city", "国家：泰国\n城市：曼谷", "900000003", "曼谷"},
		{"country only", "国家：新加坡", "900000001", ""},
		{"country group", "国家：新马泰", "900000001,900000002,900000003", ""},
		{"country alias label", "国别：新马泰", "900000001,900000002,900000003", ""},
		{"country location label", "所在国家：新马泰", "900000001,900000002,900000003", ""},
		{"numeric country exact", "国家：900392000", "900392000", ""},
		{"numeric country not substring", "国家：编号900392000", "", ""},
		{"country metadata independent of ID range", "城市：伦敦", "50", "伦敦"},
		{"database city beats text seed", "国家：日本\n城市：东京", "900392000", "900392001"},
		{"country without database city keeps text", "国家：日本\n城市：大阪", "900392000", "大阪"},
		{"country and city same line", "国家：日本 城市：东京", "900392000", "900392001"},
		{"country and city compact", "国家：日本城市：东京", "900392000", "900392001"},
		{"group in city field", "城市：新马泰", "900000001,900000002,900000003", ""},
		{"group exact tokens only", "城市：介绍新马泰路线", "", ""},
		{"ignore other body locations", "城市：杭州\n介绍人：曼谷\n旅游：新马泰", "330000", "330100"},
		{"ignore unlabelled body", "旅游曼谷，新马泰", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			province, city := materialImportRegionCodesFromIndex(tt.text, index)
			if province != tt.province || city != tt.city {
				t.Fatalf("codes = (%q, %q), want (%q, %q)", province, city, tt.province, tt.city)
			}
			fields := materialImportRegionFieldsFromText(tt.text)
			fieldProvince, fieldCity := materialImportRegionCodesForFieldsFromIndex(fields, index)
			if fieldProvince != province || fieldCity != city {
				t.Fatalf("parsed-fields entry differs: (%q, %q)", fieldProvince, fieldCity)
			}
		})
	}
}

func TestMaterialImportRegionMatcherDomesticBaselineParity(t *testing.T) {
	index := materialImportMatcherFixture()
	for _, text := range []string{
		"城市：杭州", "城市：杭州/宁波", "城市：常驻杭州及宁波",
		"城市：黔南", "省份：浙江 城市：杭州",
	} {
		wantProvince, wantCity := baselineMaterialImportRegionCodesFromIndex(text, index)
		province, city := materialImportRegionCodesFromIndex(text, index)
		if province != wantProvince || city != wantCity {
			t.Fatalf("%q compiled = (%q, %q), baseline = (%q, %q)", text, province, city, wantProvince, wantCity)
		}
	}
}

func TestMaterialImportRegionFieldsCountryBoundaryAndDeduplication(t *testing.T) {
	fields := materialImportRegionFieldsFromText("国家：日本 城市：东京\n城市：东京")
	if len(fields.provinceTexts) != 1 || fields.provinceTexts[0] != "日本" {
		t.Fatalf("country fields = %#v, want only 日本", fields.provinceTexts)
	}
	if len(fields.cityTexts) != 1 || fields.cityTexts[0] != "东京" {
		t.Fatalf("city fields = %#v, want only 东京", fields.cityTexts)
	}
}

func TestMaterialImportRegionMatcherCompiledOnceConcurrent(t *testing.T) {
	index := materialImportMatcherFixture()
	const workers = 16
	results := make(chan *materialImportRegionMatcher, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- index.regionMatcher()
		}()
	}
	wg.Wait()
	close(results)
	var first *materialImportRegionMatcher
	for matcher := range results {
		if first == nil {
			first = matcher
		} else if first != matcher {
			t.Fatal("index compiled more than once")
		}
	}
}

func TestMaterialImportRegionCodesEmptyFieldsSkipsIndex(t *testing.T) {
	// A cancelled context is sufficient: this path must return before any
	// dictionary access, even with no region cache/database configured.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	province, city, err := materialImportRegionCodesForFields(ctx, materialImportRegionFields{})
	if err != nil || province != "" || city != "" {
		t.Fatalf("empty fields = (%q, %q, %v)", province, city, err)
	}
}

func TestMaterialImportRegionOverseasTextFitsStorage(t *testing.T) {
	index := materialImportMatcherFixture()
	fields := materialImportRegionFields{}
	var id int64 = 800000000
	for country, cities := range location.OverseasRegionCities() {
		id++
		row := &legacyCMSRegionOption{Id: id, Level: 1, Title: country}
		index.provincesByName[country] = row
		index.optionsById[id] = row
		fields.cityTexts = append(fields.cityTexts, cities...)
	}
	province, city := materialImportRegionCodesForFieldsFromIndex(fields, index)
	if province == "" || city == "" {
		t.Fatal("known overseas values were discarded")
	}
	if len([]rune(province)) > profileRegionMaxRunes || len([]rune(city)) > profileRegionMaxRunes {
		t.Fatalf("region exceeds existing storage contract: %q / %q", province, city)
	}
}
