package introfee

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	keywordPattern      = regexp.MustCompile(`(?:介绍费|介绍费用|推荐费|牵线费|居间费|对接费|感谢费|辛苦费|包含费|服务费|中介费|中介服务费|大米)`)
	amountPattern       = regexp.MustCompile(`(?:介绍费|介绍费用|推荐费|牵线费|居间费|对接费|感谢费|辛苦费|包含费|服务费|中介费|中介服务费|大米)[\s\p{Zs}]*[:：=；;，,]?[\s\p{Zs}]*([¥￥]?[\s\p{Zs}]*[0-9０-９][0-9０-９,，.]*(?:[\s\p{Zs}]*(?:元|万))?)`)
	prefixAmountPattern = regexp.MustCompile(`([¥￥]?[\s\p{Zs}]*[0-9０-９][0-9０-９,，.]*)[\s\p{Zs}]*(?:介绍费|介绍费用|推荐费|牵线费|居间费|对接费|感谢费|辛苦费|包含费|服务费|中介费|中介服务费|大米)`)
	numberPattern       = regexp.MustCompile(`[0-9０-９]+(?:[0-9０-９,，.]*)`)
)

type Match struct {
	Matched        bool
	Keyword        string
	Amount         string
	OriginalLine   string
	NormalizedLine string
}

type Fields struct {
	FirstAmount   string
	FirstSuffix   string
	ForeignAmount string
}

type Policy struct {
	RemoveTail bool
	Suffix     string
}

func Normalize(text string) string {
	text = norm.NFKC.String(text)
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		if (r >= '\u180b' && r <= '\u180d') || (r >= '\ufe00' && r <= '\ufe0f') || (r >= '\U000e0100' && r <= '\U000e01ef') {
			return -1
		}
		return r
	}, text)
}

func Parse(line string) Match {
	normalized := Normalize(line)
	keyword := keywordPattern.FindString(normalized)
	if keyword == "" {
		return Match{OriginalLine: line, NormalizedLine: normalized}
	}
	match := amountPattern.FindStringSubmatch(normalized)
	amount := ""
	if len(match) > 1 {
		amount = normalizeNumber(match[1])
	} else if match = prefixAmountPattern.FindStringSubmatch(normalized); len(match) > 1 {
		amount = normalizeNumber(match[1])
	}
	return Match{Matched: true, Keyword: "介绍费", Amount: amount, OriginalLine: line, NormalizedLine: normalized}
}

func ParseFields(text string) Fields {
	text = Normalize(text)
	for _, line := range strings.Split(text, "\n") {
		if !keywordPattern.MatchString(line) {
			continue
		}
		match := Parse(line)
		fields := Fields{FirstAmount: match.Amount}
		numbers := numberPattern.FindAllString(line, -1)
		if fields.FirstAmount == "" && len(numbers) > 0 {
			fields.FirstAmount = normalizeNumber(numbers[0])
		}
		if len(numbers) > 1 && strings.Contains(line, "国外") {
			fields.ForeignAmount = normalizeNumber(numbers[1])
		}
		if fields.FirstAmount != "" {
			keyword := keywordPattern.FindString(line)
			keywordIndex := strings.Index(line, keyword)
			amountIndex := strings.Index(line, fields.FirstAmount)
			if keywordIndex >= 0 && amountIndex >= 0 {
				between := strings.TrimSpace(line[keywordIndex+len(keyword) : amountIndex])
				between = strings.Trim(between, "：:；;，,./／|｜、·• ")
				if between != "" && !strings.Contains(between, "国内") && !strings.Contains(between, "国外") {
					fields.FirstSuffix = between
				}
				if fields.FirstSuffix == "" && keywordIndex > 0 {
					prefix := strings.TrimSpace(line[:keywordIndex])
					if prefix != "" && !strings.Contains(prefix, "国内") && !strings.Contains(prefix, "国外") {
						fields.FirstSuffix = strings.Trim(prefix, "：:；;，,./／|｜、·• ")
					}
				}
			}
		}
		return fields
	}
	return Fields{}
}

func normalizeNumber(value string) string {
	value = strings.Trim(strings.TrimSpace(value), ",，")
	return strings.Join(strings.Fields(value), " ")
}

func Replace(line, suffix string) string {
	match := Parse(line)
	if !match.Matched {
		return line
	}
	if strings.TrimSpace(suffix) == "" {
		return ""
	}
	prefix := ""
	if keywordIndex := strings.Index(match.NormalizedLine, keywordPattern.FindString(match.NormalizedLine)); keywordIndex > 0 {
		candidate := strings.TrimSpace(match.NormalizedLine[:keywordIndex])
		if candidate != "" && prefixAmountPattern.FindString(match.NormalizedLine) == "" {
			prefix = candidate + " "
		}
	}
	if match.Amount == "" {
		return strings.TrimSpace(prefix + match.Keyword + " " + strings.TrimSpace(suffix))
	}
	return strings.TrimSpace(prefix + match.Keyword + " " + match.Amount + " " + strings.TrimSpace(suffix))
}

func RemoveLine(line string) string {
	if !Parse(line).Matched {
		return line
	}
	return ""
}

func Process(text string, policy Policy) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for index, line := range lines {
		if !Parse(line).Matched {
			continue
		}
		if policy.RemoveTail && strings.TrimSpace(policy.Suffix) == "" {
			cut := index
			if previous := previousVisibleLine(lines, index); previous > 0 && visibleLength(lines[previous]) < 6 {
				cut = previous
			}
			return strings.TrimSpace(strings.Join(lines[:cut], "\n"))
		}
		if strings.TrimSpace(policy.Suffix) != "" {
			lines[index] = Replace(line, policy.Suffix)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func previousVisibleLine(lines []string, before int) int {
	for index := before - 1; index >= 0; index-- {
		if strings.TrimSpace(Normalize(lines[index])) != "" {
			return index
		}
	}
	return -1
}

func visibleLength(line string) int {
	count := 0
	for _, r := range Normalize(line) {
		if unicode.IsSpace(r) {
			continue
		}
		count++
	}
	return count
}
