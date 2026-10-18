package sys

import (
	"context"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"
)

func relayConfig(ctx context.Context, botID int64) (*entity.YoubanBotRelayConfig, error) {
	var row *entity.YoubanBotRelayConfig
	columns := dao.YoubanBotRelayConfig.Columns()
	err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(columns.BotId, botID).Scan(&row)
	return row, gerror.Wrap(err, "读取双向机器人设置失败")
}

func relayConfigByIdentity(ctx context.Context, botID int64) (*entity.YoubanBotRelayConfig, error) {
	row, err := relayConfig(ctx, botID)
	if err == nil && row == nil {
		columns := dao.YoubanBotRelayConfig.Columns()
		err = dao.YoubanBotRelayConfig.Ctx(ctx).Where(columns.SourceBotId, botID).Scan(&row)
	}
	return row, gerror.Wrap(err, "读取双向机器人设置失败")
}

func (s *sSysBot) AdminRelayBotList(ctx context.Context, in *sysin.BotListInp) ([]*sysin.RelayBotModel, int, error) {
	account, err := s.relayAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	var cfg *entity.YoubanBotRelayConfig
	err = dao.YoubanBotRelayConfig.Ctx(ctx).Where(do.YoubanBotRelayConfig{TenantId: account.TenantId, AccountId: account.AccountId}).Scan(&cfg)
	if err != nil {
		return nil, 0, err
	}
	if cfg == nil {
		return []*sysin.RelayBotModel{}, 0, nil
	}
	bot, err := s.relayBotById(ctx, cfg.BotId)
	if err != nil {
		return nil, 0, err
	}
	return []*sysin.RelayBotModel{{Id: bot.Id, BotName: bot.BotName, BotUsername: bot.BotUsername, Status: bot.Status, Enabled: cfg.Enabled, BlockLinks: cfg.BlockLinks, AuditDatabase: cfg.AuditDatabase, AuditGroupId: cfg.AuditGroupId}}, 1, nil

}

func (s *sSysBot) AdminRelaySettings(ctx context.Context, in *sysin.RelayBotInp) (*sysin.RelaySettingsModel, error) {
	botID, err := s.relayCanonicalBotID(ctx, in.BotId)
	if err != nil {
		return nil, err
	}
	in.BotId = botID
	if err := in.Filter(); err != nil {
		return nil, err
	}
	if _, err := s.relayBotById(ctx, in.BotId); err != nil {
		return nil, err
	}
	cfg, err := relayConfig(ctx, botID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &entity.YoubanBotRelayConfig{BotId: in.BotId, AuditDatabase: 1}
	}
	admins := make([]*entity.YoubanBotRelayAdmin, 0)
	err = dao.YoubanBotRelayAdmin.Ctx(ctx).Where(dao.YoubanBotRelayAdmin.Columns().BotId, botID).OrderAsc(dao.YoubanBotRelayAdmin.Columns().Id).Scan(&admins)
	return &sysin.RelaySettingsModel{YoubanBotRelayConfig: cfg, Admins: admins}, gerror.Wrap(err, "读取双向机器人管理员失败")
}

