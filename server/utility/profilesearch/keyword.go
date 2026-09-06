package profilesearch

import (
	"regexp"
	"strings"
)

var (
	labelPattern      = regexp.MustCompile(`(?i)^\s*(?:资料)?编号\s*[:：=]?\s*`)
	identifierPattern = regexp.MustCompile(`^[A-Z0-9_-]{2,32}$`)
)

// NormalizeKeyword removes the optional profile-number label used by operators.
func NormalizeKeyword(value string) string {
	return strings.TrimSpace(labelPattern.ReplaceAllString(strings.TrimSpace(value), ""))
}

// Identifier returns a normalized profile number when value has an identifier shape.
func Identifier(value string) (string, bool) {
	value = strings.ToUpper(NormalizeKeyword(value))
	if !identifierPattern.MatchString(value) {
		return "", false
	}
	if strings.IndexFunc(value, func(r rune) bool { return r >= '0' && r <= '9' }) < 0 {
		return "", false
	}
	if strings.IndexFunc(value, func(r rune) bool { return r >= 'A' && r <= 'Z' }) < 0 {
		return "", false
	}
	return value, true
}
