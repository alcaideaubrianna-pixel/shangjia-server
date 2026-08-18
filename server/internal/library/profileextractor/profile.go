package profileextractor

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Fields contains values extracted from a source profile. Zero values mean unknown.
type Fields struct {
	Height int
	Weight int
	Cup    string
}

type Analysis struct {
	Fields
	HeightMentioned   bool
	WeightMentioned   bool
	CupMentioned      bool
	HeightSourceEmpty bool
	WeightSourceEmpty bool
	CupSourceEmpty    bool
}

// Merge fills only missing source values and keeps the source as authoritative when present.
func Merge(text string, height int, weight int, cup string) Fields {
	parsed := Parse(text)
	if height > 0 {
		parsed.Height = height
	}
	if weight > 0 {
		parsed.Weight = weight
	}
	if normalized := NormalizeCup(cup); normalized != "" {
		parsed.Cup = normalized
	}
	return parsed
}

// Refresh replaces fields explicitly present in new text and preserves known
// values when the new text does not mention that field.
func Refresh(text string, current Fields) Fields {
	parsed := Parse(text)
	if parsed.Height == 0 {
		parsed.Height = current.Height
	}
	if parsed.Weight == 0 {
		parsed.Weight = current.Weight
	}
	if parsed.Cup == "" {
		parsed.Cup = NormalizeCup(current.Cup)
	}
	return parsed
}

// NormalizeCup accepts a structured cup value without allowing generic tags
// to leak into the cup_size column.
func NormalizeCup(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "罩杯")
	value = strings.TrimSuffix(value, "CUP")
	value = strings.TrimSpace(value)
	if len(value) != 1 || value[0] < 'A' || value[0] > 'H' {
		return ""
	}
	return value
}

var (
	heightPattern = regexp.MustCompile(`(?i)(?:身高|净身高|升高|身长|高度|height|高)(?:\s*\([^)]*\))?\s*:?\s*([0-9]+(?:\.[0-9]+)?\s*(?:cm|厘米|米|m)?)`)
	weightPattern = regexp.MustCompile(`(?i)(?:体重|净体重|重量|斤数|weight|重)(?:\s*\([^)]*\))?\s*:?\s*([0-9]+(?:\.[0-9]+)?\s*(?:kg|公斤|千克|斤)?)`)
	cupPattern    = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|杯|cup|胸围|胸部|上围|胸)\s*:?\s*[^\n;,:]{0,8}?([a-h])\+?`)
	crossCup      = regexp.MustCompile(`(?i)(?:^|[^0-9])([0-9]{2,3})\s*([a-h])\+?([^a-z]|$)`)
	compactFields = regexp.MustCompile(`(?:^|[^0-9])(1[8-9]|[2-7][0-9]|80)\s*(?:岁|周岁|虚岁)?\s*[/,\s-]+(1[4-9][0-9]|20[0-9])\s*[/,\s-]+([0-9]{2,3})(?:[^0-9]|$)`)
	heightLabel   = regexp.MustCompile(`(?i)(?:身高|净身高|升高|身长|高度|height)\s*(?:\([^)]*\))?\s*:`)
	weightLabel   = regexp.MustCompile(`(?i)(?:体重|净体重|重量|斤数|weight)\s*(?:\([^)]*\))?\s*:`)
	cupLabel      = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)\s*:`)
	heightEmpty   = regexp.MustCompile(`(?i)(?:身高|净身高|升高|身长|高度|height)\s*(?:\([^)]*\))?\s*:\s*(?:$|\n|体重|罩杯|职业|是否)`)
	weightEmpty   = regexp.MustCompile(`(?i)(?:体重|净体重|重量|斤数|weight)\s*(?:\([^)]*\))?\s*:\s*(?:$|\n|罩杯|职业|是否)`)
	cupEmpty      = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)\s*:\s*(?:$|\n|职业|学历|是否)`)
)

// Parse applies the profile regular expressions used by FeiNiu_bot.
func Parse(text string) Fields {
	return Analyze(text).Fields
}

func Analyze(text string) Analysis {
	text = normalizeText(text)
	result := Analysis{
		HeightMentioned:   heightLabel.MatchString(text),
		WeightMentioned:   weightLabel.MatchString(text),
		CupMentioned:      cupLabel.MatchString(text),
		HeightSourceEmpty: heightEmpty.MatchString(text),
		WeightSourceEmpty: weightEmpty.MatchString(text),
		CupSourceEmpty:    cupEmpty.MatchString(text),
	}
	if match := heightPattern.FindStringSubmatch(text); len(match) > 1 {
		result.HeightMentioned = true
		result.Height = parseHeight(match[1])
	}
	if match := weightPattern.FindStringSubmatch(text); len(match) > 1 {
		result.WeightMentioned = true
		result.Weight = parseWeight(match[1])
	}
	if compact := compactFields.FindStringSubmatch(text); len(compact) > 3 {
		result.HeightMentioned = true
		result.WeightMentioned = true
		if result.Height == 0 {
			result.Height = parseHeight(compact[2])
		}
		if result.Weight == 0 {
			result.Weight = parseWeight(compact[3] + "斤")
		}
	}
	if match := cupPattern.FindStringSubmatch(text); len(match) > 1 {
		result.CupMentioned = true
		result.Cup = strings.ToUpper(match[1])
	} else if match := crossCup.FindStringSubmatch(text); len(match) > 2 {
		result.CupMentioned = true
		result.Cup = strings.ToUpper(match[2])
	}
	return result
}

func normalizeText(text string) string {
	text = norm.NFKC.String(text)
	var builder strings.Builder
	for _, char := range text {
		if unicode.Is(unicode.Cf, char) || unicode.Is(unicode.M, char) {
			continue
		}
		builder.WriteRune(char)
	}
	replacer := strings.NewReplacer(
		"\r", "\n", "\u3000", " ", "\u00a0", " ", "：", ":", "；", ";", "，", ",",
		"（", "(", "）", ")", "－", "-", "—", "-", "➕", "+",
	)
	return strings.ToLower(replacer.Replace(builder.String()))
}

func parseHeight(value string) int {
	normalized := strings.ToLower(strings.TrimSpace(value))
	number, ok := firstDecimal(normalized)
	if !ok {
		return 0
	}
	if number > 0 && number < 3 || (strings.HasSuffix(normalized, "米") && !strings.HasSuffix(normalized, "厘米")) || (strings.HasSuffix(normalized, "m") && !strings.HasSuffix(normalized, "cm")) {
		number *= 100
	}
	height := int(math.Round(number))
	if height < 140 || height > 210 {
		return 0
	}
	return height
}

func parseWeight(value string) int {
	normalized := strings.ToLower(strings.TrimSpace(value))
	number, ok := firstDecimal(normalized)
	if !ok {
		return 0
	}
	if number >= 1000 && number < 3000 {
		number = math.Floor(number / 10)
	} else if number >= 900 && number < 1000 {
		compact := int(number) / 10
		if compact >= 50 && compact <= 150 {
			number = float64(compact)
		}
	}
	if strings.Contains(normalized, "kg") || strings.Contains(normalized, "公斤") || strings.Contains(normalized, "千克") {
		number *= 2
	} else if !strings.Contains(normalized, "斤") && number < 70 {
		number *= 2
	}
	weight := int(math.Round(number))
	if weight < 50 || weight > 300 {
		return 0
	}
	return weight
}

func firstDecimal(value string) (float64, bool) {
	match := regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`).FindString(value)
	if match == "" {
		return 0, false
	}
	number, err := strconv.ParseFloat(match, 64)
	return number, err == nil
}
