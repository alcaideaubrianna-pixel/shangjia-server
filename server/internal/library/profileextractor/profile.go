package profileextractor

import (
	"regexp"
	"strconv"
	"strings"
)

// Fields contains values extracted from a source profile. Zero values mean unknown.
type Fields struct {
	Height int
	Weight int
	Cup    string
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
	heightPattern = regexp.MustCompile(`(?i)(?:身高|净身高|高|身长|高度|height)(?:\s*[\(（][^\)）]*[\)）])?\s*[:：]?\s*([0-9]{2,3})([^0-9]|$)`)
	weightPattern = regexp.MustCompile(`(?i)(?:体重|净体重|重量|重|斤数|weight)(?:\s*[\(（][^\)）]*[\)）])?\s*[:：]?\s*([0-9]{2,3})([^0-9]|$)`)
	cupPattern    = regexp.MustCompile(`(?i)(?:罩杯|置杯|杯|cup|胸围|胸|上围)\s*[:：]?\s*(?:[0-9]{2,3}\s*)?([a-h])([^a-z]|$)`)
	crossCup      = regexp.MustCompile(`(?i)([0-9]{2,3})\s*([a-h])([^a-z]|$)`)
)

// Parse applies the profile regular expressions used by FeiNiu_bot.
func Parse(text string) Fields {
	text = strings.ToLower(strings.ReplaceAll(text, "\u00a0", " "))
	result := Fields{}
	result.Height = firstNumber(heightPattern, text)
	result.Weight = firstNumber(weightPattern, text)
	if match := cupPattern.FindStringSubmatch(text); len(match) > 1 {
		result.Cup = strings.ToUpper(match[1])
	} else if match := crossCup.FindStringSubmatch(text); len(match) > 2 {
		result.Cup = strings.ToUpper(match[2])
	}
	return result
}

func firstNumber(pattern *regexp.Regexp, text string) int {
	match := pattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return 0
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value <= 0 {
		return 0
	}
	return value
}
