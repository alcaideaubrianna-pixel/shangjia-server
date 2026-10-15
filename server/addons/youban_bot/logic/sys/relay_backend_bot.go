package sys

import (
	"context"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

const relayTwoWayBotOwner = "youban_two_way_bot"

func relayAccountConfigQuery(ctx context.Context, account *botProfileAccount) *gdb.Model {
	return dao.YoubanBotRelayConfig.Ctx(ctx).Where(do.YoubanBotRelayConfig{TenantId: account.TenantId, AccountId: account.AccountId})
}

func (s *sSysBot) relayOwnedBackendBots(ctx context.Context, tenantID, accountID, botID int64, lock bool) ([]*publishsysin.RelayOwnedBotModel, error) {
	if s.relayOwnedBots != nil {
		return s.relayOwnedBots(ctx, tenantID, accountID, botID, lock)
	}
	bots, err := gatewayservice.ProviderOwnedBots(ctx, relayTwoWayBotOwner, tenantID, accountID, botID, lock)
	if err != nil {
		return nil, err
	}
	items := make([]*publishsysin.RelayOwnedBotModel, 0, len(bots))
	for _, bot := range bots {
		items = append(items, &publishsysin.RelayOwnedBotModel{
			BotModel:  publishsysin.BotModel{Id: bot.ReferenceID, TenantId: bot.TenantID, BotName: bot.Name, BotUsername: bot.Username, BotToken: bot.Token, BotType: "two_way", Status: bot.Status},
			AccountId: bot.AccountID, CreatedBy: bot.AccountID,
		})
	}
	return items, nil
}

func (s *sSysBot) relaySourceBot(ctx context.Context, cfg *entity.YoubanBotRelayConfig) (*publishsysin.RelayOwnedBotModel, error) {
	if cfg == nil || cfg.BotId <= 0 || cfg.TenantId <= 0 || cfg.AccountId <= 0 {
		return nil, nil
	}
	rows, err := s.relayOwnedBackendBots(ctx, cfg.TenantId, cfg.AccountId, cfg.BotId, false)
	if err != nil || len(rows) != 1 {
		return nil, err
	}
	return rows[0], nil
}

func (s *sSysBot) relaySourceBotValid(ctx context.Context, cfg *entity.YoubanBotRelayConfig) (bool, error) {
	bot, err := s.relaySourceBot(ctx, cfg)
	return bot != nil && bot.Status == 1, err
}

func (s *sSysBot) relayBotById(ctx context.Context, botID int64) (*botsysin.BotModel, error) {
	cfg, err := relayConfig(ctx, botID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return s.botById(ctx, botID)
	}
	bot, err := s.relaySourceBot(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if bot == nil {
		return nil, gerror.New("双向机器人不存在或已不属于当前账户")
	}
	return &botsysin.BotModel{Id: bot.Id, BotName: bot.BotName, BotUsername: bot.BotUsername, BotToken: bot.BotToken, Status: bot.Status}, nil
}

func (s *sSysBot) relayEnsureConfig(ctx context.Context, account *botProfileAccount, twoWayBotID int64) (*entity.YoubanBotRelayConfig, error) {
	if account == nil || account.TenantId <= 0 || account.AccountId <= 0 || twoWayBotID <= 0 {
		return nil, gerror.New("租户账户或双向机器人无效")
	}
	var config *entity.YoubanBotRelayConfig
	err := dao.YoubanBotRelayConfig.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		bots, err := s.relayOwnedBackendBots(ctx, account.TenantId, account.AccountId, twoWayBotID, true)
		if err != nil {
			return err
		}
		if len(bots) != 1 {
			return gerror.New("只能配置当前租户账户自己创建的双向机器人")
		}
		if err = dao.YoubanBotRelayConfig.Ctx(ctx).Where(dao.YoubanBotRelayConfig.Columns().BotId, twoWayBotID).Scan(&config); err != nil {
			return err
		}
		if config != nil {
			if config.TenantId != account.TenantId || config.AccountId != account.AccountId {
				return gerror.New("该双向机器人已属于其他租户账户")
			}
			return nil
		}
		_, err = dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{
			BotId: twoWayBotID, TenantId: account.TenantId, AccountId: account.AccountId,
			OwnerId: account.AccountId, SourceBotId: twoWayBotID, Enabled: 0, AuditDatabase: 1,
		}).Insert()
		if err != nil {
			return gerror.Wrap(err, "初始化双向机器人设置失败")
		}
		return dao.YoubanBotRelayConfig.Ctx(ctx).Where(dao.YoubanBotRelayConfig.Columns().BotId, twoWayBotID).Scan(&config)
	})
	return config, err
}

// relayAttachBackendBot remains an idempotent compatibility boundary for old
// callers. It now attaches extension settings to the original two-way Bot and
// never creates a mirror row in hg_youban_bot_bot.
func (s *sSysBot) relayAttachBackendBot(ctx context.Context, account *botProfileAccount, source *publishsysin.RelayOwnedBotModel) (int64, error) {
	if source == nil {
		return 0, gerror.New("双向机器人无效")
	}
	_, err := s.relayEnsureConfig(ctx, account, source.Id)
	return source.Id, err
}

func (s *sSysBot) relayBackendBotChoices(ctx context.Context, account *botProfileAccount) (string, error) {
	bots, err := s.relayOwnedBackendBots(ctx, account.TenantId, account.AccountId, 0, false)
	if err != nil {
		return "", err
	}
	if len(bots) == 0 {
		return "请先在后台创建双向机器人，然后返回这里刷新。", nil
	}
	return "请选择列表中的双向机器人按钮。", nil
}
