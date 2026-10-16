package sys

import (
	"context"
	"strconv"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"
	publishservice "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/contexts"

	"github.com/gogf/gf/v2/errors/gerror"
)

type relayActorKey struct{}

// Telegram actors come only from authenticated Telegram updates, never request input.
func relayActorContext(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, relayActorKey{}, userID)
}
func relayOwned(cfg *entity.YoubanBotRelayConfig) bool {
	return cfg != nil && cfg.TenantId > 0 && cfg.AccountId > 0 && cfg.SourceBotId > 0
}
func (s *sSysBot) relayAccount(ctx context.Context) (*botProfileAccount, error) {
	if id, ok := ctx.Value(relayActorKey{}).(int64); ok && id > 0 {
		return s.relayBoundAccount(ctx, id)
	}
	user := contexts.GetUser(ctx)
	if user == nil || user.App != sysin.BotAppApi || user.Id <= 0 {
		return nil, gerror.New("请在主机器人绑定上架端账号后使用双向机器人管理")
	}
	capability, err := publishservice.SysPublish().AccountCapability(ctx, sysin.BotAppApi, user.Id)
	if err != nil {
		return nil, err
	}
	if capability.TenantId <= 0 || capability.AccountId <= 0 {
		return nil, gerror.New("租户账户无效")
	}
	return &botProfileAccount{TenantId: capability.TenantId, AccountId: capability.AccountId, App: sysin.BotAppApi}, nil
}
func (s *sSysBot) relayCheckAccess(ctx context.Context, botID int64) error {
	cfg, err := relayConfigByIdentity(ctx, botID)
	if err != nil {
		return err
	}
	if !relayOwned(cfg) {
		return gerror.New("双向配置未归属租户账户，请创建自己的双向机器人；旧配置需人工迁移")
	}
	if cfg.SourceBotId > 0 {
		valid, err := s.relaySourceBotValid(ctx, cfg)
		if err != nil {
			return err
		}
		if !valid {
			return gerror.New("后台Bot不可用或已变更归属，请先在后台检查")
		}
	}
	if id, ok := ctx.Value(relayActorKey{}).(int64); ok && id > 0 {
		allowed, err := s.relayIsAdmin(ctx, cfg, id)
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}
		return gerror.New("无权管理其他租户账户的双向机器人")
	}
	account, err := s.relayAccount(ctx)
	if err != nil {
		return err
	}
	if cfg.TenantId != account.TenantId || cfg.AccountId != account.AccountId {
		return gerror.New("无权管理其他租户账户的双向机器人")
	}
	return nil
}

func (s *sSysBot) relayCanonicalBotID(ctx context.Context, botID int64) (int64, error) {
	if err := s.relayCheckAccess(ctx, botID); err != nil {
		return 0, err
	}
	cfg, err := relayConfigByIdentity(ctx, botID)
	if err != nil {
		return 0, err
	}
	if cfg == nil || cfg.BotId <= 0 {
		return 0, gerror.New("双向机器人配置不存在")
	}
	return cfg.BotId, nil
}
func (s *sSysBot) relayOwnerTelegramIDs(ctx context.Context, cfg *entity.YoubanBotRelayConfig) ([]string, error) {
	if !relayOwned(cfg) {
		return nil, nil
	}
	var binds []*entity.YoubanBotAccountBind
	err := dao.YoubanBotAccountBind.Ctx(ctx).Where(do.YoubanBotAccountBind{App: sysin.BotAppApi, AccountId: cfg.AccountId, Status: 1}).Scan(&binds)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, bind := range binds {
		id, err := strconv.ParseInt(bind.TelegramUserId, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		account, err := s.relayBoundAccount(ctx, id)
		if err != nil {
			continue
		}
		if account.TenantId == cfg.TenantId && account.AccountId == cfg.AccountId {
			ids = append(ids, bind.TelegramUserId)
		}
	}
	return ids, nil
}

func (s *sSysBot) relayBoundAccount(ctx context.Context, userID int64) (*botProfileAccount, error) {
	if s.relayAccountForUser != nil {
		return s.relayAccountForUser(ctx, userID)
	}
	return s.boundProfileAccountByUser(ctx, userID)
}
