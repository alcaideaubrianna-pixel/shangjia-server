package sys

import (
	"context"
	"regexp"
	"strings"
	"time"

	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

var managedBotUsernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}bot$`)

func (s *sSysPublish) AdminBotUsernameCheck(ctx context.Context, in *sysin.BotUsernameCheckInp) (res *sysin.BotUsernameCheckModel, err error) {
	if in == nil {
		return nil, gerror.New("Bot用户名检查参数不能为空")
	}
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	username, err := normalizeManagedBotUsername(in.BotUsername)
	if err != nil {
		return nil, err
	}
	account, err := s.adminTgAccountForBot(ctx, in.TgAccountId, current.TenantId, current.Id)
	if err != nil {
		return nil, err
	}
	if account.Status != sysin.PublishTgAccountStatusAuthorized {
		return nil, gerror.New("TG账号尚未授权，请先完成登录")
	}

	available, checkErr := s.checkManagedBotUsername(ctx, account, username)
	if checkErr != nil {
		return &sysin.BotUsernameCheckModel{
			Available:   false,
			BotUsername: username,
			Message:     managedBotErrorMessage(checkErr),
		}, nil
	}
	if !available {
		return &sysin.BotUsernameCheckModel{
			Available:   false,
			BotUsername: username,
			Message:     "该 Bot 用户名已被占用",
		}, nil
	}
	return &sysin.BotUsernameCheckModel{
		Available:   true,
		BotUsername: username,
		Message:     "Bot 用户名可用",
	}, nil
}

func (s *sSysPublish) AdminBotCreate(ctx context.Context, in *sysin.BotCreateInp) (res *sysin.BotModel, err error) {
	if in == nil {
		return nil, gerror.New("Bot创建参数不能为空")
	}
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.BotName)
	if name == "" {
		return nil, gerror.New("Bot名称不能为空")
	}
	username, err := normalizeManagedBotUsername(in.BotUsername)
	if err != nil {
		return nil, err
	}
	account, err := s.adminTgAccountForBot(ctx, in.TgAccountId, current.TenantId, current.Id)
	if err != nil {
		return nil, err
	}
	if account.Status != sysin.PublishTgAccountStatusAuthorized {
		return nil, gerror.New("TG账号尚未授权，请先完成登录")
	}
	managerUsername, err := s.officialManagerUsername(ctx)
	if err != nil {
		return nil, err
	}
	var created *tg.User
	var token string
	err = s.executeTelegramAccountOperation(ctx, account.Id, 90*time.Second, func(clientCtx context.Context, client *telegram.Client) error {
		manager, resolveErr := resolveManagedBotUser(clientCtx, client, managerUsername)
		if resolveErr != nil {
			return resolveErr
		}
		if !manager.BotCanManageBots {
			return gerror.New("官方Bot尚未开启Bot Management Mode")
		}
		available, checkErr := client.API().BotsCheckUsername(clientCtx, username)
		if checkErr != nil {
			return checkErr
		}
		if !available {
			return gerror.New("该 Bot 用户名已被占用")
		}
		createdUser, createErr := client.API().BotsCreateBot(clientCtx, &tg.BotsCreateBotRequest{
			Name:      name,
			Username:  username,
			ManagerID: &tg.InputUser{UserID: manager.ID, AccessHash: manager.AccessHash},
		})
		if createErr != nil {
			return createErr
		}
		created, _ = createdUser.(*tg.User)
		if created == nil || created.ID <= 0 {
			return gerror.New("Telegram未返回新Bot的有效身份信息")
		}
		return nil
	})
	if err != nil {
		g.Log().Errorf(ctx, "创建Managed Bot失败 tgAccountId:%d username:%s err:%+v", in.TgAccountId, username, err)
		return nil, gerror.New(managedBotErrorMessage(err))
	}
	managerToken, err := botService.SysBot().OfficialBotToken(ctx)
	if err != nil {
		return nil, err
	}
	managerBot, err := s.telegramBot(ctx, managerToken)
	if err != nil {
		return nil, gerror.Wrap(err, "读取官方Bot客户端失败")
	}
	token, err = managerBot.GetManagedBotToken(ctx, &tgbot.GetManagedBotTokenParams{UserID: created.ID})
	if err != nil {
		g.Log().Errorf(ctx, "获取Managed Bot Token失败 botId:%d username:%s err:%+v", created.ID, username, err)
		return nil, gerror.New(managedBotErrorMessage(err))
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, gerror.New("官方Bot未返回新Bot Token，请稍后重试")
	}

	botUsername := username
	if created != nil && strings.TrimSpace(created.Username) != "" {
		botUsername = strings.TrimPrefix(strings.TrimSpace(created.Username), "@")
	}
	status := in.Status
	if status == 0 {
		status = 1
	}
	if status != 1 && status != 2 {
		return nil, gerror.New("Bot状态不合法")
	}
	now := gtime.Now()
	id, err := g.DB().Model(publishBotTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":    current.TenantId,
		"bot_name":     name,
		"bot_username": botUsername,
		"bot_token":    token,
		"remark":       strings.TrimSpace(in.Remark),
		"status":       status,
		"created_by":   current.Id,
		"created_at":   now,
		"updated_by":   current.Id,
		"updated_at":   now,
	}).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "保存Bot配置失败")
	}
	s.clearTelegramBotCache()
	if err = gatewayservice.Gateway().Refresh(ctx); err != nil {
		g.Log().Warningf(ctx, "刷新TG Bot Gateway失败 bot:%d err:%+v", id, err)
	}
	return &sysin.BotModel{
		Id:          id,
		TenantId:    current.TenantId,
		BotName:     name,
		BotUsername: botUsername,
		BotToken:    token,
		Remark:      strings.TrimSpace(in.Remark),
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *sSysPublish) checkManagedBotUsername(ctx context.Context, account *sysin.TgAccountModel, username string) (available bool, err error) {
	returnValue := false
	err = s.executeTelegramAccountOperation(ctx, account.Id, 45*time.Second, func(clientCtx context.Context, client *telegram.Client) error {
		returnValue, err = client.API().BotsCheckUsername(clientCtx, username)
		return err
	})
	return returnValue, err
}

func (s *sSysPublish) officialManagerUsername(ctx context.Context) (string, error) {
	token, err := botService.SysBot().OfficialBotToken(ctx)
	if err != nil {
		return "", err
	}
	profile, err := s.telegramBotProfile(ctx, token)
	if err != nil {
		return "", gerror.Wrap(err, "读取官方Bot信息失败")
	}
	username := strings.TrimPrefix(strings.TrimSpace(profile.Username), "@")
	if username == "" {
		return "", gerror.New("官方Bot没有配置用户名")
	}
	return username, nil
}

func (s *sSysPublish) adminTgAccountForBot(ctx context.Context, id int64, tenantId int64, accountId int64) (*sysin.TgAccountModel, error) {
	if id <= 0 {
		return nil, gerror.New("请选择TG账号")
	}
	var item *sysin.TgAccountModel
	if err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		Scan(&item); err != nil {
		return nil, gerror.Wrap(err, "读取TG账号失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("TG账号不存在或无权使用")
	}
	return item, nil
}

func normalizeManagedBotUsername(value string) (string, error) {
	username := strings.TrimPrefix(strings.TrimSpace(value), "@")
	if !managedBotUsernamePattern.MatchString(username) {
		return "", gerror.New("Bot用户名格式不合法，需以bot结尾且只支持字母、数字和下划线")
	}
	return username, nil
}

func resolveManagedBotUser(ctx context.Context, client *telegram.Client, username string) (*tg.User, error) {
	resolved, err := client.API().ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, err
	}
	for _, item := range resolved.Users {
		if user, ok := item.(*tg.User); ok && user.Bot {
			return user, nil
		}
	}
	return nil, gerror.New("无法解析官方Bot")
}

func managedBotErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToUpper(err.Error())
	switch {
	case strings.Contains(message, "USERNAME_OCCUPIED"):
		return "该 Bot 用户名已被占用"
	case strings.Contains(message, "BOT 用户名已被占用"):
		return "该 Bot 用户名已被占用"
	case strings.Contains(message, "USERNAME_INVALID"):
		return "Bot 用户名格式不合法"
	case strings.Contains(message, "USERNAME_PURCHASE_AVAILABLE"):
		return "该 Bot 用户名需要购买后才能使用"
	case strings.Contains(message, "CREATE_BOT_BLOCKED"):
		return "当前TG账号已被Telegram限制创建机器人，请切换其他TG账号后重试"
	case strings.Contains(message, "BOT_CREATE_LIMIT_EXCEEDED"):
		return "当前TG账号创建的机器人数量已达上限，请切换其他TG账号后重试"
	case strings.Contains(message, "MANAGER_PERMISSION_MISSING"):
		return "官方Bot尚未开启Bot Management Mode"
	case strings.Contains(message, "官方BOT尚未开启BOT MANAGEMENT MODE"):
		return "官方Bot尚未开启Bot Management Mode"
	case strings.Contains(message, "SESSION_PASSWORD_NEEDED"):
		return "TG账号需要二次验证，请先完成授权"
	case strings.Contains(message, "AUTH_KEY_DUPLICATED"):
		return "TG账号 session 被重复使用，请停止该账号的其他任务后重试"
	case strings.Contains(message, "SESSION_REVOKED"), strings.Contains(message, "SESSION_EXPIRED"):
		return "TG账号 session 已失效，请重新登录"
	case strings.Contains(message, "ACCOUNT_NOT_FOUND"):
		return "TG账号不存在或已被删除，请重新选择账号"
	case strings.Contains(message, "USER_ID_INVALID"):
		return "官方Bot身份无效，请检查官方Bot配置"
	case strings.Contains(message, "FLOOD_WAIT"):
		return "Telegram请求过于频繁，请稍后再试"
	case strings.Contains(message, "CONTEXT DEADLINE EXCEEDED"):
		return "Telegram连接超时，请检查网络或代理配置"
	default:
		return "Telegram操作失败，请稍后重试"
	}
}
