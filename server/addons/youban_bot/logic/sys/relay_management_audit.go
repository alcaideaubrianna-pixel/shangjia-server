package sys

import (
	"context"
	"fmt"
	"strings"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/gogf/gf/v2/errors/gerror"
)

// These controls exist only in the authenticated main-Bot management panel.
// They are intentionally absent from the dedicated relay Bot's /help commands.
func (s *sSysBot) relayManagementAudit(ctx context.Context, cfg *entity.YoubanBotRelayConfig, command string) (bool, string, error) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false, "", nil
	}
	switch fields[0] {
	case "审计设置":
		state := "关闭"
		if cfg.AuditDatabase == 1 {
			state = "开启"
		}
		group := cfg.AuditGroupId
		if group == "" {
			group = "未配置"
		}
		return true, fmt.Sprintf("聊天存档：%s\n转发群：%s\n转发群必须开启话题，Bot须拥有管理话题权限。固定双方共用一个话题，默认标题为昵称-昵称。", state, group), nil
	case "聊天存档":
		if len(fields) != 2 || (fields[1] != "开启" && fields[1] != "关闭") {
			return true, "", gerror.New("格式：聊天存档 开启 或 聊天存档 关闭")
		}
		enabled := 0
		if fields[1] == "开启" {
			enabled = 1
		}
		_, err := dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(cfg.Id).Data(do.YoubanBotRelayConfig{AuditDatabase: enabled}).Update()
		return true, "新收到的聊天消息存档已" + fields[1] + "；已有存档保留。", err
	case "转发群":
		if len(fields) != 2 || !sysin.RelayIdentityValid(fields[1]) || !strings.HasPrefix(fields[1], "-") {
			return true, "", gerror.New("请填写开启话题的超级群ID，例如：转发群 -1001234567890")
		}
		bot, err := s.relayBotById(ctx, cfg.BotId)
		if err != nil {
			return true, "", err
		}
		if err = s.validateRelayAuditGroup(ctx, bot.BotToken, cfg.BotId, fields[1]); err != nil {
			return true, "", err
		}
		_, err = dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(cfg.Id).Data(do.YoubanBotRelayConfig{AuditGroupId: fields[1]}).Update()
		return true, "实时转发群已更新；固定双方共用一个话题。", err
	case "关闭转发":
		_, err := dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(cfg.Id).Data(do.YoubanBotRelayConfig{AuditGroupId: ""}).Update()
		return true, "实时转发已关闭，已有群消息和话题保留。", err
	}
	return false, "", nil
}
