package sys

import (
	"context"
	"strconv"
	"strings"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
)

type relayPeer struct {
	ID   string
	Name string
	Kind string
}

func (s *sSysBot) relayKnownUser(ctx context.Context, botID int64, identity string) (*relayPeer, error) {
	var rows []*struct {
		TelegramUserId    string `json:"telegram_user_id"`
		TelegramUsername  string `json:"telegram_username"`
		TelegramFirstName string `json:"telegram_first_name"`
		TelegramLastName  string `json:"telegram_last_name"`
	}
	c := dao.YoubanBotUser.Columns()
	mod := dao.YoubanBotUser.Ctx(ctx).Where(c.BotId, botID).Where(c.Status, 1)
	if strings.HasPrefix(identity, "@") {
		mod = mod.Where("LOWER("+c.TelegramUsername+")=?", strings.ToLower(strings.TrimPrefix(identity, "@")))
	} else {
		mod = mod.Where(c.TelegramUserId, identity)
	}
	if err := mod.Limit(2).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "查询Bot用户失败")
	}
	if len(rows) > 1 {
		return nil, gerror.New("用户名存在冲突，请使用用户ID")
	}
	if len(rows) == 0 {
		return nil, nil
	}
	u := rows[0]
	return &relayPeer{ID: u.TelegramUserId, Name: firstNonEmpty(strings.TrimSpace(u.TelegramFirstName+" "+u.TelegramLastName), u.TelegramUsername, u.TelegramUserId), Kind: "user"}, nil
}

func (s *sSysBot) relayResolvePeer(ctx context.Context, botID int64, identity string) (*relayPeer, error) {
	if !strings.HasPrefix(identity, "-") {
		user, err := s.relayKnownUser(ctx, botID, identity)
		if err != nil {
			return nil, err
		}
		if user != nil {
			if err := s.relayVerifyUsername(ctx, botID, identity, user.ID); err != nil {
				return nil, err
			}
			return user, nil
		}
	}
	var groups []*struct {
		ChatId    string `json:"chat_id"`
		ChatTitle string `json:"chat_title"`
	}
	c := dao.YoubanBotChannelCache.Columns()
	mod := dao.YoubanBotChannelCache.Ctx(ctx).Where(c.BotId, botID).WhereIn(c.ChatType, []string{"group", "supergroup"})
	if strings.HasPrefix(identity, "@") {
		mod = mod.Where("LOWER("+c.ChatUsername+")=?", strings.ToLower(strings.TrimPrefix(identity, "@")))
	} else {
		mod = mod.Where(c.ChatId, identity)
	}
	if err := mod.Limit(2).Scan(&groups); err != nil {
		return nil, gerror.Wrap(err, "查询Bot群聊失败")
	}
	if len(groups) > 1 {
		return nil, gerror.New("群用户名存在冲突，请使用群ID")
	}
	if len(groups) == 1 {
		if err := s.relayVerifyUsername(ctx, botID, identity, groups[0].ChatId); err != nil {
			return nil, err
		}
		return &relayPeer{ID: groups[0].ChatId, Name: firstNonEmpty(groups[0].ChatTitle, groups[0].ChatId), Kind: "group"}, nil
	}
	return nil, gerror.New("未找到聊天对象，请先让用户私聊当前Bot，或将当前Bot添加到群中并发送消息")
}

func (s *sSysBot) relayValidatePeer(ctx context.Context, botID int64, peer *relayPeer) error {
	row, err := s.relayBotById(ctx, botID)
	if err != nil {
		return err
	}
	bot, err := s.relayBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	chat, err := bot.GetChat(ctx, &tgbot.GetChatParams{ChatID: peer.ID})
	if err != nil {
		return gerror.Wrap(err, "Bot无法访问绑定对象，请先完成私聊或加群")
	}
	if chat.Type != models.ChatTypePrivate && chat.Type != models.ChatTypeGroup && chat.Type != models.ChatTypeSupergroup {
		return gerror.New("仅支持用户私聊和群聊绑定")
	}
	if chat.Type != models.ChatTypePrivate {
		member, err := bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{ChatID: peer.ID, UserID: bot.ID()})
		if err != nil {
			return gerror.Wrap(err, "检查Bot群权限失败")
		}
		if member == nil || member.Type == models.ChatMemberTypeLeft || member.Type == models.ChatMemberTypeBanned {
			return gerror.New("Bot已离开绑定群")
		}
	}
	return nil
}

func relayPairKey(left, right string) string {
	if left > right {
		left, right = right, left
	}
	return left + ":" + right
}

func (s *sSysBot) relayResolvePendingAdmins(ctx context.Context, botID int64, msg *models.Message) error {
	if msg.From == nil || msg.From.Username == "" || msg.From.IsBot || msg.Chat.Type != models.ChatTypePrivate {
		return nil
	}
	c := dao.YoubanBotRelayAdmin.Columns()
	_, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Where(c.BotId, botID).Where(c.UserId, "").Where(c.Username, "@"+strings.ToLower(msg.From.Username)).Data(do.YoubanBotRelayAdmin{UserId: strconv.FormatInt(msg.From.ID, 10)}).Update()
	return gerror.Wrap(err, "确认双向管理员身份失败")
}

func (s *sSysBot) relayBot(ctx context.Context, token string) (*tgbot.Bot, error) {
	if s.relayClientForToken != nil {
		return s.relayClientForToken(ctx, token)
	}
	return s.telegramBot(ctx, token)
}

func (s *sSysBot) relayVerifyUsername(ctx context.Context, botID int64, identity, id string) error {
	if !strings.HasPrefix(identity, "@") {
		return nil
	}
	row, err := s.relayBotById(ctx, botID)
	if err != nil {
		return err
	}
	bot, err := s.relayBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	chat, err := bot.GetChat(ctx, &tgbot.GetChatParams{ChatID: id})
	if err != nil {
		return gerror.Wrap(err, "无法确认当前用户名，请使用聊天ID")
	}
	if chat == nil || !strings.EqualFold(chat.Username, strings.TrimPrefix(identity, "@")) {
		return gerror.New("用户名已变化，请让对象重新发送消息或使用聊天ID")
	}
	return nil
}
