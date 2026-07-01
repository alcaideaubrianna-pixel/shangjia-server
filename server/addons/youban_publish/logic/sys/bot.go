package sys

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"golang.org/x/net/proxy"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
	"hotgo/internal/library/contexts"
)

func (s *sSysPublish) AdminBotList(ctx context.Context, in *sysin.BotListInp) (list []*sysin.BotModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.BotListInp{}
	}
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		Where("tenant_id", current.TenantId).
		WhereNull("deleted_at")
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(bot_name LIKE ? OR bot_username LIKE ? OR remark LIKE ?)", like, like, like)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取Bot列表失败")
	}
	if list == nil {
		list = []*sysin.BotModel{}
	}
	return list, totalCount, nil
}

func (s *sSysPublish) ServerBotList(ctx context.Context, in *sysin.BotListInp) (list []*sysin.BotModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.BotListInp{}
	}
	return s.botList(ctx, in)
}

func (s *sSysPublish) ServerBotSave(ctx context.Context, in *sysin.BotSaveInp) (err error) {
	if in == nil {
		return gerror.New("Bot配置不能为空")
	}
	if in.TenantId <= 0 {
		return gerror.New("请选择账号归属")
	}
	return s.saveBot(ctx, in, in.TenantId, contexts.GetUserId(ctx))
}

func (s *sSysPublish) ServerBotDelete(ctx context.Context, in *sysin.BotDeleteInp) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	in.Ids = uniqueIds(in.Ids)
	if _, err = g.DB().Model(publishBotTable).Safe().Ctx(ctx).WhereIn("id", in.Ids).Data(g.Map{
		"deleted_by": contexts.GetUserId(ctx),
		"deleted_at": gtime.Now(),
	}).Update(); err != nil {
		return gerror.Wrap(err, "删除Bot配置失败")
	}
	s.clearTelegramBotCache()
	return nil
}

func (s *sSysPublish) ServerBotRefresh(ctx context.Context, in *sysin.BotRefreshInp) (list []*sysin.BotRefreshModel, err error) {
	if in == nil || len(in.Ids) == 0 {
		return nil, gerror.New("请选择要刷新的Bot")
	}
	return s.refreshBots(ctx, uniqueIds(in.Ids), 0, contexts.GetUserId(ctx))
}

func (s *sSysPublish) AdminBotSave(ctx context.Context, in *sysin.BotSaveInp) (err error) {
	if in == nil {
		return gerror.New("Bot配置不能为空")
	}
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.saveBot(ctx, in, current.TenantId, current.Id)
}

func (s *sSysPublish) botList(ctx context.Context, in *sysin.BotListInp) (list []*sysin.BotModel, totalCount int, err error) {
	mod := g.DB().Model(publishBotTable).Safe().Ctx(ctx).WhereNull("deleted_at")
	if in.TenantId > 0 {
		mod = mod.Where("tenant_id", in.TenantId)
	}
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(bot_name LIKE ? OR bot_username LIKE ? OR remark LIKE ?)", like, like, like)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取Bot列表失败")
	}
	if list == nil {
		list = []*sysin.BotModel{}
	}
	return list, totalCount, nil
}

