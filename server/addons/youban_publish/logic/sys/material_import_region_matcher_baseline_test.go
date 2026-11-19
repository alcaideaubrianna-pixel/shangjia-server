package sys

import "strings"

// Historical dictionary loops copied from dfad8b3c, test-only. Production has
// only the compiled matcher. Both benchmark variants use the same dictionary
// and input; overseas group results intentionally differ because the baseline
// predates overseas token expansion. This is not an equivalent-work speed claim.

func baselineMaterialImportRegionCodesFromIndex(text string, index *legacyCMSRegionIndex) (string, string) {
	if index == nil {
		return "", ""
	}
	fields := materialImportRegionFieldsFromText(text)
	provinceIDs := make(map[int64]struct{})
	cityIDs := make(map[int64]struct{})
	for _, provinceText := range fields.provinceTexts {
		normalizedText := normalizeMaterialImportMatchText(provinceText)
		for name, province := range index.provincesByName {
			if province != nil && materialImportRegionNameMatches(normalizedText, name) {
				provinceIDs[province.Id] = struct{}{}
			}
		}
	}
	allowedProvinceIDs := make(map[int64]struct{}, len(provinceIDs))
	for provinceID := range provinceIDs {
		allowedProvinceIDs[provinceID] = struct{}{}
	}
	for _, cityText := range fields.cityTexts {
		baselineMaterialImportMatchCityText(cityText, index, allowedProvinceIDs, provinceIDs, cityIDs)
	}
	if len(provinceIDs) == 0 {
		for _, provinceText := range fields.provinceTexts {
			baselineMaterialImportMatchCityText(provinceText, index, nil, provinceIDs, cityIDs)
		}
	}
	for provinceID := range provinceIDs {
		if !materialImportMunicipalityProvince(provinceID) {
			continue
		}
		for _, city := range index.childrenByPid[provinceID] {
			if city != nil && city.Level == 2 {
				cityIDs[city.Id] = struct{}{}
				break
			}
		}
	}
	return joinMaterialImportRegionIDs(provinceIDs), joinMaterialImportRegionIDs(cityIDs)
}

func baselineMaterialImportMatchCityText(text string, index *legacyCMSRegionIndex, allowedProvinceIDs map[int64]struct{}, provinceIDs map[int64]struct{}, cityIDs map[int64]struct{}) {
	normalizedText := normalizeMaterialImportMatchText(text)
	for name, province := range index.provincesByName {
		if province == nil || !materialImportMunicipalityProvince(province.Id) || !materialImportRegionNameMatches(normalizedText, name) {
			continue
		}
		if len(allowedProvinceIDs) > 0 {
			if _, ok := allowedProvinceIDs[province.Id]; !ok {
				continue
			}
		}
		provinceIDs[province.Id] = struct{}{}
	}
	for name, cities := range index.citiesByName {
		for _, city := range cities {
			if city == nil || !baselineMaterialImportRegionOptionMatches(normalizedText, name, city) || !materialImportRegionParentAllowed(city.Pid, allowedProvinceIDs) {
				continue
			}
			cityIDs[city.Id] = struct{}{}
			if city.Pid > 0 {
				provinceIDs[city.Pid] = struct{}{}
			}
		}
	}
	for name, districts := range index.districtsByName {
		for _, district := range districts {
			if district == nil || district.Pid <= 0 || !baselineMaterialImportRegionOptionMatches(normalizedText, name, district) || baselineMaterialImportRegionNameShadowed(normalizedText, name, index.districtsByName) || baselineMaterialImportDistrictAliasConflictsWithCity(normalizedText, name, district, index) {
				continue
			}
			city := index.optionsById[district.Pid]
			if city == nil || city.Level != 2 || !materialImportRegionParentAllowed(city.Pid, allowedProvinceIDs) {
				continue
			}
			cityIDs[city.Id] = struct{}{}
			if city.Pid > 0 {
				provinceIDs[city.Pid] = struct{}{}
			}
		}
	}
}

func baselineMaterialImportDistrictAliasConflictsWithCity(normalizedText string, name string, district *legacyCMSRegionOption, index *legacyCMSRegionIndex) bool {
	if district == nil || index == nil || materialImportRegionNameMatches(normalizedText, name) {
		return false
	}
	for _, alias := range materialImportRegionTitleAliases(district.Title) {
		if !materialImportRegionNameMatches(normalizedText, alias) {
			continue
		}
		normalizedAlias := normalizeMaterialImportMatchText(alias)
		for cityName := range index.citiesByName {
			if normalizeMaterialImportMatchText(cityName) == normalizedAlias {
				return true
			}
		}
	}
	return false
}

func baselineMaterialImportRegionOptionMatches(normalizedText string, name string, option *legacyCMSRegionOption) bool {
	if materialImportRegionNameMatches(normalizedText, name) {
		return true
	}
	if option == nil {
		return false
	}
	for _, alias := range materialImportRegionTitleAliases(option.Title) {
		if materialImportRegionNameMatches(normalizedText, alias) {
			return true
		}
	}
	return false
}

func baselineMaterialImportRegionNameShadowed(normalizedText string, name string, candidates map[string][]*legacyCMSRegionOption) bool {
	normalizedName := normalizeMaterialImportMatchText(name)
	for candidate := range candidates {
		normalizedCandidate := normalizeMaterialImportMatchText(candidate)
		if len([]rune(normalizedCandidate)) <= len([]rune(normalizedName)) || !strings.Contains(normalizedCandidate, normalizedName) {
			continue
		}
		if materialImportRegionNameMatches(normalizedText, candidate) {
			return true
		}
	}
	return false
}
