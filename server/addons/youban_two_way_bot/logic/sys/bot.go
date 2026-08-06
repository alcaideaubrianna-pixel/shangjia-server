package sys

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/net/proxy"

	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/addons/youban_two_way_bot/internal/model/entity"
	"hotgo/addons/youban_two_way_bot/model/input/sysin"
	"hotgo/addons/youban_two_way_bot/service"
)

type sSysTwoWayBot struct {
	botMu         sync.Mutex
	bots          map[string]*tgbot.Bot
	runtimeMu     sync.Mutex
	runtimeCancel context.CancelFunc
	runtimeDone   chan struct{}
}

func init() {
	twoWayBot := NewSysTwoWayBot()
	service.RegisterSysTwoWayBot(twoWayBot)
	gatewayservice.RegisterProvider(&twoWayBotGatewayProvider{twoWayBot: twoWayBot})
	gatewayservice.RegisterProvider(&cooperationGatewayProvider{twoWayBot: twoWayBot})
	gatewayservice.RegisterFeature(&twoWayChatGatewayFeature{twoWayBot: twoWayBot})
	gatewayservice.RegisterFeature(&cooperationGatewayFeature{twoWayBot: twoWayBot})
}

type twoWayBotGatewayProvider struct{ twoWayBot *sSysTwoWayBot }

func (p *twoWayBotGatewayProvider) Name() string { return "youban_two_way_bot" }

func (p *twoWayBotGatewayProvider) ListEnabledBots(ctx context.Context) ([]gatewayservice.BotBinding, error) {
	rows, err := p.twoWayBot.enabledBots(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]gatewayservice.BotBinding, 0, len(rows))
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.BotToken) == "" {
			continue
		}
		items = append(items, gatewayservice.BotBinding{Owner: p.Name(), ReferenceID: row.Id, TenantID: row.TenantId, Token: row.BotToken})
	}
	return items, nil
}

func (p *twoWayBotGatewayProvider) HandleUpdate(ctx context.Context, binding gatewayservice.BotBinding, update *models.Update) error {
	row, err := p.twoWayBot.botById(ctx, binding.ReferenceID, binding.TenantID)
	if err != nil {
		return err
	}
	bot, err := p.twoWayBot.telegramBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	return p.twoWayBot.handleTelegramUpdate(ctx, bot, row, update)
}

type cooperationGatewayProvider struct{ twoWayBot *sSysTwoWayBot }

func (p *cooperationGatewayProvider) Name() string { return "youban_platform_cooperation" }

func (p *cooperationGatewayProvider) ListEnabledBots(ctx context.Context) ([]gatewayservice.BotBinding, error) {
	var rows []*struct {
		Id       int64  `json:"id"`
		TenantId int64  `json:"tenantId"`
		BotToken string `json:"botToken"`
	}
	err := g.DB().Model("hg_youban_two_way_bot_cooperation_config").As("cc").
		Fields("cc.id,cc.tenant_id,pb.bot_token").
		InnerJoin("hg_youban_publish_bot pb", "pb.id=cc.bot_id").
		Where("cc.status", 1).WhereNull("cc.deleted_at").WhereNull("pb.deleted_at").
		Where("NOT EXISTS (SELECT 1 FROM hg_youban_two_way_bot_bot tw WHERE tw.bot_token=pb.bot_token AND tw.status=1 AND tw.deleted_at IS NULL)").
		Scan(&rows)
	if err != nil {
		return nil, err
	}
	items := make([]gatewayservice.BotBinding, 0, len(rows))
	for _, row := range rows {
		items = append(items, gatewayservice.BotBinding{Owner: p.Name(), ReferenceID: row.Id, TenantID: row.TenantId, Token: row.BotToken})
	}
	return items, nil
}

func (p *cooperationGatewayProvider) HandleUpdate(ctx context.Context, binding gatewayservice.BotBinding, update *models.Update) error {
	if update == nil || update.Message == nil || !strings.EqualFold(string(update.Message.Chat.Type), "private") {
		return nil
	}
	bot, err := p.twoWayBot.telegramBot(ctx, binding.Token)
	if err != nil {
		return err
	}
	_, err = p.twoWayBot.handleCooperationPrivateMessage(ctx, bot, &entity.YoubanTwoWayBotBot{Id: -binding.ReferenceID, TenantId: binding.TenantID, BotToken: binding.Token}, update.Message)
	return err
}

func NewSysTwoWayBot() *sSysTwoWayBot {
	return &sSysTwoWayBot{}
}

func (s *sSysTwoWayBot) telegramBot(ctx context.Context, token string) (*tgbot.Bot, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, gerror.New("Bot Token不能为空")
	}
	s.botMu.Lock()
	if s.bots != nil {
		if bot := s.bots[token]; bot != nil {
			s.botMu.Unlock()
			return bot, nil
		}
	}
	s.botMu.Unlock()

	client, err := telegramHTTPClient(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := tgbot.New(token,
		tgbot.WithHTTPClient(21*time.Second, client),
		tgbot.WithSkipGetMe(),
		tgbot.WithAllowedUpdates([]string{"message", "edited_message"}),
		tgbot.WithDefaultHandler(func(updateCtx context.Context, bot *tgbot.Bot, update *models.Update) {
			if update == nil {
				return
			}
			row, err := s.botByToken(updateCtx, token)
			if err != nil {
				g.Log().Warningf(updateCtx, "读取双向机器人Polling配置失败：%+v", err)
				return
			}
			if row == nil || row.Status != sysin.TwoWayBotStatusEnabled {
				return
			}
			if err = s.handleTelegramUpdate(updateCtx, bot, row, update); err != nil {
				g.Log().Warningf(updateCtx, "处理双向机器人Polling消息失败 botId:%d err:%+v", row.Id, err)
			}
		}),
		tgbot.WithErrorsHandler(func(err error) {
			g.Log().Warningf(ctx, "双向机器人 Telegram SDK 错误：%+v", err)
		}),
	)
	if err != nil {
		return nil, err
	}
	s.botMu.Lock()
	if s.bots == nil {
		s.bots = map[string]*tgbot.Bot{}
	}
	s.bots[token] = bot
	s.botMu.Unlock()
	return bot, nil
}

func telegramHTTPClient(ctx context.Context) (*http.Client, error) {
	conf, err := publishTelegramConfig(ctx)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{}
	proxyUrl := strings.TrimSpace(conf.ProxyUrl)
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

func telegramUserTitle(user *models.User) string {
	if user == nil {
		return "unknown"
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name != "" {
		return name
	}
	if user.Username != "" {
		return "@" + strings.TrimPrefix(user.Username, "@")
	}
	return "用户"
}
