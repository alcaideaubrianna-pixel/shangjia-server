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
)

type Match struct {
	Matched        bool
	Keyword        string
	Amount         string
	OriginalLine   string
	NormalizedLine string
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
		amount = strings.Join(strings.Fields(strings.TrimSpace(match[1])), " ")
	} else if match = prefixAmountPattern.FindStringSubmatch(normalized); len(match) > 1 {
		amount = strings.Join(strings.Fields(strings.TrimSpace(match[1])), " ")
	}
	return Match{Matched: true, Keyword: keyword, Amount: amount, OriginalLine: line, NormalizedLine: normalized}
}

func Replace(line, suffix string) string {
	match := Parse(line)
	if !match.Matched {
		return line
	}
	if strings.TrimSpace(suffix) == "" {
		return ""
	}
	if match.Amount == "" {
		return strings.TrimSpace(match.Keyword + " " + strings.TrimSpace(suffix))
	}
	return strings.TrimSpace(match.Keyword + " " + match.Amount + " " + strings.TrimSpace(suffix))
}

func RemoveLine(line string) string {
	if !Parse(line).Matched {
		return line
	}
	return ""
}
