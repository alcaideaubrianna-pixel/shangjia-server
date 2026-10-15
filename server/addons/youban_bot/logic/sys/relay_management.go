package sys

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
)

// Management has its own main-Bot command; it never registers relay /help globally.
type relayManagementFeature struct{}

func (relayManagementFeature) Key() string         { return "relay_management" }
func (relayManagementFeature) Command() string     { return "relayadmin" }
func (relayManagementFeature) Description() string { return "双向机器人管理" }
func (relayManagementFeature) Handle(ctx context.Context, s *sSysBot, fc *botFeatureContext) (bool, error) {
	if fc.Msg == nil || fc.Msg.From == nil {
		return true, nil
	}
	reply := func(text string) (bool, error) { return true, s.reply(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), text) }
	if fc.Msg.Chat.Type != models.ChatTypePrivate {
		return reply("请在机器人私聊中使用双向机器人管理。")
	}
	args := strings.Fields(fc.Args)
	if len(args) == 0 {
		text, err := s.relayManagementList(ctx, fc.Msg.From.ID)
		if err != nil {
			return true, err
		}
		return reply(text)
	}
	botID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || botID <= 0 {
		return reply("格式：<code>/relayadmin BotID</code>，先发送 <code>/relayadmin</code> 查看列表。")
	}
	cfg, err := relayConfig(ctx, botID)
	if err != nil {
		return true, err
	}
	if cfg == nil {
		return reply("无权管理该双向机器人，或该机器人尚未配置。")
	}
	allowed, err := s.relayIsAdmin(ctx, cfg, fc.Msg.From.ID)
	if err != nil {
		return true, err
	}
	if !allowed {
		return reply("仅该双向机器人配置的管理员可以管理。")
	}
	command := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(fc.Args), args[0]))
	if command == "" {
		state := "停用"
		if cfg.Enabled == 1 {
			state = "启用"
		}
		prefix := fmt.Sprintf("/relayadmin %d", botID)
		lines := []string{fmt.Sprintf("<b>双向机器人 #%d</b> · %s", botID, state)}
		for _, action := range []string{"启用", "停用", "当前设置", "绑定列表", "屏蔽列表", "黑名单", "屏蔽跳转 开启", "屏蔽跳转 关闭"} {
			lines = append(lines, "<code>"+prefix+" "+action+"</code>")
		}
		lines = append(lines, "绑定和屏蔽命令可直接追加，例如：", "<code>"+prefix+" 绑定 @alice @bob</code>", "<code>"+prefix+" 屏蔽 关键词</code>", "添加管理员、审计和转发群配置请在后台 BOT 管理的双向机器人插件中设置。")
		return reply(strings.Join(lines, "\n"))
	}
	var result string
	err = relayMutate(ctx, botID, func(ctx context.Context) error {
		current, err := relayConfig(ctx, botID)
		if err != nil {
			return err
		}
		if current == nil {
			return gerror.New("双向机器人配置已移除")
		}
		allowed, err := s.relayIsAdmin(ctx, current, fc.Msg.From.ID)
		if err != nil {
			return err
		}
		if !allowed {
			return gerror.New("管理员权限已变化，请重新打开管理菜单")
		}
		if command == "启用" || command == "停用" {
			enabled := 0
			if command == "启用" {
				enabled = 1
			}
			_, err := dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(current.Id).Data(do.YoubanBotRelayConfig{Enabled: enabled}).Update()
			result = "双向机器人已" + command + "。"
			return err
		}
		cmd := parseRelayCommand(command)
		if cmd.Name == "" || strings.HasPrefix(cmd.Name, "/") {
			return gerror.New("未识别的管理操作，请发送 /relayadmin BotID 查看菜单")
		}
		result, err = s.executeRelayCommand(ctx, current, cmd)
		return err
	})
	if err != nil {
		return reply(html.EscapeString(err.Error()))
	}
	return reply(result)
}

func (s *sSysBot) relayManagementList(ctx context.Context, userID int64) (string, error) {
	var configs []*entity.YoubanBotRelayConfig
	if err := dao.YoubanBotRelayConfig.Ctx(ctx).OrderAsc(dao.YoubanBotRelayConfig.Columns().BotId).Scan(&configs); err != nil {
		return "", gerror.Wrap(err, "双向机器人配置不可用，请检查数据库迁移")
	}
	lines := []string{"<b>双向机器人管理</b>"}
	for _, cfg := range configs {
		allowed, err := s.relayIsAdmin(ctx, cfg, userID)
		if err != nil {
			return "", err
		}
		if !allowed {
			continue
		}
		bot, err := s.botById(ctx, cfg.BotId)
		if err != nil {
			return "", err
		}
		state := "停用"
		if cfg.Enabled == 1 {
			state = "启用"
		}
		lines = append(lines, fmt.Sprintf("%s @%s · %s\n<code>/relayadmin %d</code>", html.EscapeString(bot.BotName), html.EscapeString(bot.BotUsername), state, cfg.BotId))
	}
	if len(lines) == 1 {
		lines = append(lines, "暂无可管理的双向机器人。请在后台 BOT 管理的双向机器人插件中配置，并添加管理员。")
	}
	return strings.Join(lines, "\n\n"), nil
}
