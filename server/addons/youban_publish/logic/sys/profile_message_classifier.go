package sys

import (
	"regexp"
	"strings"
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

func profileMessageIgnoredNotice(text string) bool {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), "")
	if normalized == "" {
		return false
	}
	for _, prefix := range []string{
		"❌重复投稿",
		"✅提交成功",
		"收录失败",
		"视频收录成功",
		"全网无纠纷，认准唯一客服",
	} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return strings.Contains(normalized, "投稿已自动审核通过并成功分发")
}

func classifyProfileMessage(text string, media []collectMediaItem) profileMessageClassification {
	text = strings.TrimSpace(text)
	if profileMessageIgnoredNotice(text) {
		return profileMessageClassification{Kind: profileMessageKindIgnore}
	}
	if len(media) > 0 && profileMessageAllMediaType(media, "video") && !profileMessageHasProfileText(text) {
		return profileMessageClassification{
			Kind:  profileMessageKindVerify,
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
	return (profileMessageFieldPattern.MatchString(normalized) && profileMessageNonIndexFieldPattern.MatchString(normalized)) || profileMessageCompactValuePattern.MatchString(normalized)
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
