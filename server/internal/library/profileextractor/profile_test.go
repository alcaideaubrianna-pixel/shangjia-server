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
