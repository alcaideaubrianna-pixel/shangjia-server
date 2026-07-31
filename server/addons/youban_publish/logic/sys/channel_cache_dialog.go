package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type tgDialogCache struct {
	AccessHash      string
	CanAddAdmins    int
	CanInviteUsers  int
	CanPostMessages int
	ChannelId       string
	ChannelTitle    string
	ChannelUsername string
	IsBroadcast     int
	IsMegagroup     int
	ManagementRole  string
}

func resolveChannelCacheDisplayType(item *sysin.ChannelCacheModel) string {
	if item == nil {
		return ""
	}
	channelId := strings.TrimSpace(item.ChannelId)
	if strings.HasPrefix(channelId, "-") || item.IsMegagroup == 1 {
		return "group"
	}
	if item.IsBroadcast == 1 {
		return "channel"
	}
	return ""
}

func (s *sSysPublish) fetchTgAccountChannelCaches(ctx context.Context, item *sysin.TgAccountModel) ([]*tgDialogCache, error) {
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("TG账号不存在")
	}
	var channels []*tgDialogCache
	err := s.executeTelegramAccountOperation(ctx, item.Id, 90*time.Second, func(runCtx context.Context, client *telegram.Client) error {
		var fetchErr error
		channels, fetchErr = fetchTgAccountChannelCachesWithClient(runCtx, client)
		return fetchErr
	})
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG频道对话失败")
	}
	return channels, nil
}

func fetchTgAccountChannelCachesWithClient(ctx context.Context, client *telegram.Client) ([]*tgDialogCache, error) {
	if client == nil {
		return nil, gerror.New("Telegram客户端未初始化")
	}
	const pageLimit = 100
	const maxPages = 1000
	channels := make([]*tgDialogCache, 0)
	seenChannels := make(map[string]struct{})
	seenOffsets := make(map[string]struct{})
	offsetDate := 0
	offsetID := 0
	offsetPeer := tg.InputPeerClass(&tg.InputPeerEmpty{})
	for page := 0; page < maxPages; page++ {
		offsetKey := fmt.Sprintf("%d:%d:%T", offsetDate, offsetID, offsetPeer)
		if _, exists := seenOffsets[offsetKey]; exists {
			return nil, gerror.New("Telegram频道分页游标未推进")
		}
		seenOffsets[offsetKey] = struct{}{}
		dialogs, err := client.API().MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			Limit:      pageLimit,
			OffsetDate: offsetDate,
			OffsetID:   offsetID,
			OffsetPeer: offsetPeer,
		})
		if err != nil {
			return nil, gerror.Wrapf(err, "读取Telegram频道对话失败 page:%d", page+1)
		}
		for _, chat := range tgDialogsChats(dialogs) {
			cache := tgDialogCacheFromChat(chat)
			if cache == nil || strings.TrimSpace(cache.ChannelId) == "" {
				continue
			}
			if _, exists := seenChannels[cache.ChannelId]; exists {
				continue
			}
			seenChannels[cache.ChannelId] = struct{}{}
			channels = append(channels, cache)
		}
		nextDate, nextID, nextPeer, ok := tgDialogsNextOffset(dialogs)
		if !ok {
			return channels, nil
		}
		offsetDate = nextDate
		offsetID = nextID
		offsetPeer = nextPeer
	}
	return nil, gerror.New("Telegram频道对话超过最大分页数量")
}

func tgDialogsChats(dialogs tg.MessagesDialogsClass) []tg.ChatClass {
	switch data := dialogs.(type) {
	case *tg.MessagesDialogs:
		return data.GetChats()
	case *tg.MessagesDialogsSlice:
		return data.GetChats()
	default:
		return []tg.ChatClass{}
	}
}

