package sysin

import (
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

// Prefixes are parsed outside the expression. text: escapes reserved prefixes.
func collectKeywordParts(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "text:") {
		return strings.TrimPrefix(value, "text:"), false
	}
	if strings.HasPrefix(value, "regex:") {
		return strings.TrimPrefix(value, "regex:"), true
	}
	return value, false
}

func NormalizeCollectKeywords(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		pattern, regex := collectKeywordParts(value)
		if strings.TrimSpace(pattern) == "" {
			return nil, gerror.New("关键词内容不能为空")
		}
		key := "text:" + strings.ToLower(pattern)
		if regex {
			if _, err := regexp.Compile("(?i:" + pattern + ")"); err != nil {
				return nil, gerror.Newf("关键词正则无效 %q：%s", value, err.Error())
			}
			// Regex escapes are case sensitive: \d and \D are different.
			key = "regex:" + pattern
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, value)
		}
	}
	return result, nil
}

func MatchCollectKeyword(text, value string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	pattern, regex := collectKeywordParts(value)
	if strings.TrimSpace(pattern) == "" {
		return false
	}
	if regex {
		expression, err := regexp.Compile("(?i:" + pattern + ")")
		return err == nil && expression.MatchString(text)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
}
