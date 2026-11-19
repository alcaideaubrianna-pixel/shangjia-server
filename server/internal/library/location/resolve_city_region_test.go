package location

import (
	"testing"

	"hotgo/internal/model/entity"
)

func TestResolveCityRegionDirectory(t *testing.T) {
	d := overseasTestDirectory()
	for _, tt := range []struct {
		value, province, city string
		matched               bool
	}{
		{"东京", "704", "东京", true},
		{"杭州", "330000", "330100", true},
		{"440300", "440000", "440300", true},
		{"10000002", "10000001", "10000002", true},
		{"新马泰", "701,702,703", "", true},
		{"新加坡", "701", "", true},
		{"日韩", "704,705", "", true},
		{"新马", "701,702", "", true},
		{"马泰", "702,703", "", true},
		{"位置：新马泰", "701,702,703", "", true},
		{"泰国/曼谷", "703", "曼谷", true},
		{"东京,440300,吉隆坡", "704,440000,702", "东京,440300,17021", true},
		{"新马泰,东京", "701,702,703,704", "东京", true},
		{"东京,东京", "704", "东京", true},
		{`["440300","330100"]`, "440000,330000", "440300,330100", true},
		{"维也纳", "奥地利", "维也纳", true},
		{"", "", "", false},
		{"未知城市", "", "未知城市", false},
		{"东京,未知城市", "", "东京,未知城市", false},
		{" 东京/未知城市 ", "", " 东京/未知城市 ", false},
		{"999999", "", "999999", false},
		{"新马泰风格", "", "新马泰风格", false},
		{"新/马/泰", "", "新/马/泰", false},
	} {
		t.Run(tt.value, func(t *testing.T) {
			province, city, matched := d.resolveCityRegion(tt.value)
			if province != tt.province || city != tt.city || matched != tt.matched {
				t.Fatalf("resolveCityRegion(%q) = (%q,%q,%v), want (%q,%q,%v)",
					tt.value, province, city, matched, tt.province, tt.city, tt.matched)
			}
		})
	}
}

func TestResolveCityRegionDoesNotChangeExplicitProvincePolicy(t *testing.T) {
	d := overseasTestDirectory()
	province, city, _ := d.normalize("泰国", "东京")
	if province != "703" || city != "东京" {
		t.Fatalf("ordinary normalization changed explicit province: %q,%q", province, city)
	}
	province, city, matched := d.resolveCityRegion("东京")
	if province != "704" || city != "东京" || !matched {
		t.Fatalf("independent lookup retained an unrelated old province: %q,%q,%v", province, city, matched)
	}
}

func TestResolveCityRegionUnknownOrAmbiguousNeverPartiallyMatches(t *testing.T) {
	d := overseasTestDirectory()
	d.citiesByName["重名"] = []*entity.SysProvinces{
		{Id: 440301, Pid: 440000, Level: 2, Title: "重名市"},
		{Id: 330101, Pid: 330000, Level: 2, Title: "重名市"},
	}
	for _, value := range []string{"重名", "东京,重名", "440300,unknown"} {
		province, city, matched := d.resolveCityRegion(value)
		if matched || province != "" || city != value {
			t.Fatalf("ambiguous/unknown input partially matched: %q => %q,%q,%v", value, province, city, matched)
		}
	}
	delete(d.byCode, "703")
	delete(d.provincesByName, "泰国")
	province, city, matched := d.resolveCityRegion("新马泰")
	if province != "701,702,泰国" || city != "" || !matched {
		t.Fatalf("missing dictionary country invented an ID: %q,%q,%v", province, city, matched)
	}
}
