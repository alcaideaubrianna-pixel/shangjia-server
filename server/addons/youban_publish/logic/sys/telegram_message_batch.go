package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

type telegramDeleteMessagesResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func telegramDeleteMessagesBatch(ctx context.Context, token string, messages []telegramDeleteMessage) error {
	if len(messages) < telegramDeleteMessagesBatchThreshold || len(messages) > telegramDeleteMessagesMaxItems {
		return gerror.New("批量删除消息数量不在允许范围")
	}
	chatID := normalizeTelegramChannelChatID(messages[0].TargetChatId)
	if chatID == "" {
		return gerror.New("批量删除频道不能为空")
	}
	ids := make([]string, 0, len(messages))
	for _, item := range messages {
		if normalizeTelegramChannelChatID(item.TargetChatId) != chatID || item.MessageId <= 0 {
			return gerror.New("批量删除消息必须属于同一频道且消息ID有效")
		}
		ids = append(ids, strconv.FormatInt(item.MessageId, 10))
	}
	base := strings.TrimRight(os.Getenv("YOUBAN_TELEGRAM_RUNTIME_API_URL"), "/")
	if base == "" {
		base = strings.TrimRight(os.Getenv("YOUBAN_TELEGRAM_BOT_API_URL"), "/")
	}
	if base == "" {
		base = "https://api.telegram.org"
	}
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("message_ids", "["+strings.Join(ids, ",")+"]")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/deleteMessages", base, token), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result telegramDeleteMessagesResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return gerror.Newf("Telegram批量删除失败：%s", result.Description)
	}
	return nil
}
