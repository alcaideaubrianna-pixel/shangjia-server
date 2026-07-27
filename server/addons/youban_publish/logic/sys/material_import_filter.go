package sys

import "strings"

// materialImportIgnoredNotice reports Telegram bot replies that must never become materials.
func materialImportIgnoredNotice(text string) bool {
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
