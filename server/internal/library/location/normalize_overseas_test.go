package location

import (
	"strconv"
	"testing"

	"hotgo/internal/model/entity"
)

// IDs here belong only to the in-memory fixture, not to a country-code scheme.
func overseasTestDirectory() *regionDirectory {
	d := &regionDirectory{
		byCode:          make(map[string]*entity.SysProvinces),
		provincesByName: make(map[string][]*entity.SysProvinces),
		citiesByName:    make(map[string][]*entity.SysProvinces),
	}
	for _, row := range []*entity.SysProvinces{
		{Id: 701, Level: 1, Title: "新加坡"},
		{Id: 702, Level: 1, Title: "马来西亚"},
		{Id: 703, Level: 1, Title: "泰国"},
		{Id: 704, Level: 1, Title: "日本"},
		{Id: 705, Level: 1, Title: "韩国"},
		{Id: 17021, Pid: 702, Level: 2, Title: "吉隆坡"},
		{Id: 440000, Level: 1, Title: "广东省"},
		{Id: 440300, Pid: 440000, Level: 2, Title: "深圳市"},
		{Id: 330000, Level: 1, Title: "浙江省"},
		{Id: 330100, Pid: 330000, Level: 2, Title: "杭州市"},
		{Id: 10000001, Level: 1, Title: "字典长编码国家"},
		{Id: 10000002, Pid: 10000001, Level: 2, Title: "字典长编码城市"},
	} {
		d.byCode[strconv.FormatInt(row.Id, 10)] = row
		key := normalizeRegionLookupName(row.Title)
		if row.Level == 1 {
			d.provincesByName[key] = append(d.provincesByName[key], row)
		} else {
			d.citiesByName[key] = append(d.citiesByName[key], row)
		}
	}
	return d
}

func TestRegionDirectoryOverseas(t *testing.T) {
	d := overseasTestDirectory()
	for _, tt := range []struct {
		province, city, wantProvince, wantCity string
	}{
		{"新马泰", "", "701,702,703", ""},
		{"", "新马泰", "701,702,703", ""},
		{"", "新马", "701,702", ""},
		{"", "马泰", "702,703", ""},
		{"", "日韩", "704,705", ""},
		{"", "新加坡", "701", ""},
		{"", "位置：新马泰", "701,702,703", ""},
		{"日韩", "", "704,705", ""},
		{"新马", "", "701,702", ""},
		{"马泰", "", "702,703", ""},
		{"泰国", "曼谷", "703", "曼谷"},
		{"", "曼谷", "703", "曼谷"},
		{"曼谷", "", "703", "曼谷"},
		{"", "吉隆坡", "702", "17021"},
		{"702", "吉隆坡", "702", "17021"},
		{"", "泰国/曼谷", "703", "曼谷"},
		{"", "维也纳", "奥地利", "维也纳"},
		{"", "曼谷,吉隆坡", "703,702", "曼谷,17021"},
		{`["701","702"]`, "", "701,702", ""},
		{"999999", "", "999999", ""},
		{"", "999999", "", "999999"},
		{"10000001", "字典长编码城市", "10000001", "10000002"},
		{"", "10000002", "10000001", "10000002"},
		{"广东", "深圳", "440000", "440300"},
		{"广 东", "深 圳市", "440000", "440300"},
		{"广东", "杭州", "440000", "330100"},
		{"浙江", "深圳", "330000", "440300"},
		{"新马泰风格", "", "新马泰风格", ""},
		{"", "新马泰风格", "", "新马泰风格"},
		{"", "未知国家", "", "未知国家"},
		{"新", "泰", "新", "泰"},
		{"未知国家", "未知城市", "未知国家", "未知城市"},
	} {
		t.Run(tt.province+"/"+tt.city, func(t *testing.T) {
			province, city, _ := d.normalize(tt.province, tt.city)
			if province != tt.wantProvince || city != tt.wantCity {
				t.Fatalf("got (%q,%q), want (%q,%q)", province, city, tt.wantProvince, tt.wantCity)
			}
		})
	}
}

func TestRegionDirectoryWrongParentAmbiguousCity(t *testing.T) {
	d := overseasTestDirectory()
	d.citiesByName["重名"] = []*entity.SysProvinces{
		{Id: 330101, Pid: 330000, Level: 2, Title: "重名市"},
		{Id: 7021, Pid: 702, Level: 2, Title: "重名市"},
	}
	province, city, _ := d.normalize("广东", "重名")
	if province != "440000" || city != "重名" {
		t.Fatalf("ambiguous city under wrong parent was guessed: %q,%q", province, city)
	}
}

func TestRegionDirectoryMissingCountryDoesNotInventCode(t *testing.T) {
	d := overseasTestDirectory()
	delete(d.provincesByName, "泰国")
	delete(d.byCode, "703")
	province, city, _ := d.normalize("新马泰", "")
	if province != "701,702,泰国" || city != "" {
		t.Fatalf("missing country produced a synthetic code: %q,%q", province, city)
	}
	province, city, _ = d.normalize("", "新马泰")
	if province != "701,702,泰国" || city != "" {
		t.Fatalf("city country group invented a missing ID: %q,%q", province, city)
	}
	province, city, _ = d.normalize("", "曼谷")
	if province != "泰国" || city != "曼谷" {
		t.Fatalf("missing dictionary lost text city: %q,%q", province, city)
	}
}
