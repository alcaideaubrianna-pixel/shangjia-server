package sys

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"hotgo/internal/library/location"
)

var materialImportMatchTextReplacer = strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "", "\u00a0", "", "　", "", "，", ",")

var materialImportRegionInlineLabel = regexp.MustCompile(`(?:所在省份|现居省份|常住省份|常驻省份|省份|所在省|现居省|所在城市|现居城市|常住城市|常驻城市|城市|所在市|所在地区|地区|所在国家|国家|国别|昵称|年龄|身高|体重)\s*[:：]`)

func materialImportUniqueRegionFields(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := values[:0]
	for _, value := range values {
		key := normalizeMaterialImportMatchText(value)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

type materialImportRegionKind uint8

const (
	materialImportProvince materialImportRegionKind = iota
	materialImportCity
	materialImportDistrict
	materialImportOverseasCity
)

type materialImportRegionTarget struct {
	kind       materialImportRegionKind
	provinceID int64
	cityID     int64
	cityText   string
	overseas   bool
}

type materialImportRegionPattern struct {
	name    string
	length  int
	targets []materialImportRegionTarget
}

type materialImportRegionNode struct {
	next    map[rune]int
	fail    int
	output  int // nearest terminal suffix, or zero (the root never matches)
	pattern int // pattern index plus one; zero means no terminal
}

// Immutable after compilation. Neither matching nor alias resolution walks the
// source dictionary. Failure links scan complex values once, including suffix
// matches needed for longest-name and city/district alias precedence.
type materialImportRegionMatcher struct {
	exact          map[string]int
	patterns       []materialImportRegionPattern
	nodes          []materialImportRegionNode
	municipalities map[int64]int64
}

func materialImportRegionCodesForFields(ctx context.Context, fields materialImportRegionFields) (province, city string, err error) {
	if len(fields.provinceTexts) == 0 && len(fields.cityTexts) == 0 {
		return "", "", nil
	}
	index, err := getLegacyCMSRegionIndex(ctx)
	if err != nil {
		return "", "", err
	}
	province, city = materialImportRegionCodesForFieldsFromIndex(fields, index)
	return province, city, nil
}

func materialImportRegionCodesForFieldsFromIndex(fields materialImportRegionFields, index *legacyCMSRegionIndex) (string, string) {
	if index == nil || len(fields.provinceTexts) == 0 && len(fields.cityTexts) == 0 {
		return "", ""
	}
	matcher := index.regionMatcher()
	provinces, cities := make(map[int64]struct{}), make(map[int64]struct{})
	textCities := make(map[string]struct{})
	for _, value := range fields.provinceTexts {
		matcher.matchValue(value, true, nil, provinces, cities, textCities)
	}
	// Province hints are a snapshot, not the progressively discovered results.
	allowed := make(map[int64]struct{}, len(provinces))
	for id := range provinces {
		allowed[id] = struct{}{}
	}
	for _, value := range fields.cityTexts {
		matcher.matchValue(value, false, allowed, provinces, cities, textCities)
	}
	if len(provinces) == 0 {
		for _, value := range fields.provinceTexts {
			matcher.matchValue(value, false, nil, provinces, cities, textCities)
		}
	}
	for id := range provinces {
		if cityID := matcher.municipalities[id]; cityID > 0 {
			cities[cityID] = struct{}{}
		}
	}
	city := joinMaterialImportRegionIDs(cities)
	if len(textCities) > 0 {
		names := make([]string, 0, len(textCities))
		for name := range textCities {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			next := name
			if city != "" {
				next = city + "," + name
			}
			if len([]rune(next)) <= profileRegionMaxRunes {
				city = next
			}
		}
	}
	return joinMaterialImportRegionIDs(provinces), city
}

func (index *legacyCMSRegionIndex) regionMatcher() *materialImportRegionMatcher {
	index.compileOnce.Do(func() {
		index.compiled = compileMaterialImportRegionMatcher(index)
	})
	return index.compiled
}

func compileMaterialImportRegionMatcher(index *legacyCMSRegionIndex) *materialImportRegionMatcher {
	m := &materialImportRegionMatcher{
		exact: make(map[string]int), nodes: []materialImportRegionNode{{next: make(map[rune]int)}},
		municipalities: make(map[int64]int64),
	}
	provinceNames := make(map[string][]int64)
	overseasCities := location.OverseasRegionCities()
	countryNames := make(map[string]bool, len(overseasCities))
	for country := range overseasCities {
		countryNames[normalizeMaterialImportMatchText(country)] = true
	}
	for name, row := range index.provincesByName {
		if row == nil {
			continue
		}
		target := materialImportRegionTarget{kind: materialImportProvince, provinceID: row.Id, overseas: countryNames[normalizeMaterialImportMatchText(row.Title)] || countryNames[normalizeMaterialImportMatchText(name)]}
		m.add(strconv.FormatInt(row.Id, 10), target)
		for _, alias := range append(materialImportRegionTitleAliases(row.Title), name) {
			m.add(alias, target)
			key := normalizeMaterialImportMatchText(alias)
			provinceNames[key] = append(provinceNames[key], row.Id)
		}
		if materialImportMunicipalityProvince(row.Id) {
			for _, child := range index.childrenByPid[row.Id] {
				if child != nil && child.Level == 2 && (m.municipalities[row.Id] == 0 || child.Id < m.municipalities[row.Id]) {
					m.municipalities[row.Id] = child.Id
				}
			}
		}
	}
	for name, rows := range index.citiesByName {
		for _, row := range rows {
			if row == nil {
				continue
			}
			target := materialImportRegionTarget{kind: materialImportCity, provinceID: row.Pid, cityID: row.Id}
			m.add(name, target)
			for _, alias := range materialImportRegionTitleAliases(row.Title) {
				m.add(alias, target)
			}
		}
	}
	for name, rows := range index.districtsByName {
		for _, row := range rows {
			if row == nil {
				continue
			}
			city := index.optionsById[row.Pid]
			if city == nil || city.Level != 2 {
				continue
			}
			target := materialImportRegionTarget{kind: materialImportDistrict, provinceID: city.Pid, cityID: city.Id}
			m.add(name, target)
			for _, alias := range materialImportRegionTitleAliases(row.Title) {
				m.add(alias, target)
			}
		}
	}
	for country, cities := range overseasCities {
		for _, provinceID := range provinceNames[normalizeMaterialImportMatchText(country)] {
			for _, city := range cities {
				if normalizeMaterialImportMatchText(city) == normalizeMaterialImportMatchText(country) {
					continue // country-only Singapore/Cambodia must not invent a city
				}
				hasNumericCity := false
				if patternID, ok := m.exact[normalizeMaterialImportMatchText(city)]; ok {
					for _, target := range m.patterns[patternID].targets {
						if target.kind == materialImportCity && target.provinceID == provinceID {
							hasNumericCity = true
							break
						}
					}
				}
				if !hasNumericCity {
					m.add(city, materialImportRegionTarget{kind: materialImportOverseasCity, provinceID: provinceID, cityText: city, overseas: true})
				}
			}
		}
	}
	m.buildFailureLinks()
	return m
}

func (m *materialImportRegionMatcher) add(name string, target materialImportRegionTarget) {
	name = normalizeMaterialImportMatchText(name)
	length := len([]rune(name))
	if length < 2 {
		return
	}
	patternID, exists := m.exact[name]
	if !exists {
		patternID = len(m.patterns)
		m.exact[name] = patternID
		m.patterns = append(m.patterns, materialImportRegionPattern{name: name, length: length})
		// Numeric dictionary IDs are exact field tokens, never substrings.
		if _, err := strconv.ParseInt(name, 10, 64); err == nil {
			m.patterns[patternID].targets = append(m.patterns[patternID].targets, target)
			return
		}
		node := 0
		for _, r := range name {
			next, ok := m.nodes[node].next[r]
			if !ok {
				next = len(m.nodes)
				m.nodes = append(m.nodes, materialImportRegionNode{next: make(map[rune]int)})
				m.nodes[node].next[r] = next
			}
			node = next
		}
		m.nodes[node].pattern = patternID + 1
	}
	for _, previous := range m.patterns[patternID].targets {
		if previous == target {
			return
		}
	}
	m.patterns[patternID].targets = append(m.patterns[patternID].targets, target)
}

func (m *materialImportRegionMatcher) buildFailureLinks() {
	queue := make([]int, 0, len(m.nodes))
	for _, child := range m.nodes[0].next {
		queue = append(queue, child)
	}
	for cursor := 0; cursor < len(queue); cursor++ {
		node := queue[cursor]
		for r, child := range m.nodes[node].next {
			failure := m.nodes[node].fail
			for failure != 0 && m.nodes[failure].next[r] == 0 {
				failure = m.nodes[failure].fail
			}
			m.nodes[child].fail = m.nodes[failure].next[r]
			suffix := m.nodes[child].fail
			if m.nodes[suffix].pattern != 0 {
				m.nodes[child].output = suffix
			} else {
				m.nodes[child].output = m.nodes[suffix].output
			}
			queue = append(queue, child)
		}
	}
}

func materialImportRegionTargetAllowed(target materialImportRegionTarget, provinceOnly bool, allowed map[int64]struct{}) bool {
	if provinceOnly {
		return target.kind == materialImportProvince
	}
	if target.kind == materialImportProvince && !target.overseas && !materialImportMunicipalityProvince(target.provinceID) {
		return false
	}
	return materialImportRegionParentAllowed(target.provinceID, allowed)
}

// Resolve each name before emitting IDs. An ambiguous alias is ignored unless a
// province hint leaves a single destination. A real city beats a district alias
// of the same spelling; an explicit longer district title still wins by length.
func (m *materialImportRegionMatcher) resolve(patternID int, provinceOnly bool, allowed map[int64]struct{}) (materialImportRegionTarget, bool) {
	var result materialImportRegionTarget
	found, ambiguous := false, false
	bestRank := -1
	for _, target := range m.patterns[patternID].targets {
		if !materialImportRegionTargetAllowed(target, provinceOnly, allowed) {
			continue
		}
		rank := 0
		if target.kind == materialImportCity {
			rank = 1
		}
		if rank < bestRank {
			continue
		}
		if rank > bestRank {
			result, found, ambiguous, bestRank = target, true, false, rank
			continue
		}
		if !found {
			result, found = target, true
		} else if result.provinceID != target.provinceID || result.cityID != target.cityID || result.cityText != target.cityText {
			ambiguous = true
		}
	}
	return result, found && !ambiguous
}

func (m *materialImportRegionMatcher) matchValue(value string, provinceOnly bool, allowed map[int64]struct{}, provinces, cities map[int64]struct{}, textCities map[string]struct{}) {
	// Expansion is limited to exact tokens in an already extracted location
	// value. Never substitute abbreviations in the profile body.
	for _, term := range location.ExpandRegionTerms(value) {
		text := normalizeMaterialImportMatchText(term)
		emit := func(target materialImportRegionTarget) {
			if target.provinceID > 0 {
				provinces[target.provinceID] = struct{}{}
			}
			if target.cityID > 0 {
				cities[target.cityID] = struct{}{}
			}
			if target.cityText != "" {
				textCities[target.cityText] = struct{}{}
			}
		}
		if id, ok := m.exact[text]; ok {
			if target, resolved := m.resolve(id, provinceOnly, allowed); resolved {
				emit(target)
			}
			continue
		}
		runes := []rune(text)
		// At each start retain only the longest matching name. A subsequent
		// left-to-right pass suppresses contained suffixes without pairwise
		// comparisons between dictionary names.
		best := make([]int, len(runes))
		node := 0
		for end, r := range runes {
			for node != 0 && m.nodes[node].next[r] == 0 {
				node = m.nodes[node].fail
			}
			node = m.nodes[node].next[r]
			for terminal := node; terminal != 0; terminal = m.nodes[terminal].output {
				id := m.nodes[terminal].pattern - 1
				if id < 0 {
					continue
				}
				pattern := m.patterns[id]
				start := end + 1 - pattern.length
				// Latin names cannot match fragments of unrelated words.
				first, last := runes[start], runes[end]
				if first <= unicode.MaxASCII && unicode.IsLetter(first) && start > 0 && unicode.IsLetter(runes[start-1]) ||
					last <= unicode.MaxASCII && unicode.IsLetter(last) && end+1 < len(runes) && unicode.IsLetter(runes[end+1]) {
					continue
				}
				if _, ok := m.resolve(id, provinceOnly, allowed); ok && (best[start] == 0 || m.patterns[best[start]-1].length < pattern.length) {
					best[start] = id + 1
				}
			}
		}
		for start := 0; start < len(best); start++ {
			if best[start] == 0 {
				continue
			}
			id := best[start] - 1
			if target, ok := m.resolve(id, provinceOnly, allowed); ok {
				emit(target)
			}
			start += m.patterns[id].length - 1
		}
	}
}
