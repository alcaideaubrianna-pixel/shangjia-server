package sys

import (
	"context"
	"fmt"
	"net/url"
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
		if len(fields) != 2 {
			return true, "", gerror.New("格式：转发群 -1001234567890、转发群 @公开群用户名，或转发群 https://t.me/公开群用户名")
		}
		groupID, err := relayAuditGroupIdentity(fields[1])
		if err != nil {
			return true, "", err
		}
		bot, err := s.relayBotById(ctx, cfg.BotId)
		if err != nil {
			return true, "", err
		}
		if err = s.validateRelayAuditGroup(ctx, bot.BotToken, cfg.BotId, groupID); err != nil {
			return true, "", err
		}
		_, err = dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(cfg.Id).Data(do.YoubanBotRelayConfig{AuditGroupId: groupID}).Update()
		return true, "实时转发群已更新；固定双方共用一个话题。", err
	case "关闭转发":
		_, err := dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(cfg.Id).Data(do.YoubanBotRelayConfig{AuditGroupId: ""}).Update()
		return true, "实时转发已关闭，已有群消息和话题保留。", err
	}
	return false, "", nil
}

func relayAuditGroupIdentity(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "-") {
		if !sysin.RelayIdentityValid(value) {
			return "", gerror.New("群ID格式不正确")
		}
		return value, nil
	}
	if strings.HasPrefix(value, "@") {
		if len(strings.TrimPrefix(value, "@")) < 5 {
			return "", gerror.New("公开群用户名格式不正确")
		}
		return "@" + strings.ToLower(strings.TrimPrefix(value, "@")), nil
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		u, err := url.Parse(value)
		if err != nil || u.Host != "t.me" {
			return "", gerror.New("仅支持 t.me 公开群链接")
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 1 || parts[0] == "" || strings.HasPrefix(parts[0], "+") {
			return "", gerror.New("私有群邀请链接无法自动解析，请改用群ID；公开群请使用 https://t.me/群用户名")
		}
		return "@" + strings.ToLower(parts[0]), nil
	}
	return "", gerror.New("群目标不支持，请使用 -100群ID、@公开群用户名或 https://t.me/公开群用户名")
}
