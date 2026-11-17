package sys

import (
	"hotgo/internal/library/botnotice"
	"regexp"
	"strings"
	"unicode/utf8"
)

type profileMessageKind string

const (
	profileMessageKindIgnore  profileMessageKind = "ignore"
	profileMessageKindDisplay profileMessageKind = "display"
	profileMessageKindVerify  profileMessageKind = "verify"
)

type profileMessageClassification struct {
	Kind  profileMessageKind
	Text  string
	Media []collectMediaItem
}

var profileMessageFieldPattern = regexp.MustCompile(`(?i)(?:编号|昵称|姓名|名字|性别|年龄|身高|体重|职业|工作|省份|所在省份|城市|所在城市|地区|所在地|微信|联系方式|年龄段|name|nickname|username|number|no\.?|sex|age|height|weight|job|occupation|province|city|location)\s*[:：=]`)
var profileMessageNonIndexFieldPattern = regexp.MustCompile(`(?i)(?:昵称|姓名|名字|性别|年龄|身高|体重|职业|工作|省份|所在省份|城市|所在城市|地区|所在地|微信|联系方式|年龄段|name|nickname|username|sex|age|height|weight|job|occupation|province|city|location)\s*[:：=]`)
var profileMessageCompactValuePattern = regexp.MustCompile(`(?i)(?:年龄|身高|体重|age|height|weight)\s*[-+]?\d`)

func isCollectBotSystemNotice(sourceType, text string) bool {
	return strings.EqualFold(strings.TrimSpace(sourceType), "bot") && profileMessageIgnoredNotice(text)
}

func profileMessageIgnoredNotice(text string) bool {
	return botnotice.Matches(text)
}

func classifyProfileMessage(text string, media []collectMediaItem) profileMessageClassification {
	text = stripTelegramAttemptMarkers(text)
	text = strings.TrimSpace(text)
	if profileMessageIgnoredNotice(text) {
		return profileMessageClassification{Kind: profileMessageKindIgnore}
	}
	if len(media) > 0 && utf8.RuneCountInString(text) > 20 {
		return profileMessageClassification{
			Kind:  profileMessageKindDisplay,
			Text:  text,
			Media: media,
		}
	}
	if profileMessageAllMediaType(media, "video") && !profileMessageHasProfileText(text) {
		return profileMessageClassification{
			Kind:  profileMessageKindVerify,
			Media: media,
		}
	}
	if text != "" && len(media) > 0 && !profileMessageAllMediaType(media, "video") && !profileMessageOnlyIndexText(text) {
		return profileMessageClassification{
			Kind:  profileMessageKindDisplay,
			Text:  text,
			Media: media,
		}
	}
	if text != "" && profileMessageHasProfileText(text) {
		return profileMessageClassification{
			Kind:  profileMessageKindDisplay,
			Text:  text,
			Media: media,
		}
	}
	return profileMessageClassification{Kind: profileMessageKindIgnore}
}

func profileMessageHasProfileText(text string) bool {
	normalized := strings.TrimSpace(strings.ReplaceAll(text, "\u00a0", " "))
	if normalized == "" || profileMessageIgnoredNotice(normalized) {
		return false
	}
	// Any known profile field is sufficient to identify a display record.
	// Do not require two fields: short records often contain only one field
	// such as "城市：成都" or "年龄：25".
	return profileMessageNonIndexFieldPattern.MatchString(normalized) || profileMessageCompactValuePattern.MatchString(normalized)
}

func profileMessageOnlyIndexText(text string) bool {
	normalized := strings.TrimSpace(strings.ReplaceAll(text, "\u00a0", " "))
	return normalized != "" && profileMessageFieldPattern.MatchString(normalized) && !profileMessageNonIndexFieldPattern.MatchString(normalized)
}

func profileMessageAllMediaType(items []collectMediaItem, mediaType string) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.Type), mediaType) {
			return false
		}
	}
	return true
}

func profileMessageHasMediaType(items []collectMediaItem, mediaType string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Type), mediaType) {
			return true
		}
	}
	return false
}
