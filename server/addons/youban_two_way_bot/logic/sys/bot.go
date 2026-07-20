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

	"hotgo/addons/youban_two_way_bot/model/input/sysin"
	"hotgo/addons/youban_two_way_bot/service"
)

type sSysTwoWayBot struct {
	botMu         sync.Mutex
	bots          map[string]*tgbot.Bot
	runtimeMu     sync.Mutex
	runtimeCancel context.CancelFunc
	runtimeCtx    context.Context
	runtimeDone   chan struct{}
	pollingBots   map[string]struct{}
}

func init() {
	service.RegisterSysTwoWayBot(NewSysTwoWayBot())
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
