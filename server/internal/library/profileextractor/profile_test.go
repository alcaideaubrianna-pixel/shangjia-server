package profileextractor

import (
	"testing"
	"time"
)

func TestParseProfileFields(t *testing.T) {
	result := Parse("年龄：23\n身高：168\n体重: 98\n胸围 36C")
	if result.Age != 23 || result.Height != 168 || result.Weight != 98 || result.Cup != "C" {
		t.Fatalf("unexpected fields: %+v", result)
	}
}

func TestParseBirthYearAsAge(t *testing.T) {
	currentYear := time.Now().Year()
	for _, test := range []struct {
		text string
		year int
	}{
		{"年龄:02", 2002},
		{"年龄：[05年]", 2005},
		{"Age：06", 2006},
	} {
		if result := Parse(test.text); result.Age != currentYear-test.year {
			t.Fatalf("text %q got %+v, want age %d", test.text, result, currentYear-test.year)
		}
	}
	if result := Parse("03年 175净身高"); result.Age != 0 {
		t.Fatalf("unlabelled birth year unexpectedly parsed age: %+v", result)
	}
}

func TestRefreshAgeReplacesMentionedAndPreservesMissing(t *testing.T) {
	if result := Refresh("年龄：24", Fields{Age: 23}); result.Age != 24 {
		t.Fatalf("unexpected refreshed age: %+v", result)
	}
	if result := Refresh("身高：170", Fields{Age: 23}); result.Age != 23 {
		t.Fatalf("unexpected preserved age: %+v", result)
	}
}

func TestParseVirginTriState(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"是否是处女：YES", 1},
		{"是否chu女：是", 1},
		{"是否c:首次", 1},
		{"是否是处女：不", 2},
		{"是否chu女:no", 2},
		{"是不是处女：不是", 2},
		{"处女座：天秤座", 0},
		{"是否是处女：不确定", 0},
	}
	for _, test := range tests {
		if got := Parse(test.text).Virgin; got != test.want {
			t.Fatalf("text %q virgin %d, want %d", test.text, got, test.want)
		}
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
	kgResult := Parse("广州 24/160/45 B")
	if kgResult.Height != 160 || kgResult.Weight != 90 || kgResult.Cup != "B" {
		t.Fatalf("unexpected compact kg fields: %+v", kgResult)
	}
}

