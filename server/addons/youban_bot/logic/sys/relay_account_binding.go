package sys

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

// Configuration references the existing Bot identity. It never copies a token or creates a Bot.
func (s *sSysBot) relayAttachBackendBot(ctx context.Context, account *botProfileAccount, source *publishsysin.RelayOwnedBotModel) (int64, error) {
	if account == nil || source == nil || source.Id <= 0 || source.TenantId != account.TenantId || source.AccountId != account.AccountId || source.BotType != "two_way" || source.Status != 1 {
		return 0, gerror.New("租户账户或Bot身份无效")
	}
	c := dao.YoubanBotRelayConfig.Columns()
	var old *entity.YoubanBotRelayConfig
	if err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(c.BotId, source.Id).LockUpdate().Scan(&old); err != nil {
		return 0, err
	}
	if old != nil {
		if old.TenantId != account.TenantId || old.AccountId != account.AccountId {
			return 0, gerror.New("该Bot配置归属不一致")
		}
		return source.Id, nil
	}
	_, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: source.Id, TenantId: account.TenantId, AccountId: account.AccountId, Enabled: 0, AuditDatabase: 1}).Insert()
	return source.Id, gerror.Wrap(err, "初始化双向机器人配置失败")
}
