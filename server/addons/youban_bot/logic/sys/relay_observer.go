package sys

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
)

// Gateway features may consume a private message before the legacy Bot provider
// runs. Observe peer metadata here as well, without incrementing legacy counters
// or changing privileges. Existing user/group tables remain the directory source.
func (s *sSysBot) relayObserveChat(ctx context.Context, botID int64, msg *models.Message) error {
	lastAt := gtime.Now()
	if msg.Date > 0 {
		lastAt = gtime.NewFromTime(time.Unix(int64(msg.Date), 0))
	}
	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	text := strings.TrimSpace(firstNonEmpty(msg.Text, msg.Caption))
	if msg.From != nil && !msg.From.IsBot {
		c := dao.YoubanBotUser.Columns()
		data := do.YoubanBotUser{BotId: botID, TelegramUserId: strconv.FormatInt(msg.From.ID, 10), TelegramUsername: msg.From.Username, TelegramFirstName: msg.From.FirstName, TelegramLastName: msg.From.LastName, ChatId: chatID, ChatType: string(msg.Chat.Type), ChatTitle: botMessageChatTitle(msg), LastMessageText: text, LastMessageAt: lastAt, Status: 1}
		if _, err := dao.YoubanBotUser.Ctx(ctx).Data(data).OnConflict(c.BotId, c.TelegramUserId).OnDuplicateEx(c.CreatedAt, c.MessageCount, c.IsSuperAdmin).Save(); err != nil {
			return gerror.Wrap(err, "更新双向Bot用户资料失败")
		}
	}
	if msg.Chat.Type == models.ChatTypeGroup || msg.Chat.Type == models.ChatTypeSupergroup {
		c := dao.YoubanBotChannelCache.Columns()
		data := do.YoubanBotChannelCache{BotId: botID, ChatId: chatID, ChatType: string(msg.Chat.Type), ChatTitle: botMessageChatTitle(msg), ChatUsername: msg.Chat.Username, IsBroadcast: 0, IsMegagroup: 1, LastMessageText: text, LastMessageAt: lastAt}
		if _, err := dao.YoubanBotChannelCache.Ctx(ctx).Data(data).OnConflict(c.BotId, c.ChatId).OnDuplicateEx(c.CreatedAt, c.MessageCount).Save(); err != nil {
			return gerror.Wrap(err, "更新双向Bot群聊资料失败")
		}
	}
	return nil
}
