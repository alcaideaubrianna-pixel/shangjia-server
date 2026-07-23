package sys

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

func feiNiuAccountUsername(title string, channelId int64, chatId int64) string {
	base := feiNiuAccountUsernameBase(title)
	if base == "" {
		if channelId > 0 {
			base = fmt.Sprintf("channel%d", channelId)
		} else if chatId > 0 {
			base = fmt.Sprintf("chat%d", chatId)
		} else {
			base = "channel"
		}
	}
	if suffix := feiNiuAccountUsernameSuffix(channelId, chatId); suffix != "" {
		base += suffix
	}
	if len(base) < 5 {
		base += "001"
	}
	if len(base) > 32 {
		base = base[:32]
	}
	return base
}

func feiNiuAccountUsernameBase(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	args := pinyin.NewArgs()
	var builder strings.Builder
	for _, r := range title {
		switch {
		case unicode.In(r, unicode.Han):
			if parts := pinyin.SinglePinyin(r, args); len(parts) > 0 {
				builder.WriteString(parts[0])
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(unicode.ToLower(r))
		default:
			continue
		}
	}
	res := feiNiuAccountUsernameASCII(builder.String())
	if res == "" {
		return ""
	}
	if len(res) > 24 {
		return res[:24]
	}
	return res
}

func feiNiuAccountUsernameASCII(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			continue
		}
		if r == '_' {
			builder.WriteRune(r)
		}
	}
	return strings.Trim(builder.String(), "_")
}

func feiNiuAccountUsernameSuffix(channelId int64, chatId int64) string {
	id := channelId
	if id <= 0 {
		id = chatId
	}
	if id <= 0 {
		return ""
	}
	digits := strconv.FormatInt(id, 10)
	if len(digits) > 4 {
		digits = digits[len(digits)-4:]
	}
	return "_" + digits
}
