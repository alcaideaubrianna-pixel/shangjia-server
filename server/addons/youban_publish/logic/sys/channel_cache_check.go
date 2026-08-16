package sys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type channelBotAttachTaskPayload struct {
	TenantID    int64   `json:"tenantId"`
	TGAccountID int64   `json:"tgAccountId"`
	ChannelID   string  `json:"channelId"`
	AccessHash  string  `json:"accessHash"`
	BotIDs      []int64 `json:"botIds"`
}

func (s *sSysPublish) AdminChannelCheck(ctx context.Context, in *sysin.ChannelCheckInp) (res *sysin.ChannelCheckModel, err error) {
	checkCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ctx = checkCtx
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	res, err = s.checkAdminChannelBots(ctx, in, account.TenantId, true)
	if err != nil || res == nil {
		return res, err
	}
	if err = s.persistChannelBotPermission(ctx, account.TenantId, in.ChannelId, in.TgAccountId, res.TargetChatId, res.BotResults); err != nil {
		return nil, gerror.Wrap(err, "保存频道 Bot 权限检测结果失败")
	}
	s.refreshAutoDeleteChannelCache(ctx)
	return res, nil
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
		canSend, canDelete, inChannel, message := s.checkBotChannelMember(ctx, botItem, chatID)
		result.CanSendMessage = boolToInt(canSend)
		result.CanDeleteMessages = boolToInt(canDelete)
		result.InChannel = boolToInt(inChannel)
		result.Message = message
		if !canSend || !canDelete {
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
				res.Message = channelCheckTelegramErrorMessage(err)
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
			canSend, canDelete, inChannel, message := s.checkBotChannelMember(ctx, botMap[item.BotId], chatID)
			item.CanSendMessage = boolToInt(canSend)
			item.CanDeleteMessages = boolToInt(canDelete)
			item.InChannel = boolToInt(inChannel)
			item.Message = message
			if canSend {
				item.Status = "success"
			}
		}
		if item.CanSendMessage != 1 || item.CanDeleteMessages != 1 {
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

func (s *sSysPublish) checkBotChannelMember(ctx context.Context, botItem *sysin.BotModel, chatID any) (canSend bool, canDelete bool, inChannel bool, message string) {
	bot, err := s.telegramBot(ctx, botItem.BotToken)
	if err != nil {
		return false, false, false, err.Error()
	}
	profile, err := bot.GetMe(ctx)
	if err != nil {
		return false, false, false, err.Error()
	}
	member, err := bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{
		ChatID: chatID,
		UserID: profile.ID,
	})
	if err != nil {
		return false, false, false, channelBotMemberErrorMessage(err.Error())
	}
	canSend = telegramBotCanSendMessage(member)
	canDelete = telegramBotCanDeleteMessage(member)
	inChannel = member.Type != models.ChatMemberTypeLeft && member.Type != models.ChatMemberTypeBanned
	if canSend && canDelete {
		return true, true, inChannel, "Bot 可发送和删除消息"
	}
	if !canSend && !canDelete {
		return false, false, inChannel, "Bot 已加入频道但没有发送和删除消息权限"
	}
	if !canSend {
		return false, canDelete, inChannel, "Bot 已加入频道但没有发送消息权限"
	}
	return canSend, false, inChannel, "Bot 已加入频道但没有删除消息权限"
}

func channelBotMemberErrorMessage(raw string) string {
	text := strings.ToLower(strings.TrimSpace(raw))
	if strings.Contains(text, "member list is inaccessible") {
		return "暂时无法读取频道中的 Bot 状态。请确认 Bot 已加入频道并已设置为管理员，同时开启“发布消息”和“删除消息”权限，然后重新检测"
	}
	if strings.Contains(text, "chat not found") {
		return "无法找到该频道。请确认频道仍然存在，并检查频道配置是否正确，然后重新检测"
	}
	return "暂时无法读取频道中的 Bot 状态，请确认 Bot 已加入频道并拥有发布、删除消息权限，然后重新检测"
}

func channelCheckTelegramErrorMessage(err error) string {
	if err == nil {
		return "频道检查失败，请稍后重试"
	}
	message := strings.ToUpper(err.Error())
	if strings.Contains(message, "TG账号连接正在使用") ||
		strings.Contains(message, "TG账号常驻客户端尚未就绪") ||
		strings.Contains(message, "TG账号常驻客户端正在启动") ||
		strings.Contains(message, "CONTEXT DEADLINE EXCEEDED") {
		return "TG账号正在执行其他操作，请稍后刷新后重试"
	}
	return err.Error()
}

func (s *sSysPublish) attachChannelBots(ctx context.Context, tenantId int64, tgAccountId int64, channel *sysin.ChannelCacheModel, bots []*sysin.BotModel) error {
	payload := channelBotAttachTaskPayload{TenantID: tenantId, TGAccountID: tgAccountId, ChannelID: channel.ChannelId, AccessHash: channel.AccessHash}
	for _, bot := range bots {
		if bot != nil && bot.Id > 0 {
			payload.BotIDs = append(payload.BotIDs, bot.Id)
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return gerror.Wrap(err, "创建频道Bot管理任务失败")
	}
	task, err := collectorservice.AccountTasks().SubmitAndWait(ctx, &collectorin.AccountTaskSubmit{
		TenantID: tenantId, AccountID: tgAccountId,
		TaskType: collectorin.AccountTaskTypeChannelBotAttach,
		TaskKey:  "channel-bot-attach:" + base64.RawURLEncoding.EncodeToString(body),
		Priority: -100, MaxAttempts: 3,
	}, 500*time.Millisecond)
	if err != nil {
		return gerror.Wrap(err, "提交频道Bot管理任务失败")
	}
	if task == nil || task.Status != collectorin.AccountTaskStatusCompleted {
		if task != nil && strings.TrimSpace(task.ErrorMessage) != "" {
			return gerror.New(task.ErrorMessage)
		}
		return gerror.New("频道Bot管理任务未完成，请稍后重新检测")
	}
	return nil
}

func (s *sSysPublish) attachChannelBotsWithClient(ctx context.Context, client *telegram.Client, tenantId int64, tgAccountId int64, channel *sysin.ChannelCacheModel, bots []*sysin.BotModel) error {
	if client == nil {
		return gerror.New("Telegram常驻客户端未就绪")
	}
	account, err := s.adminTgAccountById(ctx, tgAccountId, tenantId)
	if err != nil {
		return err
	}
	lastLoginAt := ""
	if account.LastLoginAt != nil {
		lastLoginAt = account.LastLoginAt.String()
	}
	channelID, err := strconv.ParseInt(channel.ChannelId, 10, 64)
	if err != nil {
		return gerror.New("频道ID无效")
	}
	accessHash, err := strconv.ParseInt(channel.AccessHash, 10, 64)
	if err != nil {
		return gerror.New("频道AccessHash无效，请刷新频道缓存")
	}
	api := client.API()
	inputChannel := &tg.InputChannel{ChannelID: channelID, AccessHash: accessHash}
	for _, botItem := range bots {
		username := strings.TrimPrefix(strings.TrimSpace(botItem.BotUsername), "@")
		if username == "" {
			return gerror.New("Bot缺少用户名，请先刷新Bot")
		}
		resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
		if err != nil {
			logChannelBotRPCError(ctx, "resolve_bot_username", tenantId, tgAccountId, lastLoginAt, channel.ChannelId, botItem, err)
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
			logChannelBotRPCError(ctx, "edit_channel_admin", tenantId, tgAccountId, lastLoginAt, channel.ChannelId, botItem, err)
			if tgerr.Is(err, "ADMINS_TOO_MUCH") {
				return gerror.New("该频道的管理员数量已达到 Telegram 上限，暂时无法自动添加 Bot。请先进入频道管理，移除不再使用的管理员或 Bot，然后重新检测")
			}
			if tgerr.Is(err, "BOTS_TOO_MUCH") {
				return gerror.New("该频道可添加的机器人数量已达到 Telegram 上限，暂时无法继续添加 Bot。请先移除不再使用的机器人，然后重新检测")
			}
			if tgerr.Is(err, "CHAT_ADMIN_REQUIRED") {
				return gerror.New("当前 Telegram 账号没有管理频道管理员的权限，请使用频道所有者或管理员账号重新检测")
			}
			if tgerr.Is(err, "FRESH_CHANGE_ADMINS_FORBIDDEN") {
				return gerror.New("当前TG登录会话处于Telegram安全冷却期，系统暂时无法自动将Bot设置为频道管理员。通常需要等待10～30分钟，然后点击“重新检测”。如需立即使用，请前往Telegram频道的「频道管理 → 管理员 → 添加管理员」，手动将所选Bot添加为管理员，并开启“发布消息”和“删除消息”权限，完成后返回页面重新检测")
			}
			return gerror.Wrap(err, "设置Bot频道管理员权限失败")
		}
	}
	return nil
}

func logChannelBotRPCError(ctx context.Context, operation string, tenantId, tgAccountId int64, tgLastLoginAt, channelId string, botItem *sysin.BotModel, err error) {
	fields := g.Map{
		"operation":        operation,
		"tenant_id":        tenantId,
		"tg_account_id":    tgAccountId,
		"tg_last_login_at": tgLastLoginAt,
		"channel_id":       channelId,
		"error":            err.Error(),
	}
	if botItem != nil {
		fields["bot_id"] = botItem.Id
		fields["bot_username"] = strings.TrimPrefix(botItem.BotUsername, "@")
	}
	if rpcErr, ok := tgerr.As(err); ok {
		fields["rpc_code"] = rpcErr.Code
		fields["rpc_type"] = rpcErr.Type
		fields["rpc_message"] = rpcErr.Message
		fields["rpc_argument"] = rpcErr.Argument
	}
	g.Log().Warning(ctx, "频道Bot Telegram RPC调用失败", fields)
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

func telegramBotCanDeleteMessage(member *models.ChatMember) bool {
	if member == nil {
		return false
	}
	switch member.Type {
	case models.ChatMemberTypeOwner:
		return true
	case models.ChatMemberTypeAdministrator:
		return member.Administrator != nil && member.Administrator.CanDeleteMessages
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
