package sys

import (
	"context"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) AdminChannelCacheList(ctx context.Context, in *sysin.ChannelCacheListInp) (list []*sysin.ChannelCacheModel, totalCount int, err error) {
	if err = ensurePublishTgChannelColumns(ctx); err != nil {
		return nil, 0, err
	}
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ChannelCacheListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	if in.TgAccountId <= 0 {
		return []*sysin.ChannelCacheModel{}, 0, nil
	}
	if err = s.ensureTgAccountBelongsAccount(ctx, in.TgAccountId, account.TenantId, account.Id); err != nil {
		return nil, 0, err
	}
	return s.channelCacheList(ctx, in, account.TenantId)
}

func (s *sSysPublish) ServerChannelCacheList(ctx context.Context, in *sysin.ChannelCacheListInp) (list []*sysin.ChannelCacheModel, totalCount int, err error) {
	if err = ensurePublishTgChannelColumns(ctx); err != nil {
		return nil, 0, err
	}
	if err = s.requireSystemSuperAdmin(ctx); err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ChannelCacheListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	if in.TgAccountId <= 0 {
		return []*sysin.ChannelCacheModel{}, 0, nil
	}
	tenantId, err := s.tenantIdForTgAccount(ctx, in.TgAccountId)
	if err != nil {
		return nil, 0, err
	}
	return s.channelCacheList(ctx, in, tenantId)
}

func (s *sSysPublish) channelCacheList(ctx context.Context, in *sysin.ChannelCacheListInp, tenantId int64) (list []*sysin.ChannelCacheModel, totalCount int, err error) {
	mod := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("tg_account_id", in.TgAccountId)
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(channel_title LIKE ? OR channel_username LIKE ? OR channel_id LIKE ?)", like, like, like)
	}
	if len(in.ManagementRoles) > 0 {
		mod = mod.WhereIn("management_role", in.ManagementRoles)
	}
	if in.CanPostMessages == 1 {
		mod = mod.Where("can_post_messages", 1)
	}
	if in.CanInviteUsers == 1 {
		mod = mod.Where("can_invite_users", 1)
	}
	if in.CanAddAdmins == 1 {
		mod = mod.Where("can_add_admins", 1)
	}
	switch in.DisplayType {
	case "channel":
		mod = mod.Where("channel_id NOT LIKE '-%'").Where("is_broadcast", 1)
	case "group":
		mod = mod.Where("(channel_id LIKE '-%' OR is_megagroup = 1)")
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("last_sync_at").OrderDesc("id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道缓存失败")
	}
	if list == nil {
		list = []*sysin.ChannelCacheModel{}
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		item.DisplayType = resolveChannelCacheDisplayType(item)
		item.ManagementRole = normalizeChannelManagementRole(item.ManagementRole)
	}
	return list, totalCount, nil
}

func (s *sSysPublish) AdminChannelCacheResolve(ctx context.Context, in *sysin.ChannelCacheResolveInp) ([]*sysin.ChannelCacheResolveModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("解析目标不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if err = s.ensureTgAccountBelongsAccount(ctx, in.TgAccountId, account.TenantId, account.Id); err != nil {
		return nil, err
	}
	displays, err := s.resolveTelegramChannelDisplays(ctx, account.TenantId, in.TgAccountId, in.TargetChatIds)
	if err != nil {
		return nil, err
	}
	list := make([]*sysin.ChannelCacheResolveModel, 0, len(in.TargetChatIds))
	for _, raw := range in.TargetChatIds {
		channelId := normalizeTelegramChannelChatID(raw)
		if channelId == "" {
			continue
		}
		display := displays[channelId]
		list = append(list, &sysin.ChannelCacheResolveModel{
			TgAccountId:     in.TgAccountId,
			ChannelId:       channelId,
			ChannelTitle:    display.Title,
			ChannelUsername: display.Username,
		})
	}
	return list, nil
}

