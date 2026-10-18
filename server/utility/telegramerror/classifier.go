// Package telegramerror centralizes Telegram Bot API media error classification.
package telegramerror

import "strings"

// IsRemoteURLRejected reports errors where Telegram rejected or could not fetch an HTTP media URL.
// It intentionally excludes timeouts and connection resets because delivery may already have succeeded.
func IsRemoteURLRejected(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, part := range []string{
		"failed to get http url content",
		"webpage_curl_failed",
		"webpage_media_empty",
		"wrong file identifier/http url specified",
		"wrong type of the web page content",
		"wrong content type",
		"file is too big",
		"request entity too large",
	} {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}
