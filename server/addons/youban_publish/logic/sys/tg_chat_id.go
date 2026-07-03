package sys

import (
	"strings"
	"unicode"
)

func normalizeTelegramChannelChatID(chatID string) string {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || strings.HasPrefix(chatID, "-") || strings.HasPrefix(chatID, "@") {
		return chatID
	}
	if len(chatID) < 10 {
		return chatID
	}
	for _, char := range chatID {
		if !unicode.IsDigit(char) {
			return chatID
		}
	}
	return "-100" + chatID
}
