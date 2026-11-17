// Package botnotice shares the existing Bot receipt classification policy.
package botnotice

import (
	"encoding/json"
	"strings"
)

func Matches(text string) bool {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), "")
	if normalized == "" {
		return false
	}
	for _, prefix := range []string{"❌重复投稿", "✅提交成功", "收录失败", "收录成功", "✅收录成功", "信息投稿推送成功", "信息已存在请勿重复提交", "⚠提交失败", "❌提交失败", "❌上架被拒绝", "❌录入失败", "❌未收录", "⚠️采集失败", "⚠️未找到待配对", "✅内容已自动转发", "✅视频已自动转发", "视频收录成功", "全网无纠纷，认准唯一客服"} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return normalized == "验证视频" || strings.Contains(normalized, "投稿已自动审核通过并成功分发")
}

// PlainUpdate is deliberately conservative: unknown message fields take the
// original path rather than risking classification of new attachment types.
func PlainUpdate(raw []byte) bool {
	var update map[string]json.RawMessage
	if json.Unmarshal(raw, &update) != nil {
		return false
	}
	var payload json.RawMessage
	for _, key := range []string{"channel_post", "edited_channel_post", "message", "edited_message"} {
		if value := update[key]; len(value) > 0 && string(value) != "null" {
			if payload != nil {
				return false
			}
			payload = value
		}
	}
	if payload == nil {
		return false
	}
	var message map[string]json.RawMessage
	if json.Unmarshal(payload, &message) != nil {
		return false
	}
	for key := range message {
		switch key {
		case "message_id", "message_thread_id", "direct_messages_topic", "date", "edit_date", "chat", "from", "sender_chat", "sender_boost_count", "sender_business_bot", "business_connection_id", "forward_origin", "is_automatic_forward", "reply_to_message", "external_reply", "quote", "reply_to_story", "via_bot", "has_protected_content", "is_from_offline", "is_paid_post", "author_signature", "text", "entities", "link_preview_options", "effect_id", "reply_markup":
		case "media_group_id":
			var group string
			if json.Unmarshal(message[key], &group) != nil || group != "" {
				return false
			}
		default:
			return false
		}
	}
	var text string
	return json.Unmarshal(message["text"], &text) == nil && Matches(text)
}
