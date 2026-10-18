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
)

// Management has its own main-Bot command; it never registers relay /help globally.
type relayManagementFeature struct{}

func (relayManagementFeature) Key() string         { return "relay_management" }
func (relayManagementFeature) Command() string     { return "relayadmin" }
func (relayManagementFeature) Description() string { return "双向机器人管理" }
func (relayManagementFeature) Handle(ctx context.Context, s *sSysBot, fc *botFeatureContext) (handled bool, resultErr error) {
	defer func() {
		if resultErr != nil && fc.Msg != nil {
			resultErr = s.relayInteractionError(ctx, fc.BotId, fc.Msg, resultErr)
		}
	}()
	if fc.Msg == nil || fc.Msg.From == nil {
		return true, nil
	}
	reply := func(text string) (bool, error) { return true, s.relayManagementReply(ctx, fc.BotId, fc.Msg, text) }
	if fc.Msg.Chat.Type != models.ChatTypePrivate {
		return reply("请在机器人私聊中使用双向机器人管理。")
	}
	ctx = relayActorContext(ctx, fc.Msg.From.ID)
	account, accountErr := s.relayAccount(ctx)
	if accountErr != nil {
		return reply("请先绑定上架端租户账户后使用双向机器人管理。发送 /bind 完成绑定。")
	}
	args := strings.Fields(fc.Args)
	if len(args) > 0 && args[0] == "创建" {
		return reply("请先在后台创建双向机器人，再返回这里刷新列表。")
	}

	if page, ok := relayManagementPage(args); ok {
		if page < 1 {
			return reply("页码不正确，请返回 /relayadmin")
		}
		text, err := s.relayManagementListPage(ctx, fc.Msg.From.ID, page)
		if err != nil {
			return true, err
		}
		return reply(text)
	}
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
	cfg, err := s.relayEnsureConfig(ctx, account, botID)
	if err != nil {
		return reply(html.EscapeString(err.Error()))
	}
	if !relayOwned(cfg) || cfg.TenantId != account.TenantId || cfg.AccountId != account.AccountId {
		return reply("无权管理其他租户账户的双向机器人；旧全局配置需管理员迁移。")
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
		_, runtimeState, err := s.relayRuntimeState(ctx, cfg)
		if err != nil {
			return true, err
		}
		lines := []string{fmt.Sprintf("<b>双向机器人 #%d</b> · %s\n实际状态：%s", botID, state, html.EscapeString(runtimeState))}
		for _, action := range []string{"启用", "停用", "当前设置", "私聊用户", "群聊列表", "绑定列表", "屏蔽列表", "黑名单", "屏蔽跳转 开启", "屏蔽跳转 关闭", "绑定", "解绑", "添加管理员", "删除管理员", "管理员列表", "屏蔽关键词", "删除关键词", "屏蔽用户", "解除屏蔽用户", "审计设置", "聊天存档 开启", "聊天存档 关闭", "转发群", "关闭转发", "同步后台Bot"} {
			lines = append(lines, "<code>"+prefix+" "+action+"</code>")
		}
		lines = append(lines, "绑定和屏蔽命令可直接追加，例如：", "<code>"+prefix+" 绑定 @alice @bob</code>", "<code>"+prefix+" 屏蔽 关键词</code>", "使用审计设置、聊天存档和转发群按钮配置聊天记录管理。", "返回：<code>/relayadmin</code>")
		return reply(strings.Join(lines, "\n"))
	}
	if err := s.relayCheckAccess(ctx, cfg.BotId); err != nil {
		return reply(html.EscapeString(err.Error()))
	}
	if prompt := relayManagementActionPrompt(botID, command); prompt != "" {
		return reply(prompt)
	}
	if command == "启用" || command == "停用" || strings.HasPrefix(command, "解绑 ") || command == "关闭转发" {
		return reply("请确认操作：\n<code>" + fmt.Sprintf("/relayadmin %d 确认 ", botID) + html.EscapeString(command) + "</code>\n取消：<code>" + fmt.Sprintf("/relayadmin %d", botID) + "</code>")
	}
	command = strings.TrimPrefix(command, "确认 ")
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
			stateConfig := *current
			stateConfig.Enabled = enabled
			_, state, stateErr := s.relayRuntimeState(ctx, &stateConfig)
			if stateErr != nil {
				return stateErr
			}
			result = "双向功能开关已" + command + "。\n实际状态：" + state
			return err
		}
		if handled, text, auditErr := s.relayManagementAudit(ctx, current, command); handled {
			result = text
			return auditErr
		}
		cmd := parseRelayCommand(command)
		if cmd.Name == "" || strings.HasPrefix(cmd.Name, "/") {
			return gerror.New("未识别的管理操作，请发送 /relayadmin BotID 查看菜单")
		}
		if cmd.Name == "解绑" {
			cmd.Name = "确认解绑"
		}
		result, err = s.executeRelayCommand(ctx, current, cmd)
		return err
	})
	if err != nil {
		return reply(html.EscapeString(err.Error()))
	}
	fields := strings.Fields(command)
	if len(fields) > 0 && (fields[0] == "绑定列表" || fields[0] == "屏蔽列表" || fields[0] == "黑名单" || fields[0] == "私聊用户" || fields[0] == "群聊列表") {
		page := 1
		if len(fields) > 1 {
			page, _ = strconv.Atoi(fields[1])
		}
		if page > 1 {
			result += fmt.Sprintf("\n上一页：<code>/relayadmin %d %s %d</code>", botID, fields[0], page-1)
		}
		result += fmt.Sprintf("\n下一页：<code>/relayadmin %d %s %d</code>", botID, fields[0], page+1)
	}
	return reply(result + fmt.Sprintf("\n\n返回：<code>/relayadmin %d</code>", botID))
}