func (s *sSysPublish) AdminChannelCacheRefresh(ctx context.Context, in *sysin.ChannelCacheRefreshInp) (res *sysin.ChannelCacheRefreshModel, err error) {
	if err = ensurePublishTgChannelColumns(ctx); err != nil {
		return nil, err
	}
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.TgAccountId <= 0 {
		return nil, gerror.New("请选择TG账号")
	}
	if err = s.ensureTgAccountBelongsAccount(ctx, in.TgAccountId, account.TenantId, account.Id); err != nil {
		return nil, err
	}
	item, err := s.adminTgAccountById(ctx, in.TgAccountId, account.TenantId)
	if err != nil {
		return nil, err
	}
	if item.Status != sysin.PublishTgAccountStatusAuthorized {
		return nil, gerror.New("TG账号未授权，请先刷新账号状态或重新扫码登录")
	}
	channels, err := s.fetchTgAccountChannelCaches(ctx, item)
	if err != nil {
		if isTelegramAuthKeyUnregistered(err) {
			s.expireTgAccountSession(context.Background(), item.Id, account.TenantId, account.Id, tgAccountSessionExpiredMessage)
			return nil, gerror.New(tgAccountSessionExpiredMessage)
		}
		return nil, gerror.Wrap(err, "同步TG账号频道失败")
	}
	now := gtime.Now()
	for _, channel := range channels {
		if err = s.upsertTgDialogCache(ctx, account.TenantId, in.TgAccountId, channel, now); err != nil {
			return nil, err
		}
	}
	return &sysin.ChannelCacheRefreshModel{
		Count:       len(channels),
		Message:     "群聊 / 频道缓存已更新",
		SyncedAt:    now.String(),
		TgAccountId: in.TgAccountId,
	}, nil
}

func (s *sSysPublish) AdminChannelCheck(ctx context.Context, in *sysin.ChannelCheckInp) (res *sysin.ChannelCheckModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.checkAdminChannelBots(ctx, in, account.TenantId, true)
}