func tgDialogsNextOffset(dialogs tg.MessagesDialogsClass) (int, int, tg.InputPeerClass, bool) {
	data, ok := dialogs.(*tg.MessagesDialogsSlice)
	if !ok || len(data.Dialogs) == 0 {
		return 0, 0, nil, false
	}
	chats := data.GetChats()
	users := data.GetUsers()
	messages := data.GetMessages()
	for i := len(data.Dialogs) - 1; i >= 0; i-- {
		dialog, ok := data.Dialogs[i].(*tg.Dialog)
		if !ok || dialog.TopMessage <= 0 || dialog.Peer == nil {
			continue
		}
		offsetPeer, ok := tgInputPeerFromPeer(dialog.Peer, chats, users)
		if !ok {
			continue
		}
		return tgDialogMessageDate(messages, dialog.Peer, dialog.TopMessage), dialog.TopMessage, offsetPeer, true
	}
	return 0, 0, nil, false
}

func tgDialogMessageDate(messages []tg.MessageClass, peer tg.PeerClass, messageId int) int {
	for _, message := range messages {
		switch item := message.(type) {
		case *tg.Message:
			if item.ID == messageId && tgPeerEqual(item.PeerID, peer) {
				return item.Date
			}
		case *tg.MessageService:
			if item.ID == messageId && tgPeerEqual(item.PeerID, peer) {
				return item.Date
			}
		}
	}
	return 0
}

func tgInputPeerFromPeer(peer tg.PeerClass, chats []tg.ChatClass, users []tg.UserClass) (tg.InputPeerClass, bool) {
	switch item := peer.(type) {
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: item.ChatID}, true
	case *tg.PeerChannel:
		for _, chat := range chats {
			channel, ok := chat.(*tg.Channel)
			if !ok || channel.ID != item.ChannelID {
				continue
			}
			input := channel.AsInputPeer()
			return input, input != nil
		}
	case *tg.PeerUser:
		for _, userClass := range users {
			user, ok := userClass.(*tg.User)
			if !ok || user.ID != item.UserID {
				continue
			}
			accessHash, ok := user.GetAccessHash()
			if !ok {
				return nil, false
			}
			return &tg.InputPeerUser{UserID: user.ID, AccessHash: accessHash}, true
		}
	}
	return nil, false
}

func tgPeerEqual(left tg.PeerClass, right tg.PeerClass) bool {
	switch l := left.(type) {
	case *tg.PeerChat:
		r, ok := right.(*tg.PeerChat)
		return ok && l.ChatID == r.ChatID
	case *tg.PeerChannel:
		r, ok := right.(*tg.PeerChannel)
		return ok && l.ChannelID == r.ChannelID
	case *tg.PeerUser:
		r, ok := right.(*tg.PeerUser)
		return ok && l.UserID == r.UserID
	default:
		return false
	}
}

func tgDialogCacheFromChat(chat tg.ChatClass) *tgDialogCache {
	switch item := chat.(type) {
	case *tg.Channel:
		if item.Left {
			return nil
		}
		return tgChannelDialogCache(item)
	case *tg.Chat:
		if item.Left || item.Deactivated || item.MigratedTo != nil {
			return nil
		}
		return tgBasicChatDialogCache(item)
	default:
		return nil
	}
}

func tgChannelDialogCache(channel *tg.Channel) *tgDialogCache {
	if channel == nil {
		return nil
	}
	accessHash, _ := channel.GetAccessHash()
	username, _ := channel.GetUsername()
	adminRights, hasAdminRights := channel.GetAdminRights()
	return &tgDialogCache{
		AccessHash:      strconv.FormatInt(accessHash, 10),
		CanAddAdmins:    boolToInt(channel.Creator || (hasAdminRights && adminRights.AddAdmins)),
		CanInviteUsers:  boolToInt(channel.Creator || (hasAdminRights && adminRights.InviteUsers)),
		CanPostMessages: boolToInt(channel.Creator || channel.Megagroup || (hasAdminRights && adminRights.PostMessages)),
		ChannelId:       strconv.FormatInt(channel.ID, 10),
		ChannelTitle:    channel.Title,
		ChannelUsername: strings.TrimPrefix(username, "@"),
		IsBroadcast:     boolToInt(channel.Broadcast),
		IsMegagroup:     boolToInt(channel.Megagroup),
		ManagementRole:  tgChannelManagementRole(channel.Creator, hasAdminRights),
	}
}