func (s *sSysBot) adminRelaySaveSettings(ctx context.Context, in *sysin.RelaySettingsInp) error {
	if err := in.Filter(); err != nil {
		return err
	}
	if err := s.relayCheckAccess(ctx, in.BotId); err != nil {
		return err
	}
	bot, err := s.relayBotById(ctx, in.BotId)
	if err != nil {
		return err
	}
	if bot.IsDefault == 1 {
		return gerror.New("默认主机器人不能配置为双向机器人，请选择独立Bot")
	}
	if in.AuditGroupId != "" {
		if err = s.validateRelayAuditGroup(ctx, bot.BotToken, in.BotId, in.AuditGroupId); err != nil {
			return err
		}
	}
	admins, err := s.resolveRelayAdmins(ctx, in.BotId, in.Admins)
	if err != nil {
		return err
	}
	return dao.YoubanBotRelayConfig.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var cfg *entity.YoubanBotRelayConfig
		err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(dao.YoubanBotRelayConfig.Columns().BotId, in.BotId).LockUpdate().Scan(&cfg)
		if err != nil {
			return err
		}
		if in.AuditGroupId != "" {
			n, err := dao.YoubanBotRelayEndpoint.Ctx(ctx).Where(do.YoubanBotRelayEndpoint{BotId: in.BotId, ChatId: in.AuditGroupId}).Count()
			if err != nil {
				return err
			}
			if n > 0 {
				return gerror.New("审计群不能作为双向绑定对象")
			}
		}
		data := do.YoubanBotRelayConfig{BotId: in.BotId, Enabled: in.Enabled, BlockLinks: in.BlockLinks, AuditDatabase: in.AuditDatabase, AuditGroupId: in.AuditGroupId}
		if cfg == nil {
			return gerror.New("请通过主机器人创建租户账户双向配置")
		} else {
			_, err = dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(cfg.Id).Data(data).Update()
		}
		if err != nil {
			return gerror.Wrap(err, "保存双向机器人设置失败")
		}
		if _, err = dao.YoubanBotRelayAdmin.Ctx(ctx).Where(dao.YoubanBotRelayAdmin.Columns().BotId, in.BotId).Delete(); err != nil {
			return gerror.Wrap(err, "更新双向管理员失败")
		}
		for _, admin := range admins {
			if _, err = dao.YoubanBotRelayAdmin.Ctx(ctx).Data(admin).Insert(); err != nil {
				return gerror.Wrap(err, "保存双向管理员失败")
			}
		}
		return nil
	})
}

func (s *sSysBot) resolveRelayAdmins(ctx context.Context, botID int64, values []string) ([]do.YoubanBotRelayAdmin, error) {
	list := make([]do.YoubanBotRelayAdmin, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		userID, username := "", strings.ToLower(value)
		if strings.HasPrefix(value, "@") {
			// Resolve only from a fresh private update, never from historical usernames.

		} else {
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil || id <= 0 {
				return nil, gerror.New("管理员必须是用户，不能是群聊")
			}
			userID = strconv.FormatInt(id, 10)
		}
		key := username
		if userID != "" {
			key = userID
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		list = append(list, do.YoubanBotRelayAdmin{BotId: botID, UserId: userID, Username: username})
	}
	return list, nil
}

func (s *sSysBot) validateRelayAuditGroup(ctx context.Context, token string, botID int64, groupID string) error {
	bot, err := s.relayBot(ctx, token)
	if err != nil {
		return err
	}
	chat, err := bot.GetChat(ctx, &tgbot.GetChatParams{ChatID: groupID})
	if err != nil {
		return gerror.Wrap(err, "无法访问审计群，请先添加机器人")
	}
	if chat.Type != models.ChatTypeSupergroup || !chat.IsForum {
		return gerror.New("审计目标必须是已开启话题的超级群")
	}
	member, err := bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{ChatID: groupID, UserID: bot.ID()})
	if err != nil {
		return gerror.Wrap(err, "检查审计群权限失败")
	}
	if member == nil || member.Administrator == nil || !member.Administrator.CanManageTopics {
		return gerror.New("请授予机器人审计群管理员及管理话题权限")
	}
	n, err := dao.YoubanBotRelayEndpoint.Ctx(ctx).Where(do.YoubanBotRelayEndpoint{BotId: botID, ChatId: groupID}).Count()
	if err != nil {
		return gerror.Wrap(err, "检查审计群绑定失败")
	}
	if n > 0 {
		return gerror.New("审计群不能同时作为双向绑定对象")
	}
	return nil
}

// Tenant/account identity is resolved through the same binding as collection management.
func (s *sSysBot) relayIsAdmin(ctx context.Context, cfg *entity.YoubanBotRelayConfig, userID int64) (bool, error) {
	if userID <= 0 || !relayOwned(cfg) {
		return false, nil
	}
	account, err := s.relayBoundAccount(ctx, userID)
	if err == nil && account.TenantId == cfg.TenantId && account.AccountId == cfg.AccountId {
		return true, nil
	}
	// Explicit delegated administrators belong to this configuration's unique Bot.
	n, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Where(do.YoubanBotRelayAdmin{BotId: cfg.BotId, UserId: strconv.FormatInt(userID, 10)}).Count()
	return n > 0, gerror.Wrap(err, "检查双向管理员失败")
}
