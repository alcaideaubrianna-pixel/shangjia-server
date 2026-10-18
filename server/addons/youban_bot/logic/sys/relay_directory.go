package sys

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

type relayPeer struct {
	ID   string
	Name string
	Kind string
}

var errRelayPeerNotFound = errors.New("未找到聊天对象")

func relayPendingUsername(identity string) string {
	identity = strings.TrimSpace(identity)
	if !strings.HasPrefix(identity, "@") {
		return ""
	}
	username := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(identity, "@")))
	if username == "" {
		return ""
	}
	return "@" + username
}

func relayPeerPending(peer *relayPeer) bool {
	return peer != nil && strings.HasPrefix(peer.ID, "@")
}

func (s *sSysBot) relayResolvePeerOrPending(ctx context.Context, botID int64, identity string) (*relayPeer, error) {
	peer, err := s.relayResolvePeer(ctx, botID, identity)
	if err == nil || relayPendingUsername(identity) == "" || !errors.Is(err, errRelayPeerNotFound) {
		return peer, err
	}
	pending := relayPendingUsername(identity)
	return &relayPeer{ID: pending, Name: pending, Kind: "pending_user"}, nil
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
	return nil, gerror.Wrap(errRelayPeerNotFound, "未找到聊天对象，请先让用户私聊当前Bot，或将当前Bot添加到群中并发送消息")
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

func (s *sSysBot) relayResolvePendingBindings(ctx context.Context, botID int64, msg *models.Message) (*entity.YoubanBotRelayBinding, error) {
	if msg.From == nil || msg.From.IsBot || msg.From.Username == "" || msg.Chat.Type != models.ChatTypePrivate {
		return nil, nil
	}
	pending := "@" + strings.ToLower(msg.From.Username)
	actual := strconv.FormatInt(msg.From.ID, 10)
	var activated *entity.YoubanBotRelayBinding
	err := dao.YoubanBotRelayBinding.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var endpoint *entity.YoubanBotRelayEndpoint
		if err := dao.YoubanBotRelayEndpoint.Ctx(ctx).Where(do.YoubanBotRelayEndpoint{BotId: botID, ChatId: pending}).Scan(&endpoint); err != nil || endpoint == nil {
			return err
		}
		var occupied *entity.YoubanBotRelayEndpoint
		if err := dao.YoubanBotRelayEndpoint.Ctx(ctx).Where(do.YoubanBotRelayEndpoint{BotId: botID, ChatId: actual}).Scan(&occupied); err != nil {
			return err
		}
		if occupied != nil && occupied.BindingId != endpoint.BindingId {
			return gerror.New("该用户已有其他绑定，请管理员先解绑")
		}
		if err := dao.YoubanBotRelayBinding.Ctx(ctx).Where(do.YoubanBotRelayBinding{Id: endpoint.BindingId, BotId: botID}).Scan(&activated); err != nil || activated == nil {
			return err
		}
		name := firstNonEmpty(strings.TrimSpace(msg.From.FirstName+" "+msg.From.LastName), "@"+msg.From.Username, actual)
		if activated.LeftChatId == pending {
			activated.LeftChatId, activated.LeftName = actual, name
		} else if activated.RightChatId == pending {
			activated.RightChatId, activated.RightName = actual, name
		} else {
			return nil
		}
		activated.PairKey = relayPairKey(activated.LeftChatId, activated.RightChatId)
		if _, err := dao.YoubanBotRelayBinding.Ctx(ctx).WherePri(activated.Id).Data(do.YoubanBotRelayBinding{PairKey: activated.PairKey, LeftChatId: activated.LeftChatId, RightChatId: activated.RightChatId, LeftName: activated.LeftName, RightName: activated.RightName}).Update(); err != nil {
			return gerror.Wrap(err, "激活待确认绑定失败")
		}
		_, err := dao.YoubanBotRelayEndpoint.Ctx(ctx).WherePri(endpoint.Id).Data(do.YoubanBotRelayEndpoint{ChatId: actual}).Update()
		return gerror.Wrap(err, "激活待确认绑定端点失败")
	})
	return activated, err
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