func tgBasicChatDialogCache(chat *tg.Chat) *tgDialogCache {
	if chat == nil {
		return nil
	}
	adminRights, hasAdminRights := chat.GetAdminRights()
	return &tgDialogCache{
		CanAddAdmins:    boolToInt(chat.Creator || (hasAdminRights && adminRights.AddAdmins)),
		CanInviteUsers:  boolToInt(chat.Creator || (hasAdminRights && adminRights.InviteUsers)),
		CanPostMessages: 1,
		ChannelId:       "-" + strconv.FormatInt(chat.ID, 10),
		ChannelTitle:    chat.Title,
		ManagementRole:  tgChannelManagementRole(chat.Creator, hasAdminRights),
	}
}

func tgChannelManagementRole(isCreator bool, hasAdminRights bool) string {
	if isCreator {
		return "owner"
	}
	if hasAdminRights {
		return "admin"
	}
	return "member"
}

func normalizeChannelManagementRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "owner", "admin", "member":
		return strings.TrimSpace(strings.ToLower(role))
	default:
		return "member"
	}
}

func channelManagementRoleText(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "owner":
		return "创建者"
	case "admin":
		return "管理员"
	default:
		return "成员"
	}
}

func (s *sSysPublish) upsertTgChannelCache(ctx context.Context, tenantId int64, tgAccountId int64, channel *tg.Channel, now *gtime.Time) error {
	return s.upsertTgDialogCache(ctx, tenantId, tgAccountId, tgChannelDialogCache(channel), now)
}

func (s *sSysPublish) upsertTgDialogCache(ctx context.Context, tenantId int64, tgAccountId int64, channel *tgDialogCache, now *gtime.Time) error {
	if channel == nil {
		return nil
	}
	data := g.Map{
		"tenant_id":         tenantId,
		"merchant_id":       tenantId,
		"tg_account_id":     tgAccountId,
		"channel_id":        channel.ChannelId,
		"access_hash":       channel.AccessHash,
		"channel_title":     channel.ChannelTitle,
		"channel_username":  strings.TrimPrefix(channel.ChannelUsername, "@"),
		"is_broadcast":      channel.IsBroadcast,
		"is_megagroup":      channel.IsMegagroup,
		"management_role":   normalizeChannelManagementRole(channel.ManagementRole),
		"can_post_messages": channel.CanPostMessages,
		"can_invite_users":  channel.CanInviteUsers,
		"can_add_admins":    channel.CanAddAdmins,
		"last_sync_at":      now,
		"updated_at":        now,
	}
	count, err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		Where("channel_id", channel.ChannelId).
		Count()
	if err != nil {
		return gerror.Wrap(err, "读取频道缓存失败")
	}
	if count > 0 {
		_, err = g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("tg_account_id", tgAccountId).
			Where("channel_id", channel.ChannelId).
			Data(data).
			Update()
	} else {
		data["created_at"] = now
		_, err = g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存频道缓存失败")
	}
	return nil
}

func (s *sSysPublish) tgChannelCacheByChannelId(ctx context.Context, tenantId int64, tgAccountId int64, channelId string) (*sysin.ChannelCacheModel, error) {
	var item *sysin.ChannelCacheModel
	ids := tgChannelCacheLookupIds(channelId)
	if len(ids) == 0 {
		return nil, gerror.New("请选择当前TG账号下的频道")
	}
	if err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		WhereIn("channel_id", ids).
		Scan(&item); err != nil {
		return nil, gerror.Wrap(err, "读取频道缓存失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("请先刷新频道缓存，并选择当前TG账号下的频道")
	}
	return item, nil
}

func tgChannelCacheLookupIds(channelId string) []string {
	raw := strings.TrimSpace(channelId)
	if raw == "" {
		return nil
	}
	ids := []string{raw}
	if strings.HasPrefix(raw, "-100") && len(raw) > 4 {
		ids = append(ids, strings.TrimPrefix(raw, "-100"))
	} else if !strings.HasPrefix(raw, "-") {
		ids = append(ids, "-100"+raw)
	}
	return uniqueStrings(ids)
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