func (s *sSysPublish) saveBot(ctx context.Context, in *sysin.BotSaveInp, tenantId int64, operatorId int64) (err error) {
	botToken := strings.TrimSpace(in.BotToken)
	if botToken == "" && in.Id <= 0 {
		return gerror.New("Bot Token不能为空")
	}
	if botToken == "" {
		bot, err := s.getBotById(ctx, in.Id, tenantId)
		if err != nil {
			return err
		}
		botToken = bot.BotToken
	}
	tgUser, err := s.telegramBotProfile(ctx, botToken)
	if err != nil {
		return gerror.Wrap(err, "校验Bot Token失败")
	}
	status := in.Status
	if status == 0 {
		status = 1
	}
	if status != 1 && status != 2 {
		return gerror.New("Bot状态不合法")
	}
	botName := strings.TrimSpace(in.BotName)
	if botName == "" {
		botName = telegramBotDisplayName(tgUser)
	}
	data := g.Map{
		"tenant_id":    tenantId,
		"bot_name":     botName,
		"bot_username": strings.TrimPrefix(strings.TrimSpace(tgUser.Username), "@"),
		"bot_token":    botToken,
		"remark":       strings.TrimSpace(in.Remark),
		"status":       status,
		"updated_by":   operatorId,
		"updated_at":   gtime.Now(),
	}
	if in.Id > 0 {
		if tenantId > 0 {
			if err = s.ensureBotsBelongTenant(ctx, []int64{in.Id}, tenantId); err != nil {
				return err
			}
		}
		mod := g.DB().Model(publishBotTable).Safe().Ctx(ctx).Where("id", in.Id).WhereNull("deleted_at")
		if tenantId > 0 {
			mod = mod.Where("tenant_id", tenantId)
		}
		_, err = mod.Data(data).Update()
	} else {
		if tenantId <= 0 {
			return gerror.New("请选择账号归属")
		}
		data["created_by"] = operatorId
		data["created_at"] = gtime.Now()
		_, err = g.DB().Model(publishBotTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存Bot配置失败")
	}
	s.clearTelegramBotCache()
	return nil
}

func (s *sSysPublish) AdminBotDelete(ctx context.Context, in *sysin.BotDeleteInp) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	in.Ids = uniqueIds(in.Ids)
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if err = s.ensureBotsBelongTenant(ctx, in.Ids, current.TenantId); err != nil {
		return err
	}
	if _, err = g.DB().Model(publishBotTable).Safe().Ctx(ctx).WhereIn("id", in.Ids).Where("tenant_id", current.TenantId).Data(g.Map{
		"deleted_by": current.Id,
		"deleted_at": gtime.Now(),
	}).Update(); err != nil {
		return gerror.Wrap(err, "删除Bot配置失败")
	}
	s.clearTelegramBotCache()
	return nil
}

func (s *sSysPublish) AdminBotRefresh(ctx context.Context, in *sysin.BotRefreshInp) (list []*sysin.BotRefreshModel, err error) {
	if in == nil || len(in.Ids) == 0 {
		return nil, gerror.New("请选择要刷新的Bot")
	}
	in.Ids = uniqueIds(in.Ids)
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.refreshBots(ctx, in.Ids, current.TenantId, current.Id)
}

func (s *sSysPublish) refreshBots(ctx context.Context, ids []int64, tenantId int64, operatorId int64) (list []*sysin.BotRefreshModel, err error) {
	var bots []*sysin.BotModel
	mod := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if err = mod.Scan(&bots); err != nil {
		return nil, gerror.Wrap(err, "读取Bot配置失败")
	}
	if len(bots) != len(ids) {
		return nil, gerror.New("存在无权操作的Bot")
	}
	now := gtime.Now()
	list = make([]*sysin.BotRefreshModel, 0, len(bots))
	for _, item := range bots {
		result := &sysin.BotRefreshModel{Id: item.Id, Status: item.Status}
		tgUser, refreshErr := s.telegramBotProfile(ctx, item.BotToken)
		if refreshErr != nil {
			result.ErrorMessage = refreshErr.Error()
			_, _ = g.DB().Model(publishBotTable).Safe().Ctx(ctx).
				Where("id", item.Id).
				WhereNull("deleted_at").
				Data(g.Map{
					"status":     2,
					"updated_by": operatorId,
					"updated_at": now,
				}).
				Update()
			result.Status = 2
			list = append(list, result)
			continue
		}
		username := strings.TrimPrefix(strings.TrimSpace(tgUser.Username), "@")
		result.BotUsername = username
		result.Status = 1
		_, err = g.DB().Model(publishBotTable).Safe().Ctx(ctx).
			Where("id", item.Id).
			WhereNull("deleted_at").
			Data(g.Map{
				"bot_name":     telegramBotDisplayName(tgUser),
				"bot_username": username,
				"status":       1,
				"updated_by":   operatorId,
				"updated_at":   now,
			}).
			Update()
		if err != nil {
			return nil, gerror.Wrap(err, "刷新Bot状态失败")
		}
		list = append(list, result)
	}
	s.clearTelegramBotCache()
	return list, nil
}