func (s *sSysPublish) checkAdminChannelBots(ctx context.Context, in *sysin.ChannelCheckInp, tenantId int64, tryAttachBot bool) (*sysin.ChannelCheckModel, error) {
	if in == nil || in.TgAccountId <= 0 {
		return nil, gerror.New("请选择TG账号")
	}
	in.TargetChatId = strings.TrimSpace(in.TargetChatId)
	if in.TargetChatId == "" {
		return nil, gerror.New("请选择频道")
	}
	in.BotIds = uniqueIds(in.BotIds)
	if len(in.BotIds) == 0 {
		return nil, gerror.New("请选择推送资料BOT")
	}
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.ensureTgAccountBelongsAccount(ctx, in.TgAccountId, tenantId, account.Id); err != nil {
		return nil, err
	}
	if err := s.ensureBotsBelongTenant(ctx, in.BotIds, tenantId); err != nil {
		return nil, err
	}
	cache, err := s.tgChannelCacheByChannelId(ctx, tenantId, in.TgAccountId, in.TargetChatId)
	if err != nil {
		return nil, err
	}
	res := &sysin.ChannelCheckModel{
		Allowed:         1,
		CanAddAdmin:     cache.CanAddAdmins,
		CanAddBot:       cache.CanInviteUsers,
		CanInviteUsers:  cache.CanInviteUsers,
		ChannelTitle:    cache.ChannelTitle,
		ChannelUsername: strings.TrimPrefix(cache.ChannelUsername, "@"),
		Message:         "检测通过",
		TargetChatId:    cache.ChannelId,
		BotResults:      []*sysin.ChannelCheckBotModel{},
	}
	bots, err := s.channelCheckBots(ctx, in.BotIds, tenantId)
	if err != nil {
		return nil, err
	}
	chatID := telegramBotChatID(cache)
	needAttach := false
	for _, botItem := range bots {
		result := &sysin.ChannelCheckBotModel{
			BotId:       botItem.Id,
			BotName:     botItem.BotName,
			BotUsername: strings.TrimPrefix(botItem.BotUsername, "@"),
			Status:      "success",
			Message:     "Bot 可发送消息",
		}
		canSend, inChannel, message := s.checkBotChannelMember(ctx, botItem, chatID)
		result.CanSendMessage = boolToInt(canSend)
		result.InChannel = boolToInt(inChannel)
		result.Message = message
		if !canSend {
			needAttach = true
			result.Status = "warning"
		}
		res.BotResults = append(res.BotResults, result)
	}
	if needAttach {
		if cache.CanAddAdmins != 1 {
			res.Allowed = 0
			res.Message = "Bot 未加入频道或无发送权限，且当前 TG 账号没有添加管理员权限，无法添加 Bot"
			return res, nil
		}
		if tryAttachBot {
			if err = s.attachChannelBots(ctx, tenantId, in.TgAccountId, cache, bots); err != nil {
				res.Allowed = 0
				res.Message = err.Error()
				return res, nil
			}
			res.Message = "已尝试设置 Bot 为频道管理员，请稍后刷新状态确认"
		} else {
			res.Message = "Bot 未加入频道或无发送权限，当前 TG 账号具备添加管理员权限"
		}
	}
	botMap := make(map[int64]*sysin.BotModel, len(bots))
	for _, botItem := range bots {
		botMap[botItem.Id] = botItem
	}
	for _, item := range res.BotResults {
		if item.CanSendMessage != 1 && tryAttachBot {
			canSend, inChannel, message := s.checkBotChannelMember(ctx, botMap[item.BotId], chatID)
			item.CanSendMessage = boolToInt(canSend)
			item.InChannel = boolToInt(inChannel)
			item.Message = message
			if canSend {
				item.Status = "success"
			}
		}
		if item.CanSendMessage != 1 {
			res.Allowed = 0
			if res.Message == "检测通过" || res.Message == "已尝试设置 Bot 为频道管理员，请稍后刷新状态确认" {
				res.Message = "Bot 仍未具备发送消息权限，请确认频道管理员权限后重试"
			}
		}
	}
	return res, nil
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

func (s *sSysPublish) fetchTgAccountChannelCaches(ctx context.Context, item *sysin.TgAccountModel) ([]*tgDialogCache, error) {
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, err
	}
	storage, err := s.telegramSessionStorage(item.SessionKey)
	if err != nil {
		return nil, err
	}
	options := telegram.Options{SessionStorage: storage}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		return nil, err
	} else if resolver != nil {
		options.Resolver = resolver
	}
	client := telegram.NewClient(conf.AppId, conf.AppHash, options)
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	channels := make([]*tgDialogCache, 0)
	seen := make(map[string]struct{})
	err = client.Run(runCtx, func(ctx context.Context) error {
		const pageLimit = 100
		offsetDate := 0
		offsetID := 0
		offsetPeer := tg.InputPeerClass(&tg.InputPeerEmpty{})
		for {
			dialogs, err := client.API().MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
				Limit:      pageLimit,
				OffsetDate: offsetDate,
				OffsetID:   offsetID,
				OffsetPeer: offsetPeer,
			})
			if err != nil {
				return err
			}
			pageChats := tgDialogsChats(dialogs)
			for _, chat := range pageChats {
				cache := tgDialogCacheFromChat(chat)
				if cache == nil || strings.TrimSpace(cache.ChannelId) == "" {
					continue
				}
				if _, ok := seen[cache.ChannelId]; ok {
					continue
				}
				seen[cache.ChannelId] = struct{}{}
				channels = append(channels, cache)
			}
			nextDate, nextID, nextPeer, ok := tgDialogsNextOffset(dialogs)
			if !ok {
				break
			}
			offsetDate = nextDate
			offsetID = nextID
			offsetPeer = nextPeer
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return channels, nil
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

func (s *sSysPublish) channelCheckBots(ctx context.Context, ids []int64, tenantId int64) ([]*sysin.BotModel, error) {
	var bots []*sysin.BotModel
	if err := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		Where("status", 1).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&bots); err != nil {
		return nil, gerror.Wrap(err, "读取Bot配置失败")
	}
	if len(bots) != len(ids) {
		return nil, gerror.New("存在不可用或无权操作的Bot")
	}
	return bots, nil
}

func (s *sSysPublish) checkBotChannelMember(ctx context.Context, botItem *sysin.BotModel, chatID any) (canSend bool, inChannel bool, message string) {
	bot, err := s.telegramBot(ctx, botItem.BotToken)
	if err != nil {
		return false, false, err.Error()
	}
	profile, err := bot.GetMe(ctx)
	if err != nil {
		return false, false, err.Error()
	}
	member, err := bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{
		ChatID: chatID,
		UserID: profile.ID,
	})
	if err != nil {
		return false, false, "Bot 未加入频道或无法读取频道成员状态：" + err.Error()
	}
	canSend = telegramBotCanSendMessage(member)
	inChannel = member.Type != models.ChatMemberTypeLeft && member.Type != models.ChatMemberTypeBanned
	if canSend {
		return true, inChannel, "Bot 可发送消息"
	}
	return false, inChannel, "Bot 已加入频道但没有发送消息权限"
}

