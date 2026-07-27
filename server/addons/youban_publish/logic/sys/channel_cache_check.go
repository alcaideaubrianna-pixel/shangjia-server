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
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
)

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
	resolvedTgAccountId, err := s.resolveTenantTgAccountId(ctx, in.TgAccountId, tenantId)
	if err != nil {
		return nil, err
	}
	in.TgAccountId = resolvedTgAccountId
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