func (s *sSysPublish) enabledBots(ctx context.Context, tenantId int64) (list []*sysin.BotModel, err error) {
	mod := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		Where("status", 1).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	} else if tenantId == 0 {
		mod = mod.Where("tenant_id", 0)
	}
	if err = mod.
		OrderAsc("id").
		Scan(&list); err != nil {
		return nil, gerror.Wrap(err, "读取Bot配置失败")
	}
	if list == nil {
		list = []*sysin.BotModel{}
	}
	return list, nil
}

func (s *sSysPublish) getBotById(ctx context.Context, id int64, tenantId int64) (bot *sysin.BotModel, err error) {
	if id <= 0 {
		return nil, gerror.New("Bot ID不能为空")
	}
	mod := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id IN(0, ?)", tenantId)
	}
	if err = mod.
		Scan(&bot); err != nil {
		return nil, gerror.Wrap(err, "读取Bot配置失败")
	}
	if bot == nil || bot.Id <= 0 {
		return nil, gerror.New("Bot配置不存在")
	}
	return bot, nil
}

func (s *sSysPublish) telegramBotProfile(ctx context.Context, botToken string) (*models.User, error) {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return nil, err
	}
	user, err := bot.GetMe(ctx)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsBot {
		return nil, gerror.New("Token不是有效的Telegram Bot")
	}
	return user, nil
}

func (s *sSysPublish) telegramBot(ctx context.Context, botToken string) (*tgbot.Bot, error) {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return nil, gerror.New("Telegram Bot Token未配置")
	}
	conf, err := service.SysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, err
	}
	cacheKey := botToken + "\n" + conf.ProxyUrl
	s.telegramBotMu.Lock()
	if s.telegramBots != nil {
		if bot := s.telegramBots[cacheKey]; bot != nil {
			s.telegramBotMu.Unlock()
			return bot, nil
		}
	}
	s.telegramBotMu.Unlock()

	client, err := telegramHTTPClient(conf.ProxyUrl)
	if err != nil {
		return nil, err
	}
	opts := []tgbot.Option{
		tgbot.WithHTTPClient(21*time.Second, client),
		tgbot.WithAllowedUpdates(tgbot.AllowedUpdates{"message", "edited_message"}),
		tgbot.WithErrorsHandler(func(err error) {
			g.Log().Warningf(ctx, "Telegram SDK错误：%+v", err)
		}),
		tgbot.WithDefaultHandler(s.telegramUpdateHandler),
	}
	if conf.WebhookSecret != "" {
		opts = append(opts, tgbot.WithWebhookSecretToken(conf.WebhookSecret))
	}
	bot, err := tgbot.New(botToken, opts...)
	if err != nil {
		return nil, err
	}
	s.telegramBotMu.Lock()
	if s.telegramBots == nil {
		s.telegramBots = map[string]*tgbot.Bot{}
	}
	s.telegramBots[cacheKey] = bot
	s.telegramBotMu.Unlock()
	return bot, nil
}

func (s *sSysPublish) clearTelegramBotCache() {
	s.telegramBotMu.Lock()
	defer s.telegramBotMu.Unlock()
	s.telegramBots = nil
}

func telegramHTTPClient(proxyUrl string) (*http.Client, error) {
	transport := &http.Transport{}
	proxyUrl = strings.TrimSpace(proxyUrl)
	if proxyUrl == "" {
		return &http.Client{Timeout: 35 * time.Second, Transport: transport}, nil
	}
	parsed, err := url.Parse(proxyUrl)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(parsed, proxy.Direct)
		if err != nil {
			return nil, err
		}
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.Dial(network, address)
		}
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsed)
	default:
		return nil, gerror.New("仅支持 http/https/socks5 代理")
	}
	return &http.Client{Timeout: 35 * time.Second, Transport: transport}, nil
}

func telegramBotDisplayName(user *models.User) string {
	if user == nil {
		return ""
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name != "" {
		return name
	}
	return strings.TrimPrefix(strings.TrimSpace(user.Username), "@")
}