func TestParseFeiNiuCupVariants(t *testing.T) {
	tests := map[string]string{
		"胸围：36c":          "C",
		"净身高172 34C➕真":    "C",
		"罩杯：H+":           "H",
		"罩杯：小b":           "B",
		"罩杯：快C":           "C",
		"罩杯：·B":           "B",
		"罩杯：bc":           "B",
		"罩杯：e和f之间":        "E",
		"罩呗：85C":          "C",
		"罩杯：未知":           "",
		"罩杯：无":            "",
		"罩杯：无胸":           "",
		"罩杯：否":            "",
		"罩杯：…":            "",
		"罩杯：实习生":          "",
		"胸围：实习":           "",
		"罩杯：86（胸围）":       "",
		"罩杯：34罩":          "",
		"罩杯：36嘟":          "",
		"罩杯：8➕":           "",
		"罩杯：34v":          "",
		"罩杯：ā":            "A",
		"罩杯：吧➕":           "B",
		"罩 C":             "C",
		"罩杯：把":            "B",
		"罩杯：有浮动":          "",
		"罩杯：不":            "",
		"罩杯：（三围）71 60 80": "",
		"罩杯：我没测过":         "",
		"罩杯：暂时不知":         "",
		"罩呗：没问":           "",
		"罩杯：不了解":          "",
		"罩杯：不懂":           "",
		"罩杯：不懂没测过":        "",
		"罩杯：男":            "",
		"罩杯：·不是":          "",
		"罩杯：忘了":           "",
		"🐻：妮妮":            "",
		"罩杯：这个不太清楚啊，微胖":   "",
		"罩杯：75kg":         "",
		"罩杯：36~37":        "",
		"罩杯：我也不知道。没有量过..": "",
		"罩杯：？": "",
		"罩杯：没有测量过，但是也不小":         "",
		"罩杯：·不清楚":                "",
		"罩杯：·不知道":                "",
		"罩杯：70吧":                 "",
		"罩杯：正常范围":                "",
		"罩杯：不确定但是不大":             "",
		"罩杯：不确定（自我感觉不小）":         "",
		"罩杯：不小":                  "",
		"罩杯：很大肉眼可见的大":            "",
		"罩杯：比鸡蛋大出一圈":             "",
		"罩杯：不是飞机 不大":             "",
		"罩杯：不大不小":                "",
		"罩杯：不大":                  "",
		"胸围：没测过反正不是平的":           "",
		"罩杯：没测过，偏小":              "",
		"罩杯：不清楚略小":               "",
		"罩杯：一个手差不多兜得住":           "",
		"罩杯：·没量过":                "",
		"罩杯：小（具体不知道）":            "",
		"罩杯：不知豆":                 "",
		"罩杯：不清楚的":                "",
		"罩杯：大概一个苹果大雪生点":          "",
		"罩杯：不清楚啊老铁":              "",
		"罩杯：不太清楚唉":               "",
		"罩杯：根据体重买的 我不知道":         "",
		"罩杯：具体不知道，买的时候是38BCD都能穿": "",
		"罩杯：是男娘":                 "",
		"罩杯: 伪娘（我是男人）":           "",
	}
	for text, want := range tests {
		if got := Parse(text).Cup; got != want {
			t.Fatalf("text %q cup %q, want %q", text, got, want)
		}
	}
	unknown := Analyze("罩杯: 伪娘（我是男人） 职业: 模特")
	if !unknown.CupSourceInvalid || !unknown.CupMentioned || unknown.Cup != "" {
		t.Fatalf("unexpected non-applicable cup analysis: %+v", unknown)
	}
	ellipsis := Analyze("罩杯：… 职业：学生")
	if !ellipsis.CupSourceInvalid || ellipsis.Cup != "" {
		t.Fatalf("unexpected ellipsis cup analysis: %+v", ellipsis)
	}
}

