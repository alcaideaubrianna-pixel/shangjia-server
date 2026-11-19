package location

import (
	"strings"
	"unicode"
)

var overseasRegionCities = OverseasRegionCities()

var overseasCityCountries = func() map[string]string {
	countries := make(map[string]string)
	for country, cities := range overseasRegionCities {
		for _, city := range cities {
			countries[city] = country
		}
	}
	return countries
}()

// OverseasRegionCities returns a fresh directory and fresh city slices. Names
// come from the existing content options; the directory never assigns DB IDs.
func OverseasRegionCities() map[string][]string {
	return map[string][]string{
		"马来西亚": {"吉隆坡", "槟城"}, "新加坡": {"新加坡"},
		"日本": {"东京", "大阪"}, "泰国": {"曼谷"},
		"美国": {"纽约", "旧金山", "洛杉矶"}, "英国": {"伦敦", "利兹"},
		"澳大利亚": {"堪培拉", "墨尔本"}, "韩国": {"首尔"},
		"奥地利": {"维也纳"}, "菲律宾": {"马尼拉"},
		"柬埔寨": {"柬埔寨"}, "阿联酋": {"迪拜"},
	}
}

// ExpandRegionTerms expands exact tokens in region values, including explicit
// location labels. Unknown terms survive; free-text substrings and single
// character abbreviations are never expanded.
func ExpandRegionTerms(value string) []string {
	var result []string
	seen := make(map[string]bool)
	for _, token := range strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(",，、/／|｜;；[]\"'", r)
	}) {
		for _, label := range []string{"所在省份", "现居省份", "常住省份", "常驻省份", "省份", "所在城市", "现居城市", "常住城市", "常驻城市", "城市", "所在地区", "现居地区", "所在位置", "所在地", "现居地", "地区", "位置", "国家"} {
			if strings.HasPrefix(token, label+":") || strings.HasPrefix(token, label+"：") {
				token = strings.TrimLeft(strings.TrimPrefix(token, label), ":：")
				break
			}
		}
		terms := []string{token}
		switch token {
		case "新马泰":
			terms = []string{"新加坡", "马来西亚", "泰国"}
		case "新马":
			terms = []string{"新加坡", "马来西亚"}
		case "马泰":
			terms = []string{"马来西亚", "泰国"}
		case "日韩":
			terms = []string{"日本", "韩国"}
		}
		for _, term := range terms {
			if term != "" && !seen[term] {
				seen[term] = true
				result = append(result, term)
			}
		}
	}
	return result
}