func (s *sSysBot) relayManagementList(ctx context.Context, userID int64) (string, error) {
	return s.relayManagementListPage(ctx, userID, 1)
}
func (s *sSysBot) relayManagementListPage(ctx context.Context, userID int64, page int) (string, error) {
	account, err := s.relayBoundAccount(ctx, userID)
	if err != nil {
		return "", gerror.New("请先绑定租户账户")
	}
	bots, err := s.relayOwnedBackendBots(ctx, account.TenantId, account.AccountId, 0, false)
	if err != nil {
		return "", err
	}
	lines := []string{fmt.Sprintf("<b>双向机器人管理</b> · 第%d页", page)}
	visible := len(bots)
	for index, bot := range bots {
		position := index + 1
		if position <= (page-1)*8 || position > page*8 {
			continue
		}
		cfg, err := relayConfig(ctx, bot.Id)
		if err != nil {
			return "", err
		}
		state := "未配置"
		if cfg != nil && cfg.Enabled == 1 {
			state = "启用"
		} else if cfg != nil {
			state = "停用"
		}
		name := bot.BotName
		if name == "" {
			name = bot.BotUsername
		}
		lines = append(lines, fmt.Sprintf("<code>/relayadmin %d</code> %s @%s · %s", bot.Id, html.EscapeString(name), html.EscapeString(bot.BotUsername), state))
	}
	if page > 1 {
		lines = append(lines, fmt.Sprintf("上一页：<code>/relayadmin 列表 %d</code>", page-1))
	}
	if visible > page*8 {
		lines = append(lines, fmt.Sprintf("下一页：<code>/relayadmin 列表 %d</code>", page+1))
	}
	if visible == 0 {
		lines = append(lines, "暂无可管理的双向机器人。请先在后台创建双向机器人，然后返回这里刷新。")
	}
	if page > 1 {
		lines = append(lines, "返回首页：<code>/relayadmin</code>")
	}
	return strings.Join(lines, "\n\n"), nil
}
