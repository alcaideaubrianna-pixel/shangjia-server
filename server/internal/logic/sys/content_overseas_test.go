package sys

import (
	"testing"

	"hotgo/internal/library/location"
	"hotgo/internal/model/input/sysin"
)

func TestContentOverseasOptionsKeepDatabaseIDsAndAddSeeds(t *testing.T) {
	country := &sysin.ContentProfileRegionOption{
		Label: "马来西亚", Province: "马来西亚", Value: "702", Code: "702", Type: "province",
		Children: []*sysin.ContentProfileRegionOption{
			{Label: "吉隆坡", City: "吉隆坡", Province: "马来西亚", Value: "17021", Code: "17021"},
		},
	}
	list := appendOverseasRegionOptions([]*sysin.ContentProfileRegionOption{
		{Label: "全国"}, country,
	})
	if country.Value != "702" || country.Code != "702" || country.Type != "country" || len(country.Children) != 2 {
		t.Fatalf("existing country lost IDs or missing seed city: %+v", country)
	}
	if country.Children[0].Value != "17021" || country.Children[1].Value != "马来西亚/槟城" {
		t.Fatalf("existing city ID or legacy fallback changed: %+v", country.Children)
	}
	for _, item := range list {
		if item.Province == "英国" {
			if item.Value != "英国" || len(item.Children) != 2 || item.Children[0].Value != "英国/伦敦" {
				t.Fatalf("missing country must use legacy text values: %+v", item)
			}
		}
	}
	again := appendOverseasRegionOptions(list)
	if len(again) != len(list) || len(country.Children) != 2 {
		t.Fatal("seed merge is not idempotent")
	}
}

func TestContentCityProvinceMapUsesSharedOverseasDirectory(t *testing.T) {
	for country, cities := range location.OverseasRegionCities() {
		for _, city := range cities {
			if cityProvinceMap[city] != country {
				t.Fatalf("%q belongs to %q, got %q", city, country, cityProvinceMap[city])
			}
		}
	}
	if cityProvinceMap["深圳"] != "广东" || cityProvinceMap["北京"] != "北京" {
		t.Fatal("domestic city mappings changed")
	}
}
