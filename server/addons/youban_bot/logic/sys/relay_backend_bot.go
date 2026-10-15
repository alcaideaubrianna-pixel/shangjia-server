package sys

import (
	"context"
	"fmt"
	"html"
	"strings"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishservice "hotgo/addons/youban_publish/service"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysBot) relayBackendBotChoices(ctx context.Context, account *botProfileAccount) (string, error) {
	bots, err := s.relayOwnedBackendBots(ctx, account.TenantId, account.AccountId, 0, false)
	if err != nil {
		return "", err
	}

	if len(bots) == 0 {
		return "请先在后台创建自己的双向机器人，再回到这里发送 /relayadmin。这里只能绑定、配置当前租户账户自己创建的Bot。", nil
	}
	lines := []string{"请选择自己在后台创建的双向机器人："}
	for i, bot := range bots {
		if i >= 20 {
			lines = append(lines, "其余Bot请在后台查看ID，再发送 /relayadmin 绑定后台Bot ID。")
			break
		}
		lines = append(lines, fmt.Sprintf("%s @%s\n<code>/relayadmin 绑定后台Bot %d</code>", html.EscapeString(bot.BotName), html.EscapeString(bot.BotUsername), bot.Id))
	}
	return strings.Join(lines, "\n\n"), nil
}

func (s *sSysBot) relayBindBackendBot(ctx context.Context, account *botProfileAccount, sourceID int64) (int64, error) {
	if account == nil || account.TenantId <= 0 || account.AccountId <= 0 || sourceID <= 0 {
		return 0, gerror.New("租户账户或Bot无效")
	}
	var botID int64
	err := dao.YoubanBotRelayConfig.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		sources, err := s.relayOwnedBackendBots(ctx, account.TenantId, account.AccountId, sourceID, true)
		if err != nil {
			return err
		}
		if len(sources) != 1 {
			return gerror.New("只能绑定当前租户账户自己在后台创建且已启用的Bot；请先在后台创建")
		}
		source := sources[0]
		botID, err = s.relayAttachBackendBot(ctx, account, source)
		return err
	})
	if err == nil {
		_ = gatewayservice.Gateway().Refresh(ctx)
	}
	return botID, err
}

// Backend deletion, disabling, token rotation and ownership changes take effect
// before routing/sending; the mirror is only a Gateway/directory adapter.
func (s *sSysBot) relaySourceBotValid(ctx context.Context, cfg *entity.YoubanBotRelayConfig) (bool, error) {
	if cfg == nil || cfg.BotId <= 0 {
		return false, nil
	}
	sources, err := s.relayOwnedBackendBots(ctx, cfg.TenantId, cfg.AccountId, cfg.BotId, false)
	if err != nil {
		return false, err
	}
	if len(sources) != 1 {
		return false, nil
	}
	source := sources[0]

	bot, err := s.botById(ctx, cfg.BotId)
	if err != nil {
		return false, err
	}
	return source.Id == bot.Id && bot.BotType == "two_way" && bot.IsDefault != 1, nil
}

func relayAccountConfigQuery(ctx context.Context, account *botProfileAccount) *gdb.Model {
	return dao.YoubanBotRelayConfig.Ctx(ctx).Where(do.YoubanBotRelayConfig{TenantId: account.TenantId, AccountId: account.AccountId})
}

func (s *sSysBot) relayOwnedBackendBots(ctx context.Context, tenantID, accountID, botID int64, lock bool) ([]*publishsysin.RelayOwnedBotModel, error) {
	if s.relayOwnedBots != nil {
		return s.relayOwnedBots(ctx, tenantID, accountID, botID, lock)
	}
	return publishservice.SysPublish().RelayOwnedBots(ctx, tenantID, accountID, botID, lock)
}
