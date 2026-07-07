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
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ChannelCacheListInp{}
	}
	if in.TgAccountId <= 0 {
		return []*sysin.ChannelCacheModel{}, 0, nil
	}
	if err = s.ensureTgAccountsBelongTenant(ctx, []int64{in.TgAccountId}, account.TenantId); err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", account.TenantId).
		Where("tg_account_id", in.TgAccountId)
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(channel_title LIKE ? OR channel_username LIKE ? OR channel_id LIKE ?)", like, like, like)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("last_sync_at").OrderDesc("id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道缓存失败")
	}
	if list == nil {
		list = []*sysin.ChannelCacheModel{}
	}
	return list, totalCount, nil
}

func (s *sSysPublish) AdminChannelCacheRefresh(ctx context.Context, in *sysin.ChannelCacheRefreshInp) (res *sysin.ChannelCacheRefreshModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.TgAccountId <= 0 {
		return nil, gerror.New("请选择TG账号")
	}
	if err = s.ensureTgAccountsBelongTenant(ctx, []int64{in.TgAccountId}, account.TenantId); err != nil {
		return nil, err
	}
	item, err := s.adminTgAccountById(ctx, in.TgAccountId, account.TenantId)
	if err != nil {
		return nil, err
	}
	if item.Status != sysin.PublishTgAccountStatusAuthorized {
		return nil, gerror.New("TG账号未授权，请先刷新账号状态或重新扫码登录")
	}
	channels, err := s.fetchTgAccountChannels(ctx, item)
	if err != nil {
		return nil, gerror.Wrap(err, "同步TG账号频道失败")
	}
	now := gtime.Now()
	for _, channel := range channels {
		if err = s.upsertTgChannelCache(ctx, account.TenantId, in.TgAccountId, channel, now); err != nil {
			return nil, err
		}
	}
	return &sysin.ChannelCacheRefreshModel{
		Count:       len(channels),
		Message:     "频道缓存已更新",
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
	if err := s.ensureTgAccountsBelongTenant(ctx, []int64{in.TgAccountId}, tenantId); err != nil {
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

func (s *sSysPublish) fetchTgAccountChannels(ctx context.Context, item *sysin.TgAccountModel) ([]*tg.Channel, error) {
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, err
	}
	sessionPath, err := s.telegramSessionPathByKey(item.SessionKey)
	if err != nil {
		return nil, err
	}
	options := telegram.Options{SessionStorage: &telegram.FileSessionStorage{Path: sessionPath}}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		return nil, err
	} else if resolver != nil {
		options.Resolver = resolver
	}
	client := telegram.NewClient(conf.AppId, conf.AppHash, options)
	runCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	channels := make([]*tg.Channel, 0)
	err = client.Run(runCtx, func(ctx context.Context) error {
		dialogs, err := client.API().MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			Limit:      100,
			OffsetPeer: &tg.InputPeerEmpty{},
		})
		if err != nil {
			return err
		}
		var chats []tg.ChatClass
		switch data := dialogs.(type) {
		case *tg.MessagesDialogs:
			chats = data.GetChats()
		case *tg.MessagesDialogsSlice:
			chats = data.GetChats()
		default:
			chats = []tg.ChatClass{}
		}
		for _, chat := range chats {
			channel, ok := chat.(*tg.Channel)
			if !ok || channel.Left {
				continue
			}
			channels = append(channels, channel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return channels, nil
}

func (s *sSysPublish) upsertTgChannelCache(ctx context.Context, tenantId int64, tgAccountId int64, channel *tg.Channel, now *gtime.Time) error {
	if channel == nil {
		return nil
	}
	channelId := strconv.FormatInt(channel.ID, 10)
	accessHash, _ := channel.GetAccessHash()
	username, _ := channel.GetUsername()
	adminRights, hasAdminRights := channel.GetAdminRights()
	data := g.Map{
		"tenant_id":         tenantId,
		"merchant_id":       tenantId,
		"tg_account_id":     tgAccountId,
		"channel_id":        channelId,
		"access_hash":       strconv.FormatInt(accessHash, 10),
		"channel_title":     channel.Title,
		"channel_username":  strings.TrimPrefix(username, "@"),
		"is_broadcast":      boolToInt(channel.Broadcast),
		"is_megagroup":      boolToInt(channel.Megagroup),
		"can_post_messages": boolToInt(channel.Creator || (hasAdminRights && adminRights.PostMessages)),
		"can_invite_users":  boolToInt(channel.Creator || (hasAdminRights && adminRights.InviteUsers)),
		"can_add_admins":    boolToInt(channel.Creator || (hasAdminRights && adminRights.AddAdmins)),
		"last_sync_at":      now,
		"updated_at":        now,
	}
	count, err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		Where("channel_id", channelId).
		Count()
	if err != nil {
		return gerror.Wrap(err, "读取频道缓存失败")
	}
	if count > 0 {
		_, err = g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("tg_account_id", tgAccountId).
			Where("channel_id", channelId).
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
	sessionPath, err := s.telegramSessionPathByKey(account.SessionKey)
	if err != nil {
		return err
	}
	options := telegram.Options{SessionStorage: &telegram.FileSessionStorage{Path: sessionPath}}
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
