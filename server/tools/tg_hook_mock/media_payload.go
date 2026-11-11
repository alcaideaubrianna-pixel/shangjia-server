package main

import (
	"fmt"
	"strings"
	"time"
)

// telegramMediaInputCounts 记录实际媒体复用和上传模式，不保存token、正文或媒体地址。
func telegramMediaInputCounts(method string, payload map[string]any) map[string]int {
	counts := map[string]int{"file_id_count": 0, "url_count": 0, "upload_count": 0}
	var references []string
	if media, ok := payload["media"].([]any); ok {
		for _, item := range media {
			if value, ok := item.(map[string]any); ok {
				references = append(references, fmt.Sprint(value["media"]))
			}
		}
	} else {
		for _, field := range []string{"photo", "video", "document", "animation"} {
			if value, ok := payload[field]; ok {
				references = append(references, fmt.Sprint(value))
			}
		}
	}
	for _, ref := range references {
		switch {
		case strings.HasPrefix(ref, "attach://"):
			counts["upload_count"]++
		case strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://"):
			counts["url_count"]++
		case ref != "" && ref != "<nil>":
			counts["file_id_count"]++
		}
	}
	return counts
}

// attachTelegramResultPayload 保留真实请求中的正文、相册信息及隐形标记，供Webhook与Attempt正确关联。
func attachTelegramResultPayload(method string, result any, payload map[string]any) {
	var messages []map[string]any
	switch value := result.(type) {
	case map[string]any:
		messages = []map[string]any{value}
	case []map[string]any:
		messages = value
	default:
		return
	}
	media, _ := payload["media"].([]any)
	group := fmt.Sprint(time.Now().UnixNano())
	for index, message := range messages {
		message["date"] = time.Now().Unix()
		if chat, ok := message["chat"].(map[string]any); ok {
			chat["type"] = "channel"
		}
		if method == "sendMessage" {
			if text, ok := payload["text"]; ok {
				message["text"] = text
			}
		} else if method == "sendMediaGroup" {
			message["media_group_id"] = group
			if index < len(media) {
				if item, ok := media[index].(map[string]any); ok {
					if caption, ok := item["caption"]; ok {
						message["caption"] = caption
					}
				}
			}
		} else if caption, ok := payload["caption"]; ok {
			message["caption"] = caption
		}
	}
}
