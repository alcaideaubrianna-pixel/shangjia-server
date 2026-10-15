package sys

import (
	"context"
	"strings"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	publishconsts "hotgo/addons/youban_publish/consts"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysBot) relayAttachBackendBot(ctx context.Context, account *botProfileAccount, source *publishsysin.RelayOwnedBotModel) (int64, error) {
	if account == nil || account.TenantId <= 0 || account.AccountId <= 0 || source == nil || source.Id <= 0 || source.TenantId != account.TenantId || source.AccountId != account.AccountId || source.CreatedBy != account.AccountId || source.Status != 1 || source.BotType != publishconsts.PublishBotTypeTwoWay {
		return 0, gerror.New("租户账户或Bot身份无效")
	}
	token := strings.TrimSpace(source.BotToken)
	if token == "" || !strings.Contains(token, ":") {
		return 0, gerror.New("后台Bot Token无效，请先在后台修正")
	}
	var botID int64
	err := dao.YoubanBotRelayConfig.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var feature *botFeatureRow
		if err := dao.YoubanBotFeature.Ctx(ctx).Where(dao.YoubanBotFeature.Columns().FeatureKey, consts.RelayFeatureKey).LockUpdate().Scan(&feature); err != nil {
			return err
		}
		if feature == nil {
			return gerror.New("双向插件尚未初始化，请联系管理员")
		}
		var old *entity.YoubanBotRelayConfig
		if err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(do.YoubanBotRelayConfig{TenantId: account.TenantId, AccountId: account.AccountId}).LockUpdate().Scan(&old); err != nil {
			return err
		}
		if old != nil {
			return gerror.New("当前租户账户已有双向配置，请返回管理菜单")
		}
		var sourceOwner *entity.YoubanBotRelayConfig
		if err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(dao.YoubanBotRelayConfig.Columns().SourceBotId, source.Id).Scan(&sourceOwner); err != nil {
			return err
		}
		if sourceOwner != nil {
			return gerror.New("该后台Bot已绑定双向配置，不能认领或重复绑定")
		}
		var bots []*entity.YoubanBotBot
		if err := dao.YoubanBotBot.Ctx(ctx).Where("("+dao.YoubanBotBot.Columns().BotToken+" = ? OR "+dao.YoubanBotBot.Columns().BotToken+" LIKE ?)", token, strings.Split(token, ":")[0]+":%").LockUpdate().Scan(&bots); err != nil {
			return err
		}
		// Existing global Bots and unowned legacy configurations are never claimed
		// implicitly merely because someone submits their token.
		if len(bots) > 0 {
			return gerror.New("该Bot已在系统注册，请联系管理员确认原归属；不能直接认领全局或其他账户的Bot")
		}
		id, err := dao.YoubanBotBot.Ctx(ctx).Data(do.YoubanBotBot{BotToken: token, BotName: source.BotName, BotUsername: source.BotUsername, IsOfficial: 0, IsDefault: 0, RunMode: "auto", Status: 1}).InsertAndGetId()
		if err != nil {
			return gerror.New("注册Bot失败，该Token可能已被使用")
		}
		botID = id
		_, err = dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: id, TenantId: account.TenantId, AccountId: account.AccountId, OwnerId: account.AccountId, SourceBotId: source.Id, Enabled: 0, AuditDatabase: 1}).Insert()
		if err != nil {
			return gerror.New("创建配置失败，账户或Bot可能已存在配置，请刷新管理列表")
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return botID, nil
}
