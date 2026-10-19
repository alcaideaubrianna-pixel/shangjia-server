// Package sensitive removes credentials and private endpoints from diagnostics.
package sensitive

import "regexp"

var tokenPattern = regexp.MustCompile(`(?i)[0-9]{5,}(?::|%3a)[a-z0-9_-]{20,}\b`)
var urlPattern = regexp.MustCompile(`(?i)(?:https?|socks5h?|postgres(?:ql)?|redis)://[^\s<>"']+`)

// Tokens preserves ordinary links while removing Telegram Bot credentials.
func Tokens(text string) string {
	return tokenPattern.ReplaceAllString(text, "[token]")
}

// Diagnostic removes complete endpoints as well as standalone credentials.
func Diagnostic(text string) string {
	return Tokens(urlPattern.ReplaceAllString(text, "[url]"))
}