func TestAnalyzeHistoricalMalformedFields(t *testing.T) {
	tests := []struct {
		text   string
		fields Fields
	}{
		{"身高:B 体重:170 罩杯:90", Fields{Height: 170, Weight: 90, Cup: "B"}},
		{"身高:100 体重:174 罩杯:B", Fields{Height: 174, Weight: 100, Cup: "B"}},
		{"罩杯：170 身高：86 体重：小b", Fields{Height: 170, Weight: 86, Cup: "B"}},
		{"年龄：[05年]身高：[165 cm]。 体重：[45kg] 🐻:C➕", Fields{Age: time.Now().Year() - 2005, Height: 165, Weight: 90, Cup: "C"}},
		{"03年 175净身高 36C", Fields{Height: 175, Cup: "C"}},
		{"170净高 42kg", Fields{Height: 170, Weight: 84}},
		{"秘书兼职178高 36F真胸", Fields{Height: 178, Cup: "F"}},
		{"174E网红 校花", Fields{Height: 174, Cup: "E"}},
		{"体重8 92 (重量在胸)", Fields{Weight: 92}},
		{"身高:一米五九 体重:九十几斤 罩杯:b", Fields{Height: 159, Weight: 90, Cup: "B"}},
		{"身高:一米七 体重:105斤 罩杯:B", Fields{Height: 170, Weight: 105, Cup: "B"}},
		{"Height：身高/170 Weight：体重/94 Bust：胸围/c", Fields{Height: 170, Weight: 94, Cup: "C"}},
		{"Age：06 Height：/163 Bust：/E", Fields{Age: time.Now().Year() - 2006, Height: 163, Cup: "E"}},
		{"高 03 175厦航空姐 🐻天然C", Fields{Height: 175, Cup: "C"}},
	}
	for _, test := range tests {
		if got := Analyze(test.text).Fields; got != test.fields {
			t.Fatalf("text %q got %+v, want %+v", test.text, got, test.fields)
		}
	}
	unknown := Analyze("身高:165 体重:94 罩杯:不清楚")
	if !unknown.CupSourceInvalid || unknown.Cup != "" {
		t.Fatalf("unexpected unknown cup analysis: %+v", unknown)
	}
	invalidWeight := Analyze("身高:166 体重:20 罩杯:B")
	if !invalidWeight.WeightSourceInvalid || invalidWeight.Weight != 0 {
		t.Fatalf("unexpected invalid weight analysis: %+v", invalidWeight)
	}
	invalidHeightText := Analyze("身高:兔子 体重:96 罩杯:a")
	if !invalidHeightText.HeightSourceInvalid || invalidHeightText.Height != 0 {
		t.Fatalf("unexpected invalid height text analysis: %+v", invalidHeightText)
	}
	invalidJin := Analyze("真熊体重47斤")
	if !invalidJin.WeightSourceInvalid || invalidJin.Weight != 0 {
		t.Fatalf("unexpected explicit invalid jin analysis: %+v", invalidJin)
	}
	invalidWeightText := Analyze("体重:图 罩杯:B+")
	if !invalidWeightText.WeightSourceInvalid || invalidWeightText.Weight != 0 {
		t.Fatalf("unexpected invalid weight text analysis: %+v", invalidWeightText)
	}
	letterWeight := Analyze("Hight：177cm Weight：E")
	if !letterWeight.WeightSourceInvalid || letterWeight.Height != 177 || letterWeight.Weight != 0 {
		t.Fatalf("unexpected cup-letter weight analysis: %+v", letterWeight)
	}
	qualifiedHeight := Analyze("身高：净162穿鞋170+ 体重：82-85 罩杯：A")
	if qualifiedHeight.Height != 162 || qualifiedHeight.Weight != 82 {
		t.Fatalf("unexpected qualified height analysis: %+v", qualifiedHeight)
	}
	for _, text := range []string{"罩杯：定制", "罩杯：72/34", "罩杯：72/56/80", "罩杯：85-60-95", "罩杯：36.80", "罩杯：8", "罩杯：这个不知道", "罩杯：不知道怎么回答", "罩杯：不知道，胸小，正常吧", "罩杯：不晓得", "罩杯：不太清楚", "罩杯：不清楚哎", "罩杯：不确定", "罩杯：听不懂", "罩杯：没量过", "罩杯：没量过。", "罩杯：没量过，不是飞机场", "罩杯：没量。", "罩杯：没有量", "罩杯：没测过", "罩杯：正常", "罩杯：一般的", "罩杯：反正不小", "罩杯：一只手", "罩杯：大概一个苹果大一点", "罩杯：学生", "罩杯：出", "罩杯：胸73腰67臀83", "罩杯：X", "罩杯：-", "罩杯：+", "罩杯：·", "罩杯：·86"} {
		if analysis := Analyze(text); !analysis.CupSourceInvalid || analysis.Cup != "" {
			t.Fatalf("unexpected unknown cup analysis for %q: %+v", text, analysis)
		}
	}
	doubleColon := Analyze("身高: :165 体重::98 罩杯:：b+")
	if doubleColon.Height != 165 || doubleColon.Weight != 98 || doubleColon.Cup != "B" {
		t.Fatalf("unexpected double colon analysis: %+v", doubleColon)
	}
	variants := Analyze("身高：不穿鞋160 体重：bbw，150 罩杯：不知道,应该B")
	if variants.Height != 160 || variants.Weight != 150 || variants.Cup != "B" {
		t.Fatalf("unexpected text variants: %+v", variants)
	}
	if huge := Analyze("罩杯：最大号"); huge.Cup != "H" {
		t.Fatalf("unexpected huge cup analysis: %+v", huge)
	}
	if small := Analyze("罩杯：小"); small.Cup != "A" {
		t.Fatalf("unexpected small cup analysis: %+v", small)
	}
	if bare := Analyze("身高：裸174cm"); bare.Height != 174 {
		t.Fatalf("unexpected bare height analysis: %+v", bare)
	}
	if netBody := Analyze("身高：净身164 体重：100 🐻杯:B➕"); netBody.Height != 164 || netBody.Weight != 100 || netBody.Cup != "B" {
		t.Fatalf("unexpected net body height analysis: %+v", netBody)
	}
	if shoes := Analyze("身高：穿鞋160 体重：80 罩杯:A"); shoes.Height != 160 {
		t.Fatalf("unexpected shoes height analysis: %+v", shoes)
	}
	if heels := Analyze("身高：穿高跟170+ 体重：82-85 罩杯:A"); heels.Height != 170 {
		t.Fatalf("unexpected heels height analysis: %+v", heels)
	}
	if barefoot := Analyze("身高：脱鞋170 体重：120 罩杯:C"); barefoot.Height != 170 {
		t.Fatalf("unexpected barefoot height analysis: %+v", barefoot)
	}
	if almost := Analyze("身高：快165 体重：快90 罩杯:B"); almost.Height != 165 || almost.Weight != 90 {
		t.Fatalf("unexpected approximate dimensions: %+v", almost)
	}
	if real := Analyze("身高：真实170 体重：90斤 🐻：c"); real.Height != 170 || real.Weight != 90 || real.Cup != "C" {
		t.Fatalf("unexpected real dimensions: %+v", real)
	}
	if noisy := Analyze("身高:Q160 体重:90 罩杯:c"); noisy.Height != 160 {
		t.Fatalf("unexpected noisy height analysis: %+v", noisy)
	}
	if typoUnit := Analyze("身高168m 体重98"); typoUnit.Height != 168 || typoUnit.Weight != 98 {
		t.Fatalf("unexpected typo height unit analysis: %+v", typoUnit)
	}
	prefixedNumbers := Analyze("身高:.155 体重:目前100不定")
	if prefixedNumbers.Height != 155 || prefixedNumbers.Weight != 100 {
		t.Fatalf("unexpected prefixed numbers: %+v", prefixedNumbers)
	}
	currentWeight := Analyze("身高:162cm 体重:目前：50kg 罩杯:32C")
	if currentWeight.Height != 162 || currentWeight.Weight != 100 || currentWeight.Cup != "C" {
		t.Fatalf("unexpected current weight analysis: %+v", currentWeight)
	}
	if underWeight := Analyze("身高:150 体重:不到80 罩杯:不知道"); underWeight.Weight != 80 {
		t.Fatalf("unexpected upper-bound weight analysis: %+v", underWeight)
	}
	if commaWeight := Analyze("身高165 体重:，100 罩杯:b"); commaWeight.Weight != 100 {
		t.Fatalf("unexpected comma-prefixed weight analysis: %+v", commaWeight)
	}
	if almostWeight := Analyze("身高158 体重:快80 罩杯:没量过"); almostWeight.Weight != 80 {
		t.Fatalf("unexpected approximate weight analysis: %+v", almostWeight)
	}
	if weight := Analyze("体重：无虚假 “88”"); weight.Weight != 88 {
		t.Fatalf("unexpected qualified weight: %+v", weight)
	}
	pair := Analyze("身高:姐姐163/妹妹165 体重:姐姐103/妹妹87 罩杯:姐姐C/妹妹B")
	if pair.Height != 163 || pair.Weight != 103 || pair.Cup != "C" {
		t.Fatalf("unexpected paired profile analysis: %+v", pair)
	}
	if outOfRange := Analyze("罩杯：J杯"); !outOfRange.CupSourceInvalid || outOfRange.Cup != "" {
		t.Fatalf("unexpected out-of-range cup analysis: %+v", outOfRange)
	}
	if outOfRangeText := Analyze("罩杯: V纯天然无隆胸 职业:老师"); !outOfRangeText.CupSourceInvalid || outOfRangeText.Cup != "" {
		t.Fatalf("unexpected described out-of-range cup analysis: %+v", outOfRangeText)
	}
}
