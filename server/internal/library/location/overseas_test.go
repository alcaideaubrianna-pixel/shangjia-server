package location

import (
	"reflect"
	"testing"
)

func TestOverseasRegionCitiesCopy(t *testing.T) {
	cities := OverseasRegionCities()
	cities["日本"][0] = "changed"
	delete(cities, "泰国")
	cities["unknown"] = []string{"unknown"}
	next := OverseasRegionCities()
	if next["日本"][0] != "东京" || len(next["泰国"]) != 1 || next["unknown"] != nil {
		t.Fatal("caller mutated the shared directory")
	}
}

func TestExpandRegionTerms(t *testing.T) {
	tests := []struct {
		value string
		want  []string
	}{
		{"", nil},
		{"新马泰", []string{"新加坡", "马来西亚", "泰国"}},
		{"新马", []string{"新加坡", "马来西亚"}},
		{"马泰", []string{"马来西亚", "泰国"}},
		{"日韩", []string{"日本", "韩国"}},
		{"省份：新马泰/日韩，泰国", []string{"新加坡", "马来西亚", "泰国", "日本", "韩国"}},
		{"位置: 日韩；城市：东京", []string{"日本", "韩国", "东京"}},
		{"新/马/泰", []string{"新", "马", "泰"}},
		{"新乡,马鞍山,泰安", []string{"新乡", "马鞍山", "泰安"}},
		{"喜欢新马泰风格", []string{"喜欢新马泰风格"}},
		{"日韩语学习", []string{"日韩语学习"}},
		{"未知地点", []string{"未知地点"}},
		{`["701","702"]`, []string{"701", "702"}},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := ExpandRegionTerms(tt.value); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExpandRegionTerms(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
