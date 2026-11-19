package sys

import (
	"sort"

	"hotgo/internal/model/input/sysin"
)

// appendOverseasRegionOptions enriches DB countries without replacing their IDs
// or existing city nodes. Only missing seed cities use the legacy text value.
func appendOverseasRegionOptions(list []*sysin.ContentProfileRegionOption) []*sysin.ContentProfileRegionOption {
	existing := make(map[string]*sysin.ContentProfileRegionOption, len(list))
	for _, item := range list {
		if item != nil {
			existing[item.Province] = item
		}
	}
	countries := make([]string, 0, len(overseasRegionSeeds))
	for country := range overseasRegionSeeds {
		countries = append(countries, country)
	}
	sort.Strings(countries)
	for _, country := range countries {
		item := existing[country]
		if item == nil {
			item = &sysin.ContentProfileRegionOption{
				Label: country, Value: country, Code: country, Type: "country",
				Province: country, Children: []*sysin.ContentProfileRegionOption{},
			}
			list = append(list, item)
		}
		item.Type = "country"
		cities := make(map[string]bool, len(item.Children))
		for _, city := range item.Children {
			if city != nil {
				cities[city.City] = true
			}
		}
		for _, city := range overseasRegionSeeds[country] {
			if city == country || cities[city] {
				continue
			}
			item.Children = append(item.Children, &sysin.ContentProfileRegionOption{
				Label: city, Value: country + "/" + city, Code: country + "/" + city,
				Type: "city", Province: country, City: city,
			})
			cities[city] = true
		}
	}
	return list
}
