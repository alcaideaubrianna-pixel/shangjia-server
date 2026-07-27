package sys

import (
	"strings"
)

const publishProfileSourceType = "youban_publish"

func profileTagFieldExpr() string {
	return "CASE WHEN p.source_type = 'feiniu' THEN p.tag_params ELSE p.cup_size END"
}

func profileSummary(text string) string {
	text = strings.TrimSpace(text)
	if len([]rune(text)) <= 80 {
		return text
	}
	return string([]rune(text)[:80])
}
