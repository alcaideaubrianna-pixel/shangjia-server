package profileextractor

import "testing"

func TestParseProfileFields(t *testing.T) {
	result := Parse("身高：168\n体重: 98\n胸围 36C")
	if result.Height != 168 || result.Weight != 98 || result.Cup != "C" {
		t.Fatalf("unexpected fields: %+v", result)
	}
}

func TestParseUnknownFieldsRemainZero(t *testing.T) {
	result := Parse("性格温柔，接受长期")
	if result.Height != 0 || result.Weight != 0 || result.Cup != "" {
		t.Fatalf("unexpected fields: %+v", result)
	}
}

func TestMergeKeepsExistingValues(t *testing.T) {
	result := Merge("身高：168 体重：98 罩杯：C", 170, 0, "")
	if result.Height != 170 || result.Weight != 98 || result.Cup != "C" {
		t.Fatalf("unexpected merged fields: %+v", result)
	}
}

func TestParseFeiNiuUnitLabels(t *testing.T) {
	result := Parse("身高(cm)：156\n体重(斤)：86\n罩杯：A")
	if result.Height != 156 || result.Weight != 86 || result.Cup != "A" {
		t.Fatalf("unexpected unit-labelled fields: %+v", result)
	}
}

func TestParseFeiNiuFullWidthUnitLabels(t *testing.T) {
	result := Parse("💟身高（cm）:166\n💟体重（斤）:90\n💟罩杯:C")
	if result.Height != 166 || result.Weight != 90 || result.Cup != "C" {
		t.Fatalf("unexpected full-width unit-labelled fields: %+v", result)
	}
}

func TestParseFeiNiuCompactCup(t *testing.T) {
	result := Parse("净身高172 体重103 34C+真")
	if result.Height != 172 || result.Weight != 103 || result.Cup != "C" {
		t.Fatalf("unexpected compact fields: %+v", result)
	}
}

func TestMergeRejectsGenericTagAsCup(t *testing.T) {
	result := Merge("身高：168 体重：98 罩杯：C", 0, 0, "12,18")
	if result.Cup != "C" {
		t.Fatalf("generic tag polluted cup: %+v", result)
	}
}

func TestNormalizeStructuredCup(t *testing.T) {
	if cup := NormalizeCup(" d罩杯 "); cup != "D" {
		t.Fatalf("unexpected normalized cup: %q", cup)
	}
	if cup := NormalizeCup("颜值在线"); cup != "" {
		t.Fatalf("generic tag should not be a cup: %q", cup)
	}
}

func TestRefreshReplacesMentionedAndPreservesMissing(t *testing.T) {
	result := Refresh("身高：170", Fields{Height: 168, Weight: 98, Cup: "C"})
	if result.Height != 170 || result.Weight != 98 || result.Cup != "C" {
		t.Fatalf("unexpected refreshed fields: %+v", result)
	}
}

func TestParseFeiNiuObfuscatedUnicode(t *testing.T) {
	result := Parse("身︂高︃：162⁠­\n体︊重：110­⁠\n罩︄杯：C")
	if result.Height != 162 || result.Weight != 110 || result.Cup != "C" {
		t.Fatalf("unexpected obfuscated fields: %+v", result)
	}
}

func TestParseFeiNiuUnitsAndRanges(t *testing.T) {
	tests := []struct {
		text   string
		height int
		weight int
	}{
		{text: "身高 1.68米 体重 48kg", height: 168, weight: 96},
		{text: "身高 168cm 体重 65", height: 168, weight: 130},
		{text: "身高 161厘米 体重 42公斤", height: 161, weight: 84},
		{text: "身高 139 体重 49斤", height: 0, weight: 0},
		{text: "身高 211 体重 301斤", height: 0, weight: 0},
		{text: "升高：163 体重：78", height: 163, weight: 78},
		{text: "身高：1.67 体重：1235", height: 167, weight: 123},
		{text: "身高：168 体重：1043", height: 168, weight: 104},
	}
	for _, test := range tests {
		result := Parse(test.text)
		if result.Height != test.height || result.Weight != test.weight {
			t.Fatalf("text %q got %+v", test.text, result)
		}
	}
}

func TestParseFeiNiuCompactFields(t *testing.T) {
	result := Parse("资料 23 / 168 / 98 C杯")
	if result.Height != 168 || result.Weight != 98 || result.Cup != "C" {
		t.Fatalf("unexpected compact fields: %+v", result)
	}
}

func TestParseFeiNiuCupVariants(t *testing.T) {
	tests := map[string]string{
		"胸围：36c":       "C",
		"净身高172 34C➕真": "C",
		"罩杯：H+":        "H",
		"罩杯：小b":        "B",
		"罩杯：快C":        "C",
		"罩杯：·B":        "B",
		"罩杯：bc":        "B",
		"罩杯：e和f之间":     "E",
		"罩呗：85C":       "C",
		"罩杯：未知":        "",
	}
	for text, want := range tests {
		if got := Parse(text).Cup; got != want {
			t.Fatalf("text %q cup %q, want %q", text, got, want)
		}
	}
}