func (s *sSysPublish) attachChannelBots(ctx context.Context, tenantId int64, tgAccountId int64, channel *sysin.ChannelCacheModel, bots []*sysin.BotModel) error {
	account, err := s.adminTgAccountById(ctx, tgAccountId, tenantId)
	if err != nil {
		return err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return err
	}
	storage, err := s.telegramSessionStorage(account.SessionKey)
	if err != nil {
		return err
	}
	options := telegram.Options{SessionStorage: storage}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		return err
	} else if resolver != nil {
		options.Resolver = resolver
	}
	channelID, err := strconv.ParseInt(channel.ChannelId, 10, 64)
	if err != nil {
		return gerror.New("频道ID无效")
	}
	accessHash, err := strconv.ParseInt(channel.AccessHash, 10, 64)
	if err != nil {
		return gerror.New("频道AccessHash无效，请刷新频道缓存")
	}
	client := telegram.NewClient(conf.AppId, conf.AppHash, options)
	runCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	return client.Run(runCtx, func(ctx context.Context) error {
		api := client.API()
		inputChannel := &tg.InputChannel{ChannelID: channelID, AccessHash: accessHash}
		for _, botItem := range bots {
			username := strings.TrimPrefix(strings.TrimSpace(botItem.BotUsername), "@")
			if username == "" {
				return gerror.New("Bot缺少用户名，请先刷新Bot")
			}
			resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
			if err != nil {
				return gerror.Wrap(err, "解析Bot用户名失败")
			}
			inputUser, err := resolvedBotInputUser(resolved)
			if err != nil {
				return err
			}
			if _, err = api.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
				Channel: inputChannel,
				UserID:  inputUser,
				AdminRights: tg.ChatAdminRights{
					ChangeInfo:     false,
					PostMessages:   true,
					EditMessages:   true,
					DeleteMessages: true,
					BanUsers:       false,
					InviteUsers:    false,
					PinMessages:    false,
					AddAdmins:      false,
					Other:          true,
					PostStories:    true,
					EditStories:    true,
					DeleteStories:  true,
				},
				Rank: "资料推送",
			}); err != nil {
				return gerror.Wrap(err, "设置Bot频道管理员权限失败")
			}
		}
		return nil
	})
}

func telegramBotChatID(channel *sysin.ChannelCacheModel) any {
	if username := strings.TrimPrefix(strings.TrimSpace(channel.ChannelUsername), "@"); username != "" {
		return "@" + username
	}
	if chatID, err := strconv.ParseInt("-100"+strings.TrimSpace(channel.ChannelId), 10, 64); err == nil {
		return chatID
	}
	return channel.ChannelId
}

func telegramBotCanSendMessage(member *models.ChatMember) bool {
	if member == nil {
		return false
	}
	switch member.Type {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeMember:
		return true
	case models.ChatMemberTypeAdministrator:
		return member.Administrator == nil || member.Administrator.CanPostMessages || member.Administrator.CanManageChat
	case models.ChatMemberTypeRestricted:
		return member.Restricted != nil && member.Restricted.IsMember && member.Restricted.CanSendMessages
	default:
		return false
	}
}

func resolvedBotInputUser(resolved *tg.ContactsResolvedPeer) (*tg.InputUser, error) {
	if resolved == nil {
		return nil, gerror.New("未找到Bot账号")
	}
	for _, item := range resolved.GetUsers() {
		user, ok := item.(*tg.User)
		if !ok || !user.Bot {
			continue
		}
		accessHash, ok := user.GetAccessHash()
		if !ok {
			return nil, gerror.New("Bot AccessHash缺失")
		}
		return &tg.InputUser{UserID: user.ID, AccessHash: accessHash}, nil
	}
	return nil, gerror.New("未找到Bot账号")
}

func channelCheckAllowed(res *sysin.ChannelCheckModel) bool {
	return res != nil && res.Allowed == 1
}

func channelCheckMessage(res *sysin.ChannelCheckModel) string {
	if res == nil || strings.TrimSpace(res.Message) == "" {
		return "频道权限检测未通过"
	}
	return res.Message
}
