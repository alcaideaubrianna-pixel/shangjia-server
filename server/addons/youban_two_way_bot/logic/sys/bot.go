package sys

import (
	"context"
	"strings"
	"sync"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/addons/youban_two_way_bot/internal/model/entity"
	"hotgo/addons/youban_two_way_bot/service"
)

type sSysTwoWayBot struct {
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
	return gatewayservice.Gateway().Client(ctx, token)
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
